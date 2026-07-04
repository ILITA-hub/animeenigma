package repo

// Tests for the activity-visibility enforcement on public watchlist reads
// (design 2026-06-12): the excludeHentai parameter on the list queries and
// the users.activity_visibility lookup.

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupListVisibilityTestDB(t *testing.T) *ListRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	stmts := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT,
			name_jp TEXT,
			poster_url TEXT,
			rating TEXT,
			episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0
		)`,
		`CREATE TABLE genres (id TEXT PRIMARY KEY, name TEXT, name_ru TEXT)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, avatar TEXT, activity_visibility TEXT)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			status TEXT DEFAULT 'watching',
			score INTEGER DEFAULT 0,
			episodes INTEGER NOT NULL DEFAULT 0,
			notes TEXT,
			tags TEXT,
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
		// anime-sfw is ordinary; anime-rx is rated rx; anime-genre carries
		// the Hentai genre without the rx rating (both predicate branches).
		`INSERT INTO animes (id, name, rating) VALUES
			('anime-sfw', 'Normal Show', 'pg_13'),
			('anime-rx', 'Rated Show', 'rx'),
			('anime-genre', 'Genre Show', 'r')`,
		`INSERT INTO genres (id, name) VALUES ('g-hentai', 'Hentai')`,
		`INSERT INTO anime_genres (anime_id, genre_id) VALUES ('anime-genre', 'g-hentai')`,
		`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes) VALUES
			('e1', 'u1', 'anime-sfw', 'completed', 8, 12),
			('e2', 'u1', 'anime-rx', 'completed', 7, 2),
			('e3', 'u1', 'anime-genre', 'watching', 0, 1)`,
	}
	for _, ddl := range stmts {
		require.NoError(t, db.Exec(ddl).Error)
	}
	return NewListRepository(db)
}

func TestGetByUserPaginated_ExcludeHentai(t *testing.T) {
	r := setupListVisibilityTestDB(t)
	ctx := context.Background()
	params := &domain.PaginationParams{Page: 1, PerPage: 20}

	entries, total, err := r.GetByUserPaginated(ctx, "u1", "", "", true, domain.ListFilters{}, params)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "anime-sfw", entries[0].AnimeID)

	entries, total, err = r.GetByUserPaginated(ctx, "u1", "", "", false, domain.ListFilters{}, params)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total, "owner path (excludeHentai=false) sees everything")
	assert.Len(t, entries, 3)
}

func TestGetByUserAndStatusesPaginated_ExcludeHentai(t *testing.T) {
	r := setupListVisibilityTestDB(t)
	ctx := context.Background()
	params := &domain.PaginationParams{Page: 1, PerPage: 20}

	entries, total, err := r.GetByUserAndStatusesPaginated(ctx, "u1", []string{"completed", "watching"}, "", true, domain.ListFilters{}, params)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "anime-sfw", entries[0].AnimeID)
}

func TestGetUserWatchlistStats_ExcludeHentai(t *testing.T) {
	r := setupListVisibilityTestDB(t)
	ctx := context.Background()

	stats, err := r.GetUserWatchlistStats(ctx, "u1", nil, true)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.TotalEntries)
	assert.EqualValues(t, 12, stats.TotalEpisodes)
	assert.Equal(t, map[string]int{"completed": 1}, stats.StatusCounts,
		"status counts honor the hentai-visibility filter")

	stats, err = r.GetUserWatchlistStats(ctx, "u1", nil, false)
	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.TotalEntries)
	assert.Equal(t, map[string]int{"completed": 2, "watching": 1}, stats.StatusCounts)
}

func TestGetUserActivityVisibility(t *testing.T) {
	r := setupListVisibilityTestDB(t)
	ctx := context.Background()

	require.NoError(t, r.db.Exec(`INSERT INTO users (id, activity_visibility) VALUES
		('u-none', 'none'), ('u-nh', 'non_hentai'), ('u-null', NULL)`).Error)

	assert.Equal(t, ActivityVisibilityNone, r.GetUserActivityVisibility(ctx, "u-none"))
	assert.Equal(t, ActivityVisibilityNonHentai, r.GetUserActivityVisibility(ctx, "u-nh"))
	assert.Equal(t, ActivityVisibilityAll, r.GetUserActivityVisibility(ctx, "u-null"),
		"NULL column value defaults to all")
	assert.Equal(t, ActivityVisibilityAll, r.GetUserActivityVisibility(ctx, "missing"),
		"missing user defaults to all")
}
