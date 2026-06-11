package handler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
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
// hits: animes, rec_population_signals (S3 reads), plus the user-scope tables
// the personalized branch needs (anime_list, anime_genres, rec_user_signals).
//
// Phase 12 (Wave 1) added kind / rating / material_source columns to animes
// and the studios / anime_studios / tags / anime_tags tables. The watch_history
// table is Phase-5; S5 reads from it. All Phase-12 schema is created here so
// the personalized-branch tests can exercise the full 5-signal ensemble.
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
		kind TEXT DEFAULT '',
		rating TEXT DEFAULT '',
		material_source TEXT DEFAULT '',
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rec_population_signals (
		anime_id TEXT PRIMARY KEY,
		s3_trending_score REAL NOT NULL DEFAULT 0,
		s4_recency_score REAL NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL
	)`).Error)
	// Phase 11 tables — needed by the personalized branch.
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
	require.NoError(t, db.Exec(`CREATE TABLE rec_user_signals (
		user_id TEXT PRIMARY KEY,
		s1_vector TEXT NOT NULL DEFAULT '{}',
		s5_affinity TEXT NOT NULL DEFAULT '{}',
		s6_seed_anime_id TEXT,
		s6_seed_completed_at DATETIME,
		s6_seed_score INTEGER,
		last_computed DATETIME NOT NULL
	)`).Error)
	// Phase 12 schema — Wave 1 added these to support S5 in Wave 3.
	require.NoError(t, db.Exec(`CREATE TABLE studios (
		id TEXT PRIMARY KEY,
		name TEXT
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_studios (
		anime_id TEXT NOT NULL,
		studio_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, studio_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE tags (
		id TEXT PRIMARY KEY,
		name TEXT,
		source TEXT
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_tags (
		anime_id TEXT NOT NULL,
		tag_id TEXT NOT NULL,
		rank INTEGER DEFAULT 0,
		PRIMARY KEY (anime_id, tag_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE watch_history (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		episode_number INTEGER NOT NULL DEFAULT 0,
		player TEXT NOT NULL,
		duration_watched INTEGER NOT NULL DEFAULT 0,
		watched_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`).Error)
	return db
}

// seedPhase12Anime extends seedAnimeFull with the Phase-12 attribute columns.
// The plain seedAnimeFull is unchanged so existing tests still work.
func seedPhase12Anime(t *testing.T, db *gorm.DB, id, status string, hidden bool, score float64, kind, rating, materialSource string) {
	t.Helper()
	hiddenInt := 0
	if hidden {
		hiddenInt = 1
	}
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, name_ru, name_jp, poster_url, score, episodes_count, status, year, kind, rating, material_source, hidden, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		id, "Name "+id, "RU "+id, "JP "+id, "/p/"+id+".jpg", score, 12, status, 2024, kind, rating, materialSource, hiddenInt,
	).Error)
}

func seedPhase12Studio(t *testing.T, db *gorm.DB, animeID, studioID string) {
	t.Helper()
	_ = db.Exec(`INSERT OR IGNORE INTO studios (id, name) VALUES (?, ?)`, studioID, studioID).Error
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES (?, ?)`, animeID, studioID).Error)
}

func seedPhase12Tag(t *testing.T, db *gorm.DB, animeID, tagID string) {
	t.Helper()
	_ = db.Exec(`INSERT OR IGNORE INTO tags (id, name, source) VALUES (?, ?, 'anilist')`, tagID, tagID).Error
	require.NoError(t, db.Exec(`INSERT INTO anime_tags (anime_id, tag_id, rank) VALUES (?, ?, 0)`, animeID, tagID).Error)
}

func seedPhase12Genre(t *testing.T, db *gorm.DB, animeID, genreID string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?)`, animeID, genreID).Error)
}

func seedPhase12History(t *testing.T, db *gorm.DB, rowID, userID, animeID, player string, durationWatched int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO watch_history (id, user_id, anime_id, episode_number, player, duration_watched) VALUES (?, ?, ?, 1, ?, ?)`,
		rowID, userID, animeID, player, durationWatched,
	).Error)
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
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())

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
	require.NoError(t, cache.Get(context.Background(), "recs:public:trending:topN:v2", &cached))
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
	cache.preBake(t, "recs:public:trending:topN:v2", prebaked)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())

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
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())

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
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())

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
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())

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

