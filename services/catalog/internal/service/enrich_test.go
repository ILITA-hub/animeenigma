package service

// Tests for list enrichment (enrichAll). enrichAll loads genres + video-source
// summaries for a whole slice of anime and is called by every catalog list
// endpoint (popular, recent, schedule, related, similar, calendar, …). It MUST
// batch its DB access: the per-anime path (enrichAnime → genreRepo.GetForAnime,
// a 3-query Preload, plus one video query = 4 queries/anime) is an N+1 that a
// production trace caught issuing ~400 queries for a single popular page.
//
// Uses in-memory SQLite with raw DDL (AutoMigrate can't emit the animes table's
// `uuid DEFAULT gen_random_uuid()` on SQLite — same reason as anime_update_test
// / genre_test), and a gorm:query callback to count the queries enrichment
// actually issues. The genres/videos/anime_genres tables mirror their GORM
// models so db.Create + the m2m Preload work; animes is slimmed (the genre
// Preload only needs id + the soft-delete column).

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEnrichTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE genres (
		id TEXT PRIMARY KEY,
		name TEXT,
		name_ru TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, genre_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE videos (
		id TEXT PRIMARY KEY,
		anime_id TEXT,
		type TEXT,
		episode_number INTEGER,
		name TEXT,
		source_type TEXT,
		source_url TEXT,
		storage_key TEXT,
		quality TEXT,
		language TEXT,
		duration INTEGER,
		thumbnail_url TEXT,
		created_at DATETIME
	)`).Error)

	return db
}

func TestEnrichAll_BatchesGenreAndVideoQueries(t *testing.T) {
	db := setupEnrichTestDB(t)
	ctx := context.Background()

	genres := []domain.Genre{
		{ID: "g1", Name: "Action"},
		{ID: "g2", Name: "Comedy"},
		{ID: "g3", Name: "Drama"},
	}
	require.NoError(t, db.Create(&genres).Error)

	animeIDs := []string{"a1", "a2", "a3"}
	for i, id := range animeIDs {
		require.NoError(t, db.Exec(
			"INSERT INTO animes (id, name) VALUES (?, ?)", id, "Anime "+id).Error)
		// Two genre links per anime (g1 + a rotating second genre).
		require.NoError(t, db.Exec(
			"INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?), (?, ?)",
			id, "g1", id, genres[(i%2)+1].ID).Error)
		require.NoError(t, db.Create(&domain.Video{
			ID: "v-" + id, AnimeID: id, Type: domain.VideoType("episode"),
			EpisodeNumber: 1, SourceType: domain.SourceType("kodik"),
			Quality: "720p", Language: "russian",
		}).Error)
	}

	// Count only the queries enrichment issues (registered after seeding).
	var queries int
	require.NoError(t, db.Callback().Query().After("gorm:query").
		Register("test:count", func(*gorm.DB) { queries++ }))

	s := &CatalogService{
		genreRepo: repo.NewGenreRepository(db),
		videoRepo: repo.NewVideoRepository(db),
		log:       logger.Default(),
	}

	animes := make([]*domain.Anime, len(animeIDs))
	for i, id := range animeIDs {
		animes[i] = &domain.Anime{ID: id}
	}

	s.enrichAll(ctx, animes)

	// Correctness: every anime is enriched with its genres + a video source.
	for _, a := range animes {
		assert.Len(t, a.Genres, 2, "anime %s genres not loaded", a.ID)
		assert.NotEmpty(t, a.VideoSources, "anime %s video sources not loaded", a.ID)
	}

	// Batched: query count is a small constant, NOT 4×N. For 3 anime the old
	// per-anime path issued 12 queries; the batched path issues 2.
	assert.LessOrEqual(t, queries, 3,
		"enrichAll issued %d queries for 3 anime — expected batched (<=3), looks like N+1", queries)
}

func TestVideoSourcesFromVideos_DedupsAndPreservesFirstSeen(t *testing.T) {
	vids := []*domain.Video{
		{SourceType: "kodik", Quality: "720p", Language: "russian"},
		{SourceType: "kodik", Quality: "720p", Language: "russian"}, // exact dup
		{SourceType: "kodik", Quality: "1080p", Language: "russian"},
		{SourceType: "allanime", Quality: "720p", Language: "japanese"},
	}

	got := videoSourcesFromVideos(vids)

	require.Len(t, got, 3, "duplicate (type,quality,language) not collapsed")
	// First-seen order is preserved (the old map-range build was non-deterministic).
	assert.Equal(t, domain.VideoSource{Type: "kodik", Quality: "720p", Language: "russian"}, got[0])
	assert.Equal(t, domain.VideoSource{Type: "kodik", Quality: "1080p", Language: "russian"}, got[1])
	assert.Equal(t, domain.VideoSource{Type: "allanime", Quality: "720p", Language: "japanese"}, got[2])
}

func TestVideoSourcesFromVideos_Empty(t *testing.T) {
	assert.Empty(t, videoSourcesFromVideos(nil))
}
