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
	"github.com/cwnelson/fangorn/internal/middleware"
	"github.com/cwnelson/fangorn/internal/services"
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

	tellerSvc := services.NewTellerService(cfg)
	syncSvc := services.NewSyncService(db, tellerSvc, cfg.EncryptionKey)
	transferSvc := services.NewTransferService(db, cfg.EncryptionKey)

	authH := handlers.NewAuthHandler(cfg.AppPassword)
	configH := handlers.NewConfigHandler(cfg.TellerAppID)
	linkH := handlers.NewLinkHandler(syncSvc)
	accountsH := handlers.NewAccountsHandler(db)
	txnH := handlers.NewTransactionsHandler(db)
	syncH := handlers.NewSyncHandler(syncSvc)
	dashH := handlers.NewDashboardHandler(db)
	transferH := handlers.NewTransferHandler(transferSvc)

	mux := http.NewServeMux()

	// Auth routes
	mux.HandleFunc("POST /api/login", authH.Login)
	mux.HandleFunc("GET /api/auth/status", authH.Status)

	// API routes
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /api/config", configH.Get)
	mux.HandleFunc("POST /api/link-account", linkH.LinkAccount)
	mux.HandleFunc("GET /api/accounts", accountsH.List)
	mux.HandleFunc("GET /api/transactions", txnH.List)
	mux.HandleFunc("POST /api/sync", syncH.Sync)
	mux.HandleFunc("GET /api/dashboard", dashH.Get)
	mux.HandleFunc("POST /api/transfers", transferH.Create)
	mux.HandleFunc("GET /api/transfers", transferH.List)
	mux.HandleFunc("GET /api/transfers/{id}", transferH.Get)
	mux.HandleFunc("POST /api/transfers/{id}/refresh", transferH.Refresh)
	mux.HandleFunc("POST /api/transfers/{id}/cancel", transferH.Cancel)

	// Serve embedded frontend with SPA fallback
	frontendFS, err := fs.Sub(fangorn.FrontendAssets, "frontend/build")
	if err != nil {
		log.Fatalf("Failed to create frontend sub-filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(frontendFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file first
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if file exists in the embedded FS
		cleanPath := strings.TrimPrefix(path, "/")
		if _, err := fs.Stat(frontendFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for unmatched routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// Apply middleware
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited")
}