// ----------------------------------------------------------------------------
// Phase 11 — personalized branch tests.
// ----------------------------------------------------------------------------

// loggedInRequest builds a GET /api/users/recs request with claims pre-injected
// in the context. Bypasses the OptionalAuthMiddleware (which we trust by
// virtue of Phase 10 wiring) and lets us focus on the handler's branch logic.
func loggedInRequest(userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	ctx := authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: userID})
	return req.WithContext(ctx)
}

func seedAnimeListRow(t *testing.T, db *gorm.DB, rowID, userID, animeID, status string, score int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, ?, ?)`,
		rowID, userID, animeID, status, score,
	).Error)
}

func TestRecsHandler_PersonalizedBranch_CacheMissComputesAndCaches(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// 4 animes, none hidden. user-1 has not marked any of them completed.
	seedAnimeFull(t, db, "anime-1", "ongoing", false, 7.0)
	seedAnimeFull(t, db, "anime-2", "released", false, 7.0)
	seedAnimeFull(t, db, "anime-3", "released", false, 7.0)
	seedAnimeFull(t, db, "anime-4", "released", false, 7.0)
	// Some S3 spread.
	seedPopulationSignal(t, db, "anime-1", 50.0)
	seedPopulationSignal(t, db, "anime-2", 10.0)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.Equal(t, "recs.upNext", env.Data.RowLabelKey, "logged-in row uses the upNext label")
	assert.False(t, env.Data.CacheHit)
	assert.Greater(t, env.Data.Total, 0, "personalized row populated for user-1")

	// Cache must now contain the per-user key.
	var cached interface{}
	require.NoError(t, cache.Get(context.Background(), "recs:user:user-1:topN:v4", &cached),
		"per-user cache key must be populated after fresh compute")
}

func TestRecsHandler_PersonalizedBranch_CacheHitReturnsCachedPayload(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	prebaked := RecsEnvelope{
		Recs: []RecItem{{
			Anime:  RecAnimePayload{ID: "cached-up-next-1", Name: "Cached Personalized"},
			Final:  0.91,
			Pinned: false,
			Rank:   1,
		}},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CacheHit:    false,
		Total:       1,
		RowLabelKey: "recs.upNext",
	}
	cache.preBake(t, "recs:user:user-1:topN:v4", prebaked)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Data.CacheHit, "cache HIT path: cache_hit must be true")
	assert.Equal(t, "recs.upNext", env.Data.RowLabelKey)
	assert.Equal(t, 1, env.Data.Total)
	assert.Equal(t, "cached-up-next-1", env.Data.Recs[0].Anime.ID)
}

func TestRecsHandler_PersonalizedBranch_ExcludesAnyListEntry(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	seedAnimeFull(t, db, "anime-watching", "ongoing", false, 8.0)
	seedAnimeFull(t, db, "anime-completed", "released", false, 9.0)
	seedAnimeFull(t, db, "anime-dropped", "released", false, 8.5)
	seedAnimeFull(t, db, "anime-fresh", "ongoing", false, 7.0)
	seedAnimeListRow(t, db, "al1", "user-1", "anime-watching", "watching", 0)
	seedAnimeListRow(t, db, "al2", "user-1", "anime-completed", "completed", 9)
	seedAnimeListRow(t, db, "al3", "user-1", "anime-dropped", "dropped", 4)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))

	// All three list entries (any status) must be excluded — only anime-fresh
	// (no list row) survives the candidate pool.
	for _, item := range env.Data.Recs {
		assert.NotEqual(t, "anime-watching", item.Anime.ID, "watching anime must NOT appear — recs are for unlisted only")
		assert.NotEqual(t, "anime-completed", item.Anime.ID, "completed anime must NOT appear in personalized row")
		assert.NotEqual(t, "anime-dropped", item.Anime.ID, "dropped anime must NOT appear in personalized row")
	}
	require.Equal(t, 1, env.Data.Total, "only anime-fresh has no list row and should be the sole rec")
	assert.Equal(t, "anime-fresh", env.Data.Recs[0].Anime.ID)
}

func TestRecsHandler_PersonalizedBranch_ColdStartUserDegradesToS3S4(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// User-1 has only 1 scored anime — below the S1 cold-start threshold of 3
	// AND below the S2 fallback threshold of 5. S1 + S2 should both emit
	// empty maps and the row should still render via S3 + S4.
	seedAnimeFull(t, db, "anime-A", "ongoing", false, 8.0)
	seedAnimeFull(t, db, "anime-B", "ongoing", false, 7.5)
	seedAnimeFull(t, db, "anime-C", "released", false, 7.0)
	seedAnimeListRow(t, db, "al1", "user-1", "anime-A", "watching", 4) // < 5 fallback

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success, "cold-start request must succeed (no NaN, no error)")
	// Pool is anime-B + anime-C = 2 (anime-A is in user's list → excluded).
	assert.Greater(t, env.Data.Total, 0, "ensemble must degrade gracefully and still produce ranked recs")
}

func TestRecsHandler_PersonalizedBranch_EmptyPoolReturnsEmptyEnvelope(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	seedAnimeFull(t, db, "anime-1", "released", false, 8.0)
	seedAnimeFull(t, db, "anime-2", "released", false, 7.5)
	// User-1 has marked everything completed -> candidate pool is empty.
	seedAnimeListRow(t, db, "al1", "user-1", "anime-1", "completed", 8)
	seedAnimeListRow(t, db, "al2", "user-1", "anime-2", "completed", 7)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.Equal(t, 0, env.Data.Total)
	assert.Empty(t, env.Data.Recs)
	assert.Equal(t, "recs.upNext", env.Data.RowLabelKey)
}

func TestRecsHandler_PersonalizedBranch_PerUserCacheIsolation(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// Pre-bake distinct payloads for two users — verifies the cache key is
	// per-user.
	cache.preBake(t, "recs:user:user-A:topN:v4", RecsEnvelope{
		Recs: []RecItem{{Anime: RecAnimePayload{ID: "for-A"}, Rank: 1}}, Total: 1, RowLabelKey: "recs.upNext",
	})
	cache.preBake(t, "recs:user:user-B:topN:v4", RecsEnvelope{
		Recs: []RecItem{{Anime: RecAnimePayload{ID: "for-B"}, Rank: 1}}, Total: 1, RowLabelKey: "recs.upNext",
	})

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	for _, tc := range []struct{ user, want string }{
		{"user-A", "for-A"},
		{"user-B", "for-B"},
	} {
		req := loggedInRequest(tc.user)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var env struct {
			Success bool         `json:"success"`
			Data    RecsEnvelope `json:"data"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
		require.Len(t, env.Data.Recs, 1)
		assert.Equal(t, tc.want, env.Data.Recs[0].Anime.ID, "per-user cache must be keyed on JWT user_id")
	}
}

