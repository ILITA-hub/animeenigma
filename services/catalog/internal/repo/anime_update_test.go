package repo

// Regression tests for AnimeRepository.Update. The scheduled Shikimori
// refresh (RefreshAnimeFromShikimori / BatchRefreshAnime) and the hot
// upsertAnimeFromExternal path hand Update a *freshly mapped* anime whose
// lazily-maintained columns (provider flags, admin pin, hidden, franchise,
// external IDs) are all at their zero values. A full-row Save would silently
// overwrite those columns with zeros on every refresh cycle. Update must
// touch only Shikimori-sourced metadata columns and leave the lazy/admin
// columns intact, while still refreshing (and clearing) the metadata it owns.
//
// Uses in-memory SQLite with raw DDL (matching the production Postgres column
// set minus Postgres-only types), the same pattern as collection_test.go —
// AutoMigrate can't emit `uuid DEFAULT gen_random_uuid()`.

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAnimeUpdateTestDB(t *testing.T) *gorm.DB {
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
		mal_members INTEGER DEFAULT 0,
		mal_favorites INTEGER DEFAULT 0,
		im_db_id TEXT,
		tmdb_id TEXT,
		has_video INTEGER DEFAULT 0,
		has_dub INTEGER DEFAULT 0,
		has_kodik INTEGER DEFAULT 0,
		has_animelib INTEGER DEFAULT 0,
		has_raw INTEGER DEFAULT 0,
		has_english INTEGER DEFAULT 0,
		has_english_dub INTEGER DEFAULT 0, english_dub_checked_at DATETIME,
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

	return db
}

// seedExistingAnime inserts a row that has all the lazily-maintained /
// admin-controlled columns populated, simulating an anime that has been
// enriched over time (provider flags flipped, pinned, hidden, franchise
// backfilled, external IDs resolved).
func seedExistingAnime(t *testing.T, db *gorm.DB) string {
	t.Helper()
	imdb := "tt1234567"
	tmdb := "98765"
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	next := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	existing := &domain.Anime{
		ID:               "anime-1",
		Name:             "Old Name",
		Description:      "old description",
		Score:            7.0,
		EpisodesAired:    5,
		Status:           domain.StatusOngoing,
		ShikimoriID:      "57466",
		MALID:            "55555",
		AniListID:        "44444",
		IMDbID:           &imdb,
		TMDBID:           &tmdb,
		HasVideo:         true,
		HasDub:           true,
		HasKodik:         true,
		HasAnimeLib:      true,
		HasRaw:           true,
		HasEnglish:       true,
		Hidden:           true,
		SortPriority:     5,
		Franchise:        "monogatari",
		FranchiseChecked: true,
		NextEpisodeAt:    &next,
		CreatedAt:        created,
		UpdatedAt:        created,
	}
	require.NoError(t, db.Create(existing).Error)
	return existing.ID
}

func TestAnimeRepository_Update_PreservesLazyAndAdminFields(t *testing.T) {
	db := setupAnimeUpdateTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()
	id := seedExistingAnime(t, db)

	// A freshly mapped Shikimori anime: only metadata is set; every lazy /
	// admin / external-ID column is at its zero value (this is exactly what
	// mapAnime produces and what the refresh paths pass to Update).
	fresh := &domain.Anime{
		ID:            id,
		Name:          "New Name",
		Description:   "new description",
		Score:         8.5,
		EpisodesAired: 9,
		Status:        domain.StatusReleased,
		ShikimoriID:   "57466",
	}

	require.NoError(t, r.Update(ctx, fresh))

	var got domain.Anime
	require.NoError(t, db.Where("id = ?", id).First(&got).Error)

	// Lazy / admin / external-ID columns MUST be preserved.
	assert.True(t, got.HasVideo, "has_video wiped")
	assert.True(t, got.HasDub, "has_dub wiped")
	assert.True(t, got.HasKodik, "has_kodik wiped")
	assert.True(t, got.HasAnimeLib, "has_animelib wiped")
	assert.True(t, got.HasRaw, "has_raw wiped")
	assert.True(t, got.HasEnglish, "has_english wiped")
	assert.True(t, got.Hidden, "hidden wiped")
	assert.Equal(t, 5, got.SortPriority, "sort_priority (admin pin) wiped")
	assert.Equal(t, "monogatari", got.Franchise, "franchise wiped")
	assert.True(t, got.FranchiseChecked, "franchise_checked wiped")
	assert.Equal(t, "55555", got.MALID, "mal_id wiped")
	assert.Equal(t, "44444", got.AniListID, "ani_list_id wiped")
	require.NotNil(t, got.IMDbID)
	assert.Equal(t, "tt1234567", *got.IMDbID, "imdb_id wiped")
	require.NotNil(t, got.TMDBID)
	assert.Equal(t, "98765", *got.TMDBID, "tmdb_id wiped")

	// Shikimori-sourced metadata MUST be refreshed.
	assert.Equal(t, "New Name", got.Name)
	assert.Equal(t, "new description", got.Description)
	assert.Equal(t, 8.5, got.Score)
	assert.Equal(t, 9, got.EpisodesAired)
	assert.Equal(t, domain.StatusReleased, got.Status)

	// updated_at MUST advance — BatchRefreshAnime selects stale rows by
	// `updated_at < staleBefore`, so a refresh that doesn't bump it would
	// re-select the same anime on every run (infinite refresh churn).
	assert.True(t, got.UpdatedAt.After(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
		"updated_at not bumped by refresh; got %s", got.UpdatedAt)
}

// When an ongoing anime finishes, Shikimori returns a nil NextEpisodeAt and
// the refresh must CLEAR the stale "next episode" date — proving the metadata
// columns are force-written (not skipped on zero value).
func TestAnimeRepository_Update_ClearsNextEpisodeWhenReleased(t *testing.T) {
	db := setupAnimeUpdateTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()
	id := seedExistingAnime(t, db)

	fresh := &domain.Anime{
		ID:            id,
		Name:          "Old Name",
		Status:        domain.StatusReleased,
		ShikimoriID:   "57466",
		NextEpisodeAt: nil, // released → no next episode
	}
	require.NoError(t, r.Update(ctx, fresh))

	var got domain.Anime
	require.NoError(t, db.Where("id = ?", id).First(&got).Error)
	assert.Nil(t, got.NextEpisodeAt, "stale next_episode_at not cleared on release")
	// And the admin pin still survives this path too.
	assert.Equal(t, 5, got.SortPriority)
}

// Updating a non-existent row must still surface NotFound.
func TestAnimeRepository_Update_NotFound(t *testing.T) {
	db := setupAnimeUpdateTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	err := r.Update(ctx, &domain.Anime{ID: "does-not-exist", Name: "x"})
	require.Error(t, err)
}
