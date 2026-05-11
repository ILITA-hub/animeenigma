package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	if cfg.MegacloudExtractor.URL == "" {
		log.Warnw("MEGACLOUD_EXTRACTOR_URL is empty; megacloud-backed extraction will fail when invoked")
	}

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("scraper")

	// Build the embed extractor registry. Phase 15 plan 03 registers
	// MegacloudClient as the only extractor; Phase 19+ adds Kwik etc.
	registry := domain.NewRegistry()
	mcClient := embeds.NewMegacloudClient(cfg.MegacloudExtractor.URL, cfg.MegacloudExtractor.Timeout)
	registry.Register(mcClient)

	// Build the orchestrator. Phase 15 registers ZERO providers — every
	// business call returns ErrNotFound until Phase 16 lands AnimePahe.
	orchestrator := service.NewOrchestrator(log, registry)

	// HTTP handler + router wiring.
	scraperHandler := handler.NewScraperHandler(orchestrator, log)
	router := transport.NewRouter(scraperHandler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("scraper service ready",
			"port", cfg.Server.Port,
			"address", cfg.Server.Address(),
			"providers", len(orchestrator.HealthSnapshot(context.Background())),
			"embed_extractors", len(registry.Names()),
			"megacloud_url", cfg.MegacloudExtractor.URL,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

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
