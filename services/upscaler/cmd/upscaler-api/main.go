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
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/config"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/ffmpeg"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/handler"
	upminio "github.com/ILITA-hub/animeenigma/services/upscaler/internal/minio"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/service"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/source"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/transport"
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

	// Initialise job-capability HMAC signing (fail-closed when secret is empty).
	capability.Init(cfg.Upscaler.JobCapabilitySecret)
	if !capability.Enabled() {
		log.Warnw("job capability handles DISABLED — set JOB_CAPABILITY_SECRET to enable worker auth")
	}

	// Distributed tracing — no-op unless TRACING_ENABLED=true.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "upscaler")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Initialize database
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

	// Auto-migrate schema
	if err := db.DB.AutoMigrate(&domain.UpscaleJob{}, &domain.UpscaleSegment{}, &domain.UpscaleWorker{}, &domain.UpscaleModel{}, &domain.UpscaleEnrollToken{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Redis client
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Activity-register effect producer (non-blocking, drop-on-full).
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// DB effect callbacks + daily P95 ReadGate refresher.
	dbEffectsCtx, dbEffectsCancel := context.WithCancel(context.Background())
	defer dbEffectsCancel()
	dbEffectsStop, err := gormtrace.WireDBEffects(dbEffectsCtx, db.DB, tracing.GlobalSink(), redisCache)
	if err != nil {
		log.Warnw("db-effect callbacks disabled", "error", err)
	}
	defer dbEffectsStop()

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("upscaler")

	// ── Repositories ──────────────────────────────────────────────────────────
	jobRepo := repo.NewJobRepository(db.DB)
	segmentRepo := repo.NewSegmentRepository(db.DB)
	workerRepo := repo.NewWorkerRepository(db.DB)

	// ── Services ──────────────────────────────────────────────────────────────

	// Leaser: picks the next eligible job/segment and mints capability handles.
	leaser := service.NewLeaser(jobRepo, segmentRepo, workerRepo)

	// WS Hub: manages worker WebSocket connections.
	hub := controlplane.NewHub(leaser, workerRepo, log)

	// Enroll store: production transactional enroll path (EnrollTx, NOT Handle).
	enrollStore := controlplane.NewGormEnrollStore(db.DB)

	// Sweeper: re-leases expired segments and marks stale workers as gone.
	sweeper := service.NewSweeperWithLogger(segmentRepo, workerRepo, log)

	// Orchestrator: drives jobs queued→segmenting→upscaling→finalizing→done.
	// Constructs the source/ffmpeg/minio collaborators it needs.
	resolver := source.NewResolver(cfg.Upscaler.TorrentsDir, cfg.Upscaler.StagingDir)
	prober := source.NewProber("")       // ffprobe on PATH
	segmenter := ffmpeg.NewSegmenter("") // ffmpeg on PATH
	finalizer := ffmpeg.NewFinalizer("")
	upWriter, err := upminio.New(upminio.Config{
		Endpoint:          cfg.Upscaler.MinIO.Endpoint,
		AccessKey:         cfg.Upscaler.MinIO.AccessKey,
		SecretKey:         cfg.Upscaler.MinIO.SecretKey,
		Bucket:            cfg.Upscaler.MinIO.Bucket,
		UseSSL:            cfg.Upscaler.MinIO.UseSSL,
		UploadConcurrency: cfg.Upscaler.MinIO.UploadConcurrency,
	}, log)
	if err != nil {
		log.Fatalw("failed to init minio writer", "error", err)
	}
	orchestrator := service.NewOrchestrator(service.OrchestratorDeps{
		Jobs:           jobRepo,
		Segments:       segmentRepo,
		Resolver:       resolver,
		Prober:         prober,
		Segmenter:      segmenter,
		Finalizer:      finalizer,
		Writer:         upWriter,
		StagingDir:     cfg.Upscaler.StagingDir,
		SegmentSeconds: cfg.Upscaler.SegmentSeconds,
		Log:            log,
	})

	// Segment data-plane handler (Task 11b): capability-verified GET/PUT of
	// leased input/output segments on the {StagingDir}/{jobID} tree.
	segmentHandler := handler.NewSegmentHandler(cfg.Upscaler.StagingDir, jobRepo, segmentRepo, log)

	// Admin handler (Task 12a): CRUD for upscale jobs + fleet status.
	// Reached only via the gateway's admin-gated /api/upscale/* proxy that
	// injects X-Gateway-Internal; the router's requireGatewayInternal blocks
	// direct dials to upscaler:8096 that lack the header.
	adminHandler := handler.NewAdminHandler(
		jobRepo,
		workerRepo,
		cfg.Upscaler.DefaultScale,
		"", // defaultModel: admin must supply "model" in POST body
		log,
	)

	// Initialize router
	router := transport.NewRouter(log, metricsCollector, hub, enrollStore, segmentHandler, adminHandler)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("upscaler")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Background goroutines ─────────────────────────────────────────────────

	// Start sweeper (re-leases stale segments + marks gone workers every 30s).
	go sweeper.Run(context.Background())
	defer sweeper.Stop()

	// Start orchestrator (drives the job lifecycle every 10s + on startup).
	go orchestrator.Run(context.Background())
	defer orchestrator.Stop()

	// Start server
	go func() {
		log.Infow("starting upscaler service", "address", cfg.Server.Address())
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
