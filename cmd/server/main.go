package main

import (
	"context"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	// The production image is a bare alpine with no zoneinfo. Without the
	// embedded database, LoadLocation fails and both the household's "today" and
	// US market hours silently fall back to UTC.
	_ "time/tzdata"

	fangorn "github.com/cwnelson/fangorn"
	"github.com/cwnelson/fangorn/internal/config"
	"github.com/cwnelson/fangorn/internal/database"
	"github.com/cwnelson/fangorn/internal/handlers"
	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/middleware"
	"github.com/cwnelson/fangorn/internal/prices"
	"github.com/cwnelson/fangorn/internal/quotes"
	"github.com/cwnelson/fangorn/internal/receipts"
	"github.com/cwnelson/fangorn/internal/scheduler"
	"github.com/cwnelson/fangorn/internal/vision"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	svc := ledger.New(db)

	// Auth is still a single shared password, so there is exactly one household
	// and it is resolved once here. When real users arrive this becomes a
	// per-request lookup off the session.
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	householdID, err := svc.DefaultHouseholdID(startupCtx)
	cancelStartup()
	if err != nil {
		log.Fatalf("Could not resolve household: %v", err)
	}

	// Security prices. The provider is optional: with QUOTES_PROVIDER=none, trades
	// still work and holdings are valued at the prices they were logged at.
	var provider quotes.Provider
	switch cfg.QuotesProvider {
	case "yahoo":
		provider = quotes.NewYahoo()
	case "none":
		log.Println("Price fetching disabled (QUOTES_PROVIDER=none)")
	default:
		log.Fatalf("Unknown QUOTES_PROVIDER %q (want yahoo or none)", cfg.QuotesProvider)
	}
	refresher := prices.New(svc, provider, cfg.QuotesMarketTTL)

	// Receipt reading. Also optional: with RECEIPTS_PROVIDER=none, photos are
	// stored and every receipt waits for someone to enter it by hand.
	var extractor vision.Extractor
	switch cfg.ReceiptsProvider {
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			log.Fatalf("RECEIPTS_PROVIDER=anthropic needs ANTHROPIC_API_KEY")
		}
		extractor = vision.NewAnthropic(cfg.AnthropicAPIKey, cfg.ReceiptsModel)
		log.Printf("Receipts read with %s", cfg.ReceiptsModel)
	case "none":
		log.Println("Receipt reading disabled (RECEIPTS_PROVIDER=none)")
	default:
		log.Fatalf("Unknown RECEIPTS_PROVIDER %q (want anthropic or none)", cfg.ReceiptsProvider)
	}
	receiptProc := receipts.New(svc, extractor)

	authH := handlers.NewAuthHandler(cfg.AppPassword)
	ledgerH := handlers.NewLedgerHandler(svc, householdID)
	investmentH := handlers.NewInvestmentHandler(svc, refresher, householdID)
	receiptH := handlers.NewReceiptHandler(svc, receiptProc, householdID)
	shortcutH := handlers.NewShortcutHandler(svc, receiptH, householdID)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready(db))
	mux.HandleFunc("POST /api/login", authH.Login)
	mux.HandleFunc("POST /api/logout", authH.Logout)
	mux.HandleFunc("GET /api/auth/status", authH.Status)

	ledgerH.Register(mux)
	investmentH.Register(mux)
	receiptH.Register(mux)
	shortcutH.Register(mux)

	// The scheduler posts recurring items, refreshes prices and snapshots net
	// worth. It runs a pass immediately on boot, which is what backfills anything
	// missed while the process was down.
	sched := scheduler.New(svc, refresher, receiptProc, cfg.SchedulerInterval, cfg.SchedulerHorizonDays)
	schedCtx, stopScheduler := context.WithCancel(context.Background())
	go sched.Start(schedCtx)

	// Serve the embedded frontend with an SPA fallback.
	frontendFS, err := fs.Sub(fangorn.FrontendAssets, "frontend/build")
	if err != nil {
		log.Fatalf("Failed to create frontend sub-filesystem: %v", err)
	}
	// Go's built-in type table lacks .webmanifest and the alpine image has no
	// /etc/mime.types, so without this the manifest is sniffed as text/plain.
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		log.Fatalf("Failed to register manifest type: %v", err)
	}
	fileServer := http.FileServer(http.FS(frontendFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		cleanPath := strings.TrimPrefix(path, "/")
		if _, err := fs.Stat(frontendFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path: let the client-side router handle it.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	handler := middleware.Logging(middleware.Auth(cfg.AppPassword)(middleware.CORS(mux)))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server starting on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	stopScheduler()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	// Receipts still being read for an upload that already answered. Anything
	// not done by the deadline hands its claim back for the next boot.
	receiptProc.Wait(ctx)
	log.Println("Server exited")
}
