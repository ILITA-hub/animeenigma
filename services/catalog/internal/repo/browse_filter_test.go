package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBrowseFilterTestDB builds an in-memory SQLite with the animes +
// studios + anime_studios tables Search/ListStudios touch. Mirrors
// setupAnimeUpdateTestDB (AutoMigrate can't emit Postgres uuid defaults).
func setupBrowseFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY, name TEXT, name_ru TEXT, name_jp TEXT,
		year INTEGER, season TEXT, status TEXT, kind TEXT, score REAL,
		has_video INTEGER DEFAULT 0, has_dub INTEGER DEFAULT 0,
		has_kodik INTEGER DEFAULT 0, has_animelib INTEGER DEFAULT 0,
		has_raw INTEGER DEFAULT 0, has_english INTEGER DEFAULT 0,
		hidden INTEGER DEFAULT 0, sort_priority INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE studios (
		id TEXT PRIMARY KEY, name TEXT, created_at DATETIME, updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_studios (
		anime_id TEXT, studio_id TEXT
	)`).Error)
	return db
}

func seedBrowseAnime(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, hidden) VALUES (?, ?, 0)`, id, name).Error)
}

func TestAnimeRepository_Search_StudioFilter_ORSet(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedBrowseAnime(t, db, "a1", "Ufotable Show")
	seedBrowseAnime(t, db, "a2", "Madhouse Show")
	seedBrowseAnime(t, db, "a3", "No Studio Show")
	require.NoError(t, db.Exec(`INSERT INTO studios (id, name) VALUES ('s-ufo','Ufotable'),('s-mad','Madhouse')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES ('a1','s-ufo'),('a2','s-mad')`).Error)

	// OR-set: selecting both studios returns BOTH anime (any match), not the
	// intersection (genre uses AND; studios deliberately use OR).
	got, total, err := r.Search(ctx, domain.SearchFilters{
		StudioIDs: []string{"s-ufo", "s-mad"}, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	require.True(t, ids["a1"] && ids["a2"])
	require.False(t, ids["a3"])
}
