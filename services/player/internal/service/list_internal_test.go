package service

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupInternalListTestDB builds a minimal in-memory SQLite DB matching the
// columns the GetUserListByStatusesWithProgress query touches:
// anime_list × animes × watch_progress with a LEFT JOIN. The schema is wider
// than setupListServiceTestDB (which only covers MarkEpisodeWatched's
// IncrementEpisodes path) because the new method projects animes.name /
// name_ru / poster_url / episodes_aired which the original test schema does
// not include.
//
// SQLite-specific tweaks vs Postgres production:
//   - `to_char(al.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')` is rewritten as
//     `strftime('%Y-%m-%dT%H:%M:%SZ', al.updated_at)` via a registered scalar
//     so the production SQL text can execute unchanged. See the
//     `to_char` UDF below.
//   - `IN ?` is a GORM placeholder that GORM expands to `IN (?, ?, ...)` at
//     bind time on both Postgres and SQLite — no special handling needed.
var registerInternalListSqliteOnce sync.Once

func registerInternalListSqliteDriver() {
	registerInternalListSqliteOnce.Do(func() {
		sql.Register("sqlite3_internal_list", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				// Postgres NOW() — used by other queries in the package; defensive.
				if err := conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true); err != nil {
					return err
				}
				// Approximate Postgres to_char(timestamp, 'YYYY-MM-DD"T"HH24:MI:SS"Z"').
				// SQLite passes timestamps as strings or ISO already; we re-parse
				// then re-format to guarantee the trailing Z + T separator
				// production callers expect.
				return conn.RegisterFunc("to_char", func(ts string, _format string) string {
					// SQLite default storage for DATETIME is "YYYY-MM-DD HH:MM:SS"
					// (rfc3339 without T). Try parsing both forms.
					layouts := []string{
						"2006-01-02 15:04:05",
						"2006-01-02T15:04:05Z",
						time.RFC3339,
						"2006-01-02 15:04:05.999999999",
					}
					for _, layout := range layouts {
						if t, err := time.Parse(layout, ts); err == nil {
							return t.UTC().Format("2006-01-02T15:04:05Z")
						}
					}
					// Fall back to substituting the space with T and appending Z.
					return strings.Replace(ts, " ", "T", 1) + "Z"
				}, true)
			},
		})
	})
}

func setupInternalListTestDB(t *testing.T) (*ListService, *gorm.DB) {
	t.Helper()
	registerInternalListSqliteDriver()

	rawDB, err := sql.Open("sqlite3_internal_list", ":memory:")
	require.NoError(t, err)

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite3_internal_list",
		Conn:       rawDB,
	}, &gorm.Config{})
	require.NoError(t, err)

	ddl := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT,
			poster_url TEXT,
			episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0,
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
			review_text TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			is_rewatching INTEGER DEFAULT 0,
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
	}
	for _, d := range ddl {
		require.NoError(t, db.Exec(d).Error)
	}

	listRepo := repo.NewListRepository(db)
	log, err := logger.New(logger.Config{Level: "error", Development: false})
	require.NoError(t, err)
	svc := NewListService(listRepo, nil, nil, nil, nil, nil, log)
	return svc, db
}

func seedInternalListFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	// 2 anime + 2 list rows + 1 progress row.
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, name_ru, poster_url, episodes_count, episodes_aired) VALUES
		 ('anime-1', 'Bocchi the Rock!', 'Бочи Рок!', 'https://cdn/p1.jpg', 12, 12),
		 ('anime-2', 'Frieren', 'Фрирен', 'https://cdn/p2.jpg', 28, 14)`).Error)

	// Two list entries for user-1 with distinct statuses and timestamps.
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"al-1", "user-1", "anime-1", "planned", 0, 0,
		time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"al-2", "user-1", "anime-2", "watching", 0, 5,
		time.Now().Add(-1*time.Hour), time.Now().Add(-1*time.Hour)).Error)

	// Two watch_progress rows for anime-2 — ep 5 COMPLETED, ep 6 merely
	// sampled (completed=0). last_watched_episode must count only completed
	// rows (2026-06-11 alignment with the anime page's resume semantics), so
	// the projection below asserts 5, not 6. anime-1 deliberately has none
	// so the LEFT JOIN yielding 0 is exercised.
	require.NoError(t, db.Exec(
		`INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wp-1", "user-1", "anime-2", 5, 100, 1440, 1,
		time.Now().Add(-30*time.Minute), time.Now().Add(-30*time.Minute), time.Now().Add(-30*time.Minute)).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wp-2", "user-1", "anime-2", 6, 40, 1440, 0,
		time.Now().Add(-20*time.Minute), time.Now().Add(-20*time.Minute), time.Now().Add(-20*time.Minute)).Error)
}