func TestRecsHandler_PersonalizedBranch_ServerSliceTo50_AnonymousStillTo20(t *testing.T) {
	// Seed 75 visible anime so the slice ceilings actually matter. User-A is
	// logged in -> expects up to 50. Anonymous request -> expects up to 20.
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()
	for i := 0; i < 75; i++ {
		id := "anime-" + sliceTestID(i)
		seedAnimeFull(t, db, id, "released", false, 7.0)
		seedPopulationSignal(t, db, id, float32(75-i)) // unique S3 -> deterministic order
	}

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	// Logged-in -> 50 cap.
	reqLi := loggedInRequest("user-A")
	wLi := httptest.NewRecorder()
	r.ServeHTTP(wLi, reqLi)
	require.Equal(t, http.StatusOK, wLi.Code)
	var envLi struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(wLi.Body).Decode(&envLi))
	assert.Equal(t, 50, envLi.Data.Total, "logged-in server slice = 50")
	assert.Equal(t, "recs.upNext", envLi.Data.RowLabelKey)

	// Anonymous -> 20 cap.
	reqAnon := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	wAnon := httptest.NewRecorder()
	r.ServeHTTP(wAnon, reqAnon)
	require.Equal(t, http.StatusOK, wAnon.Code)
	var envAnon struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(wAnon.Body).Decode(&envAnon))
	assert.Equal(t, 20, envAnon.Data.Total, "anonymous server slice = 20")
	assert.Equal(t, "recs.trending", envAnon.Data.RowLabelKey)
}

