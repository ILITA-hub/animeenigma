package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeRecsCache is an in-memory implementation of the cache surface
// RecsHandler depends on (Get / Set). Get returns ErrNotFound if no key.
type fakeRecsCache struct {
	mu       sync.Mutex
	store    map[string][]byte
	notFound error
}

func newFakeRecsCache() *fakeRecsCache {
	return &fakeRecsCache{
		store:    make(map[string][]byte),
		notFound: errors.New("cache: key not found"),
	}
}

func (c *fakeRecsCache) Get(_ context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.store[key]
	if !ok {
		return c.notFound
	}
	return json.Unmarshal(v, dest)
}

func (c *fakeRecsCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.store[key] = b
	return nil
}

func (c *fakeRecsCache) ErrNotFound() error { return c.notFound }

// preBakeCache puts a payload into the cache directly so the cache-hit path
// can be exercised. Returns the bytes in case the test wants to assert
// against them later.
func (c *fakeRecsCache) preBake(t *testing.T, key string, payload interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	c.mu.Lock()
	c.store[key] = b
	c.mu.Unlock()
	return b
}

// setupRecsTestDB mirrors the production schema for the columns the handler
// hits: animes (id, name, name_ru, name_jp, poster_url, score, episodes_count,
// status, year, hidden, deleted_at) + rec_population_signals (for S3 reads).
func setupRecsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		name_ru TEXT,
		name_jp TEXT,
		poster_url TEXT,
		score REAL,
		episodes_count INTEGER,
		status TEXT,
		year INTEGER,
		aired_on DATETIME,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rec_population_signals (
		anime_id TEXT PRIMARY KEY,
		s3_trending_score REAL NOT NULL DEFAULT 0,
		s4_recency_score REAL NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL
	)`).Error)
	return db
}

func seedAnimeFull(t *testing.T, db *gorm.DB, id, status string, hidden bool, score float64) {
	t.Helper()
	hiddenInt := 0
	if hidden {
		hiddenInt = 1
	}
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, name_ru, name_jp, poster_url, score, episodes_count, status, year, hidden, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		id, "Name "+id, "RU "+id, "JP "+id, "/p/"+id+".jpg", score, 12, status, 2024, hiddenInt,
	).Error)
}

func seedPopulationSignal(t *testing.T, db *gorm.DB, animeID string, s3 float32) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO rec_population_signals (anime_id, s3_trending_score, s4_recency_score, last_computed)
		 VALUES (?, ?, 0, ?)`,
		animeID, s3, time.Now().UTC(),
	).Error)
}

func TestRecsHandler_CacheMissComputesFromEnsemble(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// Realistic fixture: multiple animes with VARYING S3 scores so per-pool
	// MinMaxNormalize has a real span (degenerate single-data-point pools
	// normalize to all-zeros, which is the expected behavior — see the
	// thin-S3 backfill test).
	seedAnimeFull(t, db, "trend-1", "released", false, 8.0)
	seedAnimeFull(t, db, "trend-2", "released", false, 7.5)
	seedAnimeFull(t, db, "ongoing-1", "ongoing", false, 7.5)
	seedAnimeFull(t, db, "boring-1", "released", false, 6.0)
	seedPopulationSignal(t, db, "trend-1", 100.0) // dominant
	seedPopulationSignal(t, db, "trend-2", 30.0)
	seedPopulationSignal(t, db, "boring-1", 5.0)
	// ongoing-1 has no S3 row -> S3 norm = 0; but S4=1.0

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.False(t, env.Data.CacheHit, "cache MISS path: cache_hit must be false")
	assert.Equal(t, "recs.trending", env.Data.RowLabelKey)
	assert.LessOrEqual(t, env.Data.Total, 20)
	assert.Greater(t, env.Data.Total, 0, "thin pool but not empty")
	require.NotEmpty(t, env.Data.Recs)
	// trend-1 has the dominant S3 score so it should rank first.
	// Math: S3 norm: trend-1=1.0, trend-2=0.263, boring-1=0, ongoing-1=0
	// S4 norm: ongoing-1=1.0, others=0
	// Final: trend-1=0.20*1.0=0.20; ongoing-1=0.10*1.0=0.10; trend-2≈0.053; boring-1=0
	assert.Equal(t, "trend-1", env.Data.Recs[0].Anime.ID, "S3 trending dominates when pool has spread")

	// Cache must now be populated for the next call
	var cached interface{}
	require.NoError(t, cache.Get(context.Background(), "recs:public:trending:topN", &cached))
}

func TestRecsHandler_CacheHitReturnsCachedPayload(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	prebaked := RecsEnvelope{
		Recs: []RecItem{{
			Anime: RecAnimePayload{ID: "cached-1", Name: "Cached Anime"},
			Final: 0.93,
			Pinned: false,
			Rank:  1,
		}},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CacheHit:    false, // handler should set this to TRUE on the way out
		Total:       1,
		RowLabelKey: "recs.trending",
	}
	cache.preBake(t, "recs:public:trending:topN", prebaked)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool          `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Data.CacheHit, "cache HIT path: cache_hit must be true")
	assert.Equal(t, 1, env.Data.Total)
	assert.Equal(t, "cached-1", env.Data.Recs[0].Anime.ID)
}

func TestRecsHandler_EmptyPoolReturnsEmptyArray(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()
	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool          `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.Equal(t, 0, env.Data.Total)
	assert.Empty(t, env.Data.Recs)
}

func TestRecsHandler_HiddenExcluded(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	seedAnimeFull(t, db, "visible-1", "ongoing", false, 8.0)
	seedAnimeFull(t, db, "hidden-1", "ongoing", true, 9.5)
	seedPopulationSignal(t, db, "hidden-1", 200.0) // would dominate if not filtered

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool          `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	for _, item := range env.Data.Recs {
		assert.NotEqual(t, "hidden-1", item.Anime.ID, "hidden anime must NEVER appear in response, even with high signal score")
	}
}

func TestRecsHandler_ThinS3PoolBackfillsViaS4(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// Thin S3: zero animes have S3 signal rows. Per spec, when the S3 pool
	// is empty, S4 ordering is the entire ranking (S3 contributes 0 to all).
	// All 5 candidates pass S11 (none hidden, none soft-deleted).
	seedAnimeFull(t, db, "s4-ongoing-1", "ongoing", false, 7.0)
	seedAnimeFull(t, db, "s4-ongoing-2", "ongoing", false, 7.5)
	seedAnimeFull(t, db, "s4-released-1", "released", false, 6.0)
	seedAnimeFull(t, db, "s4-released-2", "released", false, 5.5)
	seedAnimeFull(t, db, "s4-released-3", "released", false, 4.0)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))

	// All 5 animes should appear (pool < 20)
	assert.Equal(t, 5, env.Data.Total)

	gotIDs := make([]string, len(env.Data.Recs))
	for i, item := range env.Data.Recs {
		gotIDs[i] = item.Anime.ID
	}
	// Backfill ordering: ongoing animes (S4=1.0) must rank above all released
	// (S4=0) in this thin-S3 pool.
	ongoingPos := map[string]int{}
	releasedPos := map[string]int{}
	for i, id := range gotIDs {
		switch id {
		case "s4-ongoing-1", "s4-ongoing-2":
			ongoingPos[id] = i
		case "s4-released-1", "s4-released-2", "s4-released-3":
			releasedPos[id] = i
		}
	}
	for _, op := range ongoingPos {
		for _, rp := range releasedPos {
			assert.Less(t, op, rp, "S4 backfill: all ongoing must rank above all released when S3 is empty")
		}
	}
}
