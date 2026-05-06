package signals

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupS1TestDB creates an in-memory SQLite DB with the schema S1 needs:
// animes (the candidate universe), anime_list (the user's score history),
// rec_user_signals (where Precompute persists s1_vector). The schema mirrors
// the production tables but with engine-portable types (TEXT for jsonb, etc.).
func setupS1TestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.Exec(`CREATE TABLE rec_user_signals (
		user_id TEXT PRIMARY KEY,
		s1_vector TEXT NOT NULL DEFAULT '{}',
		s5_affinity TEXT NOT NULL DEFAULT '{}',
		s6_seed_anime_id TEXT,
		s6_seed_completed_at DATETIME,
		s6_seed_score INTEGER,
		last_computed DATETIME NOT NULL
	)`).Error)
	return db
}

func seedS1Anime(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden) VALUES (?, 0)`, id,
	).Error)
}

func seedS1List(t *testing.T, db *gorm.DB, rowID, userID, animeID string, score int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, ?, ?)`,
		rowID, userID, animeID, "watching", score,
	).Error)
}

func TestS1ScoreCluster_ID(t *testing.T) {
	db := setupS1TestDB(t)
	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	assert.Equal(t, recs.SignalID("s1"), s1.ID())
}

func TestS1ScoreCluster_Precompute_ColdStartUnderThresholdWritesEmptyVector(t *testing.T) {
	db := setupS1TestDB(t)

	// User-1 has only 2 scored anime — below the cold-start threshold of 3.
	seedS1Anime(t, db, "anime-A")
	seedS1Anime(t, db, "anime-B")
	seedS1List(t, db, "al1", "user-1", "anime-A", 8)
	seedS1List(t, db, "al2", "user-1", "anime-B", 7)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	require.NoError(t, s1.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, row, "Precompute must persist a row even on cold-start")
	assert.Equal(t, "{}", row.S1Vector, "cold-start must persist empty JSON object")
	assert.False(t, row.LastComputed.IsZero(), "LastComputed must be set")
}

func TestS1ScoreCluster_Precompute_NormalKnnPath(t *testing.T) {
	db := setupS1TestDB(t)

	// Universe: A, B, C (target user has scored), D, E (candidates).
	for _, id := range []string{"anime-A", "anime-B", "anime-C", "anime-D", "anime-E"} {
		seedS1Anime(t, db, id)
	}
	// User-1 (target): A=8, B=7, C=9 — meets cold-start threshold.
	seedS1List(t, db, "al-1A", "user-1", "anime-A", 8)
	seedS1List(t, db, "al-1B", "user-1", "anime-B", 7)
	seedS1List(t, db, "al-1C", "user-1", "anime-C", 9)
	// User-2 (positively-correlated neighbor): A=8, B=7, D=10. Overlaps user-1
	// on {A, B} with identical (8,7) → Pearson = +1.0.
	seedS1List(t, db, "al-2A", "user-2", "anime-A", 8)
	seedS1List(t, db, "al-2B", "user-2", "anime-B", 7)
	seedS1List(t, db, "al-2D", "user-2", "anime-D", 10)
	// User-3 (negatively-correlated neighbor): A=2, B=3, E=5. Overlaps user-1
	// on {A, B} with reversed direction → Pearson = -1.0 (or close to it).
	seedS1List(t, db, "al-3A", "user-3", "anime-A", 2)
	seedS1List(t, db, "al-3B", "user-3", "anime-B", 3)
	seedS1List(t, db, "al-3E", "user-3", "anime-E", 5)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	require.NoError(t, s1.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, row)

	var vec map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S1Vector), &vec))

	// D got contributions from user-2 only (user-3 didn't score D).
	// predicted = (sim * score) / |sim| = (1.0 * 10) / 1.0 = 10.
	assert.InDelta(t, 10.0, vec["anime-D"], 0.01, "D predicted = user-2 score (only neighbor who scored D)")
	// E got contributions from user-3 only (user-2 didn't score E).
	// predicted = (-1.0 * 5) / |-1.0| = -5. (Negative is allowed; the
	// per-pool normalizer collapses these into [0,1] downstream.)
	assert.InDelta(t, -5.0, vec["anime-E"], 0.01, "E predicted = -user-3 score (Pearson=-1)")
	// Already-watched A/B/C must NOT appear in the vector.
	_, hasA := vec["anime-A"]
	assert.False(t, hasA, "candidate set excludes target user's already-scored anime")
}

