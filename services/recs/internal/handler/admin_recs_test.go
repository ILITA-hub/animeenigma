// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02) — handler-level tests for the admin
// debug endpoint (GetAdminRecs) and the force-recompute endpoint
// (ForceRecompute). Reuses the Phase 11/12/13 setupRecsTestDB fixture so the
// admin payload can be exercised end-to-end against a sqlite-backed in-memory
// schema that mirrors production.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminCacheStub satisfies the Get+Set+Delete surface AdminRecsHandler
// expects. Records Delete calls so the force-recompute test can assert the
// per-user topN cache key was busted.
type adminCacheStub struct {
	store     map[string][]byte
	notFound  error
	deletes   []string
	deleteErr error
}

func newAdminCacheStub() *adminCacheStub {
	return &adminCacheStub{
		store:    map[string][]byte{},
		notFound: errors.New("cache: key not found"),
	}
}

func (c *adminCacheStub) Get(_ context.Context, key string, dest interface{}) error {
	v, ok := c.store[key]
	if !ok {
		return c.notFound
	}
	return json.Unmarshal(v, dest)
}

func (c *adminCacheStub) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.store[key] = b
	return nil
}

func (c *adminCacheStub) Delete(_ context.Context, keys ...string) error {
	c.deletes = append(c.deletes, keys...)
	return c.deleteErr
}

// ----------------------------------------------------------------------------
// GetAdminRecs tests
// ----------------------------------------------------------------------------

