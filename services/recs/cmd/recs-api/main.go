// Package main is the recs service entrypoint (port 8094).
//
// Recs service — recommendation engine extracted from player.
// Boot sequence:
//
//  1. logger.Default()
//  2. config.Load() — JWT_SECRET required; SERVER_PORT defaults to 8094;
//     CATALOG_URL defaults to http://catalog:8081.
//  3. database.New(cfg.Database) — auto-creates the shared "animeenigma" DB
//     if it doesn't exist; uses the same handle every other service uses for
//     read-only cross-table queries.
//  4. gormtrace.InstrumentGORM — no-op unless TRACING_ENABLED=true.
//  5. metrics.StartDBPoolCollector — pool stats to Prometheus.
//  6. db.AutoMigrate — service-owned tables ONLY (rec_user_signals,
//     rec_population_signals, rec_completion_co_occurrence, rec_events,
//     rec_announcement_dismissals).
//     anime_list / watch_history / animes stay owned by player + catalog;
//     recs reads them without migrating them.
//  7. FK + index backfill (idempotent raw SQL — Postgres 16 has no
//     ADD CONSTRAINT IF NOT EXISTS for FKs).
//  8. cache.New(cfg.Redis) — fatal on failure; recs caching is the service's core.
//  9. Activity-register effect producer (AR-EFFECT-01).
// 10. Wire repos → signals → orchestrators → handlers.
// 11. Population cron (60min), user-precompute cron (6h), co-occurrence cron (24h).
// 12. transport.NewRouter
// 13. http.Server + graceful shutdown on SIGINT/SIGTERM.
// 14. On shutdown: cronCancel() BEFORE srv.Shutdown so in-flight cron
//     callbacks finish before the HTTP listener closes.
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
	"github.com/ILITA-hub/animeenigma/services/recs/internal/config"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "recs")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}

	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Service-owned tables ONLY — anime_list / watch_history / animes stay
	// owned by player + catalog; recs reads them without migrating them.
	if err := db.AutoMigrate(
		&domain.RecUserSignals{},
		&domain.RecPopulationSignals{},
		&domain.RecCompletionCoOccurrence{},
		&domain.RecEvent{},
		&domain.RecAnnouncementDismissal{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// FK constraints + indexes — created via raw SQL because GORM AutoMigrate
	// does not infer FKs from struct tags alone. Postgres 16 does NOT support
	// `ALTER TABLE ... ADD CONSTRAINT IF NOT EXISTS ...` for FKs, so each
	// constraint is wrapped in a DO block that checks pg_constraint first.
	// Indexes use the native `CREATE INDEX IF NOT EXISTS` form. Idempotent.
	{
		stmts := []string{
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_user_signals_user_id_fkey') THEN
					ALTER TABLE rec_user_signals
						ADD CONSTRAINT rec_user_signals_user_id_fkey
						FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_user_signals_s6_seed_anime_id_fkey') THEN
					ALTER TABLE rec_user_signals
						ADD CONSTRAINT rec_user_signals_s6_seed_anime_id_fkey
						FOREIGN KEY (s6_seed_anime_id) REFERENCES animes(id) ON DELETE SET NULL;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_population_signals_anime_id_fkey') THEN
					ALTER TABLE rec_population_signals
						ADD CONSTRAINT rec_population_signals_anime_id_fkey
						FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_co_occurrence_seed_fkey') THEN
					ALTER TABLE rec_completion_co_occurrence
						ADD CONSTRAINT rec_co_occurrence_seed_fkey
						FOREIGN KEY (seed_anime_id) REFERENCES animes(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_co_occurrence_candidate_fkey') THEN
					ALTER TABLE rec_completion_co_occurrence
						ADD CONSTRAINT rec_co_occurrence_candidate_fkey
						FOREIGN KEY (candidate_anime_id) REFERENCES animes(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_ann_dismiss_user_fkey') THEN
					ALTER TABLE rec_announcement_dismissals
						ADD CONSTRAINT rec_ann_dismiss_user_fkey
						FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_ann_dismiss_anime_fkey') THEN
					ALTER TABLE rec_announcement_dismissals
						ADD CONSTRAINT rec_ann_dismiss_anime_fkey
						FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`CREATE INDEX IF NOT EXISTS idx_rec_user_signals_last_computed
				ON rec_user_signals (last_computed)`,
			`CREATE INDEX IF NOT EXISTS idx_rec_co_occurrence_seed
				ON rec_completion_co_occurrence (seed_anime_id, co_count DESC)`,
		}
		for _, sql := range stmts {
			if err := db.DB.Exec(sql).Error; err != nil {
				log.Fatalw("failed to enforce rec engine FK/index", "sql", sql, "error", err)
			}
		}
	}

	// Redis client. Fatal on boot failure — recs caching is the service's core
	// (per Phase 10 comment in player: "recs surface is now part of the public
	// home page").
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis (recs needs redis for caching)",
			"host", cfg.Redis.Host, "port", cfg.Redis.Port, "error", err)
	}
	defer redisCache.Close()

	// Activity-register effect producer (AR-EFFECT-01). Non-blocking +
	// drop-on-full so an analytics outage never affects recs requests.
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

	// Recs population cron (60-minute ticker).
	// Spawned with a derived context so graceful shutdown can cancel it
	// before the HTTP server stops accepting traffic.
	cronCtx, cronCancel := context.WithCancel(context.Background())

	{
		recsRepo := repo.NewRecsRepository(db.DB)
		s3 := signals.NewS3Trending(db.DB, recsRepo)
		s4 := signals.NewS4Recency(db.DB)
		popOrch := recs.NewPopulationOrchestrator(
			[]recs.SignalModule{s3, s4},
			redisCache,
			log,
		)
		// 60-minute cadence per CONTEXT.md decisions §Population cron.
		// Boot tick runs immediately so cold start has data within seconds.
		// Seed a named origin so frame-less db_writes on this cron attribute to
		// "scheduled_job/recs-population" instead of "goroutine/unknown".
		popOrch.Start(tracing.SeedBaggage(cronCtx, "scheduled_job:recs-population", ""), 60*time.Minute)
	}

	// User-signal cron (6-hour ticker) + per-user precompute orchestrator.
	// userPrecompute is hoisted to outer scope so AdminRecsHandler.ForceRecompute
	// can call the SAME synchronous precompute orchestrator the cron uses.
	var userOrch *recs.UserOrchestrator
	var userPrecompute *recs.Orchestrator
	{
		recsRepoLocal := repo.NewRecsRepository(db.DB)
		s1 := signals.NewS1ScoreCluster(db.DB, recsRepoLocal)
		s2 := signals.NewS2Metadata(db.DB)
		s5 := signals.NewS5Attribute(db.DB, recsRepoLocal)
		userPrecompute = recs.NewOrchestrator([]recs.SignalModule{s1, s2, s5})
		userOrch = recs.NewUserOrchestrator(userPrecompute, db.DB, redisCache, log)
		// 6-hour cadence per CONTEXT.md decisions §User-Signal Cron.
		// Boot tick runs immediately so logged-in users get fresh signals
		// within seconds of redeploy.
		userOrch.Start(tracing.SeedBaggage(cronCtx, "scheduled_job:recs-user-precompute", ""), 6*time.Hour)
	}

	// Co-occurrence cron (24-hour ticker).
	// Materializes rec_completion_co_occurrence at score>=7 nightly.
	{
		coOccOrch := recs.NewCoOccurrenceOrchestrator(db.DB, log)
		coOccOrch.Start(tracing.SeedBaggage(cronCtx, "scheduled_job:recs-cooccurrence", ""), 24*time.Hour)
	}

	// Handlers.
	recsRepo := repo.NewRecsRepository(db.DB)
	shikimoriSimilarClient := signals.NewHTTPShikimoriSimilarClient(cfg.CatalogURL, log)
	s6 := signals.NewS6ComboPin(db.DB, recsRepo, shikimoriSimilarClient, log)
	recsHandler := handler.NewRecsHandler(db.DB, recsRepo, redisCache, s6, log)
	adminRecsHandler := handler.NewAdminRecsHandler(db.DB, recsRepo, redisCache, s6, userPrecompute, log)
	recEventsRepo := repo.NewRecEventsRepository(db.DB)
	recEventsHandler := handler.NewRecEventsHandler(recEventsRepo, log)
	hintDeps := handler.NewGormHintDeps(db.DB, recsRepo, redisCache, userOrch)
	internalHintHandler := handler.NewInternalHintHandler(hintDeps, log)

	// Metrics collector.
	metricsCollector := metrics.NewCollector("recs")

	// Router.
	router := transport.NewRouter(recsHandler, adminRecsHandler, recEventsHandler, internalHintHandler, cfg.JWT, log, metricsCollector)

	// HTTP server.
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("recs")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting recs service",
			"address", cfg.Server.Address(),
			"db_name", cfg.Database.Database,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down recs service...")

	// Stop the recs population cron before draining HTTP traffic so a tick
	// in flight can finish on its own deadline rather than be aborted.
	cronCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("recs service stopped")
}
