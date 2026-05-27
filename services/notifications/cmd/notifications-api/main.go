// Package main is the notifications service entrypoint (port 8090).
//
// v1.0 Notifications Engine — workstream notifications.
// Boot sequence (Phase 2 expanded):
//
//  1. logger.Default()
//  2. config.Load() — JWT_SECRET required; SERVER_PORT defaults to 8090;
//     DetectorConfig populated from NOTIFICATIONS_* env vars.
//  3. database.New(cfg.Database) — auto-creates the shared "animeenigma"
//     DB if it doesn't exist; uses the same handle every other service
//     uses for read-only cross-table queries (D-01).
//  4. metrics.StartDBPoolCollector — pool stats to Prometheus.
//  5. db.AutoMigrate — service-owned tables ONLY (user_notifications,
//     parser_episode_snapshots). The read-only views in
//     internal/repo/views.go are NEVER passed here.
//  6. repo.EnsureIndexes — creates the two partial indexes GORM can't
//     express (idempotent CREATE INDEX IF NOT EXISTS).
//  7. Wire repos → service → handlers (public + internal + Phase 2 admin).
//  8. Phase 2 detector wiring: HTTPEpisodeChecker against CATALOG_URL +
//     hotCombos/snapshot/maxWatched/animeView/unreadGauge repos +
//     detectorJob + cleanupJob + scheduler.
//  9. transport.NewRouter
// 10. http.Server + graceful shutdown on SIGINT/SIGTERM.
// 11. If cfg.Detector.Enabled: scheduler.Start(ctx). Disabled mode skips
//     Start (D-RB-01 rollback toggle).
// 12. On shutdown: scheduler.Stop() BEFORE srv.Shutdown so in-flight cron
//     callbacks finish before the HTTP listener closes.
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
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/config"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/job"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/service"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Service-owned tables ONLY — the read-only views in repo/views.go
	// are intentionally absent so GORM does not try to "migrate" them
	// against the source-of-truth schemas owned by player + catalog.
	if err := db.AutoMigrate(
		&domain.UserNotification{},
		&domain.ParserEpisodeSnapshot{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Partial-index creation MUST run AFTER AutoMigrate (otherwise the
	// target table doesn't exist on first boot). IF NOT EXISTS makes
	// every subsequent boot a no-op.
	if err := repo.EnsureIndexes(context.Background(), db.DB); err != nil {
		log.Fatalw("failed to create partial indexes", "error", err)
	}

	// Repos.
	notifRepo := repo.NewNotificationRepository(db.DB)
	snapshotRepo := repo.NewSnapshotRepository(db.DB)
	maxWatchedRepo := repo.NewMaxWatchedRepository(db.DB)
	animeViewRepo := repo.NewAnimeViewRepository(db.DB)
	unreadGaugeRepo := repo.NewUnreadGaugeRepository(db.DB)

	// Services.
	notifService := service.NewNotificationService(notifRepo, log)
	episodeChecker := service.NewHTTPEpisodeChecker(cfg.Detector.CatalogURL, cfg.Detector.ParserTimeout, log)

	// Phase 2 jobs.
	hotCombos := job.NewHotCombosCollector(db.DB, log)
	detectorJob := job.NewEpisodeDetectorJobNew(
		hotCombos,
		episodeChecker,
		snapshotRepo,
		maxWatchedRepo,
		animeViewRepo,
		notifService,
		&cfg.Detector,
		log,
	)
	invalidationJob := job.NewRelevanceInvalidationJob(db.DB, log)
	cleanupJob := job.NewDismissedRetentionCleanupJob(db.DB, cfg.Detector.RetentionDays, log)
	scheduler := job.NewScheduler(detectorJob, invalidationJob, cleanupJob, unreadGaugeRepo, &cfg.Detector, log)

	// Handlers.
	notifHandler := handler.NewNotificationHandler(notifService, log)
	internalHandler := handler.NewInternalHandler(notifService, log)
	adminHandler := handler.NewAdminHandler(detectorJob, cleanupJob, log)

	// Metrics collector.
	metricsCollector := metrics.NewCollector("notifications")

	// Router.
	router := transport.NewRouter(notifHandler, internalHandler, adminHandler, cfg.JWT, log, metricsCollector)

	// HTTP server.
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Background context for the scheduler — separate from HTTP
	// per-request contexts so a SIGTERM cancels scheduler AFTER it
	// finishes in-flight runs (scheduler.Stop blocks on cron.Stop()
	// which waits for in-flight callbacks).
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()

	if cfg.Detector.Enabled {
		if err := scheduler.Start(schedCtx); err != nil {
			log.Fatalw("scheduler start failed", "error", err)
		}
	} else {
		log.Infow("detector disabled by NOTIFICATIONS_DETECTOR_ENABLED=false")
	}

	go func() {
		log.Infow("starting notifications service",
			"address", cfg.Server.Address(),
			"db_name", cfg.Database.Database,
			"detector_enabled", cfg.Detector.Enabled,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down notifications service...")

	// Stop the scheduler FIRST so in-flight cron callbacks finish before
	// the DB pool closes / HTTP listener drops. cron.Stop() blocks until
	// callbacks return.
	if cfg.Detector.Enabled {
		scheduler.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("notifications service stopped")
}
