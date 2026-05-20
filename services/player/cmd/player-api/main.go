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
	"gorm.io/gorm"
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
		// Phase 1 (workstream: social) — the legacy `reviews` table was
		// merged into anime_list by runSocialMigration (Plan 01) and the
		// associated Go type was deleted entirely by Plan 02. The six
		// review endpoints now run through ListRepository.
		//
		// New `comments` table for the comments feature; domain struct in
		// services/player/internal/domain/comment.go.
		&domain.Comment{},
		&domain.SyncJob{},
		&domain.ActivityEvent{},
		&domain.RecUserSignals{},
		&domain.RecPopulationSignals{},
		&domain.RecCompletionCoOccurrence{},
		&domain.RecEvent{}, // Phase 14 (REC-EVAL-01)
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Compound unique index for ON CONFLICT in progress UPSERTs.
	// AutoMigrate creates it on fresh boots via struct tags; this
	// idempotent raw SQL backfills it on databases that pre-date the
	// tag change. See audit Wave 1 (perf): single-column indexes
	// can't enforce the (user, anime, episode) uniqueness that
	// progress.go's ON CONFLICT clause assumes.
	if err := db.DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_watch_progress_user_anime_ep
		ON watch_progress (user_id, anime_id, episode_number)
	`).Error; err != nil {
		log.Fatalw("failed to create watch_progress compound index", "error", err)
	}

	// Audit Wave 2 T2: drop the redundant standalone idx_watch_progress_user_id.
	// The compound index above ((user_id, anime_id, episode_number)) already
	// serves WHERE user_id = $1 queries via Postgres prefix matching, so the
	// standalone secondary is pure write overhead. Idempotent: no-op when the
	// index never existed (fresh DBs post-GORM-tag-removal).
	if err := db.DB.Exec(`DROP INDEX IF EXISTS idx_watch_progress_user_id`).Error; err != nil {
		log.Fatalw("failed to drop redundant idx_watch_progress_user_id", "error", err)
	}

	// Phase 1 (workstream: social) — one-shot idempotent migration that
	// merges every legacy `reviews` row into `anime_list` then drops the
	// `reviews` table. Guarded by db.Migrator().HasTable("reviews"); after
	// the first successful run the table is gone, so subsequent boots
	// short-circuit and emit NO `social migration` log line. The helper
	// lives below main() so the test in main_test.go can exercise it
	// directly against an in-memory SQLite DB (HasTable is dialect-aware
	// and works on both Postgres + SQLite).
	if err := runSocialMigration(db.DB, log); err != nil {
		log.Fatalw("social migration failed", "error", err)
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
	//
	// Phase 14: userPrecompute is hoisted to outer scope (vs. its previous
	// block-local declaration) so the AdminRecsHandler.ForceRecompute path
	// can call the SAME synchronous precompute orchestrator the cron uses
	// — avoids spinning up a parallel module list and keeps admin debug
	// in lockstep with the production precompute schedule.
	var userOrch *recs.UserOrchestrator
	var userPrecompute *recs.Orchestrator
	{
		recsRepoLocal := repo.NewRecsRepository(db.DB)
		s1 := signals.NewS1ScoreCluster(db.DB, recsRepoLocal)
		s2 := signals.NewS2Metadata(db.DB)
		s5 := signals.NewS5Attribute(db.DB, recsRepoLocal) // Phase 12 (REC-SIG-05)
		userPrecompute = recs.NewOrchestrator([]recs.SignalModule{s1, s2, s5})
		userOrch = recs.NewUserOrchestrator(userPrecompute, db.DB, redisCache, log)
		// 6-hour cadence per CONTEXT.md decisions §User-Signal Cron.
		// Boot tick runs immediately so logged-in users get fresh signals
		// within seconds of redeploy.
		userOrch.Start(cronCtx, 6*time.Hour)
	}

	// Phase 13 (REC-SIG-06): rec engine CO-OCCURRENCE cron (24-hour ticker).
	// Materializes rec_completion_co_occurrence at score>=7 nightly. The S6
	// pin resolver reads from this table on every recs request; nightly
	// materialization is enough at production scale (~1900 rows; millisecond
	// runtime). Per CONTEXT.md decisions §B4.
	{
		coOccOrch := recs.NewCoOccurrenceOrchestrator(db.DB, log)
		coOccOrch.Start(cronCtx, 24*time.Hour)
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
	// WR-02 (Phase 8): attach the service logger so the repo can warn when
	// ListContinueWatching's INNER JOIN drops orphan or soft-deleted anime.
	progressRepo := repo.NewProgressRepository(db.DB).WithLogger(log)
	listRepo := repo.NewListRepository(db.DB)
	historyRepo := repo.NewHistoryRepository(db.DB)
	// Phase 1 (workstream: social) plan 02 — the legacy review repository
	// is gone. The six review endpoints now run through ListRepository via
	// ReviewService.
	syncRepo := repo.NewSyncRepository(db.DB)
	activityRepo := repo.NewActivityRepository(db.DB)

	// Mark stale sync jobs as failed on startup
	if err := syncRepo.MarkStaleJobsFailed(context.Background(), 1*time.Hour); err != nil {
		log.Errorw("failed to mark stale sync jobs", "error", err)
	}

	// Initialize repositories (preference)
	prefRepo := repo.NewPreferenceRepository(db.DB)
	// Phase 13 (REC-INFRA-03): recsRepo is hoisted up here (vs. its
	// previous location next to recsHandler) so ListService can take it
	// for the synchronous S6 seed-update path inside MarkEpisodeWatched.
	recsRepo := repo.NewRecsRepository(db.DB)

	// Initialize services
	prefService := service.NewPreferenceServiceWithTier2(prefRepo, log, service.Tier2Params{
		HalfLifeDays:   cfg.Tier2.HalfLifeDays,
		MinConfidence:  cfg.Tier2.MinConfidence,
		MaxHistoryRows: cfg.Tier2.MaxHistoryRows,
		DurationFloor:  cfg.Tier2.DurationFloor,
	})
	progressService := service.NewProgressService(progressRepo, prefService, log)
	// Phase 13 (REC-INFRA-03): recsRepo + redisCache wired so MarkEpisodeWatched
	// can synchronously update s6_seed_* and bust recs:user:{id}:topN when a
	// completion qualifies (status='completed' AND score>=7).
	listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, userOrch, recsRepo, redisCache, log)
	historyService := service.NewHistoryService(historyRepo, log)
	reviewService := service.NewReviewService(listRepo, activityRepo, log)

	// Phase 1 (workstream: social) plan 04 — comments CRUD pipeline.
	// Plan 03 built CommentRepository / CommentService / CommentHandler as
	// production-ready stubs; this is the wiring that makes them reachable
	// over HTTP. The chi routes are mounted in transport.NewRouter below.
	commentRepo := repo.NewCommentRepository(db.DB)
	commentService := service.NewCommentService(commentRepo, activityRepo, log)
	commentHandler := handler.NewCommentHandler(commentService, log)

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
	// Phase 13 (REC-SIG-06): also wires the S6 combo-watched-after pin
	// resolver. The HTTP shikimori-similar client points at the catalog
	// service's internal docker DNS — same convention as
	// services/player/internal/handler/{mal,shikimori}_import.go.
	shikimoriSimilarClient := signals.NewHTTPShikimoriSimilarClient("http://catalog:8081", log)
	s6 := signals.NewS6ComboPin(db.DB, recsRepo, shikimoriSimilarClient, log)
	recsHandler := handler.NewRecsHandler(db.DB, recsRepo, redisCache, s6, log)

	// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug + force-recompute.
	// userPrecompute is the same Orchestrator the user-orchestrator cron
	// uses for its 6h precompute pass — admin force-recompute calls the
	// same synchronous RunForUser path so admin clicks always recompute
	// (NOT the debounced TriggerForUser, which would NO-OP after the
	// second click within 5 minutes).
	adminRecsHandler := handler.NewAdminRecsHandler(db.DB, recsRepo, redisCache, s6, userPrecompute, log)

	// Phase 14 (REC-EVAL-01): public events endpoint. JWT-OPTIONAL so
	// anonymous trending CTR data flows. Persists to rec_events table for
	// v2.1 weight-tuning queries; increments rec_click_total /
	// rec_watched_total Prometheus counters via libs/metrics/recs.go.
	recEventsRepo := repo.NewRecEventsRepository(db.DB)
	recEventsHandler := handler.NewRecEventsHandler(recEventsRepo, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("player")

	// Initialize router
	router := transport.NewRouter(progressHandler, listHandler, historyHandler, reviewHandler, commentHandler, malImportHandler, malExportHandler, shikimoriImportHandler, reportHandler, syncHandler, activityHandler, exportHandler, prefHandler, overrideHandler, recsHandler, adminRecsHandler, recEventsHandler, cfg.JWT, log, metricsCollector)

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

// runSocialMigration is the one-shot idempotent bootstrap that merges legacy
// `reviews` rows into `anime_list` (preserving non-zero list scores, copying
// review_text + username), backfills empty `anime_list.username` from `users`,
// and drops the `reviews` table.
//
// Idempotency contract (SOCIAL-NF-02):
//   - Guard via db.Migrator().HasTable("reviews"). Dialect-aware (works on
//     both Postgres and SQLite, so the unit test can exercise the same code
//     path without an information_schema.tables probe).
//   - After the first successful run, the `reviews` table no longer exists,
//     so every subsequent invocation short-circuits and emits no log line.
//
// Failure mode: forward-only. On any step failure the function returns an
// error and the caller (main) escalates to log.Fatalw so the service
// crash-loops until ops intervenes (data integrity > availability for a
// one-shot data migration).
//
// Takes `*gorm.DB` directly (not the project's `*database.DB` wrapper) so
// the test in main_test.go can pass an in-memory SQLite DB.
func runSocialMigration(db *gorm.DB, log *logger.Logger) error {
	if !db.Migrator().HasTable("reviews") {
		// Already migrated on a previous boot — short-circuit silently.
		return nil
	}

	log.Infow("social migration: merging reviews into anime_list")

	// Dialect probe: Postgres uses gen_random_uuid() + NOW(); SQLite uses
	// a hex(randomblob(...)) expression + CURRENT_TIMESTAMP. Both support
	// `INSERT ... ON CONFLICT (cols) DO UPDATE SET ... = EXCLUDED.x` so
	// the upsert shape itself is portable.
	dialect := db.Dialector.Name()
	var newUUID, now string
	switch dialect {
	case "postgres":
		newUUID = "gen_random_uuid()"
		now = "NOW()"
	case "sqlite":
		// SQLite has no native UUID — fabricate a v4-shaped hex string.
		newUUID = "lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-' || hex(randomblob(2)) || '-' || hex(randomblob(2)) || '-' || hex(randomblob(6)))"
		now = "CURRENT_TIMESTAMP"
	default:
		newUUID = "gen_random_uuid()"
		now = "NOW()"
	}

	// Step A — upsert reviews into anime_list.
	//   * New row: status='completed', score=reviews.score, review_text +
	//     username copied verbatim.
	//   * Existing row: preserve non-zero score (user already chose one in
	//     their watchlist); overwrite review_text; only fill username when
	//     the existing row's username is empty (NULLIF guard).
	// SQLite parses `INSERT INTO ... SELECT ... ON CONFLICT` ambiguously
	// (the ON CONFLICT could bind to the SELECT or the INSERT). The fix
	// per https://sqlite.org/lang_upsert.html is an explicit `WHERE true`
	// before ON CONFLICT — Postgres accepts the same form, so it stays
	// portable across both dialects.
	upsertSQL := `
		INSERT INTO anime_list (
			id, user_id, anime_id, status, score, episodes,
			review_text, username, created_at, updated_at
		)
		SELECT ` + newUUID + `, r.user_id, r.anime_id, 'completed',
		       r.score, 0,
		       r.review_text, r.username,
		       ` + now + `, ` + now + `
		FROM reviews r
		WHERE true
		ON CONFLICT (user_id, anime_id) DO UPDATE SET
			score        = CASE WHEN anime_list.score = 0 THEN EXCLUDED.score ELSE anime_list.score END,
			review_text  = EXCLUDED.review_text,
			username     = COALESCE(NULLIF(EXCLUDED.username, ''), anime_list.username),
			updated_at   = ` + now
	if err := db.Exec(upsertSQL).Error; err != nil {
		return err
	}

	// Step B — backfill empty anime_list.username by JOINing users.
	// The JOIN syntax differs between Postgres (UPDATE ... FROM) and
	// SQLite (UPDATE ... FROM, supported since 3.33). Both are accepted
	// here. If the `users` table does not exist (legacy test fixture
	// might not provision it), skip step B rather than fail the whole
	// migration — the username is best-effort denormalization.
	if db.Migrator().HasTable("users") {
		backfillSQL := `
			UPDATE anime_list SET username = u.username, updated_at = ` + now + `
			FROM users u
			WHERE anime_list.user_id = u.id
			  AND (anime_list.username IS NULL OR anime_list.username = '')`
		if err := db.Exec(backfillSQL).Error; err != nil {
			return err
		}
	}

	// Step C — drop the now-redundant `reviews` table.
	if err := db.Exec("DROP TABLE reviews").Error; err != nil {
		return err
	}

	log.Infow("social migration complete")
	return nil
}
