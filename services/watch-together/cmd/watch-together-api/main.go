// Package main is the watch-together service entrypoint (port 8091).
//
// v1.0 Watch Together — workstream watch-together, Phase 1 Plan 01.1.
//
// Boot sequence:
//
//  1. logger.Default()
//  2. config.Load() — JWT_SECRET required; SERVER_PORT defaults to 8091;
//     MaxMembers / RoomTTL / GracePeriod populated from WATCH_TOGETHER_*
//     env vars.
//  3. metrics.NewCollector("watch-together") — service-labelled Prometheus
//     registration.
//  4. transport.NewRouter(cfg, log, collector) — Phase 1 ships /health +
//     /metrics; REST + WS land in 01.4 / 01.5.
//  5. http.Server with graceful shutdown on SIGINT/SIGTERM.
//
// No database — service is Redis-only by design (WT-FOUND-02; persistence
// deferred to v1.2 via the new persistent-rooms workstream). The Redis
// client is wired in 01.2.
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
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	metricsCollector := metrics.NewCollector("watch-together")
	router := transport.NewRouter(cfg, log, metricsCollector)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting watch-together service",
			"address", cfg.Server.Address(),
			"max_members", cfg.MaxMembers,
			"room_ttl", cfg.RoomTTL,
			"grace_period", cfg.GracePeriod,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down watch-together service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("watch-together service stopped")
}
