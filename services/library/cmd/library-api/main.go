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
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/animetosho"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/nyaa"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
	"github.com/ILITA-hub/animeenigma/services/library/internal/transport"
)

// animeToshoAdapter bridges between service.AnimeToshoSearcher (which
// the merger consumes) and *animetosho.Client (which the parser
// exposes). The two have identical SearchParams shapes but live in
// different packages — the adapter keeps the service package free of
// any parser-package import so the dependency arrow points the
// expected direction (transport → handler → service ← parser, with
// parser owned by main.go wiring).
type animeToshoAdapter struct {
	c *animetosho.Client
}

func (a animeToshoAdapter) Search(ctx context.Context, p service.AnimeToshoParams) ([]domain.Release, error) {
	return a.c.Search(ctx, animetosho.SearchParams{
		MALID: p.MALID,
		Query: p.Query,
		Limit: p.Limit,
	})
}

// main is the entry point for the library service (workstream raw-jp / v0.2).
// Phase 2 wires the Nyaa + AnimeTosho parser clients, the merger, and
// the search HTTP handler into the chi router. Auth is enforced at the
// gateway (services/gateway/internal/transport/router.go) — the library
// service trusts what the gateway forwards.
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

	// Phase 2: no AutoMigrate — search is stateless. Phase 3 reintroduces
	// AutoMigrate for library_jobs / library_episodes / library_filename_patterns.

	// Initialize parser clients.
	nyaaClient := nyaa.NewClient(nyaa.Config{
		BaseURL:     cfg.Nyaa.BaseURL,
		HTTPTimeout: cfg.Nyaa.HTTPTimeout,
		UserAgent:   cfg.Nyaa.UserAgent,
	})
	atClient := animetosho.NewClient(animetosho.Config{
		BaseURL:     cfg.AnimeTosho.BaseURL,
		HTTPTimeout: cfg.AnimeTosho.HTTPTimeout,
		UserAgent:   cfg.AnimeTosho.UserAgent,
	})

	// Aggregator + handler.
	aggregator := service.NewAggregator(nyaaClient, animeToshoAdapter{c: atClient}, log)

	// Initialize handlers.
	healthHandler := handler.NewHealthHandler()
	searchHandler := handler.NewSearchHandler(aggregator, log)

	// Initialize metrics collector.
	metricsCollector := metrics.NewCollector("library")

	// Initialize router.
	router := transport.NewRouter(
		healthHandler,
		searchHandler,
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
