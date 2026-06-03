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
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/library/internal/config"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	lminio "github.com/ILITA-hub/animeenigma/services/library/internal/minio"
	"github.com/ILITA-hub/animeenigma/services/library/migrations"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/animetosho"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/filename"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/nyaa"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
	libtorrent "github.com/ILITA-hub/animeenigma/services/library/internal/torrent"
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

// torrentClientAdapter adapts *libtorrent.Client → service.TorrentAdder
// so the worker doesn't have to import the torrent package directly.
type torrentClientAdapter struct {
	c *libtorrent.Client
}

func (a torrentClientAdapter) Add(ctx context.Context, magnetURI string) (libtorrent.DownloadHandle, error) {
	return a.c.Add(ctx, magnetURI)
}

// main is the entry point for the library service (workstream raw-jp / v0.2).
// Phase 2 wired the Nyaa + AnimeTosho parser clients, the merger, and
// the search HTTP handler into the chi router; Phase 3 adds the
// embedded torrent client, the FOR UPDATE SKIP LOCKED job queue, the
// worker pool, the disk-free guard, library-specific Prometheus
// metrics, and the admin-gated /api/library/jobs CRUD. Auth is
// enforced at the gateway — the library service trusts what the
// gateway forwards.
func main() {
	// Initialize logger.
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "library")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Root context — propagates SIGTERM down to workers + disk guard.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Initialize database. libs/database.New auto-creates the `library`
	// database on first run (idempotent across restarts).
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}

	// Apply the Phase-3 migration (job_source / job_status enums +
	// library_jobs table + partial index). Idempotent thanks to the
	// DO $$ ... EXCEPTION blocks in the SQL.
	if err := db.DB.Exec(migrations.LibraryJobsSQL).Error; err != nil {
		log.Fatalw("failed to apply library_jobs migration", "error", err)
	}
	// Phase 4 migrations: library_episodes (FK → library_jobs) and
	// library_filename_patterns (seeded). Order matters — 002 must
	// follow 001 for the FK to resolve.
	if err := db.DB.Exec(migrations.LibraryEpisodesSQL).Error; err != nil {
		log.Fatalw("failed to apply library_episodes migration", "error", err)
	}
	if err := db.DB.Exec(migrations.LibraryFilenamePatternsSQL).Error; err != nil {
		log.Fatalw("failed to apply library_filename_patterns migration", "error", err)
	}

	// Start DB pool metrics collector.
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Job repository + startup resumption (CONTEXT decision):
	// any row left in 'downloading' from a previous process is
	// flipped back to 'queued' once at boot so a worker re-claims it.
	jobRepo := repo.NewJobRepository(db.DB)
	if resumed, err := jobRepo.ResumeInterruptedDownloads(rootCtx); err != nil {
		log.Warnw("resume interrupted downloads failed", "error", err)
	} else if resumed > 0 {
		log.Infow("resumed interrupted downloads", "count", resumed)
	}
	if resumed, err := jobRepo.ResumeInterruptedEncodes(rootCtx); err != nil {
		log.Warnw("resume interrupted encodes failed", "error", err)
	} else if resumed > 0 {
		log.Infow("resumed interrupted encodes", "count", resumed)
	}

	// Phase 4: episode + filename-pattern repos.
	episodeRepo := repo.NewEpisodeRepository(db.DB)
	patternRepo := repo.NewFilenamePatternRepository(db.DB)

	// Library-specific Prometheus collectors registered against the
	// default registry so the existing /metrics endpoint exposes them.
	libMetrics := libmetrics.NewLibraryMetrics()

	// Torrent client (single per process — CONTEXT-locked).
	tc, err := libtorrent.NewClient(libtorrent.Config{
		DownloadDir:    cfg.Torrent.DownloadDir,
		MaxPeers:       cfg.Torrent.MaxPeers,
		UploadRateKBPS: cfg.Torrent.UploadRateKBPS,
		SeedDuration:   cfg.Torrent.SeedDuration,
	})
	if err != nil {
		log.Fatalw("failed to start torrent client", "error", err)
	}
	defer func() { _ = tc.Close() }()
	log.Infow("torrent client started",
		"download_dir", cfg.Torrent.DownloadDir,
		"max_peers", cfg.Torrent.MaxPeers,
		"upload_kbps", cfg.Torrent.UploadRateKBPS,
		"seed_duration", cfg.Torrent.SeedDuration,
	)

	// Disk-free guard. Run() polls every cfg.Disk.PollInterval and
	// updates library_disk_free_bytes.
	diskGuard := service.NewDiskGuard(cfg.Torrent.DownloadDir, libMetrics, log)
	go diskGuard.Run(rootCtx, cfg.Disk.PollInterval)

	// Worker pool. workers race for queued jobs via FOR UPDATE SKIP
	// LOCKED; each drives queued → downloading and stops at
	// status='encoding' (Phase 4 owns the encoder).
	pool := service.NewWorkerPool(
		cfg.Worker.Count,
		jobRepo,
		torrentClientAdapter{c: tc},
		libMetrics,
		cfg.Torrent.StallTimeout,
		cfg.Worker.ProgressTick,
		log,
	)
	pool.Start(rootCtx)
	log.Infow("worker pool started",
		"workers", cfg.Worker.Count,
		"stall_timeout", cfg.Torrent.StallTimeout,
		"progress_tick", cfg.Worker.ProgressTick,
	)

	// Phase 4: MinIO writer — idempotent bucket bootstrap at startup.
	writer, err := lminio.New(lminio.Config{
		Endpoint:          cfg.Minio.Endpoint,
		AccessKey:         cfg.Minio.AccessKey,
		SecretKey:         cfg.Minio.SecretKey,
		Bucket:            cfg.Minio.Bucket,
		UseSSL:            cfg.Minio.UseSSL,
		UploadConcurrency: cfg.Minio.UploadConcurrency,
	}, log)
	if err != nil {
		log.Fatalw("minio client init", "error", err)
	}
	if err := writer.EnsureBucket(rootCtx); err != nil {
		log.Fatalw("minio bucket bootstrap", "error", err)
	}
	log.Infow("minio writer ready",
		"endpoint", cfg.Minio.Endpoint, "bucket", cfg.Minio.Bucket, "ssl", cfg.Minio.UseSSL)

	// Phase 4: ffmpeg transcoder.
	transcoder := ffmpeg.NewTranscoder(ffmpeg.Config{
		BinaryPath:     cfg.Encode.FfmpegBin,
		FfprobePath:    cfg.Encode.FfprobeBin,
		Tmpdir:         cfg.Encode.Tmpdir,
		MaxBitrateKbps: cfg.Encode.MaxBitrateKbps,
	}, log)

	// Phase 4: filename detector — patterns loaded once at startup.
	detector, err := filename.NewDetectorFromDB(rootCtx, patternRepo, libMetrics)
	if err != nil {
		log.Fatalw("filename detector init (bad seed?)", "error", err)
	}
	log.Infow("filename detector ready")

	// Phase 4: source path resolver (walks {downloadDir}/{infohash}/).
	sourceResolver := service.NewDefaultSourceResolver(cfg.Torrent.DownloadDir)

	// Phase 06 (workstream raw-jp / v0.2): best-effort cache-bust
	// webhook fired from the encoder worker after every successful
	// status=done. nil-safe — the encoder worker handles a nil
	// invalidator gracefully (the catalog's 1h TTL covers
	// correctness if the webhook is disabled).
	catalogInvalidator := service.NewCatalogInvalidator(
		service.InvalidatorConfig{
			CatalogInternalAPIURL: cfg.CatalogInternal.APIURL,
			Timeout:               cfg.CatalogInternal.Timeout,
		},
		libMetrics,
		log,
	)

	// Phase 4: encoder worker pool.
	encoderPool := service.NewEncoderPool(
		cfg.Encode.Workers,
		jobRepo,
		episodeRepo,
		transcoder,
		writer,
		detector,
		sourceResolver,
		libMetrics,
		log,
		catalogInvalidator,
	)
	encoderPool.Start(rootCtx)
	log.Infow("encoder pool started",
		"workers", cfg.Encode.Workers,
		"tmpdir", cfg.Encode.Tmpdir,
		"ffmpeg_bin", cfg.Encode.FfmpegBin,
		"max_bitrate_kbps", cfg.Encode.MaxBitrateKbps,
	)

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

	// Aggregator + search handler.
	aggregator := service.NewAggregator(nyaaClient, animeToshoAdapter{c: atClient}, log)

	// Initialize handlers.
	// Phase 5 (LIB-09): health handler now exposes /health/extended for
	// the admin UI stats strip. It needs disk + active-torrent + active-
	// job-count sources.
	healthHandler := handler.NewHealthHandlerExtended(diskGuard, pool, jobRepo)
	searchHandler := handler.NewSearchHandler(aggregator, log)
	// Phase 5 (LIB-09): jobs handler now drives Link + Retry as well as
	// the Phase-3 CRUD. Needs the minio writer (Move + ListObjectsByPrefix)
	// + the EpisodeRepository.
	jobsHandler := handler.NewJobsHandlerWithLink(
		jobRepo,
		diskGuard,
		pool,
		writer,
		episodeRepo,
		libMetrics,
		cfg.Disk.MinFreePct,
		log,
	)

	// Phase 4: episodes handler (read-only).
	episodesHandler := handler.NewEpisodesHandler(episodeRepo, writer, log)

	// Initialize metrics collector (HTTP middleware).
	metricsCollector := metrics.NewCollector("library")

	// Initialize router.
	router := transport.NewRouter(
		healthHandler,
		searchHandler,
		jobsHandler,
		episodesHandler,
		cfg.JWT,
		log,
		metricsCollector,
	)

	// Create HTTP server. WriteTimeout is generous (120s) because
	// Phase 3+ adds long-running torrent + transcode admin endpoints;
	// parity with the themes service.
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("library")(router),
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

	// Cancel rootCtx → disk guard + worker pool start shutting down.
	rootCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Phase 4: stop the encoder pool FIRST so no new uploads kick off
	// after MinIO is being torn down. Encoder workers respond to
	// rootCtx cancellation (already issued above).
	if err := encoderPool.Stop(15 * time.Second); err != nil {
		log.Warnw("encoder pool stop", "error", err)
	}

	// Drain the worker pool next so any in-flight 'downloading' rows
	// get rewritten back to 'queued' before the DB connection closes.
	if err := pool.Stop(shutdownCtx, 20*time.Second); err != nil {
		log.Warnw("worker pool stop", "error", err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}
