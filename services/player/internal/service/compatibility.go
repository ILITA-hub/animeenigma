package service

import (
	"context"
	"errors"
	"math"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// compatRepo is the data dependency (real repo or a test fake).
type compatRepo interface {
	ListEntries(ctx context.Context, userID string) ([]domain.UserListEntry, error)
}

// CompatibilityService blends list overlap (0.5), score agreement (0.4) and
// genre similarity (0.1) into a 0..100 percent compatibility score.
type CompatibilityService struct {
	repo compatRepo

	// resultCache is optional; when set, a computed CompatibilityResult is
	// cached under an order-canonical pair key so a repeat profile view
	// doesn't reload + recompute both full watchlists (audit L606). nil in
	// unit tests (Compute falls back to a direct compute).
	resultCache cache.Cache
}

func NewCompatibilityService(r compatRepo) *CompatibilityService {
	return &CompatibilityService{repo: r}
}

// SetCache wires an optional Redis cache for compatibility results. main.go
// sets it after construction (same pattern as PreferenceService.SetCommunityCache);
// leaving it nil (tests) disables caching transparently.
func (s *CompatibilityService) SetCache(c cache.Cache) { s.resultCache = c }

// compatCacheKey builds an order-canonical Redis key for the (a, b) pair.
// Compatibility is symmetric, so (a,b) and (b,a) must hit the same entry —
// sort the two IDs before composing the key.
func compatCacheKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return "compat:" + a + ":" + b
}

// Compute blends list overlap (0.5), score agreement (0.4) and genre
// similarity (0.1) into a 0..100 percent. Results are served from (and written
// to) the optional cache under an order-canonical pair key when one is wired.
func (s *CompatibilityService) Compute(ctx context.Context, viewerID, ownerID string) (*domain.CompatibilityResult, error) {
	if s.resultCache != nil {
		key := compatCacheKey(viewerID, ownerID)
		var cached domain.CompatibilityResult
		if err := s.resultCache.Get(ctx, key, &cached); err == nil {
			metrics.CacheOperationsTotal.WithLabelValues("compat", "hit").Inc()
			return &cached, nil
		} else if !errors.Is(err, cache.ErrNotFound) {
			// On a non-miss cache error fall through to a direct compute so a
			// Redis blip never breaks the endpoint.
			metrics.CacheOperationsTotal.WithLabelValues("compat", "error").Inc()
		} else {
			metrics.CacheOperationsTotal.WithLabelValues("compat", "miss").Inc()
		}
	}

	res, err := s.compute(ctx, viewerID, ownerID)
	if err != nil {
		return nil, err
	}
	if s.resultCache != nil {
		// Best-effort write — a cache-set failure must not fail the request.
		_ = s.resultCache.Set(ctx, compatCacheKey(viewerID, ownerID), res, cache.TTLSearchResults)
	}
	return res, nil
}

// compute is the pure blend (no cache). Split out so Compute owns the cache
// lookup/store and compute owns the math.
func (s *CompatibilityService) compute(ctx context.Context, viewerID, ownerID string) (*domain.CompatibilityResult, error) {
	ve, err := s.repo.ListEntries(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	oe, err := s.repo.ListEntries(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	vm := map[string]domain.UserListEntry{}
	for _, e := range ve {
		vm[e.AnimeID] = e
	}
	om := map[string]domain.UserListEntry{}
	for _, e := range oe {
		om[e.AnimeID] = e
	}

	// overlap = Jaccard of title sets
	shared := []string{}
	for id := range vm {
		if _, ok := om[id]; ok {
			shared = append(shared, id)
		}
	}
	union := len(vm) + len(om) - len(shared)
	overlap := 0.0
	if union > 0 {
		overlap = float64(len(shared)) / float64(union)
	}

	// scoreAgreement on commonly-rated titles (both scores > 0)
	var diffSum float64
	var rated int
	for _, id := range shared {
		if vm[id].Score > 0 && om[id].Score > 0 {
			diffSum += math.Abs(float64(vm[id].Score - om[id].Score))
			rated++
		}
	}
	scoreAgreement := 0.0 // 0 when no shared titles at all
	if len(shared) > 0 {
		// neutral (1.0) when shared titles exist but none are rated by both
		scoreAgreement = 1.0
	}
	if rated > 0 {
		scoreAgreement = 1.0 - (diffSum/float64(rated))/10.0 // scores are 1..10
		if scoreAgreement < 0 {
			scoreAgreement = 0
		}
	}

	genreSim := cosineGenre(ve, oe)

	score := 0.5*overlap + 0.4*scoreAgreement + 0.1*genreSim
	sample := shared
	if len(sample) > 8 {
		sample = sample[:8]
	}
	return &domain.CompatibilityResult{
		Percent:      int(math.Round(score * 100)),
		SharedCount:  len(shared),
		SharedSample: sample,
	}, nil
}

func cosineGenre(a, b []domain.UserListEntry) float64 {
	av, bv := genreVec(a), genreVec(b)
	if len(av) == 0 || len(bv) == 0 {
		return 0
	}
	var dot, na, nb float64
	for g, c := range av {
		dot += c * bv[g]
		na += c * c
	}
	for _, c := range bv {
		nb += c * c
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func genreVec(entries []domain.UserListEntry) map[string]float64 {
	v := map[string]float64{}
	for _, e := range entries {
		for _, g := range e.GenreIDs {
			v[g]++
		}
	}
	return v
}
