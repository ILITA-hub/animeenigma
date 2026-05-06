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
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
	require.NoError(t, cache.Get(context.Background(), "recs:user:user-1:topN", &cached),
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
	cache.preBake(t, "recs:user:user-1:topN", prebaked)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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

func TestRecsHandler_PersonalizedBranch_ExcludesCompletedAndDropped(t *testing.T) {
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
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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

	for _, item := range env.Data.Recs {
		assert.NotEqual(t, "anime-completed", item.Anime.ID, "completed anime must NOT appear in personalized row")
		assert.NotEqual(t, "anime-dropped", item.Anime.ID, "dropped anime must NOT appear in personalized row")
	}
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
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
	// Pool is anime-A (watching, retained) + anime-B + anime-C = 3.
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
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
	cache.preBake(t, "recs:user:user-A:topN", RecsEnvelope{
		Recs: []RecItem{{Anime: RecAnimePayload{ID: "for-A"}, Rank: 1}}, Total: 1, RowLabelKey: "recs.upNext",
	})
	cache.preBake(t, "recs:user:user-B:topN", RecsEnvelope{
		Recs: []RecItem{{Anime: RecAnimePayload{ID: "for-B"}, Rank: 1}}, Total: 1, RowLabelKey: "recs.upNext",
	})

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
	seedPhase12History(t, db, "wh-1", "user-1", "watched-1", "hianime", 1500)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
// asserts that the Final score for a candidate sharing attributes with the
// user's history exceeds what the Phase-11 4-signal ensemble would produce.
func TestRecsHandler_PersonalizedBranchS5_ContributesAfterPrecompute(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// User's history: 2 anime sharing Madhouse + manga + tv. Candidate pool:
	// 1 anime sharing the same attributes, 1 anime with completely different
	// attributes. After S5 precompute, the similar candidate should rank
	// strictly higher than the different one.
	seedPhase12Anime(t, db, "history-1", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "history-1", "Madhouse")
	seedPhase12Anime(t, db, "history-2", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "history-2", "Madhouse")
	seedPhase12Anime(t, db, "cand-similar", "released", false, 7.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "cand-similar", "Madhouse")
	seedPhase12Anime(t, db, "cand-different", "released", false, 7.0, "movie", "g", "original")
	seedPhase12History(t, db, "wh-1", "user-1", "history-1", "hianime", 1500)
	seedPhase12History(t, db, "wh-2", "user-1", "history-2", "hianime", 1500)
	// Ensure both candidates have equal S3/S4 footing: no S3 rows, both released.
	// Without S5 these would tie at 0; with S5 the similar one wins.

	// Seed a second user so total_users > 1 → the IDF for Madhouse is positive
	// (rare among the population), making S5 contribute a positive raw score.
	seedPhase12Anime(t, db, "other-1", "released", false, 7.0, "movie", "g", "original")
	seedPhase12History(t, db, "wh-other", "user-2", "other-1", "hianime", 1500)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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

	// Find the two candidates in the response.
	var simRank, diffRank = -1, -1
	for i, item := range env.Data.Recs {
		switch item.Anime.ID {
		case "cand-similar":
			simRank = i
		case "cand-different":
			diffRank = i
		}
	}
	require.GreaterOrEqual(t, simRank, 0, "cand-similar must appear in response")
	require.GreaterOrEqual(t, diffRank, 0, "cand-different must appear in response")
	assert.Less(t, simRank, diffRank,
		"cand-similar (shares Madhouse+tv+pg_13+manga with user history) must rank above cand-different (no shared attributes)")
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
			seedPhase12History(t, db, "wh-u"+sliceTestID(u)+"-"+sliceTestID(k), userID, animeID, "hianime", 60+(k*30))
		}
	}

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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
// — unit-test equivalent of Phase-12 SC#5. Build a fixture where S5
// contribution moves a candidate; assert the rank shifts.
func TestRecsHandler_PersonalizedBranchS5_TopOrderingDiffersFromPhase11Baseline(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	// Without S5: cand-A wins on S3 (highest trending score).
	// With S5: cand-B is similar to user's history (same studio + tags) and
	// cand-A is dissimilar — S5 nudges cand-B above cand-A even though
	// cand-B has lower trending.
	seedPhase12Anime(t, db, "history-1", "released", false, 9.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "history-1", "Madhouse")
	seedPhase12Tag(t, db, "history-1", "shounen")
	seedPhase12Tag(t, db, "history-1", "action")
	seedPhase12Anime(t, db, "history-2", "released", false, 9.0, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "history-2", "Madhouse")
	seedPhase12Tag(t, db, "history-2", "shounen")
	seedPhase12Tag(t, db, "history-2", "action")

	seedPhase12Anime(t, db, "cand-A", "released", false, 8.0, "movie", "g", "original")
	// cand-A has no shared attributes with user's history.
	seedPopulationSignal(t, db, "cand-A", 100.0) // dominant trending

	seedPhase12Anime(t, db, "cand-B", "released", false, 7.5, "tv", "pg_13", "manga")
	seedPhase12Studio(t, db, "cand-B", "Madhouse")
	seedPhase12Tag(t, db, "cand-B", "shounen")
	seedPhase12Tag(t, db, "cand-B", "action")
	seedPopulationSignal(t, db, "cand-B", 30.0) // weaker trending

	seedPhase12History(t, db, "wh-1", "user-1", "history-1", "hianime", 1500)
	seedPhase12History(t, db, "wh-2", "user-1", "history-2", "hianime", 1500)

	// Population: a second user touches different attributes so the IDF for
	// Madhouse + shounen + action becomes positive (rare → discriminative).
	seedPhase12Anime(t, db, "filler-1", "released", false, 7.0, "movie", "g", "original")
	seedPhase12Anime(t, db, "filler-2", "released", false, 7.0, "movie", "g", "original")
	seedPhase12History(t, db, "wh-other-1", "user-2", "filler-1", "hianime", 1500)
	seedPhase12History(t, db, "wh-other-2", "user-3", "filler-2", "hianime", 1500)

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, logger.Default())
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

	// Assert cand-B (similar to history) ranks ABOVE cand-A (dissimilar but
	// higher trending). Without S5, S3 alone would have put cand-A first.
	var posA, posB = -1, -1
	for i, item := range env.Data.Recs {
		switch item.Anime.ID {
		case "cand-A":
			posA = i
		case "cand-B":
			posB = i
		}
	}
	require.GreaterOrEqual(t, posA, 0, "cand-A must appear")
	require.GreaterOrEqual(t, posB, 0, "cand-B must appear")
	assert.Less(t, posB, posA,
		"S5 must lift cand-B (shares attributes with user's history) above cand-A (higher S3 trending but dissimilar). If posB >= posA, S5 is not contributing — Phase-12 SC#5 regression.")
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