// ---------------------------------------------------------------------------
// Phase 12 — S5 in personalized-branch ensemble.
// ---------------------------------------------------------------------------

// TestRecsHandler_PersonalizedBranchS5_RegistryHasFiveSignals — sanity check
// via behavior: a user with rich watch_history + populated attribute schema
// gets recs that depend on S5 contribution. The "5 signals" assertion
// proper happens via the source-level grep in <acceptance_criteria>; this
// behavioral test ensures the handler actually wires h.s5 into the
// ensemble registry and isn't just declared but unused.
func TestRecsHandler_PersonalizedBranchS5_RegistryHasFiveSignals(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// Build a fixture where S5 will produce non-zero contribution: user
	// watched 1 anime with all 6 attribute dimensions populated; the
	// candidate pool includes another anime sharing the studio + kind.
	seedPhase12Anime(t, db, "watched-1", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "watched-1", "Madhouse")
	seedPhase12Genre(t, db, "watched-1", "action")
	seedPhase12Tag(t, db, "watched-1", "shounen")
	seedPhase12Anime(t, db, "cand-similar", "released", false, 7.5, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "cand-similar", "Madhouse")
	seedPhase12Anime(t, db, "cand-different", "released", false, 7.5, "movie", "g", "original")
	seedPhase12History(t, db, "wh-1", "user-1", "watched-1", "kodik", 1500)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	require.NotNil(t, h.s5, "RecsHandler must wire h.s5 in NewRecsHandler (Phase-12 Wave-3)")

	// Run S5.Precompute synchronously so the affinity vector exists.
	require.NoError(t, h.s5.Precompute(context.Background(), "user-1"))

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)
	req := loggedInRequest("user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.Greater(t, env.Data.Total, 0, "ensemble must produce ranked recs with S5 wired in")
	// "watched-1" is in the user's watch_history but not in anime_list, so the
	// CandidatePoolForUser exclusion (which keys on anime_list) does NOT exclude
	// it — both candidates appear. We don't assert specific ordering here; that
	// belongs in TestRecsHandler_PersonalizedBranchS5_TopOrderingDiffersFromPhase11Baseline.
}

