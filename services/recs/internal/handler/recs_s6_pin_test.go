// Phase 13 — handler-level tests for the S6 combo-watched-after pin
// integration in the personalized branch (computeFreshForUser). These tests
// extend the Phase 11/12 setupRecsTestDB fixture with rec_completion_co_occurrence
// rows and rec_user_signals.s6_seed_* values to drive the S6 cascade end to end.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeShikimoriClientForRecs is a no-op Shikimori similar client for tests
// that don't exercise the Shikimori fallback branch. Tests that DO exercise
// it construct their own variant with a programmable response.
type fakeShikimoriClientForRecs struct {
	response []signals.SimilarAnimeRef
}

func (f *fakeShikimoriClientForRecs) GetSimilarAnimeByLocalID(_ context.Context, _ string) ([]signals.SimilarAnimeRef, error) {
	return f.response, nil
}

func setupS6CoOccTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`CREATE TABLE rec_completion_co_occurrence (
		seed_anime_id TEXT NOT NULL,
		candidate_anime_id TEXT NOT NULL,
		co_count INTEGER NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL,
		PRIMARY KEY (seed_anime_id, candidate_anime_id)
	)`).Error)
}

// seedS6Fixture seeds rec_user_signals.s6_seed_* for a user and the optional
// rec_completion_co_occurrence rows. coOccurrences keys are candidate IDs;
// values are co_count.
func seedS6Fixture(t *testing.T, db *gorm.DB, userID, seedAnimeID, seedName string, completedAt time.Time, score int, coOccurrences map[string]int) {
	t.Helper()
	// Upsert rec_user_signals row.
	require.NoError(t, db.Exec(`INSERT INTO rec_user_signals (user_id, s6_seed_anime_id, s6_seed_completed_at, s6_seed_score, last_computed)
		VALUES (?, ?, ?, ?, ?)`,
		userID, seedAnimeID, completedAt, score, time.Now().UTC()).Error)
	// Seed the seed-anime's name into animes (test fixture treats the seed
	// as a "completed" anime — already in the user's list, so it's
	// excluded from the personalized pool by S11; we still need its row
	// so SeedName lookup works).
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, name_ru, name_jp, poster_url, score, episodes_count, status, year, hidden, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		seedAnimeID, seedName, "RU "+seedName, "JP "+seedName, "/p/seed.jpg", 9.0, 12, "released", 2024, 0,
	).Error)
	// User has the seed marked as completed (so it's filtered out of the
	// candidate pool — pin still fires regardless).
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, ?, ?)`,
		"al-seed-"+userID, userID, seedAnimeID, "completed", score).Error)

	for cand, count := range coOccurrences {
		require.NoError(t, db.Exec(`INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed)
			VALUES (?, ?, ?, ?)`,
			seedAnimeID, cand, count, time.Now().UTC()).Error)
	}
}

func TestRecsHandler_PersonalizedBranchS6_PrependsPinWhenSeedQualifies(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	// Seed candidate pool: 6 animes none completed by the user.
	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	// User completed a "Frieren" seed with score=9 within last day.
	completed := time.Now().UTC().Add(-1 * time.Hour)
	coOcc := map[string]int{
		"cand-a": 10, "cand-b": 8, "cand-c": 6, "cand-d": 4, "cand-e": 3, "cand-f": 2,
	}
	seedS6Fixture(t, db, "user-1", "seed-1", "Frieren", completed, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
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

	require.NotEmpty(t, env.Data.Recs, "personalized row must be non-empty")
	first := env.Data.Recs[0]
	assert.True(t, first.Pinned, "recs[0] must be pinned when seed qualifies")
	assert.Equal(t, "seed-1", first.PinSeedAnimeID)
	assert.Equal(t, "local", first.PinSource)
	assert.True(t, strings.HasPrefix(first.PinReason, "Because you finished "), "pin_reason starts with 'Because you finished '; got %q", first.PinReason)
	assert.Contains(t, first.PinReason, "Frieren", "pin_reason must contain seed name")
	assert.Equal(t, 1, first.Rank, "pin's rank must be 1")
	// "cand-a" (highest co_count) is the expected pin — but cand-a is in
	// the pool, so it's the top filtered local result.
	assert.Equal(t, "cand-a", first.Anime.ID)
}

func TestRecsHandler_PersonalizedBranchS6_NoPinWhenSeedAbsent(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	seedAnimeFull(t, db, "anime-x", "released", false, 7.5)
	seedAnimeFull(t, db, "anime-y", "released", false, 7.0)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-no-seed")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))

	if len(env.Data.Recs) > 0 {
		assert.False(t, env.Data.Recs[0].Pinned, "no seed → recs[0].pinned must be false")
		assert.Empty(t, env.Data.Recs[0].PinReason)
	}
}

func TestRecsHandler_PersonalizedBranchS6_NoPinWhenSeedStale(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "stale-cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	stale := time.Now().UTC().Add(-8 * 24 * time.Hour)
	coOcc := map[string]int{"stale-cand-a": 10, "stale-cand-b": 8, "stale-cand-c": 6, "stale-cand-d": 4, "stale-cand-e": 3, "stale-cand-f": 2}
	seedS6Fixture(t, db, "user-stale", "seed-stale", "Some Anime", stale, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-stale")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.NotEmpty(t, env.Data.Recs)
	assert.False(t, env.Data.Recs[0].Pinned, "stale seed (>7d) → no pin; row falls back to ensemble")
}

func TestRecsHandler_PersonalizedBranchS6_PinReasonContainsSeedName(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "name-cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	completed := time.Now().UTC().Add(-2 * time.Hour)
	coOcc := map[string]int{"name-cand-a": 10, "name-cand-b": 8, "name-cand-c": 6, "name-cand-d": 4, "name-cand-e": 3, "name-cand-f": 2}
	seedS6Fixture(t, db, "user-name", "seed-name-test", "Vinland Saga", completed, 8, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-name")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.True(t, env.Data.Recs[0].Pinned)
	assert.Equal(t, "Because you finished Vinland Saga", env.Data.Recs[0].PinReason)
}

func TestRecsHandler_PersonalizedBranchS6_PinSurvivesCacheRoundtrip(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "rt-cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	completed := time.Now().UTC().Add(-1 * time.Hour)
	coOcc := map[string]int{"rt-cand-a": 10, "rt-cand-b": 8, "rt-cand-c": 6, "rt-cand-d": 4, "rt-cand-e": 3, "rt-cand-f": 2}
	seedS6Fixture(t, db, "user-rt", "seed-rt", "Hunter x Hunter", completed, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	// First call computes fresh + caches.
	req := loggedInRequest("user-rt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env1 struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env1))
	require.True(t, env1.Data.Recs[0].Pinned)
	assert.False(t, env1.Data.CacheHit, "first call is fresh compute")
	pinReason1 := env1.Data.Recs[0].PinReason
	pinSeed1 := env1.Data.Recs[0].PinSeedAnimeID

	// Second call hits cache.
	req2 := loggedInRequest("user-rt")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var env2 struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&env2))
	assert.True(t, env2.Data.CacheHit, "second call is cache hit")
	require.True(t, env2.Data.Recs[0].Pinned, "cached payload must preserve Pinned=true")
	assert.Equal(t, pinReason1, env2.Data.Recs[0].PinReason, "pin_reason must round-trip through cache")
	assert.Equal(t, pinSeed1, env2.Data.Recs[0].PinSeedAnimeID, "pin_seed_anime_id must round-trip through cache")
}

func TestRecsHandler_PersonalizedBranchS6_PinExcludesPinnedAnimeFromRest(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	for i := 1; i <= 6; i++ {
		seedAnimeFull(t, db, "dup-cand-"+string(rune('a'+i-1)), "released", false, 7.5)
	}
	completed := time.Now().UTC().Add(-1 * time.Hour)
	coOcc := map[string]int{"dup-cand-a": 10, "dup-cand-b": 8, "dup-cand-c": 6, "dup-cand-d": 4, "dup-cand-e": 3, "dup-cand-f": 2}
	seedS6Fixture(t, db, "user-dup", "seed-dup", "Some Show", completed, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-dup")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.True(t, env.Data.Recs[0].Pinned)
	pinID := env.Data.Recs[0].Anime.ID

	// dup-cand-a is the pin AND is also a normal candidate for the
	// ensemble. It must NOT appear twice in the rec list.
	for i, item := range env.Data.Recs[1:] {
		assert.NotEqual(t, pinID, item.Anime.ID, "pin's anime ID must NOT also appear as a non-pinned rec at index %d", i+1)
	}
}
