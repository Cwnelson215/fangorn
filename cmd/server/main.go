package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	fangorn "github.com/cwnelson/fangorn"
	"github.com/cwnelson/fangorn/internal/config"
	"github.com/cwnelson/fangorn/internal/database"
	"github.com/cwnelson/fangorn/internal/handlers"
	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/middleware"
	"github.com/cwnelson/fangorn/internal/scheduler"
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

	authH := handlers.NewAuthHandler(cfg.AppPassword)
	ledgerH := handlers.NewLedgerHandler(svc, householdID)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready(db))
	mux.HandleFunc("POST /api/login", authH.Login)
	mux.HandleFunc("POST /api/logout", authH.Logout)
	mux.HandleFunc("GET /api/auth/status", authH.Status)

	ledgerH.Register(mux)

	// The scheduler posts recurring items and snapshots net worth. It runs a pass
	// immediately on boot, which is what backfills anything missed while the
	// process was down.
	sched := scheduler.New(svc, cfg.SchedulerInterval, cfg.SchedulerHorizonDays)
	schedCtx, stopScheduler := context.WithCancel(context.Background())
	go sched.Start(schedCtx)

	// Serve the embedded frontend with an SPA fallback.
	frontendFS, err := fs.Sub(fangorn.FrontendAssets, "frontend/build")
	if err != nil {
		log.Fatalf("Failed to create frontend sub-filesystem: %v", err)
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
	log.Println("Server exited")
}