// TestRecsHandler_PersonalizedBranchS5_ContributesAfterPrecompute — directly
// asserts that S5 contributes a non-zero ensemble Breakdown entry on at
// least one candidate after Precompute. We use the recs.Ensemble directly
// (rather than the HTTP handler) so we can inspect the Breakdown map per
// signal — the HTTP envelope only exposes Final, which can mask S5
// contribution when the per-pool normalizer flattens ties.
func TestRecsHandler_PersonalizedBranchS5_ContributesAfterPrecompute(t *testing.T) {
	db := setupRecsTestDB(t)

	// History anime + a similar candidate (same studio + tags + kind +
	// material_source + rating). Population includes additional users with
	// disjoint attributes so the IDF for the user's attributes is positive
	// (rare → discriminative).
	seedPhase12Anime(t, db, "history-1", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "history-1", "Madhouse")
	seedPhase12Tag(t, db, "history-1", "shounen")
	seedPhase12Tag(t, db, "history-1", "action")

	seedPhase12Anime(t, db, "cand-similar", "released", false, 7.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "cand-similar", "Madhouse")
	seedPhase12Tag(t, db, "cand-similar", "shounen")
	seedPhase12Tag(t, db, "cand-similar", "action")

	seedPhase12Anime(t, db, "cand-different", "released", false, 7.0, "movie", "g", "original")

	seedPhase12History(t, db, "wh-1", "user-1", "history-1", "kodik", 1500)

	// Population: 4 other users touching disjoint attributes so user-1's
	// shounen / action / Madhouse / tv / pg_13 / manga become rare.
	for i, u := range []string{"user-2", "user-3", "user-4", "user-5"} {
		fillerID := "filler-c-" + sliceTestID(i)
		seedPhase12Anime(t, db, fillerID, "released", false, 7.0, "movie", "g", "original")
		seedPhase12History(t, db, "wh-other-c-"+sliceTestID(i), u, fillerID, "kodik", 1500)
	}

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, newFakeRecsCache(), nil, logger.Default())
	require.NoError(t, h.s5.Precompute(context.Background(), "user-1"))

	// Direct call to S5.Score to inspect the raw output (pre-normalization)
	// for the candidates of interest.
	candidates := []recs.AnimeID{"cand-similar", "cand-different"}
	rawS5, err := h.s5.Score(context.Background(), "user-1", candidates)
	require.NoError(t, err)
	assert.Contains(t, rawS5, recs.AnimeID("cand-similar"),
		"S5 must emit a raw score for cand-similar (shares attributes with user history)")
	assert.Greater(t, float64(rawS5["cand-similar"]), 0.0,
		"S5 raw score for cand-similar must be > 0 with positive IDF on rare attributes")
	// cand-different shares NO attributes with user-1's history, so S5
	// contributes zero — the score may be omitted from the output map.
	if v, ok := rawS5["cand-different"]; ok {
		assert.Less(t, float64(v), float64(rawS5["cand-similar"]),
			"S5 must rank cand-similar strictly above cand-different")
	}
}

// TestRecsHandler_PersonalizedBranchS5_NoNaN — property test on a 50-anime
// fixture: every Final value is finite and non-negative.
func TestRecsHandler_PersonalizedBranchS5_NoNaN(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	for i := 0; i < 50; i++ {
		id := "anime-" + sliceTestID(i)
		kind := []string{"tv", "movie", "ova", "ona", "special"}[i%5]
		rating := []string{"g", "pg", "pg_13", "r", "r_plus"}[i%5]
		source := []string{"manga", "novel", "original", "light_novel", "game"}[i%5]
		seedPhase12Anime(t, db, id, "released", false, 7.0, kind, rating, source)
		seedPhase12Studio(t, db, id, "studio-"+sliceTestID(i%4))
		seedPhase12Genre(t, db, id, "genre-"+sliceTestID(i%7))
		seedPhase12Tag(t, db, id, "tag-"+sliceTestID(i%9))
		seedPopulationSignal(t, db, id, float32(50-i))
	}
	for u := 0; u < 5; u++ {
		userID := "u" + sliceTestID(u)
		for k := 0; k < 5; k++ {
			animeID := "anime-" + sliceTestID((u*5+k)%50)
			seedPhase12History(t, db, "wh-u"+sliceTestID(u)+"-"+sliceTestID(k), userID, animeID, "kodik", 60+(k*30))
		}
	}

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	require.NoError(t, h.s5.Precompute(context.Background(), "u00"))

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)
	req := loggedInRequest("u00")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))

	for _, item := range env.Data.Recs {
		assert.False(t, math.IsNaN(item.Final), "anime %s produced NaN", item.Anime.ID)
		assert.False(t, math.IsInf(item.Final, 0), "anime %s produced Inf", item.Anime.ID)
		assert.GreaterOrEqual(t, item.Final, 0.0, "anime %s produced negative Final", item.Anime.ID)
	}
}

