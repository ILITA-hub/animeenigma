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
	"github.com/ILITA-hub/animeenigma/services/player/internal/config"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
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

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "player")
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
		// Profile showcase (Steam-style wall). Pure config store; content
		// resolved on the frontend. Dark-shipped via gateway PROFILE_WALL_ADMIN_ONLY.
		&domain.ProfileShowcase{},
		// AUTO-408 — emoji reactions on reviews. FK to anime_list(id) with
		// ON DELETE CASCADE is added via raw SQL below (GORM doesn't infer FKs
		// from struct tags alone).
		&domain.ReviewReaction{},
		&domain.SyncJob{},
		&domain.ActivityEvent{},
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

	// Hero-spotlight workstream Plan 03-01 (HSB-NF-02 / HSB-BE-30): standalone
	// index on watch_progress.updated_at so the spotlight `now_watching` resolver
	// can `WHERE updated_at > NOW() - INTERVAL '5 minutes'` cheaply as
	// watch_progress grows. The GORM tag in domain/watch.go declares this index,
	// but GORM AutoMigrate silently skips adding new single-column indexes to
	// tables that already exist (no DDL change emitted), so the index never
	// appeared on the deployed DB. Raw idempotent backfill — no-op on fresh DBs
	// where AutoMigrate did create it. (Found during Plan 03-07 smoke check 8.)
	if err := db.DB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_watch_progress_updated_at
		ON watch_progress (updated_at)
	`).Error; err != nil {
		log.Fatalw("failed to create idx_watch_progress_updated_at", "error", err)
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

	// review_reactions FK — created via raw SQL because GORM AutoMigrate does
	// not infer FKs from struct tags alone. Postgres 16 does NOT support
	// `ALTER TABLE ... ADD CONSTRAINT IF NOT EXISTS ...` for FKs, so the
	// constraint is wrapped in a DO block that checks pg_constraint first.
	// Idempotent — re-running on already-migrated DBs is a no-op.
	// (The Phase 9 rec-engine FK/index statements were removed when the recs
	// engine moved out of player to services/recs — extraction 2026-06-11.)
	{
		stmts := []string{
			// AUTO-408 — review_reactions.review_id → anime_list(id) ON DELETE
			// CASCADE so removing a review/list row cleans up its reactions.
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'review_reactions_review_id_fkey') THEN
					ALTER TABLE review_reactions
						ADD CONSTRAINT review_reactions_review_id_fkey
						FOREIGN KEY (review_id) REFERENCES anime_list(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
		}
		for _, sql := range stmts {
			if err := db.DB.Exec(sql).Error; err != nil {
				log.Fatalw("failed to enforce review_reactions FK", "sql", sql, "error", err)
			}
		}
	}

	// Denormalized-username sync (2026-06-10). anime_list, comments,
	// review_reactions and activity_events each carry a username copy so list
	// pages don't join users; some historical write paths left it '' (rendered
	// as the «Пользователь» fallback) and renames never propagated. A plain FK
	// ON UPDATE CASCADE can't sync a non-key column, so this is trigger-based:
	//   1. one-shot backfill of stale/empty copies from users
	//   2. AFTER UPDATE OF username ON users → propagate to all 4 copies
	//   3. BEFORE INSERT/UPDATE ON anime_list → auto-fill empty username
	// CREATE OR REPLACE FUNCTION/TRIGGER (PG14+) makes every statement
	// idempotent — re-running on already-migrated DBs is a no-op.
	{
		stmts := []string{
			`UPDATE anime_list SET username = u.username
				FROM users u
				WHERE anime_list.user_id = u.id
				  AND anime_list.username IS DISTINCT FROM u.username`,
			`CREATE OR REPLACE FUNCTION propagate_username_change() RETURNS trigger AS $$
			BEGIN
				UPDATE anime_list SET username = NEW.username WHERE user_id = NEW.id;
				UPDATE comments SET username = NEW.username WHERE user_id = NEW.id;
				UPDATE review_reactions SET username = NEW.username WHERE user_id = NEW.id;
				UPDATE activity_events SET username = NEW.username WHERE user_id = NEW.id;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql`,
			`CREATE OR REPLACE TRIGGER trg_users_propagate_username
				AFTER UPDATE OF username ON users
				FOR EACH ROW
				WHEN (OLD.username IS DISTINCT FROM NEW.username)
				EXECUTE FUNCTION propagate_username_change()`,
			`CREATE OR REPLACE FUNCTION fill_anime_list_username() RETURNS trigger AS $$
			BEGIN
				IF NEW.username IS NULL OR NEW.username = '' THEN
					SELECT u.username INTO NEW.username FROM users u WHERE u.id = NEW.user_id;
					NEW.username := COALESCE(NEW.username, '');
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql`,
			`CREATE OR REPLACE TRIGGER trg_anime_list_fill_username
				BEFORE INSERT OR UPDATE ON anime_list
				FOR EACH ROW
				EXECUTE FUNCTION fill_anime_list_username()`,
		}
		for _, sql := range stmts {
			if err := db.DB.Exec(sql).Error; err != nil {
				log.Fatalw("failed to apply denormalized-username sync migration", "sql", sql, "error", err)
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

	// Activity-register effect producer (AR-EFFECT-01). Non-blocking +
	// drop-on-full so an analytics outage never affects player requests.
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

	// The recs engine (population / user-signal / co-occurrence crons) moved
	// out of player to services/recs (port 8094) — extraction 2026-06-11.
	// MarkEpisodeWatched now POSTs a fire-and-forget recompute hint to recs
	// instead of running these crons in-process (see RecsHintProducer below).

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

	// Initialize services
	prefService := service.NewPreferenceServiceWithTier2(prefRepo, log, service.Tier2Params{
		HalfLifeDays:   cfg.Tier2.HalfLifeDays,
		MinConfidence:  cfg.Tier2.MinConfidence,
		MaxHistoryRows: cfg.Tier2.MaxHistoryRows,
		DurationFloor:  cfg.Tier2.DurationFloor,
	})
	// Cache per-anime community popularity (audit #14) so the COUNT(DISTINCT)
	// GROUP BY stays off the hot /preferences/resolve path.
	prefService.SetCommunityCache(redisCache)
	// Phase 9 / TRIG-02 (Logic B): fire-and-forget player→library autocache
	// demand producer. When an active JP-audio watcher starts episode N of a
	// watching anime, UpdateProgress fires a next_ep demand for N+1 so the
	// library autocache Planner pre-downloads it. Same drop-on-full / nil-safe
	// contract as recsHintProducer — a slow/down library never blocks the
	// heartbeat. Start now / defer Stop so the worker drains on shutdown.
	demandProducer := service.NewDemandProducer(
		cfg.Autocache.LibraryURL,
		cfg.Autocache.DemandEnabled,
		log,
	)
	demandProducer.Start()
	defer demandProducer.Stop()

	progressService := service.NewProgressService(progressRepo, prefService, demandProducer, log)
	// Phase 4 (gacha): construct the non-blocking credit producer. Start before
	// ListService so EpisodeWatched/TitleCompleted can fire immediately once the
	// service is live. defer Stop so the worker drains any queued credits on
	// graceful shutdown before the process exits.
	gachaProducer := service.NewGachaCreditProducer(
		cfg.Gacha.InternalURL,
		cfg.Gacha.CreditEpisode,
		cfg.Gacha.CreditTitle,
		cfg.Gacha.Enabled,
		log,
	)
	gachaProducer.Start()
	defer gachaProducer.Stop()

	// Recs extraction Phase 1 — fire-and-forget recompute hints to recs:8094.
	// Replaces the in-process recs crons + the synchronous S6 seed update that
	// MarkEpisodeWatched used to run before the recs engine moved out of player.
	// Same drop-on-full / nil-safe contract as the gacha producer above.
	recsHintProducer := service.NewRecsHintProducer(
		cfg.Recs.InternalURL,
		cfg.Recs.HintEnabled,
		log,
	)
	recsHintProducer.Start()
	defer recsHintProducer.Stop()

	listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, recsHintProducer, gachaProducer, log)
	historyService := service.NewHistoryService(historyRepo, log)
	reviewService := service.NewReviewService(listRepo, activityRepo, log)

	// Phase 1 (workstream: social) plan 04 — comments CRUD pipeline.
	// Plan 03 built CommentRepository / CommentService / CommentHandler as
	// production-ready stubs; this is the wiring that makes them reachable
	// over HTTP. The chi routes are mounted in transport.NewRouter below.
	commentRepo := repo.NewCommentRepository(db.DB)
	commentService := service.NewCommentService(commentRepo, activityRepo, log)
	commentHandler := handler.NewCommentHandler(commentService, log)

	// Profile showcase (Steam-style wall, dark-shipped via gateway
	// PROFILE_WALL_ADMIN_ONLY). Pure config store; content resolved on FE.
	showcaseRepo := repo.NewShowcaseRepository(db.DB)
	showcaseService := service.NewShowcaseService(showcaseRepo, log)
	showcaseHandler := handler.NewShowcaseHandler(showcaseService, log)

	// Showcase v2 — compatibility score between two users' watchlists.
	// Dark-shipped under the same PROFILE_WALL_ADMIN_ONLY gateway gate.
	compatRepo := repo.NewCompatibilityRepository(db.DB)
	compatService := service.NewCompatibilityService(compatRepo)
	compatibilityHandler := handler.NewCompatibilityHandler(compatService, log)

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
	// AUTO-417 — feedback triage notification loop. Fire-and-forget producer
	// to the notifications service; an outage never affects reports/triage.
	feedbackNotifier := service.NewFeedbackNotifier(cfg.Notify.InternalURL, cfg.Notify.Enabled, log)
	reportHandler := handler.NewReportHandler(log, cfg.Telegram.BotToken, cfg.Telegram.AdminChatID, cfg.Reports.Dir, cfg.Maintenance.URL, feedbackNotifier)
	// Admin feedback browser reads the same on-disk report archive (REPORTS_DIR).
	adminReportsHandler := handler.NewAdminReportsHandler(log, cfg.Reports.Dir, feedbackNotifier)
	syncHandler := handler.NewSyncHandler(syncRepo, log)
	activityHandler := handler.NewActivityHandler(activityRepo, log)

	// The recs HTTP surface (anonymous trending row, admin debug/force-recompute,
	// public events telemetry) moved out of player to services/recs — extraction
	// 2026-06-11. Player now only emits recompute hints via RecsHintProducer.

	// Workstream hero-spotlight v1.0 Phase 3 — internal endpoint that the
	// catalog spotlight aggregator hits to resolve `not_time_yet` and
	// `continue_watching_new` cards. Mounted by NewRouter OUTSIDE /api with
	// no auth middleware (docker-network trust boundary).
	internalListHandler := handler.NewInternalListHandler(listService, log)

	// Aggregate anime-page context endpoint (page-fetch optimization
	// 2026-06-11) — one optional-auth round-trip replacing the page's
	// separate rating / watchers-count / progress / watchlist / my-review /
	// preference fetches.
	viewerContextHandler := handler.NewViewerContextHandler(progressService, listService, reviewService, prefService, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("player")

	// Initialize router
	router := transport.NewRouter(progressHandler, listHandler, historyHandler, reviewHandler, commentHandler, showcaseHandler, compatibilityHandler, malImportHandler, malExportHandler, shikimoriImportHandler, reportHandler, syncHandler, activityHandler, exportHandler, prefHandler, overrideHandler, adminReportsHandler, internalListHandler, viewerContextHandler, cfg.JWT, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("player")(router),
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
