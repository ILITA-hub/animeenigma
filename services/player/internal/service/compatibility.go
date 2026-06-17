package service

import (
	"context"
	"math"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// compatRepo is the data dependency (real repo or a test fake).
type compatRepo interface {
	ListEntries(ctx context.Context, userID string) ([]domain.UserListEntry, error)
}

// CompatibilityService blends list overlap (0.5), score agreement (0.4) and
// genre similarity (0.1) into a 0..100 percent compatibility score.
type CompatibilityService struct{ repo compatRepo }

func NewCompatibilityService(r compatRepo) *CompatibilityService {
	return &CompatibilityService{repo: r}
}

// Compute blends list overlap (0.5), score agreement (0.4) and genre
// similarity (0.1) into a 0..100 percent.
func (s *CompatibilityService) Compute(ctx context.Context, viewerID, ownerID string) (*domain.CompatibilityResult, error) {
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
