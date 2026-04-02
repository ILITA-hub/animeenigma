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
	"github.com/ILITA-hub/animeenigma/services/player/internal/config"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/ILITA-hub/animeenigma/services/player/internal/transport"
)

func main() {
	// Initialize logger
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Initialize database (auto-creates DB if not exists)
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	// Start DB pool metrics collector
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
		metrics.StartActivityMetricsCollector(sqlDB, 60*time.Second)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(
		&domain.WatchProgress{},
		&domain.AnimeListEntry{},
		&domain.WatchHistory{},
		&domain.UserAnimePreference{},
		&domain.Review{},
		&domain.SyncJob{},
		&domain.ActivityEvent{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Initialize repositories
	progressRepo := repo.NewProgressRepository(db.DB)
	listRepo := repo.NewListRepository(db.DB)
	historyRepo := repo.NewHistoryRepository(db.DB)
	reviewRepo := repo.NewReviewRepository(db.DB)
	syncRepo := repo.NewSyncRepository(db.DB)
	activityRepo := repo.NewActivityRepository(db.DB)

	// Mark stale sync jobs as failed on startup
	if err := syncRepo.MarkStaleJobsFailed(context.Background(), 1*time.Hour); err != nil {
		log.Errorw("failed to mark stale sync jobs", "error", err)
	}

	// Initialize repositories (preference)
	prefRepo := repo.NewPreferenceRepository(db.DB)

	// Initialize services
	prefService := service.NewPreferenceService(prefRepo, log)
	progressService := service.NewProgressService(progressRepo, prefService, log)
	listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, log)
	historyService := service.NewHistoryService(historyRepo, log)
	reviewService := service.NewReviewService(reviewRepo, listRepo, activityRepo, log)

	// Initialize MAL export service
	malExportService := service.NewMALExportService(log)

	// Initialize handlers
	progressHandler := handler.NewProgressHandler(progressService, log)
	listHandler := handler.NewListHandler(listService, log)
	historyHandler := handler.NewHistoryHandler(historyService, log)
	reviewHandler := handler.NewReviewHandler(reviewService, log)
	malImportHandler := handler.NewMALImportHandler(listService, syncRepo, log)
	malExportHandler := handler.NewMALExportHandler(malExportService, log)
	shikimoriImportHandler := handler.NewShikimoriImportHandler(listService, syncRepo, log)
	exportHandler := handler.NewExportHandler(listService, log)
	prefHandler := handler.NewPreferenceHandler(prefService, log)
	reportHandler := handler.NewReportHandler(log, cfg.Telegram.BotToken, cfg.Telegram.AdminChatID, cfg.Reports.Dir, cfg.Maintenance.URL)
	syncHandler := handler.NewSyncHandler(syncRepo, log)
	activityHandler := handler.NewActivityHandler(activityRepo, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("player")

	// Initialize router
	router := transport.NewRouter(progressHandler, listHandler, historyHandler, reviewHandler, malImportHandler, malExportHandler, shikimoriImportHandler, reportHandler, syncHandler, activityHandler, exportHandler, prefHandler, cfg.JWT, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Infow("starting player service", "address", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown
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