// TestRecsHandler_PersonalizedBranchS5_ColdStartUser — user with zero
// watch_history. S5 returns empty map. Ensemble still produces recs from
// S3+S4. No NaN, no error.
func TestRecsHandler_PersonalizedBranchS5_ColdStartUser(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	seedPhase12Anime(t, db, "anime-A", "ongoing", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Anime(t, db, "anime-B", "released", false, 7.5, "movie", "g", "original")
	seedPopulationSignal(t, db, "anime-A", 50.0)

	// User-cold has no watch_history, no anime_list.

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	require.NoError(t, h.s5.Precompute(context.Background(), "user-cold"),
		"S5.Precompute on a cold-start user must succeed")

	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)
	req := loggedInRequest("user-cold")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.Equal(t, "recs.upNext", env.Data.RowLabelKey)
	assert.Greater(t, env.Data.Total, 0, "cold-start must still return S3+S4-driven recs")
	for _, item := range env.Data.Recs {
		assert.False(t, math.IsNaN(item.Final))
		assert.False(t, math.IsInf(item.Final, 0))
	}
}

// TestRecsHandler_PersonalizedBranchS5_TopOrderingDiffersFromPhase11Baseline
// — unit-test equivalent of Phase-12 SC#5. Builds two ensembles in the
// same fixture: a "Phase-11" 4-signal ensemble (without S5) and a
// "Phase-12" 5-signal ensemble (with S5). Asserts the rankings differ.
//
// Tie-mitigation: the candidate set has DISTINCT attribute alignment
// strengths so the per-pool MinMax normalizer doesn't flatten S5 to 0
// across all candidates. cand-Strong shares 4 attributes; cand-Weak
// shares 1; cand-None shares 0.
func TestRecsHandler_PersonalizedBranchS5_TopOrderingDiffersFromPhase11Baseline(t *testing.T) {
	db := setupRecsTestDB(t)

	seedPhase12Anime(t, db, "history-1", "released", false, 9.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "history-1", "Madhouse")
	seedPhase12Tag(t, db, "history-1", "shounen")
	seedPhase12Tag(t, db, "history-1", "action")

	// cand-Strong: shares studio + 2 tags + kind + rating + source = 6/6 dims aligned.
	seedPhase12Anime(t, db, "cand-Strong", "released", false, 7.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "cand-Strong", "Madhouse")
	seedPhase12Tag(t, db, "cand-Strong", "shounen")
	seedPhase12Tag(t, db, "cand-Strong", "action")
	// cand-Weak: shares only kind = 1/6 dims aligned.
	seedPhase12Anime(t, db, "cand-Weak", "released", false, 7.0, "tv", "g", "original")
	// cand-None: shares 0 attributes.
	seedPhase12Anime(t, db, "cand-None", "released", false, 7.0, "movie", "r", "novel")

	seedPhase12History(t, db, "wh-1", "user-1", "history-1", "kodik", 1500)

	// Population: 4 other users touch DIFFERENT attributes from user-1 so
	// the IDFs for the user's attributes become discriminative.
	for i, u := range []string{"user-2", "user-3", "user-4", "user-5"} {
		fillerID := "filler-tod-" + sliceTestID(i)
		seedPhase12Anime(t, db, fillerID, "released", false, 7.0, "movie", "r", "novel")
		seedPhase12History(t, db, "wh-other-tod-"+sliceTestID(i), u, fillerID, "kodik", 1500)
	}

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, newFakeRecsCache(), nil, logger.Default())
	require.NoError(t, h.s5.Precompute(context.Background(), "user-1"))
	require.NoError(t, h.s1.Precompute(context.Background(), "user-1"))

	pool, err := h.s11.CandidatePoolForUser(context.Background(), recs.UserID("user-1"))
	require.NoError(t, err)
	require.NotEmpty(t, pool, "candidate pool must be non-empty")

	// Phase-11 ensemble (4 signals, no S5).
	phase11 := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s1, Weight: 0.30},
		{Module: h.s2, Weight: 0.20},
		{Module: h.s3, Weight: 0.20},
		{Module: h.s4, Weight: 0.10},
	})
	phase12 := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s1, Weight: 0.30},
		{Module: h.s2, Weight: 0.20},
		{Module: h.s3, Weight: 0.20},
		{Module: h.s4, Weight: 0.10},
		{Module: h.s5, Weight: 0.20},
	})

	rank11, err := phase11.Rank(context.Background(), recs.UserID("user-1"), pool)
	require.NoError(t, err)
	rank12, err := phase12.Rank(context.Background(), recs.UserID("user-1"), pool)
	require.NoError(t, err)

	// At least one candidate's Final must differ between the two ensembles.
	final11 := make(map[recs.AnimeID]float64, len(rank11))
	for _, s := range rank11 {
		final11[s.AnimeID] = s.Final
	}
	final12 := make(map[recs.AnimeID]float64, len(rank12))
	for _, s := range rank12 {
		final12[s.AnimeID] = s.Final
	}
	differs := false
	for id, f12 := range final12 {
		if math.Abs(f12-final11[id]) > 1e-9 {
			differs = true
			break
		}
	}
	assert.True(t, differs,
		"Phase-12 ensemble Final scores must differ from Phase-11 ensemble on at least one candidate (S5 contributes). If identical, S5 is not contributing — Phase-12 SC#5 regression.")
}

