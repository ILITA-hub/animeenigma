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
	"github.com/ILITA-hub/animeenigma/services/player/internal/config"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs/signals"
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
		&domain.UserPrefsVersion{},
		&domain.Review{},
		&domain.SyncJob{},
		&domain.ActivityEvent{},
		&domain.RecUserSignals{},
		&domain.RecPopulationSignals{},
		&domain.RecCompletionCoOccurrence{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Phase 9: rec engine FK constraints + last_computed indexes — created via
	// raw SQL because GORM AutoMigrate does not infer FKs from struct tags
	// alone. Postgres 16 does NOT support `ALTER TABLE ... ADD CONSTRAINT IF
	// NOT EXISTS ...` for FKs, so each constraint is wrapped in a DO block
	// that checks pg_constraint first. Indexes use the native `CREATE INDEX
	// IF NOT EXISTS` form. Idempotent — re-running on already-migrated DBs is
	// a no-op. See spec docs/superpowers/specs/2026-05-03-rec-engine-design.md
	// §4.1.
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

	// Phase 10: Redis cache (required for recs handler + population orchestrator).
	// Player did not use Redis prior to Phase 10; this is the first required
	// dependency. If Redis is unreachable on boot we treat it as fatal because
	// the recs surface is now part of the public home page.
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis (player needs redis as of Phase 10 for recs caching)",
			"host", cfg.Redis.Host, "port", cfg.Redis.Port, "error", err)
	}

	// Phase 10: rec engine population cron (60-minute ticker).
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
		popOrch.Start(cronCtx, 60*time.Minute)
	}

	// Phase 11: rec engine USER signal cron (6-hour ticker) + debounced
	// on-write trigger. The orchestrator delegates to the existing
	// recs.Orchestrator (precompute.go) for per-user RunForUser semantics.
	// Cache invalidation on success busts recs:user:{user_id}:topN so the
	// next request picks up fresh signals.
	var userOrch *recs.UserOrchestrator
	{
		recsRepo := repo.NewRecsRepository(db.DB)
		s1 := signals.NewS1ScoreCluster(db.DB, recsRepo)
		s2 := signals.NewS2Metadata(db.DB)
		s5 := signals.NewS5Attribute(db.DB, recsRepo) // Phase 12 (REC-SIG-05)
		userPrecompute := recs.NewOrchestrator([]recs.SignalModule{s1, s2, s5})
		userOrch = recs.NewUserOrchestrator(userPrecompute, db.DB, redisCache, log)
		// 6-hour cadence per CONTEXT.md decisions §User-Signal Cron.
		// Boot tick runs immediately so logged-in users get fresh signals
		// within seconds of redeploy.
		userOrch.Start(cronCtx, 6*time.Hour)
	}

	// Phase 3 backfill: synthesize watch_progress.completed=true rows for legacy
	// data (any (user, anime, ep <= anime_list.episodes) without a completed=true
	// row). Idempotent — guarded by an early-exit check so it short-circuits on
	// every restart after the first deploy. Non-fatal if it fails.
	{
		var anyCompleted int
		_ = db.DB.Raw("SELECT 1 FROM watch_progress WHERE completed = true LIMIT 1").Scan(&anyCompleted).Error
		if anyCompleted == 0 {
			log.Infow("phase 3 backfill: synthesizing watch_progress.completed=true rows from anime_list.episodes")
			if err := db.DB.Exec(`
				INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
				SELECT gen_random_uuid(), al.user_id, al.anime_id, ep, 0, 0, true,
				       COALESCE(al.completed_at, al.updated_at), NOW(), NOW()
				FROM anime_list al
				CROSS JOIN LATERAL generate_series(1, al.episodes) ep
				WHERE al.episodes > 0
				ON CONFLICT (user_id, anime_id, episode_number) DO UPDATE
				SET completed = true, updated_at = NOW()
				WHERE watch_progress.completed = false
			`).Error; err != nil {
				log.Errorw("phase 3 backfill failed (non-fatal)", "error", err)
			} else {
				log.Infow("phase 3 backfill complete")
			}
		}
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
	prefService := service.NewPreferenceServiceWithTier2(prefRepo, log, service.Tier2Params{
		HalfLifeDays:   cfg.Tier2.HalfLifeDays,
		MinConfidence:  cfg.Tier2.MinConfidence,
		MaxHistoryRows: cfg.Tier2.MaxHistoryRows,
		DurationFloor:  cfg.Tier2.DurationFloor,
	})
	progressService := service.NewProgressService(progressRepo, prefService, log)
	// Phase 13 (REC-INFRA-03): recsRepo + cache args wired in Task 7;
	// passed as nil here so the existing build stays green between
	// the Task 5 list.go signature change and the Task 7 wiring commit.
	listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, userOrch, nil, nil, log)
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
	overrideHandler := handler.NewOverrideHandler(log)
	reportHandler := handler.NewReportHandler(log, cfg.Telegram.BotToken, cfg.Telegram.AdminChatID, cfg.Reports.Dir, cfg.Maintenance.URL)
	syncHandler := handler.NewSyncHandler(syncRepo, log)
	activityHandler := handler.NewActivityHandler(activityRepo, log)

	// Phase 10: recs handler (anonymous trending row).
	recsRepo := repo.NewRecsRepository(db.DB)
	recsHandler := handler.NewRecsHandler(db.DB, recsRepo, redisCache, nil, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("player")

	// Initialize router
	router := transport.NewRouter(progressHandler, listHandler, historyHandler, reviewHandler, malImportHandler, malExportHandler, shikimoriImportHandler, reportHandler, syncHandler, activityHandler, exportHandler, prefHandler, overrideHandler, recsHandler, cfg.JWT, log, metricsCollector)

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

	// Stop the recs population cron before draining HTTP traffic so a tick
	// in flight can finish on its own deadline rather than be aborted.
	cronCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}