func TestS1ScoreCluster_Precompute_IsIdempotent(t *testing.T) {
	db := setupS1TestDB(t)

	for _, id := range []string{"anime-A", "anime-B", "anime-C", "anime-D"} {
		seedS1Anime(t, db, id)
	}
	seedS1List(t, db, "al-1A", "user-1", "anime-A", 8)
	seedS1List(t, db, "al-1B", "user-1", "anime-B", 7)
	seedS1List(t, db, "al-1C", "user-1", "anime-C", 9)
	seedS1List(t, db, "al-2A", "user-2", "anime-A", 8)
	seedS1List(t, db, "al-2B", "user-2", "anime-B", 7)
	seedS1List(t, db, "al-2D", "user-2", "anime-D", 10)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	require.NoError(t, s1.Precompute(context.Background(), "user-1"))
	first, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)

	require.NoError(t, s1.Precompute(context.Background(), "user-1"))
	second, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)

	assert.Equal(t, first.S1Vector, second.S1Vector, "running Precompute twice must produce identical vector")
}

func TestS1ScoreCluster_Precompute_IgnoresSubMinOverlapNeighbors(t *testing.T) {
	db := setupS1TestDB(t)

	for _, id := range []string{"anime-A", "anime-B", "anime-C", "anime-D"} {
		seedS1Anime(t, db, id)
	}
	// Target: A=8, B=7, C=9.
	seedS1List(t, db, "al-1A", "user-1", "anime-A", 8)
	seedS1List(t, db, "al-1B", "user-1", "anime-B", 7)
	seedS1List(t, db, "al-1C", "user-1", "anime-C", 9)
	// Neighbor: only A=10, D=10 — overlaps with target on {A} only (1 < min=2).
	// This neighbor must be skipped, so D should NOT appear in the vector.
	seedS1List(t, db, "al-2A", "user-2", "anime-A", 10)
	seedS1List(t, db, "al-2D", "user-2", "anime-D", 10)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	require.NoError(t, s1.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	var vec map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S1Vector), &vec))
	_, hasD := vec["anime-D"]
	assert.False(t, hasD, "candidate D must be omitted because the only neighbor with a D score had overlap < 2")
}

func TestS1ScoreCluster_Score_NoUserRow(t *testing.T) {
	db := setupS1TestDB(t)
	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	got, err := s1.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A"})
	require.NoError(t, err)
	assert.Empty(t, got, "no rec_user_signals row -> empty map (cold-start before first Precompute)")
}

func TestS1ScoreCluster_Score_EmptyVector(t *testing.T) {
	db := setupS1TestDB(t)

	// Persist a cold-start row (empty {} vector).
	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		 VALUES (?, ?, ?, datetime('now'))`,
		"user-1", "{}", "{}",
	).Error)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	got, err := s1.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A", "anime-B"})
	require.NoError(t, err)
	assert.Empty(t, got, "empty vector -> empty map (cold-start gate persisted)")
}

func TestS1ScoreCluster_Score_ReadsPersistedVectorForCandidates(t *testing.T) {
	db := setupS1TestDB(t)

	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		 VALUES (?, ?, ?, datetime('now'))`,
		"user-1", `{"anime-A": 8.5, "anime-B": 7.2, "anime-C": 6.0}`, "{}",
	).Error)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	got, err := s1.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A", "anime-B", "anime-NEW"})
	require.NoError(t, err)
	assert.Equal(t, recs.RawScore(8.5), got["anime-A"])
	assert.Equal(t, recs.RawScore(7.2), got["anime-B"])
	_, hasC := got["anime-C"]
	assert.False(t, hasC, "C is in the vector but not a candidate -> omitted from output")
	_, hasNew := got["anime-NEW"]
	assert.False(t, hasNew, "anime-NEW is a candidate but absent from vector -> omitted (normalizer treats absent as zero)")
}

func TestS1ScoreCluster_Score_MalformedVectorReturnsError(t *testing.T) {
	db := setupS1TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		 VALUES (?, ?, ?, datetime('now'))`,
		"user-1", "this is not valid json", "{}",
	).Error)

	r := repo.NewRecsRepository(db)
	s1 := NewS1ScoreCluster(db, r)
	_, err := s1.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A"})
	require.Error(t, err, "malformed JSONB vector must surface as an error")
}
