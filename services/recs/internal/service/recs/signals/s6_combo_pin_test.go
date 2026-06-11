package signals

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupS6TestDB creates an in-memory SQLite DB with every table the S6
// resolver touches: animes (for SeedName lookup), rec_user_signals (for
// the s6_seed_* fields), rec_completion_co_occurrence (for the score=7
// materialized read), anime_list (for the score=5 live query).
func setupS6TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT
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
	require.NoError(t, db.Exec(`CREATE TABLE rec_completion_co_occurrence (
		seed_anime_id TEXT NOT NULL,
		candidate_anime_id TEXT NOT NULL,
		co_count INTEGER NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL,
		PRIMARY KEY (seed_anime_id, candidate_anime_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status TEXT,
		score INTEGER DEFAULT 0
	)`).Error)
	return db
}

// fakeShikimoriSimilarClient programmable client implementing
// shikimoriSimilarClient. Records call counts so tests can assert the
// cascade reached (or skipped) the Shikimori branch.
type fakeShikimoriSimilarClient struct {
	mu        sync.Mutex
	response  []SimilarAnimeRef
	err       error
	callCount int
}

func (f *fakeShikimoriSimilarClient) GetSimilarAnimeByLocalID(_ context.Context, _ string) ([]SimilarAnimeRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func (f *fakeShikimoriSimilarClient) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

// seedSeed seeds the rec_user_signals row + the seed anime's name row.
func seedSeed(t *testing.T, db *gorm.DB, userID, seedAnimeID, seedName string, completedAt time.Time, score int) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name) VALUES (?, ?)`, seedAnimeID, seedName).Error)
	require.NoError(t, db.Exec(`INSERT INTO rec_user_signals (user_id, s6_seed_anime_id, s6_seed_completed_at, s6_seed_score, last_computed)
		VALUES (?, ?, ?, ?, ?)`,
		userID, seedAnimeID, completedAt, score, time.Now().UTC()).Error)
}

func seedCoOcc(t *testing.T, db *gorm.DB, seed, cand string, count int) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed)
		VALUES (?, ?, ?, ?)`,
		seed, cand, count, time.Now().UTC()).Error)
}

func newS6ForTest(db *gorm.DB, fake shikimoriSimilarClient) *S6ComboPin {
	recsRepo := repo.NewRecsRepository(db)
	return NewS6ComboPin(db, recsRepo, fake, logger.Default())
}

// --- Cascade tests ---

func TestS6_NoSeed_ReturnsNil(t *testing.T) {
	db := setupS6TestDB(t)
	// Insert an empty signals row.
	require.NoError(t, db.Exec(`INSERT INTO rec_user_signals (user_id, last_computed) VALUES (?, ?)`, "u1", time.Now().UTC()).Error)
	s6 := newS6ForTest(db, &fakeShikimoriSimilarClient{})
	got, err := s6.Resolve(context.Background(), "u1", []string{"c1", "c2"})
	require.NoError(t, err)
	assert.Nil(t, got, "no seed → nil PinCandidate")
}

func TestS6_NoUserRow_ReturnsNil(t *testing.T) {
	db := setupS6TestDB(t)
	s6 := newS6ForTest(db, &fakeShikimoriSimilarClient{})
	got, err := s6.Resolve(context.Background(), "no-such-user", []string{"c1"})
	require.NoError(t, err)
	assert.Nil(t, got, "missing rec_user_signals row → nil PinCandidate")
}

func TestS6_StaleSeed_ReturnsNil(t *testing.T) {
	db := setupS6TestDB(t)
	stale := time.Now().UTC().Add(-8 * 24 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Seed Name", stale, 8)
	seedCoOcc(t, db, "seed-A", "cand-1", 5) // would qualify if seed weren't stale

	fake := &fakeShikimoriSimilarClient{}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	assert.Nil(t, got, "seed >7 days old → nil PinCandidate")
	assert.Equal(t, 0, fake.Calls(), "stale seed must NOT consult Shikimori")
}

func TestS6_LocalPoolFiveOrMore_ReturnsLocal(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Frieren", completed, 9)

	// Six co-occurrences, all in candidate pool.
	candIDs := []string{"c1", "c2", "c3", "c4", "c5", "c6"}
	for i, c := range candIDs {
		seedCoOcc(t, db, "seed-A", c, 10-i) // c1 highest count
	}

	fake := &fakeShikimoriSimilarClient{}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", candIDs)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "c1", got.AnimeID, "highest co_count picked")
	assert.Equal(t, "local", got.Source)
	assert.Equal(t, "seed-A", got.SeedAnimeID)
	assert.Equal(t, "Frieren", got.SeedName)
	assert.Equal(t, 0, fake.Calls(), "local cascade satisfied; Shikimori must NOT be called")
}

func TestS6_LocalPoolBelowFive_FallsToShikimori(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Mushishi", completed, 10)

	// Three co-occurrences post-S11 (below the 5-threshold).
	candIDs := []string{"local-1", "local-2", "local-3"}
	for i, c := range candIDs {
		seedCoOcc(t, db, "seed-A", c, 5-i)
	}

	// Shikimori has 4 valid candidates in pool.
	fake := &fakeShikimoriSimilarClient{
		response: []SimilarAnimeRef{
			{ShikimoriID: "1", LocalID: "shiki-1"},
			{ShikimoriID: "2", LocalID: "shiki-2"},
			{ShikimoriID: "3", LocalID: "shiki-3"},
			{ShikimoriID: "4", LocalID: "shiki-4"},
		},
	}

	pool := append([]string{}, candIDs...)
	pool = append(pool, "shiki-1", "shiki-2", "shiki-3", "shiki-4")

	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "shikimori_similar", got.Source)
	assert.Equal(t, "shiki-1", got.AnimeID, "first Shikimori candidate in pool")
	assert.Equal(t, "Mushishi", got.SeedName)
	assert.Equal(t, 1, fake.Calls(), "Shikimori must be invoked exactly once")
}

