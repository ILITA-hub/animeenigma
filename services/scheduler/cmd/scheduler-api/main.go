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
	"github.com/ILITA-hub/animeenigma/libs/maintenancegate"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
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

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "scheduler")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Initialize database (auto-creates DB if not exists)
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}

	// Start DB pool metrics collector
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Auto-migrate MAL export tables
	if err := db.AutoMigrate(
		&domain.ExportJob{},
		&domain.AnimeLoadTask{},
		&domain.MALShikimoriMapping{},
	); err != nil {
		log.Warnw("failed to auto-migrate tables", "error", err)
	}

	// anime_load_tasks indexes (audit #16). The table shipped with none (the SQL
	// migration was never applied and the model declares no index tags), so the
	// pending-poll (status + priority DESC + updated_at) and the dedup lookup
	// (mal_id + status) seq-scanned. Created idempotently here so they apply to
	// the live DB on deploy; mirrored in docker/postgres/init.sql for fresh
	// installs. The partial UNIQUE enforces "one in-flight task per mal_id" — it
	// is best-effort (logs and continues if pre-existing duplicates block it; the
	// app-level dedup check at repo/task.go still guards correctness).
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_load_tasks_poll ON anime_load_tasks (status, priority DESC, updated_at ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_load_tasks_mal_status ON anime_load_tasks (mal_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_load_tasks_export_job ON anime_load_tasks (export_job_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_load_tasks_unique_inflight ON anime_load_tasks (mal_id) WHERE status IN ('pending','processing')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			log.Warnw("failed to create anime_load_tasks index", "stmt", stmt, "error", err)
		}
	}

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Activity-register effect producer (AR-EFFECT-01). Non-blocking +
	// drop-on-full so an analytics outage never affects scheduler jobs.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// AR-EFFECT-01 (D-15): GORM db_write/db_read effect callbacks + the daily-P95
	// ReadGate refresher (D-03), reusing the existing Redis client as HashReader.
	dbEffectsCtx, dbEffectsCancel := context.WithCancel(context.Background())
	defer dbEffectsCancel()
	dbEffectsStop, err := gormtrace.WireDBEffects(dbEffectsCtx, db.DB, tracing.GlobalSink(), redisCache)
	if err != nil {
		log.Warnw("db-effect callbacks disabled", "error", err)
	}
	defer dbEffectsStop()

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
	topAnimeJob := jobs.NewTopAnimeSyncJob(&cfg.Jobs, log)
	calendarJob := jobs.NewCalendarSyncJob(&cfg.Jobs, log)
	animeLoaderJob := jobs.NewAnimeLoaderJob(taskRepo, exportJobRepo, taskProcessor, log)
	// Phase A — daily playback-health probe trigger.
	probeTriggerJob := jobs.NewProbeTriggerJob(&cfg.Jobs, log)
	// Phase 03 (v4.0) — daily read-threshold recompute trigger (D-03).
	readThresholdJob := jobs.NewReadThresholdJob(&cfg.Jobs, log)
	// Stage 2b — daily provider-ranking recompute trigger.
	providerRankingJob := jobs.NewProviderRankingJob(&cfg.Jobs, log)
	// Active subtitle-provider health probe trigger (every 5 min).
	subtitleProbeJob := jobs.NewSubtitleProbeTriggerJob(&cfg.Jobs, log)
	// Phase 09 — Logic A ongoing-push autocache producer (TRIG-01). Runs the
	// shared-DB enumeration join itself and fires per-ongoing demand POSTs to the
	// library /internal endpoint. Disabled (nil) when no library URL is configured
	// so the nil-guarded registration in JobService.Start skips it cleanly.
	var autocacheLogicAJob *jobs.AutocacheLogicAJob
	if cfg.Jobs.LibraryInternalURL != "" {
		autocacheLogicAJob = jobs.NewAutocacheLogicAJob(db.DB, cfg.Jobs.LibraryInternalURL, cfg.Jobs.AutocacheActiveWatcherDays, log)
	}
	// Phase 11 — daily autocache storage-need prediction producer (OBS-05). Reads
	// the shared animeenigma DB only (no external URL), so it is constructed
	// UNCONDITIONALLY (always on, unlike Logic A's nil-on-empty-URL guard).
	autocachePredictionJob := jobs.NewAutocachePredictionJob(db.DB, cfg.Jobs.AutocacheActiveWatcherDays, cfg.Jobs.AutocacheAvgRawEpBytes, log)

	// Initialize services
	jobService := service.NewJobService(shikimoriJob, cleanupJob, topAnimeJob, calendarJob, probeTriggerJob, readThresholdJob, providerRankingJob, subtitleProbeJob, autocacheLogicAJob, autocachePredictionJob, log)

	// Graceful-degradation Phase 3: heavy crons skip their tick while the
	// governor-published level is Elevated+ (Redis ae:degradation:level;
	// fail-open — missing key/governor reads as Normal).
	shedWatcher := cache.NewDegradationWatcher(redisCache, 5*time.Second)
	shedWatcher.Start(context.Background())
	jobService.SetShedChecker(shedWatcher)

	// P3 maintenance enforcement: gate + status-ping shikimori_sync,
	// subtitle_probe, and playability_canary against policy-service's
	// /admin/policy routines. Fail-open (unreachable ⇒ jobs still run).
	jobService.SetMaintenanceGate(maintenancegate.New(cfg.Jobs.PolicyServiceURL, 3*time.Second))
	exportService := service.NewExportService(exportJobRepo, taskRepo, log)

	// Start job scheduler
	if err := jobService.Start(
		cfg.Jobs.ShikimoriSyncCron,
		cfg.Jobs.CleanupCron,
		cfg.Jobs.TopAnimeSyncCron,
		cfg.Jobs.CalendarSyncCron,
		cfg.Jobs.PlaybackProbeCron,
		cfg.Jobs.ReadThresholdCron,
		cfg.Jobs.ProviderRankingCron,
		cfg.Jobs.SubtitleProbeCron,
		cfg.Jobs.AutocacheLogicACron,
		cfg.Jobs.AutocachePredictionCron,
	); err != nil {
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
		Handler:      tracing.HTTPMiddleware("scheduler")(router),
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
