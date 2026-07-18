package signals

import (
	"context"
	"fmt"
	"math"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S9MalPopularity is the relative MAL-popularity signal (spec 2026-07-18) used
// by the announces path. The raw score is log1p(animes.mal_members) — the
// anticipation proxy Jikan reports for a title (for an unaired announcement this
// is dominated by plan-to-watch adds). It is log-scaled because member counts
// are heavy-tailed (a hyped sequel has millions, a niche original thousands);
// without the log a single mega-title would compress every other candidate to
// ~0 under the ensemble's per-pool min-max normalization.
//
// "Relative" is intentional and free: the signal returns an ABSOLUTE per-title
// value and the ensemble normalizer rescales it within the current candidate
// pool, so the score expresses popularity RELATIVE to the other titles being
// ranked (e.g. the current announced set), not against all of MAL.
//
// Ranking-only by contract: the announces handler never gates on S9 — a niche
// sequel the user would love must not be dropped for low popularity. Candidates
// with mal_members == 0 (never enriched) are omitted; the normalizer treats
// absent entries as zero. Stateless request-time signal, mirrors S2/S8.
type S9MalPopularity struct {
	db *gorm.DB
}

// NewS9MalPopularity wires S9 with a DB handle.
func NewS9MalPopularity(db *gorm.DB) *S9MalPopularity {
	return &S9MalPopularity{db: db}
}

// ID returns the stable signal identifier "s9".
func (s *S9MalPopularity) ID() recs.SignalID { return recs.SignalID("s9") }

// Precompute is a no-op — S9 is request-time only, like S2/S8.
func (s *S9MalPopularity) Precompute(_ context.Context, _ recs.UserID) error { return nil }

// Score returns log1p(mal_members) for each candidate with mal_members > 0.
// Popularity is user-independent, so userID is unused (the announces handler
// still calls it per-user through the ensemble). Candidates with no members are
// omitted.
func (s *S9MalPopularity) Score(ctx context.Context, _ recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}

	type row struct {
		ID         string
		MalMembers int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).
		Table("animes").
		Select("id, mal_members").
		Where("id IN ? AND mal_members > 0", candidates).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("s9: load mal_members: %w", err)
	}

	for _, r := range rows {
		out[recs.AnimeID(r.ID)] = recs.RawScore(math.Log1p(float64(r.MalMembers)))
	}
	return out, nil
}
