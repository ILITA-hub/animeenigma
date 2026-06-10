package service

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// registerSQLiteGreatestForServiceTest mirrors the helper used in the repo
// package — registers a SQLite driver "sqlite3_with_greatest_svc" exposing a
// GREATEST(a, b) scalar so production-shape SQL using GREATEST (Postgres-only)
// can execute on the test SQLite DB. The driver name is distinct from the
// repo-package helper so the two test packages don't fight over registration.
var registerSQLiteGreatestSvcOnce sync.Once

func registerSQLiteGreatestForServiceTest() {
	registerSQLiteGreatestSvcOnce.Do(func() {
		sql.Register("sqlite3_with_greatest_svc", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("greatest", func(a, b int64) int64 {
					if a > b {
						return a
					}
					return b
				}, true); err != nil {
					return err
				}
				// Postgres NOW() — emulate with an ISO-8601 string so list_repo's
				// UPDATE…FROM SQL can execute against SQLite. (mattn driver
				// can't auto-convert Go time.Time to SQLite storage.)
				return conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true)
			},
		})
	})
}

// setupListServiceTestDB builds an in-memory SQLite DB with all tables that
// ListService.MarkEpisodeWatched touches: anime_list, watch_progress, animes
// (for the UPDATE…FROM JOIN inside IncrementEpisodes), watch_history,
// activity_events, anime_preferences. Returns a configured ListService.
func setupListServiceTestDB(t *testing.T) (*ListService, *gorm.DB) {
	registerSQLiteGreatestForServiceTest()

	rawDB, err := sql.Open("sqlite3_with_greatest_svc", ":memory:")
	require.NoError(t, err)

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite3_with_greatest_svc",
		Conn:       rawDB,
	}, &gorm.Config{})
	require.NoError(t, err)

	tableSQL := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			episodes_count INTEGER DEFAULT 0,
			deleted_at DATETIME
		)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			status TEXT DEFAULT 'plan_to_watch',
			score INTEGER DEFAULT 0,
			episodes INTEGER NOT NULL DEFAULT 0,
			notes TEXT,
			tags TEXT,
			-- Phase 1 (workstream: social) — review_text + username are
			-- new columns on AnimeListEntry. domain.AnimeListEntry maps
			-- them with GORM tags so production INSERTs reference them
			-- unconditionally; this hand-rolled SQLite schema must mirror
			-- the struct shape or db.Create fails with "no such column".
			review_text TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			is_rewatching INTEGER DEFAULT 0,
			rewatch_count INTEGER DEFAULT 0,
			priority TEXT,
			mal_id INTEGER,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
		`CREATE TABLE watch_progress (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			episode_number INTEGER NOT NULL,
			progress INTEGER DEFAULT 0,
			duration INTEGER DEFAULT 0,
			completed INTEGER DEFAULT 0,
			watch_count INTEGER DEFAULT 1,
			dropped_off_at INTEGER,
			last_watched_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id, episode_number)
		)`,
		`CREATE TABLE watch_history (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			episode_number INTEGER NOT NULL,
			player TEXT NOT NULL,
			language TEXT NOT NULL,
			watch_type TEXT NOT NULL,
			translation_id TEXT,
			translation_title TEXT,
			duration_watched INTEGER DEFAULT 0,
			session_id TEXT,
			watched_at DATETIME
		)`,
		`CREATE TABLE activity_events (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			username TEXT,
			anime_id TEXT,
			type TEXT,
			old_value TEXT,
			new_value TEXT,
			content TEXT,
			created_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE anime_preferences (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			player TEXT,
			language TEXT,
			watch_type TEXT,
			translation_id TEXT,
			translation_title TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
		// Empty genres + join table so Preload("Anime.Genres") doesn't blow up.
		`CREATE TABLE genres (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT
		)`,
		`CREATE TABLE anime_genres (
			anime_id TEXT,
			genre_id TEXT
		)`,
	}
	for _, ddl := range tableSQL {
		require.NoError(t, db.Exec(ddl).Error)
	}

	listRepo := repo.NewListRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	prefRepo := repo.NewPreferenceRepository(db)
	progressRepo := repo.NewProgressRepository(db)
	log, err := logger.New(logger.Config{Level: "error", Development: false})
	require.NoError(t, err)

	// userOrchestrator / recsRepo / cache nil — these tests don't exercise
	// the Phase 11 debounced trigger path or the Phase 13 synchronous S6
	// seed-update path; MarkEpisodeWatched nil-guards each before invoking.
	svc := NewListService(listRepo, activityRepo, prefRepo, progressRepo, nil, nil, nil, nil, log)
	return svc, db
}

// readWatchProgressCompleted returns whether a watch_progress row exists for
// (user, anime, ep) and whether its completed flag is true. Bypasses the
// repo layer for direct fact-checking.
func readWatchProgressCompleted(t *testing.T, db *gorm.DB, userID, animeID string, episode int) (exists bool, completed bool) {
	t.Helper()
	var c int
	err := db.Raw(`SELECT completed FROM watch_progress WHERE user_id=? AND anime_id=? AND episode_number=?`,
		userID, animeID, episode).Scan(&c).Error
	require.NoError(t, err)
	if c == 0 {
		// Distinguish "row missing" from "row present, completed=false (0)" via a count probe.
		var count int64
		err := db.Raw(`SELECT COUNT(*) FROM watch_progress WHERE user_id=? AND anime_id=? AND episode_number=?`,
			userID, animeID, episode).Scan(&count).Error
		require.NoError(t, err)
		if count == 0 {
			return false, false
		}
		return true, false
	}
	return true, true
}

// TestListService_MarkEpisodeWatched_FlipsWatchProgressCompleted is the headline
// Phase 3 service-level invariant: calling MarkEpisodeWatched produces a
// watch_progress row with completed=true for the marked episode, regardless of
// which internal branch fires (existing entry incremented, no-op for already-
// marked episode, or auto-create for first-time mark).
func TestListService_MarkEpisodeWatched_FlipsWatchProgressCompleted(t *testing.T) {
	t.Run("auto-create branch (anime not in user's list yet)", func(t *testing.T) {
		svc, db := setupListServiceTestDB(t)
		ctx := context.Background()

		// Seed an anime row so IncrementEpisodes' JOIN with animes resolves.
		now := time.Now()
		require.NoError(t, db.Exec(`INSERT INTO animes (id, name, episodes_count, deleted_at) VALUES (?, ?, ?, NULL)`,
			"anime-1", "Test Anime", 24).Error)

		req := &domain.MarkEpisodeWatchedRequest{Episode: 1}
		_, err := svc.MarkEpisodeWatched(ctx, "user-1", "anime-1", req)
		require.NoError(t, err)

		exists, completed := readWatchProgressCompleted(t, db, "user-1", "anime-1", 1)
		require.True(t, exists, "watch_progress row must exist after MarkEpisodeWatched (auto-create branch)")
		assert.True(t, completed, "watch_progress.completed must be true after MarkEpisodeWatched")
		_ = now // keep import live if needed
	})

	t.Run("increment branch (anime already in user's list, lower episode count)", func(t *testing.T) {
		svc, db := setupListServiceTestDB(t)
		ctx := context.Background()

		now := time.Now()
		require.NoError(t, db.Exec(`INSERT INTO animes (id, name, episodes_count, deleted_at) VALUES (?, ?, ?, NULL)`,
			"anime-1", "Test Anime", 24).Error)
		require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, episodes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"al-1", "user-1", "anime-1", "watching", 4, now, now).Error)

		req := &domain.MarkEpisodeWatchedRequest{Episode: 5}
		_, err := svc.MarkEpisodeWatched(ctx, "user-1", "anime-1", req)
		require.NoError(t, err)

		exists, completed := readWatchProgressCompleted(t, db, "user-1", "anime-1", 5)
		require.True(t, exists)
		assert.True(t, completed, "watch_progress.completed must be true after IncrementEpisodes branch")
	})

	t.Run("session_id is persisted to watch_history (Phase 5 G-04-lite)", func(t *testing.T) {
		svc, db := setupListServiceTestDB(t)
		ctx := context.Background()

		// Seed both anime + an existing watching list entry so MarkEpisodeWatched
		// hits the increment-and-record branch (the auto-create branch returns
		// early without writing to watch_history — a separate, pre-existing
		// concern).
		now := time.Now()
		require.NoError(t, db.Exec(`INSERT INTO animes (id, name, episodes_count, deleted_at) VALUES (?, ?, ?, NULL)`,
			"anime-1", "Test Anime", 24).Error)
		require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, episodes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"al-1", "user-1", "anime-1", "watching", 2, now, now).Error)

		req := &domain.MarkEpisodeWatchedRequest{
			Episode:   3,
			Player:    "kodik",
			Language:  "en",
			WatchType: "sub",
			SessionID: "11111111-2222-3333-4444-555555555555",
		}
		_, err := svc.MarkEpisodeWatched(ctx, "user-1", "anime-1", req)
		require.NoError(t, err)

		var sessionID string
		err = db.Raw(`SELECT session_id FROM watch_history WHERE user_id=? AND anime_id=? AND episode_number=?`,
			"user-1", "anime-1", 3).Scan(&sessionID).Error
		require.NoError(t, err)
		assert.Equal(t, "11111111-2222-3333-4444-555555555555", sessionID,
			"session_id from request must round-trip into watch_history.session_id")
	})

	t.Run("idempotent: re-marking an already-completed episode keeps completed=true", func(t *testing.T) {
		svc, db := setupListServiceTestDB(t)
		ctx := context.Background()

		now := time.Now()
		require.NoError(t, db.Exec(`INSERT INTO animes (id, name, episodes_count, deleted_at) VALUES (?, ?, ?, NULL)`,
			"anime-1", "Test Anime", 24).Error)
		require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, episodes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"al-1", "user-1", "anime-1", "watching", 5, now, now).Error)

		req := &domain.MarkEpisodeWatchedRequest{Episode: 5}
		// First call: IncrementEpisodes will be a no-op (5 < 5 is false), service
		// hits the "episode already marked" log path, then unconditionally calls MarkCompleted.
		_, err := svc.MarkEpisodeWatched(ctx, "user-1", "anime-1", req)
		require.NoError(t, err)

		// Second call: same path, MarkCompleted is idempotent.
		_, err = svc.MarkEpisodeWatched(ctx, "user-1", "anime-1", req)
		require.NoError(t, err)

		exists, completed := readWatchProgressCompleted(t, db, "user-1", "anime-1", 5)
		require.True(t, exists)
		assert.True(t, completed, "watch_progress.completed must be true after repeated MarkEpisodeWatched calls")
	})
}
