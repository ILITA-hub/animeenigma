// Package main is the analytics clickstream service entrypoint (port 8092).
//
// Boot: logger → config → database.New (auto-creates DB) → AutoMigrateAll +
// EnsureView → batcher.Start → purge cron → router → HTTP server with
// graceful shutdown (batcher drains on stop).
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
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/config"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/ingest"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/job"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/transport"
	"github.com/robfig/cron/v3"
)

// countingSink, repoEraser, and repoPurger are defined in adapters.go.

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("config load failed", "error", err)
	}

	if cfg.IPSalt == "" || cfg.IPSalt == "change-me-in-production" || cfg.IPSalt == "dev-salt-change-in-production" {
		log.Warnw("ANALYTICS_IP_SALT is unset or a placeholder — IP hashes are guessable; set a strong secret in docker/.env")
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("db connect failed", "error", err)
	}
	defer db.Close()

	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	if err := repo.AutoMigrateAll(db.DB); err != nil {
		log.Fatalw("migrate failed", "error", err)
	}
	if err := repo.EnsureView(db.DB); err != nil {
		log.Fatalw("view create failed", "error", err)
	}

	store := repo.NewPostgresStore(db.DB)
	batcher := ingest.New(store, ingest.Config{
		MaxBatch:      cfg.MaxBatch,
		FlushInterval: cfg.FlushInterval,
		BufferSize:    cfg.BufferSize,
	}).WithLogger(log).WithDropHook(func() { observ.EventsDropped.Inc() })
	batcher.Start()

	collectHandler := handler.NewCollectHandler(countingSink{batcher}, cfg.IPSalt)
	adminHandler := handler.NewAdminHandler(repoEraser{db: db})

	purgeJob := job.NewPurgeJob(repoPurger{db: db}, cfg.RetentionDays, log)
	c := cron.New()
	_, _ = c.AddFunc(cfg.PurgeCron, func() {
		_, _ = purgeJob.RunOnce(context.Background())
	})
	c.Start()
	defer c.Stop()

	collector := metrics.NewCollector("analytics")
	router := transport.NewRouter(collectHandler, adminHandler, log, collector)

	srv := &http.Server{Addr: cfg.Server.Address(), Handler: router}
	go func() {
		log.Infow("analytics service listening", "addr", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("server error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	batcher.Stop()
}
