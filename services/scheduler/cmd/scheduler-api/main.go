package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/jobs"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/service"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/transport"
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

	// Auto-migrate MAL export tables
	if err := db.AutoMigrate(
		&domain.ExportJob{},
		&domain.AnimeLoadTask{},
		&domain.MALShikimoriMapping{},
	); err != nil {
		log.Warnw("failed to auto-migrate tables", "error", err)
	}

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Initialize repositories
	exportJobRepo := repo.NewExportJobRepository(db.DB)
	taskRepo := repo.NewTaskRepository(db.DB)
	mappingRepo := repo.NewMappingRepository(db.DB)

	// Initialize MAL resolver and task processor
	malResolver := service.NewMALResolver(mappingRepo, &cfg.Jobs, log)
	taskProcessor := service.NewTaskProcessor(taskRepo, exportJobRepo, malResolver, log)

	// Initialize jobs
	shikimoriJob := jobs.NewShikimoriSyncJob(&cfg.Jobs, log)
	cleanupJob := jobs.NewCleanupJob(db.DB, redisCache, &cfg.Jobs, log)
	animeLoaderJob := jobs.NewAnimeLoaderJob(taskRepo, exportJobRepo, taskProcessor, log)

	// Initialize services
	jobService := service.NewJobService(shikimoriJob, cleanupJob, log)
	exportService := service.NewExportService(exportJobRepo, taskRepo, log)

	// Start job scheduler
	if err := jobService.Start(cfg.Jobs.ShikimoriSyncCron, cfg.Jobs.CleanupCron); err != nil {
		log.Fatalw("failed to start job scheduler", "error", err)
	}
	defer jobService.Stop()

	// Start anime loader worker
	if err := animeLoaderJob.Start(context.Background()); err != nil {
		log.Fatalw("failed to start anime loader", "error", err)
	}
	defer animeLoaderJob.Stop()

	// Initialize handlers
	jobHandler := handler.NewJobHandler(jobService, log)
	taskHandler := handler.NewTaskHandler(exportService, animeLoaderJob, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("scheduler")

	// Initialize router
	router := transport.NewRouter(jobHandler, taskHandler, log, metricsCollector)

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
		log.Infow("starting scheduler service", "address", cfg.Server.Address())
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