func TestGetUserListByStatusesWithProgress_EmptyStatuses_ReturnsEmpty(t *testing.T) {
	svc, _ := setupInternalListTestDB(t)
	ctx := context.Background()

	out, err := svc.GetUserListByStatusesWithProgress(ctx, "user-1", []string{})
	require.NoError(t, err)
	assert.NotNil(t, out, "must return non-nil empty slice, not nil")
	assert.Len(t, out, 0)
}

func TestGetUserListByStatusesWithProgress_HappyPath_JoinsCorrectly(t *testing.T) {
	svc, db := setupInternalListTestDB(t)
	seedInternalListFixture(t, db)
	ctx := context.Background()

	out, err := svc.GetUserListByStatusesWithProgress(ctx, "user-1", []string{"planned", "watching"})
	require.NoError(t, err)
	require.Len(t, out, 2)

	// Items ORDER BY al.updated_at DESC → watching (anime-2, -1h) before planned (anime-1, -2h).
	assert.Equal(t, "anime-2", out[0].AnimeID, "watching row (more recent updated_at) must come first")
	assert.Equal(t, "watching", out[0].Status)
	assert.Equal(t, "Frieren", out[0].Name)
	assert.Equal(t, "Фрирен", out[0].NameRU)
	assert.Equal(t, "https://cdn/p2.jpg", out[0].PosterURL)
	assert.Equal(t, 28, out[0].EpisodesCount)
	assert.Equal(t, 14, out[0].EpisodesAired)
	assert.Equal(t, 5, out[0].LastWatchedEpisode,
		"must project max COMPLETED episode_number (ep 6 is sampled but not completed)")
	assert.NotEmpty(t, out[0].UpdatedAt)

	assert.Equal(t, "anime-1", out[1].AnimeID)
	assert.Equal(t, "planned", out[1].Status)
	assert.Equal(t, "Bocchi the Rock!", out[1].Name)
}

func TestGetUserListByStatusesWithProgress_LeftJoinMissingProgress_YieldsZero(t *testing.T) {
	svc, db := setupInternalListTestDB(t)
	seedInternalListFixture(t, db)
	ctx := context.Background()

	// Only fetch the planned row — that anime has NO watch_progress entry,
	// so the LEFT JOIN must produce last_watched_episode = 0 (not error,
	// not negative, not omitted).
	out, err := svc.GetUserListByStatusesWithProgress(ctx, "user-1", []string{"planned"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "anime-1", out[0].AnimeID)
	assert.Equal(t, 0, out[0].LastWatchedEpisode,
		"missing watch_progress row must yield last_watched_episode=0 via LEFT JOIN + COALESCE")
}

func TestGetUserListByStatusesWithProgress_FiltersByUser(t *testing.T) {
	svc, db := setupInternalListTestDB(t)
	seedInternalListFixture(t, db)
	ctx := context.Background()

	// Different user — must return zero items even though the fixture has
	// list rows for user-1 with matching statuses.
	out, err := svc.GetUserListByStatusesWithProgress(ctx, "user-other", []string{"planned", "watching"})
	require.NoError(t, err)
	assert.Len(t, out, 0)
}

func TestGetUserListByStatusesWithProgress_OnlyMatchingStatuses(t *testing.T) {
	svc, db := setupInternalListTestDB(t)
	seedInternalListFixture(t, db)
	ctx := context.Background()

	// Asking for "postponed" only — neither seeded row matches.
	out, err := svc.GetUserListByStatusesWithProgress(ctx, "user-1", []string{"postponed"})
	require.NoError(t, err)
	assert.Len(t, out, 0)
}
