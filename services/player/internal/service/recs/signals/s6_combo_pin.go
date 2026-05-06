// Package signals — s6_combo_pin.go: the S6 "Because you finished X" pin
// resolver. Phase 13 (REC-SIG-06).
//
// S6 is NOT a SignalModule (Decision §B2): it doesn't fit the
// Score(ctx, userID, candidates) shape because it returns a single pinned
// anime ID, not a per-candidate score map. It runs AFTER the ensemble has
// ranked the row and is invoked by the recs handler to optionally prepend
// a Pinned RecItem at index 0.
//
// Cascade order (binding spec §3.2 + Decision §B5):
//
//  1. Read user's s6_seed_anime_id / completed_at / score from
//     rec_user_signals. If seed nil OR completed_at < now()-7d, return nil.
//  2. LOCAL co-occurrence at score>=7 (materialized table). Filter through
//     the supplied candidatePool (already S11-filtered by handler). If >=5
//     post-filter, return top-1 with Source="local".
//  3. SHIKIMORI /similar fallback. Filter through candidatePool. If >=1
//     post-filter, return top-1 with Source="shikimori_similar".
//  4. SCORE-5 local fallback (live query, NOT materialized). Filter through
//     candidatePool. If >=1 post-filter, return top-1 with Source="local".
//  5. Final no-match — return nil. NEVER fall to score>0 (spec §3.2).
//
// Pin window: 7 days from s6_seed_completed_at. Older seeds → nil.
package signals

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"gorm.io/gorm"
)

// Cascade tunables (Decision §B5).
const (
	s6PinWindow            = 7 * 24 * time.Hour
	s6LocalPoolThreshold   = 5
	s6ScoreThresholdHigh   = 7
	s6ScoreThresholdMedium = 5
	s6CandidateLookupLimit = 50
)

// PinCandidate is the result of a successful S6 cascade resolution.
type PinCandidate struct {
	AnimeID     string
	SeedAnimeID string
	SeedName    string
	Source      string // "local" | "shikimori_similar"
}

// SimilarAnimeRef is the narrow shape S6 needs from the Shikimori-similar
// catalog endpoint: just the local DB ID (so we can compare against the
// candidatePool) plus the original Shikimori ID for debugging.
type SimilarAnimeRef struct {
	ShikimoriID string
	LocalID     string
}

// shikimoriSimilarClient is the narrow interface S6 needs from the catalog
// service. Production wires HTTPShikimoriSimilarClient (defined below);
// tests inject a fake.
type shikimoriSimilarClient interface {
	GetSimilarAnimeByLocalID(ctx context.Context, seedLocalID string) ([]SimilarAnimeRef, error)
}

// S6ComboPin is the cascade resolver.
type S6ComboPin struct {
	db        *gorm.DB
	recsRepo  *repo.RecsRepository
	shikimori shikimoriSimilarClient
	log       *logger.Logger
}

func NewS6ComboPin(db *gorm.DB, recsRepo *repo.RecsRepository, shikimori shikimoriSimilarClient, log *logger.Logger) *S6ComboPin {
	return &S6ComboPin{db: db, recsRepo: recsRepo, shikimori: shikimori, log: log}
}