func TestAdminRecsHandler_GetAdminRecs_ReturnsBreakdownForFiveSignals(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	// 4 candidate animes with attribute spread so all 5 signals contribute.
	seedPhase12Anime(t, db, "anime-1", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Anime(t, db, "anime-2", "released", false, 7.0, "movie", "g", "original")
	seedPhase12Anime(t, db, "anime-3", "ongoing", false, 7.5, "tv", "pg_13", "manga")
	seedPhase12Anime(t, db, "anime-4", "released", false, 8.5, "tv", "r", "novel")
	seedPopulationSignal(t, db, "anime-1", 100.0)
	seedPopulationSignal(t, db, "anime-3", 50.0)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	precompute := recs.NewOrchestrator([]recs.SignalModule{
		signals.NewS1ScoreCluster(db, recsRepo),
		signals.NewS2Metadata(db),
		signals.NewS5Attribute(db, recsRepo),
	})
	h := NewAdminRecsHandler(db, recsRepo, cache, s6, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/user-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.Equal(t, "user-1", env.Data.UserID)
	require.NotEmpty(t, env.Data.Recs, "must return at least one row")

	// Every row has all 5 signal keys in breakdown + weights + a top_contributor.
	for _, row := range env.Data.Recs {
		for _, sig := range []string{"s1", "s2", "s3", "s4", "s5"} {
			_, hasBd := row.Breakdown[sig]
			_, hasW := row.Weights[sig]
			assert.True(t, hasBd, "row rank %d: breakdown missing %q", row.Rank, sig)
			assert.True(t, hasW, "row rank %d: weights missing %q", row.Rank, sig)
		}
		assert.NotEmpty(t, row.TopContributor)
	}

	// signal_versions hardcoded.
	assert.Equal(t, "v1.0", env.Data.SignalVersions["s1"])
	assert.Equal(t, "v1.0", env.Data.SignalVersions["s5"])
	assert.NotEmpty(t, env.Data.ComputedAt)
}

func TestAdminRecsHandler_GetAdminRecs_SlicesToTop50(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	for i := 0; i < 75; i++ {
		id := "anime-" + sliceTestID(i)
		seedPhase12Anime(t, db, id, "released", false, 7.0, "tv", "pg_13", "manga")
		seedPopulationSignal(t, db, id, float32(75-i))
	}

	recsRepo := repo.NewRecsRepository(db)
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, nil, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/user-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.LessOrEqual(t, len(env.Data.Recs), 50, "admin top-50 slice cap")
	assert.GreaterOrEqual(t, len(env.Data.Recs), 50, "75 candidates should produce 50 ranked rows")
}

func TestAdminRecsHandler_GetAdminRecs_FilteredOutHasAllReasons(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	// 1 visible + 1 completed + 1 dropped + 1 hidden + 1 active.
	seedAnimeFull(t, db, "visible-1", "released", false, 7.0)
	seedAnimeFull(t, db, "completed-1", "released", false, 8.0)
	seedAnimeFull(t, db, "completed-2", "released", false, 8.0)
	seedAnimeFull(t, db, "dropped-1", "released", false, 6.5)
	seedAnimeFull(t, db, "hidden-1", "released", true, 9.0)
	seedAnimeListRow(t, db, "al-c1", "user-1", "completed-1", "completed", 8)
	seedAnimeListRow(t, db, "al-c2", "user-1", "completed-2", "completed", 9)
	seedAnimeListRow(t, db, "al-d1", "user-1", "dropped-1", "dropped", 4)

	recsRepo := repo.NewRecsRepository(db)
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, nil, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/user-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.Len(t, env.Data.FilteredOut, 4, "expected 2 completed + 1 dropped + 1 hidden")
	reasons := map[string]int{}
	for _, e := range env.Data.FilteredOut {
		reasons[e.Reason]++
	}
	assert.Equal(t, 2, reasons["status=completed"])
	assert.Equal(t, 1, reasons["status=dropped"])
	assert.Equal(t, 1, reasons["hidden=true"])
}

func TestAdminRecsHandler_GetAdminRecs_PinRowOnTopWithS6Source(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	// 6 candidate animes (S6 local cascade requires >= 5 matches to fire local).
	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	completed := time.Now().UTC().Add(-1 * time.Hour)
	coOcc := map[string]int{
		"cand-a": 10, "cand-b": 8, "cand-c": 6, "cand-d": 4, "cand-e": 3, "cand-f": 2,
	}
	seedS6Fixture(t, db, "user-1", "seed-1", "Grand Blue", completed, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, s6, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/user-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.NotEmpty(t, env.Data.Recs)

	// Top row should be the pin (rank 1, pinned=true, top_contributor=s6_pin).
	assert.True(t, env.Data.Recs[0].Pinned, "rank 1 must be the pinned row")
	assert.Equal(t, 1, env.Data.Recs[0].Rank)
	assert.Equal(t, "s6_pin", env.Data.Recs[0].TopContributor)
	assert.Equal(t, "local", env.Data.Recs[0].PinSource, "with co-occurrences >= 5 the cascade picks 'local'")
	assert.NotEmpty(t, env.Data.Recs[0].PinSeedAnimeID)
	// contributor_detail should carry pin metadata for the top row.
	require.NotNil(t, env.Data.Recs[0].ContributorDetail)
	assert.Contains(t, env.Data.Recs[0].ContributorDetail, "pin_source")
}

func TestAdminRecsHandler_GetAdminRecs_EmptyUserReturnsEmptyResponse(t *testing.T) {
	// User-with-no-history hitting the admin endpoint must NOT 404 — we
	// return a 200 with empty recs + empty filtered_out so the admin can
	// confirm "this user has nothing to recommend yet".
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	recsRepo := repo.NewRecsRepository(db)
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, nil, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/no-such-user", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.Empty(t, env.Data.Recs)
	assert.Empty(t, env.Data.FilteredOut)
}

// TestAdminRecsHandler_GetAdminRecs_PreS12RankIsPermutation verifies the
// Phase-4 S12 wiring: every non-pinned admin row carries a pre_s12_rank, and
// the set of pre_s12_rank values across the ensemble body is a permutation of
// 1..N (the re-rank reorders, never adds/drops). With genre-diverse fixtures
// the post-rerank `rank` order differs from pre_s12_rank for at least one row.
func TestAdminRecsHandler_GetAdminRecs_PreS12RankIsPermutation(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	// 3 candidates: two genre-clones lead on S3, one diverse trails — same
	// shape as the public interleaving test so S12 actually reorders.
	seedPhase12Anime(t, db, "clone-A", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Anime(t, db, "clone-B", "released", false, 8.0, "tv", "pg_13", "manga")
	seedPhase12Anime(t, db, "diverse-C", "released", false, 8.0, "movie", "g", "original")
	seedPopulationSignal(t, db, "clone-A", 100.0)
	seedPopulationSignal(t, db, "clone-B", 50.0)
	seedPopulationSignal(t, db, "diverse-C", 1.0)
	seedPhase12Genre(t, db, "clone-A", "g1")
	seedPhase12Genre(t, db, "clone-A", "g2")
	seedPhase12Genre(t, db, "clone-B", "g1")
	seedPhase12Genre(t, db, "clone-B", "g2")
	seedPhase12Genre(t, db, "diverse-C", "g3")
	seedPhase12Genre(t, db, "diverse-C", "g4")

	recsRepo := repo.NewRecsRepository(db)
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	// No S6 — the body is the whole ensemble, so pre_s12_rank covers all rows.
	h := NewAdminRecsHandler(db, recsRepo, cache, nil, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/user-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.Len(t, env.Data.Recs, 3)

	// pre_s12_rank across the body must be a permutation of 1..N.
	n := len(env.Data.Recs)
	seen := make(map[int]bool, n)
	differs := false
	for _, row := range env.Data.Recs {
		assert.GreaterOrEqual(t, row.PreS12Rank, 1, "every non-pin row must carry a pre_s12_rank >= 1")
		assert.LessOrEqual(t, row.PreS12Rank, n)
		assert.False(t, seen[row.PreS12Rank], "pre_s12_rank %d duplicated — must be a permutation", row.PreS12Rank)
		seen[row.PreS12Rank] = true
		if row.Rank != row.PreS12Rank {
			differs = true
		}
	}
	for i := 1; i <= n; i++ {
		assert.True(t, seen[i], "pre_s12_rank permutation missing value %d", i)
	}
	assert.True(t, differs, "S12 must reorder at least one row (post-rerank rank != pre_s12_rank)")

	// Concretely: diverse-C (pre_s12_rank 3) must be pulled to post-rerank rank 2.
	posOf := func(id string) AdminRecRow {
		for _, row := range env.Data.Recs {
			if row.Anime.ID == id {
				return row
			}
		}
		t.Fatalf("row %q not found", id)
		return AdminRecRow{}
	}
	c := posOf("diverse-C")
	assert.Equal(t, 3, c.PreS12Rank, "diverse-C was 3rd on raw Final")
	assert.Equal(t, 2, c.Rank, "S12 pulls diverse-C to post-rerank rank 2")
}

// TestAdminRecsHandler_GetAdminRecs_PinRowHasZeroPreS12Rank verifies the pin
// row (not part of the ensemble ranking) serializes pre_s12_rank as 0 while
// the body rows still carry their real pre-rerank positions.
func TestAdminRecsHandler_GetAdminRecs_PinRowHasZeroPreS12Rank(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	completed := time.Now().UTC().Add(-1 * time.Hour)
	coOcc := map[string]int{
		"cand-a": 10, "cand-b": 8, "cand-c": 6, "cand-d": 4, "cand-e": 3, "cand-f": 2,
	}
	seedS6Fixture(t, db, "user-1", "seed-1", "Grand Blue", completed, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, s6, precompute, logger.Default())

	r := chi.NewRouter()
	r.Get("/api/admin/recs/{user_id}", h.GetAdminRecs)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/recs/user-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool              `json:"success"`
		Data    AdminRecsResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.NotEmpty(t, env.Data.Recs)

	require.True(t, env.Data.Recs[0].Pinned, "rank 1 must be the pin")
	assert.Equal(t, 0, env.Data.Recs[0].PreS12Rank, "pin row is not ensemble-ranked → pre_s12_rank 0")
	// Body rows (rank >= 2) carry real pre_s12_rank values.
	for _, row := range env.Data.Recs[1:] {
		assert.GreaterOrEqual(t, row.PreS12Rank, 1, "body row must carry a real pre_s12_rank")
	}
}

// ----------------------------------------------------------------------------
// ForceRecompute tests
// ----------------------------------------------------------------------------

func TestAdminRecsHandler_ForceRecompute_HappyPath(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newAdminCacheStub()

	// Seed candidate pool so the handler has something to count after recompute.
	for i := 0; i < 5; i++ {
		seedAnimeFull(t, db, "anime-"+sliceTestID(i), "released", false, 7.0)
	}

	recsRepo := repo.NewRecsRepository(db)
	// Empty precompute orchestrator — no real signal modules; just verifies
	// the synchronous RunForUser is called and latency is measured.
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, nil, precompute, logger.Default())

	r := chi.NewRouter()
	r.Post("/api/admin/recs/{user_id}/recompute", h.ForceRecompute)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/recs/user-1/recompute", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool                   `json:"success"`
		Data    ForceRecomputeResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	assert.True(t, env.Success)
	assert.NotEmpty(t, env.Data.ComputedAt)
	assert.GreaterOrEqual(t, env.Data.LatencyMs, int64(0))
	// Cache was busted (fire-and-forget log on err — happy path: no err).
	assert.Contains(t, cache.deletes, "recs:user:user-1:topN:v4")
}

func TestAdminRecsHandler_ForceRecompute_EmptyUserID(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newAdminCacheStub()

	recsRepo := repo.NewRecsRepository(db)
	precompute := recs.NewOrchestrator([]recs.SignalModule{})
	h := NewAdminRecsHandler(db, recsRepo, cache, nil, precompute, logger.Default())

	r := chi.NewRouter()
	// Mount with optional URL param to allow empty.
	r.Post("/api/admin/recs/recompute", h.ForceRecompute)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/recs/recompute", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "missing user_id param must yield 400")
}
