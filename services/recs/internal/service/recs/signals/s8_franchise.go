package signals

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S8Franchise is the franchise/sequel-proximity signal (spec 2026-07-17):
// "a new entry in a franchise you scored highly". Seeds are every anime in
// the user's anime_list with score > 5 and a non-empty animes.franchise;
// affinity per franchise is the user's BEST score across its entries. A
// candidate in franchise F scores clamp((best-5)/5, 0, 1) — so a 9/10
// franchise yields 0.8 and a 10/10 yields 1.0.
//
// Positive-only by design: low/dropped franchises contribute 0 here —
// negative pressure is S7's job (no double-penalty). Stateless request-time
// signal, mirrors S2/S7's pattern.
//
// Score nullability: `al.score > 5` in SQL naturally excludes NULL scores
// (NULL comparisons are never TRUE) — unscored list rows are not affinity
// evidence.
type S8Franchise struct {
	db *gorm.DB
}

const (
	// s8NeutralScore is the score at/below which a franchise entry carries
	// no positive affinity (5/10 = neutral).
	s8NeutralScore = 5.0
	// s8ScoreSpan maps (score - neutral) onto [0, 1]: span of 5 means a
	// 10/10 hits exactly 1.0.
	s8ScoreSpan = 5.0
)

// NewS8Franchise wires S8 with a DB handle.
func NewS8Franchise(db *gorm.DB) *S8Franchise {
	return &S8Franchise{db: db}
}

// ID returns the stable signal identifier "s8".
func (s *S8Franchise) ID() recs.SignalID { return recs.SignalID("s8") }

// Precompute is a no-op — S8 is request-time only, like S2/S7.
func (s *S8Franchise) Precompute(_ context.Context, _ recs.UserID) error { return nil }

// Score returns clamp((best_franchise_score-5)/5, 0, 1) for each candidate
// whose franchise the user has scored > 5. Candidates without a franchise,
// with an unknown franchise, or for anonymous callers are omitted (the
// normalizer treats absent entries as zero).
func (s *S8Franchise) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 || userID == "" {
		return out, nil
	}

	// 1. Franchise affinity: best user score per franchise.
	type affRow struct {
		Franchise string
		Best      float64
	}
	var affRows []affRow
	if err := s.db.WithContext(ctx).
		Table("anime_list AS al").
		Select("a.franchise AS franchise, MAX(al.score) AS best").
		Joins("JOIN animes a ON a.id = al.anime_id").
		Where("al.user_id = ? AND al.score > ? AND a.franchise <> ''", userID, s8NeutralScore).
		Group("a.franchise").
		Scan(&affRows).Error; err != nil {
		return nil, fmt.Errorf("s8: load franchise affinity: %w", err)
	}
	if len(affRows) == 0 {
		return out, nil
	}
	affinity := make(map[string]float64, len(affRows))
	for _, r := range affRows {
		affinity[r.Franchise] = r.Best
	}

	// 2. Candidate → franchise map (only rows with a franchise).
	type candRow struct {
		ID        string
		Franchise string
	}
	var candRows []candRow
	if err := s.db.WithContext(ctx).
		Table("animes").
		Select("id, franchise").
		Where("id IN ? AND franchise <> ''", candidates).
		Scan(&candRows).Error; err != nil {
		return nil, fmt.Errorf("s8: load candidate franchises: %w", err)
	}

	// 3. clamp((best-5)/5, 0, 1); omit zero contributions.
	for _, c := range candRows {
		best, ok := affinity[c.Franchise]
		if !ok {
			continue
		}
		v := (best - s8NeutralScore) / s8ScoreSpan
		if v <= 0 {
			continue
		}
		if v > 1 {
			v = 1
		}
		out[c.ID] = recs.RawScore(v)
	}
	return out, nil
}
