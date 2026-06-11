package recs

import (
	"context"
	"fmt"
	"sort"
)

// WeightedSignal pairs a signal with its weight in the final sum.
// Weights are NOT renormalized when individual signals emit zero — total
// scores sit lower honestly. See spec §2.2.
type WeightedSignal struct {
	Module SignalModule
	Weight float64
}

// Recommendation is the per-anime ensemble output for a single user.
// Breakdown carries the per-signal NormalizedScore so the admin debug
// page (Phase 14) can render every column populated.
type Recommendation struct {
	AnimeID   AnimeID
	Final     float64
	Breakdown map[SignalID]NormalizedScore
}

// Ensemble aggregates SignalModule outputs by per-pool min-max normalization
// followed by a weighted sum. Filter (S11) and pin (S6) layers wrap the
// ensemble at the call site; this struct is concerned only with scoring.
type Ensemble struct {
	signals []WeightedSignal
}

// NewEnsemble constructs an ensemble. The order of signals does not affect
// the math but is preserved for reproducible breakdown ordering.
func NewEnsemble(signals []WeightedSignal) *Ensemble {
	return &Ensemble{signals: signals}
}

// Rank computes the weighted-sum score for each candidate and returns
// recommendations sorted by Final descending. Empty pool returns nil.
// Any signal error short-circuits and is returned wrapped.
func (e *Ensemble) Rank(ctx context.Context, userID UserID, candidates []AnimeID) ([]Recommendation, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	normalized := make(map[SignalID]map[AnimeID]NormalizedScore, len(e.signals))

	for _, ws := range e.signals {
		raw, err := ws.Module.Score(ctx, userID, candidates)
		if err != nil {
			return nil, fmt.Errorf("recs: signal %q score: %w", ws.Module.ID(), err)
		}
		normalized[ws.Module.ID()] = MinMaxNormalize(raw, candidates)
	}

	out := make([]Recommendation, 0, len(candidates))
	for _, id := range candidates {
		breakdown := make(map[SignalID]NormalizedScore, len(e.signals))
		var final float64
		for _, ws := range e.signals {
			score := normalized[ws.Module.ID()][id]
			breakdown[ws.Module.ID()] = score
			final += ws.Weight * float64(score)
		}
		out = append(out, Recommendation{
			AnimeID:   id,
			Final:     final,
			Breakdown: breakdown,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Final > out[j].Final
	})

	return out, nil
}

// RecommendationWithBreakdown is the admin-debug payload for a single anime:
// every per-signal raw score, normalized score, weighted contribution, the
// final sum, and the top contributor signal_id. Phase 14 (REC-ADMIN-01).
//
// Used by the /api/admin/recs/{user_id} endpoint to render the per-row
// breakdown table. The narrow public Rank API stays unchanged so the public
// /api/users/recs path is unaffected.
//
// TopContributor is the SignalID with the largest Weighted contribution to
// Final. Ties (including the all-zero cold-start case) are broken by the
// order signals appear in the ensemble registry — the first signal always
// wins. Documented behavior; tests assert it.
type RecommendationWithBreakdown struct {
	AnimeID        AnimeID
	Final          float64
	Raw            map[SignalID]RawScore       // pre-normalization signal output
	Breakdown      map[SignalID]NormalizedScore // per-pool normalized [0,1]
	Weighted       map[SignalID]float64         // weight × Breakdown
	TopContributor SignalID
}

// RankWithBreakdown is the admin-debug parallel to Rank. It mirrors Rank's
// per-pool min-max normalization + weighted-sum math but returns a richer
// payload per candidate (Raw / Breakdown / Weighted / TopContributor).
//
// Phase 14 (REC-ADMIN-01). Used only by the AdminRecsHandler; the public
// Rank API is unchanged. Empty pool returns nil.
func (e *Ensemble) RankWithBreakdown(ctx context.Context, userID UserID, candidates []AnimeID) ([]RecommendationWithBreakdown, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	raw := make(map[SignalID]map[AnimeID]RawScore, len(e.signals))
	normalized := make(map[SignalID]map[AnimeID]NormalizedScore, len(e.signals))
	for _, ws := range e.signals {
		r, err := ws.Module.Score(ctx, userID, candidates)
		if err != nil {
			return nil, fmt.Errorf("recs: signal %q score: %w", ws.Module.ID(), err)
		}
		raw[ws.Module.ID()] = r
		normalized[ws.Module.ID()] = MinMaxNormalize(r, candidates)
	}

	out := make([]RecommendationWithBreakdown, 0, len(candidates))
	for _, id := range candidates {
		perRaw := make(map[SignalID]RawScore, len(e.signals))
		perBreakdown := make(map[SignalID]NormalizedScore, len(e.signals))
		perWeighted := make(map[SignalID]float64, len(e.signals))
		var final float64
		var topSig SignalID
		// topVal initialized to -1 so the FIRST signal always wins ties,
		// including the all-zero cold-start case where every Weighted entry
		// is 0 (>=topVal=-1 picks the first iteration; subsequent equal
		// entries never beat the strict > comparison).
		topVal := -1.0
		for _, ws := range e.signals {
			score := normalized[ws.Module.ID()][id]
			w := ws.Weight * float64(score)
			perRaw[ws.Module.ID()] = raw[ws.Module.ID()][id]
			perBreakdown[ws.Module.ID()] = score
			perWeighted[ws.Module.ID()] = w
			final += w
			if w > topVal {
				topVal = w
				topSig = ws.Module.ID()
			}
		}
		out = append(out, RecommendationWithBreakdown{
			AnimeID:        id,
			Final:          final,
			Raw:            perRaw,
			Breakdown:      perBreakdown,
			Weighted:       perWeighted,
			TopContributor: topSig,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Final > out[j].Final
	})

	return out, nil
}
