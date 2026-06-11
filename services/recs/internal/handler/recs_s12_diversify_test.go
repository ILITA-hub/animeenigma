// Phase 4 (S12) — handler-level tests for the S12 diversification re-rank
// wired into both rec rows (computeFresh trending + computeFreshForUser).
//
// These exercise the HTTP path with a real sqlite fixture so the
// RecsHandler's self-constructed Diversifier (GormAttrLoader over
// anime_genres / anime_studios) sees real attribute sets. The interleaving
// assertion is the canonical regression: a genre-monotone lead must be
// broken up by a diverse trailing item once S12 re-ranks.
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecsHandler_S12_InterleavesDiverseItem verifies the trending row's S12
// re-rank pulls a genre-diverse trailing candidate ahead of a near-clone of
// the leader.
//
// Fixture (all released, no aired_on → S4=0 for all; Final = 0.20·norm(S3)):
//   - clone-A: genres {g1,g2}, S3=100 (leader)
//   - clone-B: genres {g1,g2}, S3=95  (near-clone of A)
//   - diverse-C: genres {g3,g4}, S3=90 (trails on raw Final)
//
// Without S12 the order is A, B, C (S3-desc). With S12 (λ=0.3): A picked
// first. Then B's MMR score = FinalB − 0.3·Jaccard(B,{A}) = FinalB − 0.3·1.0,
// while C's = FinalC − 0.3·0 = FinalC. Normalized S3: A=1.0, B≈0.5, C=0.0
// → Final A=0.20, B≈0.10, C=0.0. B's MMR ≈ 0.10 − 0.30 = −0.20; C's = 0.0.
// C > B, so C lands at position 2 → interleaved.
func TestRecsHandler_S12_InterleavesDiverseItem(t *testing.T) {
	db := setupRecsTestDB(t)
	cache := newFakeRecsCache()

	seedAnimeFull(t, db, "clone-A", "released", false, 8.0)
	seedAnimeFull(t, db, "clone-B", "released", false, 8.0)
	seedAnimeFull(t, db, "diverse-C", "released", false, 8.0)
	seedPopulationSignal(t, db, "clone-A", 100.0)
	seedPopulationSignal(t, db, "clone-B", 50.0)
	seedPopulationSignal(t, db, "diverse-C", 1.0)
	// A and B share an identical genre set; C is disjoint.
	seedPhase12Genre(t, db, "clone-A", "g1")
	seedPhase12Genre(t, db, "clone-A", "g2")
	seedPhase12Genre(t, db, "clone-B", "g1")
	seedPhase12Genre(t, db, "clone-B", "g2")
	seedPhase12Genre(t, db, "diverse-C", "g3")
	seedPhase12Genre(t, db, "diverse-C", "g4")

	recsRepo := repo.NewRecsRepository(db)
	h := NewRecsHandler(db, recsRepo, cache, nil, logger.Default())
	require.NotNil(t, h.diversifier, "RecsHandler must wire its Diversifier")

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
	require.Len(t, env.Data.Recs, 3)

	got := []string{
		env.Data.Recs[0].Anime.ID,
		env.Data.Recs[1].Anime.ID,
		env.Data.Recs[2].Anime.ID,
	}
	// Leader unchanged; diverse-C interleaved at position 2; clone-B demoted.
	assert.Equal(t, "clone-A", got[0], "highest-Final leader stays at position 1")
	assert.Equal(t, "diverse-C", got[1], "S12 must pull the genre-diverse item to position 2")
	assert.Equal(t, "clone-B", got[2], "the near-clone of the leader is demoted")

	// Rank fields must be sequential post-rerank.
	for i, item := range env.Data.Recs {
		assert.Equal(t, i+1, item.Rank, "rank must equal post-rerank position")
	}
}