func TestS6_ShikimoriResponseFilteredEmpty_FallsToScoreFive(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Vinland Saga", completed, 9)

	// rec_completion_co_occurrence at score=7 has 0 entries for seed-A.
	// Shikimori returns 4 anime but ALL 4 are excluded by candidatePool
	// (e.g., user already completed them, so S11 stripped them).
	fake := &fakeShikimoriSimilarClient{
		response: []SimilarAnimeRef{
			{ShikimoriID: "1", LocalID: "excluded-1"},
			{ShikimoriID: "2", LocalID: "excluded-2"},
			{ShikimoriID: "3", LocalID: "excluded-3"},
			{ShikimoriID: "4", LocalID: "excluded-4"},
		},
	}

	// Live score=5 query data: two users completed (seed-A, score=5) + (cand-5x, score=5)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (1,'oa','seed-A','completed',5)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (2,'oa','cand-5x','completed',5)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (3,'ob','seed-A','completed',5)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (4,'ob','cand-5x','completed',5)`).Error)

	pool := []string{"cand-5x"}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "cand-5x", got.AnimeID)
	assert.Equal(t, "local", got.Source, "score=5 fallback resolves to a local source")
	assert.Equal(t, 1, fake.Calls(), "Shikimori was tried before falling to score=5")
}

func TestS6_ScoreFiveFallback_ReturnsLocal(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Steins;Gate", completed, 8)

	// No score=7 co-occurrences. Shikimori empty.
	fake := &fakeShikimoriSimilarClient{response: []SimilarAnimeRef{}}

	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (1,'ux','seed-A','completed',6)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (2,'ux','only-cand','completed',6)`).Error)

	pool := []string{"only-cand"}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "only-cand", got.AnimeID)
	assert.Equal(t, "local", got.Source)
}

func TestS6_AllCascadesEmpty_ReturnsNil(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Bocchi the Rock", completed, 9)
	fake := &fakeShikimoriSimilarClient{response: []SimilarAnimeRef{}}

	pool := []string{"some-anime-not-in-coocc"}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	assert.Nil(t, got, "all three cascades empty → nil PinCandidate, NOT an error")
}

func TestS6_NeverFallsBelowFive(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Some Anime", completed, 9)

	// Only score=3 co-occurrences exist in anime_list.
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (1,'low','seed-A','completed',3)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (2,'low','cand-low','completed',3)`).Error)

	fake := &fakeShikimoriSimilarClient{response: []SimilarAnimeRef{}}
	pool := []string{"cand-low"}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	assert.Nil(t, got, "score=3 must NOT bubble up to a pin (spec §3.2: never below 5)")
}

func TestS6_FilterRespectsCandidatePool(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-A", "Kaguya-sama", completed, 10)

	// 6 co-occurrences exist in the materialized table.
	for i := 0; i < 6; i++ {
		seedCoOcc(t, db, "seed-A", "cand-"+string(rune('a'+i)), 10-i)
	}

	// But ONLY 3 of them are in the post-S11 candidate pool.
	pool := []string{"cand-d", "cand-e", "cand-f"} // not the top 3 by co_count

	// Shikimori provides exactly one candidate that IS in the pool.
	fake := &fakeShikimoriSimilarClient{
		response: []SimilarAnimeRef{{LocalID: "cand-d"}},
	}
	s6 := newS6ForTest(db, fake)
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	require.NotNil(t, got)
	// Filtered count is 3 (below threshold 5) → falls to Shikimori.
	assert.Equal(t, "shikimori_similar", got.Source, "filtered local count was 3 (<5); cascade must reach Shikimori")
	assert.Equal(t, "cand-d", got.AnimeID)
	assert.Equal(t, 1, fake.Calls())
}

func TestS6_PinCandidateHasSeedNameAndID(t *testing.T) {
	db := setupS6TestDB(t)
	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedSeed(t, db, "u1", "seed-FRIEREN", "Frieren: Beyond Journey's End", completed, 10)
	for i := 0; i < 6; i++ {
		seedCoOcc(t, db, "seed-FRIEREN", "c"+string(rune('1'+i)), 10-i)
	}

	pool := []string{"c1", "c2", "c3", "c4", "c5", "c6"}
	s6 := newS6ForTest(db, &fakeShikimoriSimilarClient{})
	got, err := s6.Resolve(context.Background(), "u1", pool)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "seed-FRIEREN", got.SeedAnimeID)
	assert.Equal(t, "Frieren: Beyond Journey's End", got.SeedName)
}

// --- HTTPShikimoriSimilarClient tests ---

func TestS6_HTTPClient_Returns404HandledGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewHTTPShikimoriSimilarClient(srv.URL, logger.Default())
	got, err := c.GetSimilarAnimeByLocalID(context.Background(), "anime-X")
	require.NoError(t, err, "404 from catalog must NOT be a fatal error")
	assert.Empty(t, got, "404 returns empty slice")
}

func TestS6_HTTPClient_DecodesSuccessfulResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": []map[string]string{
				{"shikimori_id": "111", "local_id": "local-111"},
				{"shikimori_id": "222", "local_id": "local-222"},
			},
		})
	}))
	defer srv.Close()

	c := NewHTTPShikimoriSimilarClient(srv.URL, logger.Default())
	got, err := c.GetSimilarAnimeByLocalID(context.Background(), "anime-X")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "local-111", got[0].LocalID)
	assert.Equal(t, "local-222", got[1].LocalID)
}
