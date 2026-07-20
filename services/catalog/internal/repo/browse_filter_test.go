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
		has_english_dub INTEGER DEFAULT 0, english_dub_checked_at DATETIME,
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

func TestAnimeRepository_Search_ProviderColumns(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedBrowseAnime(t, db, "k", "Kodik only")
	seedBrowseAnime(t, db, "d", "Dub only")
	seedBrowseAnime(t, db, "ae", "First-party only")
	require.NoError(t, db.Exec(`UPDATE animes SET has_kodik=1 WHERE id='k'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_dub=1 WHERE id='d'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_video=1 WHERE id='ae'`).Error)

	cases := []struct {
		key, wantID string
	}{
		{"kodik", "k"}, {"dub", "d"}, {"ae", "ae"},
	}
	for _, c := range cases {
		got, total, err := r.Search(ctx, domain.SearchFilters{
			Providers: []string{c.key}, Page: 1, PageSize: 20,
		})
		require.NoError(t, err, c.key)
		require.Equal(t, int64(1), total, c.key)
		require.Equal(t, c.wantID, got[0].ID, c.key)
	}
}

func TestAnimeRepository_Search_KindsFilter_ORSet(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedBrowseAnime(t, db, "tv", "A TV Show")
	seedBrowseAnime(t, db, "mv", "A Movie")
	seedBrowseAnime(t, db, "ova", "An OVA")
	require.NoError(t, db.Exec(`UPDATE animes SET kind='tv' WHERE id='tv'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET kind='movie' WHERE id='mv'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET kind='ova' WHERE id='ova'`).Error)

	// OR-set: selecting tv+movie returns BOTH, not the (empty) intersection.
	got, total, err := r.Search(ctx, domain.SearchFilters{
		Kinds: []string{"tv", "movie"}, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	require.True(t, ids["tv"] && ids["mv"])
	require.False(t, ids["ova"])

	// Single kind still works.
	got, total, err = r.Search(ctx, domain.SearchFilters{
		Kinds: []string{"ova"}, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "ova", got[0].ID)
}

func TestAnimeRepository_ListStudios_OrderedByCount(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedBrowseAnime(t, db, "a1", "A1")
	seedBrowseAnime(t, db, "a2", "A2")
	require.NoError(t, db.Exec(`INSERT INTO studios (id, name) VALUES ('s-big','Big'),('s-small','Small'),('s-empty','Empty')`).Error)
	// Big has 2 anime, Small has 1, Empty has 0 (must be excluded).
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES ('a1','s-big'),('a2','s-big'),('a1','s-small')`).Error)

	got, err := r.ListStudios(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2) // Empty excluded (no anime)
	require.Equal(t, "s-big", got[0].ID)   // count 2 first
	require.Equal(t, "s-small", got[1].ID) // count 1 second
}
