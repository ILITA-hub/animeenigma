package repo

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// registerSQLiteGreatestOnce registers a custom SQLite driver
// "sqlite3_with_greatest" that exposes a GREATEST(a, b) scalar function so
// that production-shape SQL using GREATEST (Postgres-only) is exercisable
// against an in-memory SQLite test DB. Idempotent.
var registerSQLiteGreatestOnce sync.Once

func registerSQLiteWithGreatest() {
	registerSQLiteGreatestOnce.Do(func() {
		sql.Register("sqlite3_with_greatest", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				greatest := func(a, b int64) int64 {
					if a > b {
						return a
					}
					return b
				}
				return conn.RegisterFunc("greatest", greatest, true)
			},
		})
	})
}

// setupProgressTestDB returns an in-memory SQLite ProgressRepository with the
// watch_progress table created in a shape compatible with both UpsertProgress
// and MarkCompleted (composite unique constraint on user_id, anime_id,
// episode_number — same as production). Registers a GREATEST UDF for SQLite
// so UpsertProgress's GREATEST(watch_progress.duration, ?) Postgres expression
// can execute. Returns the underlying *gorm.DB so tests can seed via raw
// inserts when they want to bypass the upsert path.
func setupProgressTestDB(t *testing.T) (*ProgressRepository, *gorm.DB) {
	registerSQLiteWithGreatest()

	rawDB, err := sql.Open("sqlite3_with_greatest", ":memory:")
	require.NoError(t, err)

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite3_with_greatest",
		Conn:       rawDB,
	}, &gorm.Config{})
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE watch_progress (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		episode_number INTEGER NOT NULL,
		progress INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		completed INTEGER DEFAULT 0,
		last_watched_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE (user_id, anime_id, episode_number)
	)`).Error
	require.NoError(t, err)

	return NewProgressRepository(db), db
}

// seedProgressRow inserts a watch_progress row directly via raw SQL, bypassing
// UpsertProgress's GREATEST(...) expression which is Postgres-only.
func seedProgressRow(t *testing.T, db *gorm.DB, userID, animeID string, episode, progress, duration int, completed bool) {
	t.Helper()
	now := time.Now()
	err := db.Exec(`INSERT INTO watch_progress
		(id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"seed-"+userID+"-"+animeID+"-"+itoa(episode), userID, animeID, episode, progress, duration, completed, now, now, now).Error
	require.NoError(t, err)
}

// itoa: tiny stdlib-free int-to-string for test seed IDs.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// TestProgressRepository_UpsertProgress_PreservesCompletedTrue is the
// regression test for the heartbeat bug Phase 3 fixes: the old Upsert always
// overwrote completed with the struct's zero value (false), which meant any
// future "completed=true" row would be flipped back on the next heartbeat.
// UpsertProgress must NOT touch the completed column.
func TestProgressRepository_UpsertProgress_PreservesCompletedTrue(t *testing.T) {
	repo, db := setupProgressTestDB(t)
	ctx := context.Background()

	// Pre-seed a row with completed=true (skip MarkCompleted to keep this
	// test independent of MarkCompleted's correctness).
	seedProgressRow(t, db, "user-1", "anime-1", 5, 0, 0, true)

	// Heartbeat save (this is what UpdateProgress sends — Completed is the
	// zero value because the service no longer hardcodes it).
	heartbeat := &domain.WatchProgress{
		UserID:        "user-1",
		AnimeID:       "anime-1",
		EpisodeNumber: 5,
		Progress:      1200,
		Duration:      1440,
	}
	require.NoError(t, repo.UpsertProgress(ctx, heartbeat))

	// Read back — completed must STILL be true.
	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 5)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed, "UpsertProgress must not flip completed=true back to false")
	assert.Equal(t, 1200, p.Progress, "progress should be updated by heartbeat")
}

func TestProgressRepository_MarkCompleted_CreatesRowIfMissing(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 3))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 3)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed)
	assert.Equal(t, 0, p.Progress, "synthetic backfill row has progress=0")
	assert.Equal(t, 0, p.Duration, "synthetic backfill row has duration=0")
	assert.Equal(t, 3, p.EpisodeNumber)
}

func TestProgressRepository_MarkCompleted_Idempotent(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 7))
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 7))
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 7))

	// Should still have exactly one row, completed=true.
	results, err := repo.GetByUserAndAnime(ctx, "user-1", "anime-1")
	require.NoError(t, err)
	require.Len(t, results, 1, "MarkCompleted must be idempotent — no duplicate rows")
	assert.True(t, results[0].Completed)
}

// TestProgressRepository_MarkCompleted_FlipsExistingFalseRow ensures that
// when a heartbeat already created a watch_progress row (completed=false,
// real progress data), MarkCompleted flips completed=true while preserving
// the existing progress and duration values.
func TestProgressRepository_MarkCompleted_FlipsExistingFalseRow(t *testing.T) {
	repo, db := setupProgressTestDB(t)
	ctx := context.Background()

	// Seed a heartbeat-style row (completed=false, real progress data) via
	// raw SQL to avoid GREATEST() in UpsertProgress under SQLite.
	seedProgressRow(t, db, "user-1", "anime-1", 9, 500, 1440, false)

	// Confirm pre-condition.
	pre, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 9)
	require.NoError(t, err)
	require.NotNil(t, pre)
	require.False(t, pre.Completed, "seeded row starts with completed=false")
	require.Equal(t, 500, pre.Progress)

	// Mark completed.
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 9))

	// Read back: completed=true, progress and duration UNCHANGED.
	post, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 9)
	require.NoError(t, err)
	require.NotNil(t, post)
	assert.True(t, post.Completed, "MarkCompleted flips completed to true")
	assert.Equal(t, 500, post.Progress, "MarkCompleted must preserve existing progress")
	assert.Equal(t, 1440, post.Duration, "MarkCompleted must preserve existing duration")
}

// TestProgressRepository_MarkCompleted_PreservesLastWatchedAtMonotonic
// guarantees that calling MarkCompleted bumps last_watched_at forward.
func TestProgressRepository_MarkCompleted_PreservesLastWatchedAtMonotonic(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 4))
	first, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 4)
	require.NoError(t, err)
	firstTS := first.LastWatchedAt

	time.Sleep(10 * time.Millisecond)

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 4))
	second, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 4)
	require.NoError(t, err)
	assert.True(t, !second.LastWatchedAt.Before(firstTS),
		"second MarkCompleted must update last_watched_at to a value >= the first")
}
