package repo

// Regression test for the final-review fix: AnimeRepository.GetByID must
// preload Studios (in addition to Genres), or the anime detail endpoint
// (GetAnime -> GetByID) never serializes `studios` and the anime page's
// Studio row silently never renders.
//
// Uses in-memory SQLite with raw DDL, matching anime_update_test.go /
// character_test.go: AutoMigrate can't emit `uuid DEFAULT gen_random_uuid()`,
// so the animes/studios/anime_studios tables are created by hand and every
// seeded row supplies an explicit string ID.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func setupAnimeStudiosTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		name_en TEXT,
		name_ru TEXT,
		name_jp TEXT,
		description TEXT,
		year INTEGER,
		season TEXT,
		status TEXT,
		kind TEXT,
		rating TEXT,
		material_source TEXT,
		franchise TEXT,
		franchise_checked INTEGER DEFAULT 0,
		episodes_count INTEGER DEFAULT 0,
		episodes_aired INTEGER DEFAULT 0,
		episode_duration INTEGER DEFAULT 0,
		score REAL,
		poster_url TEXT,
		shikimori_id TEXT,
		mal_id TEXT,
		ani_list_id TEXT,
		im_db_id TEXT,
		tmdb_id TEXT,
		has_video INTEGER DEFAULT 0,
		has_dub INTEGER DEFAULT 0,
		has_kodik INTEGER DEFAULT 0,
		has_animelib INTEGER DEFAULT 0,
		has_raw INTEGER DEFAULT 0,
		has_english INTEGER DEFAULT 0,
		hidden INTEGER DEFAULT 0,
		sort_priority INTEGER DEFAULT 0,
		next_episode_at DATETIME,
		next_episode_source TEXT DEFAULT 'shikimori',
		aired_on DATETIME,
		released_on DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE studios (
		id TEXT PRIMARY KEY,
		name TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE anime_studios (
		anime_id TEXT NOT NULL,
		studio_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, studio_id)
	)`).Error)

	// GetByID also preloads Genres; an empty anime_genres table lets that
	// preload run as a no-op without touching genre data in this test.
	require.NoError(t, db.Exec(`CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, genre_id)
	)`).Error)

	return db
}

// TestAnimeRepo_GetByID_PreloadsStudios is the regression guard for the
// missing `.Preload("Studios")` on GetByID: it FAILS (len(Studios) == 0) if
// the preload is removed, and PASSES once it's present.
func TestAnimeRepo_GetByID_PreloadsStudios(t *testing.T) {
	db := setupAnimeStudiosTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	studio := domain.Studio{ID: "studio-1", Name: "Madhouse"}
	require.NoError(t, db.Create(&studio).Error)

	anime := domain.Anime{
		ID:     "anime-1",
		Name:   "Frieren",
		Status: domain.StatusReleased,
	}
	require.NoError(t, db.Create(&anime).Error)
	require.NoError(t, db.Model(&anime).Association("Studios").Append(&studio))

	got, err := r.GetByID(ctx, "anime-1")
	require.NoError(t, err)

	require.Len(t, got.Studios, 1, "expected the associated studio to be preloaded onto the anime")
	assert.Equal(t, "Madhouse", got.Studios[0].Name)
}