// TestRecsHandler_S12_PinSeedsSimilarity verifies that the S6 pin seeds the
// S12 MMR similarity: when a pin resolves with genre set {g1,g2}, a near-clone
// of the pin (also {g1,g2}) in the ensemble body is demoted below a genre-
// diverse candidate, even though the clone has a higher raw Final.
//
// Fixture (released; Final = 0.20·norm(S3)):
//   - pin seed → S6 local cascade pin = clone-of-pin (highest co_count).
//
// The pin (genres {g1,g2}) is prepended at rank 1 AND seeds the diversifier.
// Among the remaining ensemble body, clone-X ({g1,g2}, high S3) must land
// BELOW diverse-Y ({g3,g4}, lower S3) because clone-X's MMR score is docked
// by its similarity to the picked pin.
func TestRecsHandler_S12_PinSeedsSimilarity(t *testing.T) {
	db := setupRecsTestDB(t)
	setupS6CoOccTable(t, db)
	cache := newFakeRecsCache()

	// Candidate pool: the pin target (clone-pin) plus the body candidates.
	// clone-pin and clone-X share genres {g1,g2}; diverse-Y is disjoint.
	seedAnimeFull(t, db, "clone-pin", "released", false, 7.5)
	seedAnimeFull(t, db, "clone-X", "released", false, 7.5)
	seedAnimeFull(t, db, "diverse-Y", "released", false, 7.5)
	// 3 filler candidates so the local cascade reaches its >=5 threshold.
	for _, id := range []string{"fill-1", "fill-2", "fill-3"} {
		seedAnimeFull(t, db, id, "released", false, 7.0)
	}
	seedPopulationSignal(t, db, "clone-X", 100.0)  // highest body Final
	seedPopulationSignal(t, db, "diverse-Y", 50.0) // lower body Final

	seedPhase12Genre(t, db, "clone-pin", "g1")
	seedPhase12Genre(t, db, "clone-pin", "g2")
	seedPhase12Genre(t, db, "clone-X", "g1")
	seedPhase12Genre(t, db, "clone-X", "g2")
	seedPhase12Genre(t, db, "diverse-Y", "g3")
	seedPhase12Genre(t, db, "diverse-Y", "g4")

	// Seed S6: clone-pin is the top co-occurrence so the local cascade pins it.
	completed := time.Now().UTC().Add(-1 * time.Hour)
	coOcc := map[string]int{
		"clone-pin": 20, "clone-X": 8, "diverse-Y": 6, "fill-1": 4, "fill-2": 3, "fill-3": 2,
	}
	seedS6Fixture(t, db, "user-pin", "seed-pin", "Pin Seed", completed, 9, coOcc)

	recsRepo := repo.NewRecsRepository(db)
	s6 := signals.NewS6ComboPin(db, recsRepo, &fakeShikimoriClientForRecs{}, logger.Default())
	h := NewRecsHandler(db, recsRepo, cache, s6, logger.Default())
	r := chi.NewRouter()
	r.Get("/api/users/recs", h.GetRecs)

	req := loggedInRequest("user-pin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool         `json:"success"`
		Data    RecsEnvelope `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&env))
	require.NotEmpty(t, env.Data.Recs)

	// Pin is rank 1 and is clone-pin.
	require.True(t, env.Data.Recs[0].Pinned, "rank 1 must be the pin")
	assert.Equal(t, "clone-pin", env.Data.Recs[0].Anime.ID)

	// Locate clone-X and diverse-Y in the ensemble body (rank >= 2).
	posOf := func(id string) int {
		for i, item := range env.Data.Recs {
			if item.Anime.ID == id {
				return i
			}
		}
		return -1
	}
	xPos := posOf("clone-X")
	yPos := posOf("diverse-Y")
	require.GreaterOrEqual(t, xPos, 1, "clone-X must be in the body")
	require.GreaterOrEqual(t, yPos, 1, "diverse-Y must be in the body")
	assert.Less(t, yPos, xPos,
		"pin-seeded S12: diverse-Y must rank above clone-X (clone-X is docked for similarity to the pinned clone)")

	// Ranks must be sequential across the whole row.
	for i, item := range env.Data.Recs {
		assert.Equal(t, i+1, item.Rank, "rank must equal position post-rerank")
	}
}
