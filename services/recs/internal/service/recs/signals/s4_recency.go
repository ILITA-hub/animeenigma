// Package signals contains the concrete SignalModule implementations that
// plug into the Phase 9 ensemble framework. Each file owns one signal:
//
//   - s3_trending.go  — last-30-day distinct-user watch count (population)
//   - s4_recency.go   — pure metadata: ongoing OR aired_on within 90 days
//   - s11_filter.go   — candidate-pool filter (NOT a SignalModule; see file)
//
// The ensemble (services/player/internal/service/recs/ensemble.go) treats
// every SignalModule identically — adding a new signal is purely additive.
// See spec docs/superpowers/specs/2026-05-03-rec-engine-design.md §3.
package signals

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S4Recency is the recency-boost signal. It is a stateless, pure-metadata
// function over the animes table:
//
//	status = 'ongoing'                        -> 1.0
//	status = 'released' AND aired_on >= -90d  -> 0.7
//	otherwise                                  -> 0.0
//
// S4 reads from the live animes table (NOT from the precomputed
// rec_population_signals.s4_recency_score column). The orchestrator (Phase 10
// PopulationOrchestrator) writes the persisted column for offline analysis
// and admin debug; the runtime ensemble path uses Score() directly so a
// brand-new ongoing anime gets the recency boost on the very next request
// without waiting for the next 60-minute cron tick.
type S4Recency struct {
	db *gorm.DB
}

// NewS4Recency wires S4 with the player service DB handle.
func NewS4Recency(db *gorm.DB) *S4Recency {
	return &S4Recency{db: db}
}

// ID returns the stable signal identifier "s4".
func (s *S4Recency) ID() recs.SignalID { return recs.SignalID("s4") }

// Precompute is a no-op. The S4 score is a pure function of (status, aired_on)
// — no per-user state, no aggregation needed. The PopulationOrchestrator
// optionally persists S4 scores to rec_population_signals for debug, but the
// runtime path computes on demand.
func (s *S4Recency) Precompute(_ context.Context, _ recs.UserID) error {
	return nil
}

// animeMetaRow is the projection used by Score. We only SELECT what we need.
type animeMetaRow struct {
	ID      string
	Status  string
	AiredOn *time.Time
}

// Score returns raw scores for each candidate per the rules above. Candidates
// that don't appear in the animes table are omitted from the returned map
// (the normalizer treats absent entries as zero). Empty candidate slice
// returns an empty map without hitting the DB.
func (s *S4Recency) Score(ctx context.Context, _ recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}

	var rows []animeMetaRow
	err := s.db.WithContext(ctx).
		Table("animes").
		Select("id, status, aired_on").
		Where("id IN ?", candidates).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)

	for _, r := range rows {
		switch {
		case r.Status == "ongoing":
			out[r.ID] = recs.RawScore(1.0)
		case r.Status == "released" && r.AiredOn != nil && !r.AiredOn.Before(cutoff):
			out[r.ID] = recs.RawScore(0.7)
		default:
			// Explicit zero so the test that asserts presence-with-zero passes.
			// MinMaxNormalize treats present-zero and absent identically.
			out[r.ID] = recs.RawScore(0)
		}
	}

	return out, nil
}