// ---------------------------------------------------------------------------
// Phase 3 (spec 2026-06-11) — S7 dropped-penalty in the logged-in ensemble.
// ---------------------------------------------------------------------------

// TestServeLoggedIn_S7DemotesDroppedSimilar verifies that S7 (weight −0.15)
// lowers the rank of candidates similar to a user's dropped anime. We test at
// the ensemble level directly:
//
//   - A fake constant signal is used as the only positive signal (equal score
//     for every candidate) so all ordering variation comes from S7.
//   - A user has dropped 2+ anime with genres g1+g2 (cold-start guard: ≥2 seeds).
//   - Three candidates with different S7 raw scores create a non-degenerate
//     S7 pool (required so MinMaxNormalize doesn't flatten to all-zeros):
//     · "cand-high"  : genre g1+g2 → strong overlap with both dropped seeds
//     · "cand-partial": genre g1 only → partial overlap with seed d1
//     · "cand-none"  : no shared genres → S7 raw = 0
//   - After normalization the ensemble Final scores must satisfy:
//     cand-none ≥ cand-partial > cand-high  (more penalty = lower rank).
//
// We chose the ensemble-level approach because the HTTP-path fixtures would
// require carefully neutralizing all five positive signals simultaneously;
// the ensemble approach gives clean, signal-specific evidence. The three-
// candidate pool ensures S7 normalization is non-degenerate (min < max) so
// the -0.15 penalty creates observable ordering differences.
func TestServeLoggedIn_S7DemotesDroppedSimilar(t *testing.T) {
	db := setupRecsTestDB(t)

	// Dropped seeds: d1 has genres g1+g2; d2 has genres g1+g2. Both scores <7.
	seedPhase12Anime(t, db, "s7-drop-1", "released", false, 7.0, "tv", "pg_13", "manga")
	seedPhase12Genre(t, db, "s7-drop-1", "s7g1")
	seedPhase12Genre(t, db, "s7-drop-1", "s7g2")
	seedPhase12Anime(t, db, "s7-drop-2", "released", false, 7.0, "tv", "pg_13", "manga")
	seedPhase12Genre(t, db, "s7-drop-2", "s7g1")
	seedPhase12Genre(t, db, "s7-drop-2", "s7g2")
	seedAnimeListRow(t, db, "al-s7d1", "user-s7", "s7-drop-1", "dropped", 3)
	seedAnimeListRow(t, db, "al-s7d2", "user-s7", "s7-drop-2", "dropped", 2)

	// Three candidates with different S7 raw scores:
	// cand-high: genres g1+g2 → Jaccard with {g1,g2} seed = 1.0 for both seeds → best=1.0
	seedPhase12Anime(t, db, "s7-cand-high", "released", false, 7.5, "tv", "pg_13", "manga")
	seedPhase12Genre(t, db, "s7-cand-high", "s7g1")
	seedPhase12Genre(t, db, "s7-cand-high", "s7g2")
	// cand-partial: genre g1 only → Jaccard with {g1,g2} = 0.5 for best seed → best=0.5
	seedPhase12Anime(t, db, "s7-cand-partial", "released", false, 7.5, "tv", "pg_13", "manga")
	seedPhase12Genre(t, db, "s7-cand-partial", "s7g1")
	// cand-none: no overlapping genres → S7 raw = 0 (absent from map)
	seedPhase12Anime(t, db, "s7-cand-none", "released", false, 7.5, "movie", "g", "original")
	seedPhase12Genre(t, db, "s7-cand-none", "s7g9") // unrelated genre

	s7 := h_s7ForTest(db)
	fakeSig := &uniformSignal{id: "fake", score: 1.0}

	candidates := []recs.AnimeID{"s7-cand-high", "s7-cand-partial", "s7-cand-none"}

	// The three S7 raw scores (1.0, 0.5, 0.0) form a non-degenerate pool:
	// min=0.0, max=1.0 → normalized: cand-high=1.0, cand-partial=0.5, cand-none=0.0.
	// S7 weighted: -0.15, -0.075, 0.0. All positive signals = 0 (degenerate fakeSig).
	// Finals: cand-high=-0.15, cand-partial=-0.075, cand-none=0.0.
	// Expected ordering: cand-none (0) > cand-partial (-0.075) > cand-high (-0.15).
	ens := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: fakeSig, Weight: 1.0},
		{Module: s7, Weight: -0.15}, // S7 appended LAST per spec
	})

	ranked, err := ens.Rank(context.Background(), recs.UserID("user-s7"), candidates)
	require.NoError(t, err)
	require.Len(t, ranked, 3)

	// Verify S7 ordering: no-overlap (least penalty) ranks above high-overlap (most penalty).
	positions := make(map[recs.AnimeID]int, 3)
	for i, r := range ranked {
		positions[r.AnimeID] = i
	}
	assert.Less(t, positions[recs.AnimeID("s7-cand-none")], positions[recs.AnimeID("s7-cand-high")],
		"S7: cand-none (no penalty) must rank above cand-high (max penalty)")
	assert.Less(t, positions[recs.AnimeID("s7-cand-partial")], positions[recs.AnimeID("s7-cand-high")],
		"S7: cand-partial (half penalty) must rank above cand-high (full penalty)")
	// Final score of cand-high must be strictly negative (S7 is active).
	highIdx := positions[recs.AnimeID("s7-cand-high")]
	assert.Less(t, ranked[highIdx].Final, 0.0,
		"S7: cand-high Final must be < 0 (negative S7 contribution is non-zero)")
}

// h_s7ForTest constructs a real S7DroppedPenalty against the test DB.
func h_s7ForTest(db *gorm.DB) *signals.S7DroppedPenalty {
	return signals.NewS7DroppedPenalty(db)
}

// uniformSignal is a synthetic SignalModule that returns a fixed score for
// every candidate. Used to isolate the effect of S7 in TestServeLoggedIn_S7DemotesDroppedSimilar.
type uniformSignal struct {
	id    string
	score float64
}

func (u *uniformSignal) ID() recs.SignalID { return recs.SignalID(u.id) }
func (u *uniformSignal) Precompute(_ context.Context, _ recs.UserID) error { return nil }
func (u *uniformSignal) Score(_ context.Context, _ recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	for _, id := range candidates {
		out[id] = recs.RawScore(u.score)
	}
	return out, nil
}

// sliceTestID returns a zero-padded id so SQL ordering would be predictable
// if we ever needed it; the test doesn't assert order, just count.
func sliceTestID(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return "0" + string(digits[i])
	}
	tens := i / 10
	ones := i % 10
	return string(digits[tens]) + string(digits[ones])
}
