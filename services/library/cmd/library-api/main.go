package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/config"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/library/internal/transport"
)

// main is the entry point for the library service (workstream raw-jp / v0.2).
// Phase 1 is pure scaffolding: load config, connect to Postgres (auto-create
// the `library` DB if absent), start the DB pool metrics collector, and run
// the chi router with /health + /metrics. No domain models, no AutoMigrate,
// no handlers — those land in Phase 3 (job queue) and Phase 4 (encoder).
func main() {
	// Initialize logger.
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Initialize database. libs/database.New auto-creates the `library`
	// database on first run (idempotent across restarts).
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	// Start DB pool metrics collector.
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Phase 1: no AutoMigrate — there are no domain types yet. Phase 3
	// reintroduces AutoMigrate for library_jobs / library_episodes /
	// library_filename_patterns.

	// Initialize handlers.
	healthHandler := handler.NewHealthHandler()

	// Initialize metrics collector.
	metricsCollector := metrics.NewCollector("library")

	// Initialize router.
	router := transport.NewRouter(
		healthHandler,
		cfg.JWT,
		log,
		metricsCollector,
	)

	// Create HTTP server. WriteTimeout is generous (120s) because Phase 3+
	// will add long-running torrent + transcode admin endpoints; keep parity
	// with the themes service so we don't have to revisit this later.
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server.
	go func() {
		log.Infow("starting library service", "address", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}