// Resolve runs the cascade. Returns (nil, nil) when no pin should fire —
// the handler treats that as "serve the row without a pin". Returns an
// error only on unrecoverable infra failures (e.g. the user-signals read
// fails); intermediate-step failures (Shikimori timeout, score=5 query
// hiccup) are logged and the cascade continues to the next branch.
func (s *S6ComboPin) Resolve(ctx context.Context, userID string, candidatePool []string) (*PinCandidate, error) {
	// (1) Read seed.
	sigs, err := s.recsRepo.GetUserSignals(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sigs == nil || sigs.S6SeedAnimeID == nil || sigs.S6SeedCompletedAt == nil {
		return nil, nil
	}
	if time.Since(*sigs.S6SeedCompletedAt) > s6PinWindow {
		return nil, nil
	}
	seedID := *sigs.S6SeedAnimeID

	// O(1) pool-membership set.
	poolSet := make(map[string]struct{}, len(candidatePool))
	for _, a := range candidatePool {
		poolSet[a] = struct{}{}
	}

	seedName := s.lookupSeedName(ctx, seedID) // best-effort

	// (2) LOCAL cascade at score>=7 (materialized).
	localCands, err := s.recsRepo.GetTopCoOccurrences(ctx, seedID, s6ScoreThresholdHigh, s6CandidateLookupLimit)
	if err != nil {
		s.log.Warnw("s6: GetTopCoOccurrences score=7 failed", "seed", seedID, "error", err)
	}
	filtered := filterByPool(localCands, poolSet)
	if len(filtered) >= s6LocalPoolThreshold {
		return &PinCandidate{AnimeID: filtered[0], SeedAnimeID: seedID, SeedName: seedName, Source: "local"}, nil
	}

	// (3) SHIKIMORI fallback.
	if s.shikimori != nil {
		sim, simErr := s.shikimori.GetSimilarAnimeByLocalID(ctx, seedID)
		if simErr != nil {
			s.log.Warnw("s6: shikimori /similar failed", "seed", seedID, "error", simErr)
		} else {
			simIDs := make([]string, 0, len(sim))
			for _, ref := range sim {
				if ref.LocalID != "" {
					simIDs = append(simIDs, ref.LocalID)
				}
			}
			filtered2 := filterByPool(simIDs, poolSet)
			if len(filtered2) >= 1 {
				return &PinCandidate{AnimeID: filtered2[0], SeedAnimeID: seedID, SeedName: seedName, Source: "shikimori_similar"}, nil
			}
		}
	}

	// (4) SCORE-5 fallback (live query against anime_list).
	localCands5, err := s.recsRepo.GetTopCoOccurrences(ctx, seedID, s6ScoreThresholdMedium, s6CandidateLookupLimit)
	if err != nil {
		s.log.Warnw("s6: GetTopCoOccurrences score=5 failed", "seed", seedID, "error", err)
	}
	filtered3 := filterByPool(localCands5, poolSet)
	if len(filtered3) >= 1 {
		return &PinCandidate{AnimeID: filtered3[0], SeedAnimeID: seedID, SeedName: seedName, Source: "local"}, nil
	}

	// (5) Final no-match — silent omission. NEVER fall to score>0.
	return nil, nil
}

func filterByPool(cands []string, pool map[string]struct{}) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		if _, ok := pool[c]; ok {
			out = append(out, c)
		}
	}
	return out
}

// lookupSeedName fetches animes.name for the seed. Best-effort: returns
// "" on any failure (the cascade still produces a PinCandidate; the
// frontend pin_reason just renders without the name part).
func (s *S6ComboPin) lookupSeedName(ctx context.Context, seedAnimeID string) string {
	var name string
	err := s.db.WithContext(ctx).
		Table("animes").
		Select("name").
		Where("id = ?", seedAnimeID).
		Limit(1).
		Pluck("name", &name).Error
	if err != nil {
		s.log.Warnw("s6: seed name lookup failed", "seed", seedAnimeID, "error", err)
		return ""
	}
	return name
}

// --- HTTPShikimoriSimilarClient ---
//
// Production implementation that calls the catalog service's
// /api/anime/{id}/similar endpoint (Phase 13 Task 1) over the internal
// docker DNS at "http://catalog:8081" — matches the existing convention
// in services/player/internal/handler/mal_import.go and
// shikimori_import.go which both hardcode the same internal URL.

type HTTPShikimoriSimilarClient struct {
	catalogURL string
	httpClient *http.Client
	log        *logger.Logger
}

func NewHTTPShikimoriSimilarClient(catalogURL string, log *logger.Logger) *HTTPShikimoriSimilarClient {
	return &HTTPShikimoriSimilarClient{
		catalogURL: catalogURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		log:        log,
	}
}

// catalogSimilarResponse mirrors the httputil.OK envelope shape that the
// catalog handler emits — a wrapped {success, data} object where data is
// the []domain.SimilarAnime slice.
type catalogSimilarResponse struct {
	Success bool                       `json:"success"`
	Data    []catalogSimilarDataItem   `json:"data"`
}

type catalogSimilarDataItem struct {
	ShikimoriID string `json:"shikimori_id"`
	LocalID     string `json:"local_id"`
}

func (c *HTTPShikimoriSimilarClient) GetSimilarAnimeByLocalID(ctx context.Context, seedLocalID string) ([]SimilarAnimeRef, error) {
	url := fmt.Sprintf("%s/api/anime/%s/similar", c.catalogURL, seedLocalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog /similar status %d", resp.StatusCode)
	}
	var env catalogSimilarResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	refs := make([]SimilarAnimeRef, 0, len(env.Data))
	for _, d := range env.Data {
		refs = append(refs, SimilarAnimeRef{ShikimoriID: d.ShikimoriID, LocalID: d.LocalID})
	}
	return refs, nil
}
