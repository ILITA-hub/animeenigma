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

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/config"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/ingest"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/job"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	svc "github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
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

	// Postgres store is ALWAYS constructed — it is the dual-write primary /
	// source of truth and the reversible-rollback path. The PG tables/view are
	// migrated above regardless of backend.
	pgStore := repo.NewPostgresStore(db.DB)

	// Select the EventStore on ANALYTICS_STORE_BACKEND (default "postgres" keeps
	// the system exactly as today). "clickhouse"/"dualwrite" open the native CH
	// connection + run EnsureSchema. On CH failure: clickhouse → Fatal (explicit
	// CH-only must not silently fall back); dualwrite → WARN + degrade to PG-only
	// (a CH outage must never take analytics down — T-01-07 reversibility).
	log.Infow("analytics store backend", "backend", cfg.StoreBackend)
	var store domain.EventStore = pgStore
	var chConn driver.Conn
	if cfg.StoreBackend == "clickhouse" || cfg.StoreBackend == "dualwrite" {
		conn, err := repo.OpenClickHouse(repo.CHConfig{
			Addr:     cfg.ClickHouse.Addr(),
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.User,
			Password: cfg.ClickHouse.Password,
		})
		if err == nil {
			err = repo.EnsureSchema(context.Background(), conn)
		}
		if err != nil {
			if cfg.StoreBackend == "clickhouse" {
				log.Fatalw("clickhouse backend selected but unavailable", "error", err)
			}
			log.Warnw("dualwrite: clickhouse unavailable at boot — degrading to postgres-only", "error", err)
		} else {
			chConn = conn
			defer chConn.Close()
			chStore := repo.NewClickHouseStore(chConn)
			if cfg.StoreBackend == "clickhouse" {
				store = chStore
			} else {
				store = repo.NewDualWriteStore(pgStore, chStore, log)
			}
		}
	}

	batcher := ingest.New(store, ingest.Config{
		MaxBatch:      cfg.MaxBatch,
		FlushInterval: cfg.FlushInterval,
		BufferSize:    cfg.BufferSize,
	}).WithLogger(log).WithDropHook(func() { observ.EventsDropped.Inc() })
	batcher.Start()

	collectHandler := handler.NewCollectHandler(countingSink{batcher}, cfg.IPSalt)
	// Log-only FE error sink — one structured Warnw line per accepted frontend
	// error (→ ClickHouse otel_logs via the OTel filelog receiver) + a
	// kind-labelled Prometheus counter. No DB write, no event-store enqueue.
	clientErrorHandler := handler.NewClientErrorHandler(handler.NewLoggingClientErrorSink(log), cfg.IPSalt)
	// Player telemetry handler shares the same batcher Sink — player resolve/stall
	// rows flow through the identical InsertBatch write path as clickstream rows,
	// stored under effect_kind "player_resolve" / "player_stall".
	playerTelemetryHandler := handler.NewPlayerTelemetryHandler(countingSink{batcher})
	// The effects handler shares the same batcher Sink — effect rows flow
	// through the identical InsertBatch write path as clickstream rows.
	effectsHandler := handler.NewEffectsHandler(countingSink{batcher})
	adminHandler := handler.NewAdminHandler(repoEraser{db: db, chConn: chConn})

	// Redis publisher for the daily db_read P95 recompute (D-03). The hash is
	// the fan-out channel the 7 GORM services snapshot into their ReadGate. A
	// Redis outage at boot is NON-fatal: the recompute endpoint then degrades to
	// a no-op (nil writer) and the GORM services keep their last good 48h hash —
	// analytics' core ingestion must never depend on Redis being up.
	var readThresholdHandler *handler.ReadThresholdHandler
	var playerRankingHandler *handler.PlayerRankingHandler
	redisCache, rerr := cache.New(cfg.Redis)
	if rerr != nil {
		log.Warnw("redis unavailable — read-threshold recompute disabled this boot", "error", rerr)
	} else {
		defer redisCache.Close()
		readThresholdSvc := svc.NewReadThresholdService(chConn, redisThresholdWriter{cache: redisCache})
		readThresholdHandler = handler.NewReadThresholdHandler(readThresholdSvc)
		// Stage 2b daily provider-reliability recompute + player_ranking Redis
		// publish. Needs ClickHouse for the telemetry aggregates, so it is only
		// wired when the CH backend is active (chConn != nil); the service's own
		// Recompute also nil-guards conn/redis defensively.
		if chConn != nil {
			playerRankingSvc := svc.NewPlayerRankingService(chConn, redisRankingWriter{cache: redisCache})
			playerRankingHandler = handler.NewPlayerRankingHandler(playerRankingSvc)
		}
	}

	purgeJob := job.NewPurgeJob(repoPurger{db: db}, cfg.RetentionDays, log)
	c := cron.New()
	_, _ = c.AddFunc(cfg.PurgeCron, func() {
		_, _ = purgeJob.RunOnce(context.Background())
	})
	c.Start()
	defer c.Stop()

	collector := metrics.NewCollector("analytics")
	router := transport.NewRouter(collectHandler, clientErrorHandler, playerTelemetryHandler, effectsHandler, adminHandler, readThresholdHandler, playerRankingHandler, log, collector)

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
