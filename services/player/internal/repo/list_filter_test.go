package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFilterTestDB(t *testing.T) *ListRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	stmts := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY, name TEXT, name_ru TEXT, name_jp TEXT,
			poster_url TEXT, episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0, kind TEXT, year INTEGER DEFAULT 0
		)`,
		`CREATE TABLE genres (id TEXT PRIMARY KEY, name TEXT, name_ru TEXT)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, anime_id TEXT NOT NULL,
			status TEXT, score INTEGER, episodes INTEGER NOT NULL DEFAULT 0,
			notes TEXT, tags TEXT, review_text TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '', is_rewatching INTEGER DEFAULT 0,
			rewatch_count INTEGER DEFAULT 0, priority TEXT, mal_id INTEGER,
			started_at DATETIME, completed_at DATETIME,
			created_at DATETIME, updated_at DATETIME
		)`,
		`INSERT INTO animes (id, name, kind, year) VALUES
			('a-tv-2020', 'TV 2020', 'tv', 2020),
			('a-movie-2010', 'Movie 2010', 'movie', 2010),
			('a-ova-2024', 'OVA 2024', 'ova', 2024)`,
		`INSERT INTO genres (id, name, name_ru) VALUES
			('g-action', 'Action', 'Экшен'),
			('g-comedy', 'Comedy', 'Комедия')`,
		`INSERT INTO anime_genres (anime_id, genre_id) VALUES
			('a-tv-2020','g-action'),('a-tv-2020','g-comedy'),
			('a-movie-2010','g-action'),
			('a-ova-2024','g-comedy')`,
		`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES
			('l1','u1','a-tv-2020','completed',8),
			('l2','u1','a-movie-2010','completed',7),
			('l3','u1','a-ova-2024','watching',9)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}
	return NewListRepository(db)
}

func idsOfEntries(entries []*domain.AnimeListEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.AnimeID)
	}
	return out
}

func TestGetByUserPaginated_KindFilter(t *testing.T) {
	repo := setupFilterTestDB(t)
	f := domain.ListFilters{Kinds: []string{"tv", "ova"}}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.ElementsMatch(t, []string{"a-tv-2020", "a-ova-2024"}, idsOfEntries(entries))
}

func TestGetByUserPaginated_YearRange(t *testing.T) {
	repo := setupFilterTestDB(t)
	min, max := 2015, 2022
	f := domain.ListFilters{YearMin: &min, YearMax: &max}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, []string{"a-tv-2020"}, idsOfEntries(entries))
}

func TestGetByUserPaginated_GenreAND(t *testing.T) {
	repo := setupFilterTestDB(t)
	f := domain.ListFilters{GenreIDs: []string{"g-action", "g-comedy"}}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, []string{"a-tv-2020"}, idsOfEntries(entries))
}

func TestGetByUserPaginated_GenreSingle(t *testing.T) {
	repo := setupFilterTestDB(t)
	f := domain.ListFilters{GenreIDs: []string{"g-action"}}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.ElementsMatch(t, []string{"a-tv-2020", "a-movie-2010"}, idsOfEntries(entries))
}
