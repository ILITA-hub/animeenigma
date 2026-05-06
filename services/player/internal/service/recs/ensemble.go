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
