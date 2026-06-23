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
	"github.com/ILITA-hub/animeenigma/services/library/internal/autocache"
	"github.com/ILITA-hub/animeenigma/services/library/internal/config"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	lminio "github.com/ILITA-hub/animeenigma/services/library/internal/minio"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/animetosho"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/filename"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/jackett"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/nyaa"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
	libtorrent "github.com/ILITA-hub/animeenigma/services/library/internal/torrent"
	"github.com/ILITA-hub/animeenigma/services/library/internal/transport"
	"github.com/ILITA-hub/animeenigma/services/library/migrations"
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

// plannerSearchAdapter bridges autocache.searcher (the Planner's local seam,
// autocache.SearchQuery/SearchResult) to *service.TieredSearcher (whose FetchAll
// takes service.SearchParams). Keeps the autocache package free of any
// service-layer import — main.go owns the wiring, as with the other adapters.
type plannerSearchAdapter struct {
	t *service.TieredSearcher
}

func (a plannerSearchAdapter) FetchAll(ctx context.Context, q autocache.SearchQuery) (autocache.SearchResult, error) {
	res, err := a.t.FetchAll(ctx, service.SearchParams{
		Query: q.Query,
		MALID: q.MALID,
		Limit: q.Limit,
	})
	if err != nil {
		return autocache.SearchResult{}, err
	}
	return autocache.SearchResult{Releases: res.Releases}, nil
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
	// 004: extend job_source enum with 'jackett' (idempotent ADD VALUE).
	if err := db.DB.Exec(migrations.JackettSourceSQL).Error; err != nil {
		log.Fatalw("failed to apply jackett_source migration", "error", err)
	}
	// 005 (Phase 07): autocache pool — episode_source/episode_track enums +
	// five accounting-ledger columns on library_episodes. Runs AFTER 002.
	if err := db.DB.Exec(migrations.AutocachePoolSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_pool migration", "error", err)
	}
	// 006 (Phase 07): singleton autocache_config table holding the
	// live-editable §3.5 tunables + master `enabled` switch (POOL-04 +
	// POOL-05). Idempotent (CREATE TABLE IF NOT EXISTS + seeded via
	// ON CONFLICT DO NOTHING). Independent of 005.
	if err := db.DB.Exec(migrations.AutocacheConfigSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_config migration", "error", err)
	}
	// 007 (Phase 08): autocache_demand intake table + autocache_demand_reason
	// enum — the durable backfill-demand sink the ae serve MISS path writes
	// into. Idempotent (enum guard + CREATE TABLE IF NOT EXISTS). Independent
	// of 005/006.
	if err := db.DB.Exec(migrations.AutocacheDemandSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_demand migration", "error", err)
	}
	// 008 (Phase 09): extend the job_source enum with 'autocache' (idempotent
	// ADD VALUE IF NOT EXISTS) so the Planner can tag its enqueued rows. Enum
	// extension → order-free.
	if err := db.DB.Exec(migrations.AutocacheJobSourceSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_job_source migration", "error", err)
	}
	// 009 (Phase 09): add the nullable library_jobs.episode column (intended
	// episode persisted at enqueue) so single-flight dedup on
	// (shikimori_id, episode) works before filename detection. Alters
	// library_jobs → must follow 001 (already applied above).
	if err := db.DB.Exec(migrations.LibraryJobsEpisodeSQL).Error; err != nil {
		log.Fatalw("failed to apply library_jobs_episode migration", "error", err)
	}
	// 010 (Phase 09): extend the autocache_demand_reason enum with 'ongoing'
	// (Logic A). Idempotent ADD VALUE IF NOT EXISTS → order-free.
	if err := db.DB.Exec(migrations.AutocacheDemandOngoingSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_demand_ongoing migration", "error", err)
	}
	// 011 (v4.1 fix): add the newline-delimited `titles` column to autocache_demand
	// so the Planner searches trackers by anime title, not "<mal_id> <episode>".
	// Idempotent ADD COLUMN IF NOT EXISTS; must follow 007 (autocache_demand).
	if err := db.DB.Exec(migrations.AutocacheDemandTitlesSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_demand_titles migration", "error", err)
	}
	// 012 (v4.1): the append-only autocache_trigger_log (cause→effect — who/when/
	// what-watched per user-driven trigger + the target episode). Independent table.
	if err := db.DB.Exec(migrations.AutocacheTriggerLogSQL).Error; err != nil {
		log.Fatalw("failed to apply autocache_trigger_log migration", "error", err)
	}
	// 013 (audit L578): two partial indexes on library_jobs serving the Phase-09
	// hot queries — idx_library_jobs_shikimori_episode (HasActiveForEpisode) and
	// idx_library_jobs_autocache_inflight (SumInflightJobBytes, on every
	// EnsureRoom). Idempotent CREATE INDEX IF NOT EXISTS; must follow 001
	// (library_jobs), 008 (the 'autocache' job_source enum value its partial
	// predicate references) and 009 (episode column), all applied above.
	if err := db.DB.Exec(migrations.LibraryJobsEpisodeIndexSQL).Error; err != nil {
		log.Fatalw("failed to apply library_jobs_episode_index migration", "error", err)
	}

	// Start DB pool metrics collector.
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Redis client (added v4.0 Phase 3 — library had none before). The db_read
	// P95 ReadGate refresher needs a Redis handle to snapshot read_thresholds.
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Activity-register effect producer (AR-EFFECT-01). Non-blocking +
	// drop-on-full so an analytics outage never affects library jobs.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// AR-EFFECT-01 (D-15): GORM db_write/db_read effect callbacks + the daily-P95
	// ReadGate refresher (D-03). rootCtx cancellation (SIGTERM) stops the refresher.
	dbEffectsStop, err := gormtrace.WireDBEffects(rootCtx, db.DB, tracing.GlobalSink(), redisCache)
	if err != nil {
		log.Warnw("db-effect callbacks disabled", "error", err)
	}
	defer dbEffectsStop()

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

	// Phase 07 (POOL-02): one-time admin-content migration into the unified
	// pool. For every pre-existing admin episode still on the legacy
	// "{shikimori_id}/{ep}/" prefix, server-side-Move its MinIO objects into
	// aeProvider/<mal>/RAW/<ep>/ and repoint minio_path AFTER the copy succeeds.
	// Runs ONCE here — after migrations 005/006 applied (above) and the writer
	// is ready, but BEFORE the worker pools / HTTP server start (spec §10:
	// admin rows must be in the metered pool before the Phase-10 evictor).
	// Idempotent (skips aeProvider/ rows) + restart-safe; Warnw (never Fatalw)
	// on error so a partial/retryable migration re-runs next boot instead of
	// crash-looping. Mirrors the jobRepo.ResumeInterrupted* boot-once pattern.
	poolMigrator := autocache.NewMigrator(episodeRepo, writer, log)
	if migrated, err := poolMigrator.Migrate(rootCtx); err != nil {
		log.Warnw("autocache pool migration failed (will retry next boot)", "error", err)
	} else {
		log.Infow("autocache pool migration complete", "migrated", migrated)
	}

	// Phase 4: ffmpeg transcoder.
	transcoder := ffmpeg.NewTranscoder(ffmpeg.Config{
		BinaryPath:     cfg.Encode.FfmpegBin,
		FfprobePath:    cfg.Encode.FfprobeBin,
		Tmpdir:         cfg.Encode.Tmpdir,
		MaxBitrateKbps: cfg.Encode.MaxBitrateKbps,
		Threads:        cfg.Encode.Threads,
		Nice:           cfg.Encode.Nice,
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
		"threads", cfg.Encode.Threads,
		"nice", cfg.Encode.Nice,
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

	// Legacy two-provider aggregator (Nyaa + AnimeTosho) — now the FALLBACK
	// tier behind Jackett.
	aggregator := service.NewAggregator(nyaaClient, animeToshoAdapter{c: atClient}, log)

	// Jackett primary tier. Constructed only when an API key is configured;
	// a nil client makes the TieredSearcher a transparent pass-through to
	// the aggregator (identical to the pre-Jackett behaviour), so the
	// feature dark-ships safely behind JACKETT_API_KEY.
	var jackettSearcher service.JackettSearcher
	if cfg.Jackett.Enabled {
		jackettSearcher = jackett.NewClient(jackett.Config{
			BaseURL:     cfg.Jackett.BaseURL,
			APIKey:      cfg.Jackett.APIKey,
			Categories:  cfg.Jackett.Categories,
			HTTPTimeout: cfg.Jackett.HTTPTimeout,
			UserAgent:   cfg.Jackett.UserAgent,
		})
		log.Infow("jackett primary search tier enabled",
			"base_url", cfg.Jackett.BaseURL,
			"categories", cfg.Jackett.Categories,
			"timeout", cfg.Jackett.HTTPTimeout,
		)
	} else {
		log.Infow("jackett disabled (no JACKETT_API_KEY) — search uses Nyaa+AnimeTosho only")
	}
	tieredSearcher := service.NewTieredSearcher(jackettSearcher, aggregator, log)

	// Initialize handlers.
	// Phase 5 (LIB-09): health handler now exposes /health/extended for
	// the admin UI stats strip. It needs disk + active-torrent + active-
	// job-count sources.
	healthHandler := handler.NewHealthHandlerExtended(diskGuard, pool, jobRepo)
	searchHandler := handler.NewSearchHandler(tieredSearcher, log)
	// Phase 07 (POOL-04 + POOL-05): live-editable autocache config repo. Hoisted
	// ABOVE the jobs handler + Planner (was constructed lower) so the Phase-10
	// Evictor — which the jobs handler AND the Planner both need injected — can be
	// built from it before either consumer is constructed. It only needs db.DB.
	autocacheConfigRepo := repo.NewAutocacheConfigRepository(db.DB)

	// Phase 10 (EVICT-05): the autocache Evictor — the SINGLE budget-gate instance
	// shared by BOTH pre-admit paths (admin upload handler + autocache Planner) and
	// the periodic Accountant/reclaim sweep. Constructed from the already-built
	// autocacheConfigRepo (live budget + freshness windows), episodeRepo (pool
	// bytes + Stale candidates + ListPool + DeleteByID), writer (DeletePrefix), and
	// libMetrics — no reconstruction. Constructor arg order matches Plan 02:
	// (config, pool, objects, metrics, log). Started for its sweep; Stopped on
	// graceful shutdown (below).
	// WR-01: jobRepo is injected as the in-flight reservation seam (Σ size_bytes of
	// non-terminal autocache jobs) so the budget counts materialized + in-flight,
	// closing the admit→materialize overshoot. Arg order: (config, pool, jobs,
	// objects, metrics, log).
	evictor := autocache.NewEvictor(autocacheConfigRepo, episodeRepo, jobRepo, writer, libMetrics, log)
	evictor.Start(rootCtx)
	log.Infow("autocache evictor started")

	// Phase 5 (LIB-09): jobs handler now drives Link + Retry as well as
	// the Phase-3 CRUD. Needs the minio writer (Move + ListObjectsByPrefix)
	// + the EpisodeRepository. Phase 10: + the Evictor (second pre-admit gate).
	jobsHandler := handler.NewJobsHandlerWithLink(
		jobRepo,
		diskGuard,
		evictor,
		pool,
		writer,
		episodeRepo,
		libMetrics,
		cfg.Disk.MinFreePct,
		log,
	)

	// Phase 4: episodes handler (read-only).
	episodesHandler := handler.NewEpisodesHandler(episodeRepo, writer, log)

	// Phase 07 (POOL-04 + POOL-05): live-editable autocache config —
	// singleton GET/PATCH at /api/library/autocache/config. The typed
	// Get/Patch accessor stores the master `enabled` switch + §3.5
	// tunables for the downloader/evictor (Phases 8-10). autocacheConfigRepo
	// is constructed above (hoisted for the Phase-10 Evictor wiring).
	autocacheConfigHandler := handler.NewAutocacheConfigHandler(autocacheConfigRepo, log)

	// Phase 08 (SERVE-01/02/03): Docker-network-only serve-signal handler. The
	// ae-resolution producer (catalog, Plan 03) POSTs HIT → /internal/library/
	// autocache/fetch (BumpFetch + serve_total{hit}) and MISS → .../demand
	// (records a backfill demand + serve_total{miss}, gated by the master
	// `enabled` switch). Reuses the already-constructed episodeRepo (BumpFetch),
	// the new demandRepo, the autocacheConfigRepo (enabled switch), and the
	// existing libMetrics — no reconstruction.
	demandRepo := repo.NewDemandRepository(db.DB)
	triggerLogRepo := repo.NewTriggerLogRepository(db.DB)
	autocacheInternalHandler := handler.NewAutocacheInternalHandler(
		handler.NewGormAutocacheInternalDeps(episodeRepo, demandRepo, autocacheConfigRepo, triggerLogRepo),
		libMetrics,
		log,
	)

	// Phase 09 (TRIG-03/04/05): the autocache Planner — a config-gated ticker
	// that drains autocache_demand and turns wanted (mal_id, episode) rows into
	// RAW download jobs (single-flight dedup + RAW/quality/seeder gating). It
	// reuses the already-constructed demandRepo (Drain/Delete), episodeRepo
	// (present-check), jobRepo (HasActiveForEpisode + Create), autocacheConfigRepo
	// (live enabled + sweep_interval_min), and the tieredSearcher (adapted to the
	// Planner's local search seam) — no reconstruction. The master `enabled`
	// switch is read live inside the loop, so the Planner always starts but
	// no-ops when disabled (no boot-time gating). It owns its own ticker, so the
	// 120s WriteTimeout doesn't apply.
	planner := autocache.NewPlanner(
		demandRepo,
		episodeRepo,
		jobRepo,
		plannerSearchAdapter{t: tieredSearcher},
		autocacheConfigRepo,
		evictor, // Phase 10: pre-admit budget gate before enqueue (EVICT-04/05).
		libMetrics,
		log,
	)
	planner.Start(rootCtx)
	log.Infow("autocache planner started")

	// Initialize metrics collector (HTTP middleware).
	metricsCollector := metrics.NewCollector("library")

	// Initialize router.
	router := transport.NewRouter(
		healthHandler,
		searchHandler,
		jobsHandler,
		episodesHandler,
		autocacheConfigHandler,
		autocacheInternalHandler,
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

	// Phase 09: stop the autocache Planner first so it enqueues no new jobs
	// while the worker/encoder pools drain. It already observes rootCtx
	// cancellation (issued above); Stop() waits for the loop goroutine to exit.
	planner.Stop()

	// Phase 10: stop the Evictor sweep goroutine before MinIO teardown so no
	// in-flight DeletePrefix races the writer shutdown. It observes rootCtx
	// cancellation (issued above); Stop() waits for the loop goroutine to exit.
	evictor.Stop()

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
