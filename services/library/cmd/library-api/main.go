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
	"github.com/ILITA-hub/animeenigma/services/library/internal/config"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/migrations"
	"github.com/ILITA-hub/animeenigma/services/library/internal/parser/animetosho"
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

	// Apply the Phase-3 migration (job_source / job_status enums +
	// library_jobs table + partial index). Idempotent thanks to the
	// DO $$ ... EXCEPTION blocks in the SQL.
	if err := db.DB.Exec(migrations.LibraryJobsSQL).Error; err != nil {
		log.Fatalw("failed to apply library_jobs migration", "error", err)
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
	healthHandler := handler.NewHealthHandler()
	searchHandler := handler.NewSearchHandler(aggregator, log)
	jobsHandler := handler.NewJobsHandler(
		jobRepo,
		diskGuard,
		pool,
		libMetrics,
		cfg.Disk.MinFreePct,
		log,
	)

	// Initialize metrics collector (HTTP middleware).
	metricsCollector := metrics.NewCollector("library")

	// Initialize router.
	router := transport.NewRouter(
		healthHandler,
		searchHandler,
		jobsHandler,
		cfg.JWT,
		log,
		metricsCollector,
	)

	// Create HTTP server. WriteTimeout is generous (120s) because
	// Phase 3+ adds long-running torrent + transcode admin endpoints;
	// parity with the themes service.
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
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

	// Drain the worker pool first so any in-flight 'downloading' rows
	// get rewritten back to 'queued' before the DB connection closes.
	if err := pool.Stop(shutdownCtx, 20*time.Second); err != nil {
		log.Warnw("worker pool stop", "error", err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}
