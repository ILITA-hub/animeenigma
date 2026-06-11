package signals

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupS2TestDB creates an in-memory SQLite DB with the minimal schema S2
// needs: animes (id only — S2 doesn't read other columns), anime_list (the
// user's score history that drives the seed set), anime_genres (m2m).
func setupS2TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status TEXT,
		score INTEGER DEFAULT 0
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL
	)`).Error)
	return db
}

func seedS2Anime(t *testing.T, db *gorm.DB, id string, genres []string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden) VALUES (?, 0)`, id,
	).Error)
	for _, g := range genres {
		require.NoError(t, db.Exec(
			`INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?)`,
			id, g,
		).Error)
	}
}

func seedS2List(t *testing.T, db *gorm.DB, rowID, userID, animeID string, score int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, 'watching', ?)`,
		rowID, userID, animeID, score,
	).Error)
}

func TestS2Metadata_ID(t *testing.T) {
	db := setupS2TestDB(t)
	s2 := NewS2Metadata(db)
	assert.Equal(t, recs.SignalID("s2"), s2.ID())
}

func TestS2Metadata_Precompute_NoOp(t *testing.T) {
	db := setupS2TestDB(t)
	s2 := NewS2Metadata(db)
	// S2 is request-time only; Precompute should be a clean no-op for any user.
	assert.NoError(t, s2.Precompute(context.Background(), "user-1"))
	assert.NoError(t, s2.Precompute(context.Background(), ""))
}

func TestS2Metadata_Score_NoScoredAnime(t *testing.T) {
	db := setupS2TestDB(t)
	seedS2Anime(t, db, "anime-A", []string{"action", "drama"})
	seedS2Anime(t, db, "anime-B", []string{"action"})

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A", "anime-B"})
	require.NoError(t, err)
	assert.Empty(t, got, "user with no scored anime -> cold-start, empty map")
}

func TestS2Metadata_Score_AllSeedScoresBelowFallback(t *testing.T) {
	db := setupS2TestDB(t)
	seedS2Anime(t, db, "anime-A", []string{"action"})
	seedS2Anime(t, db, "anime-B", []string{"action"})
	seedS2List(t, db, "al-1A", "user-1", "anime-A", 4) // below fallback threshold (5)

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-1", []recs.AnimeID{"anime-B"})
	require.NoError(t, err)
	assert.Empty(t, got, "all seeds below fallback threshold -> cold-start, empty map")
}

func TestS2Metadata_Score_PrimaryThresholdUsedWhenAvailable(t *testing.T) {
	db := setupS2TestDB(t)
	seedS2Anime(t, db, "anime-A", []string{"action", "drama"})
	seedS2Anime(t, db, "anime-B", []string{"action", "drama", "romance"})
	seedS2Anime(t, db, "anime-C", []string{"mecha"})

	// User-1 scored A=8 (>= 7 primary threshold, used as seed).
	seedS2List(t, db, "al-1A", "user-1", "anime-A", 8)
	// User-1 also scored a low-score anime — must NOT pollute the seed set
	// because the primary threshold (>=7) is satisfied above.
	seedS2Anime(t, db, "anime-LOW", []string{"action", "drama", "comedy", "romance", "isekai"})
	seedS2List(t, db, "al-1LOW", "user-1", "anime-LOW", 5)

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-1", []recs.AnimeID{"anime-B", "anime-C"})
	require.NoError(t, err)

	// Jaccard(B={action,drama,romance}, A={action,drama}) = 2/3 ≈ 0.667.
	assert.InDelta(t, 0.667, float64(got["anime-B"]), 0.01)
	// C={mecha} ∩ A={action,drama} = 0 -> omitted from output.
	_, hasC := got["anime-C"]
	assert.False(t, hasC, "candidate with zero genre overlap is omitted (normalizer treats absent as 0)")
}

func TestS2Metadata_Score_FallbackThresholdWhenNoPrimarySeed(t *testing.T) {
	db := setupS2TestDB(t)
	seedS2Anime(t, db, "anime-A", []string{"action", "drama"})
	seedS2Anime(t, db, "anime-B", []string{"action"})
	// User-2 has only one scored anime — below primary (7) but at fallback (5).
	seedS2List(t, db, "al-2A", "user-2", "anime-A", 6)

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-2", []recs.AnimeID{"anime-B"})
	require.NoError(t, err)
	// Jaccard(B={action}, A={action,drama}) = 1/2 = 0.5.
	assert.InDelta(t, 0.5, float64(got["anime-B"]), 0.01)
}

func TestS2Metadata_Score_EmptyCandidates(t *testing.T) {
	db := setupS2TestDB(t)
	seedS2Anime(t, db, "anime-A", []string{"action"})
	seedS2List(t, db, "al-1A", "user-1", "anime-A", 8)

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-1", []recs.AnimeID{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS2Metadata_Score_SeedAnimeWithNoGenres(t *testing.T) {
	db := setupS2TestDB(t)
	seedS2Anime(t, db, "anime-A", nil) // seed has no genres at all
	seedS2Anime(t, db, "anime-B", []string{"action"})
	seedS2List(t, db, "al-1A", "user-1", "anime-A", 9)

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-1", []recs.AnimeID{"anime-B"})
	require.NoError(t, err)
	assert.Empty(t, got, "seed without genres -> no signal possible (Jaccard with empty set is 0)")
}

func TestS2Metadata_Score_MaxAcrossSeedSet(t *testing.T) {
	db := setupS2TestDB(t)
	// Two seeds: A and B. Candidate C overlaps strongly with B, weakly with A.
	// The "max" rule keeps the strongest match.
	seedS2Anime(t, db, "anime-A", []string{"action", "drama"})
	seedS2Anime(t, db, "anime-B", []string{"romance", "slice-of-life"})
	seedS2Anime(t, db, "anime-C", []string{"romance", "slice-of-life", "comedy"})
	seedS2List(t, db, "al-1A", "user-1", "anime-A", 8)
	seedS2List(t, db, "al-1B", "user-1", "anime-B", 9)

	s2 := NewS2Metadata(db)
	got, err := s2.Score(context.Background(), "user-1", []recs.AnimeID{"anime-C"})
	require.NoError(t, err)
	// Jaccard(C, B) = 2/3. Jaccard(C, A) = 0. max = 2/3.
	assert.InDelta(t, 0.667, float64(got["anime-C"]), 0.01)
}
