package recs

import "context"

// SignalModule is the pluggable contract every ranking signal implements.
// Concrete implementations live in services/player/internal/service/recs/signals/.
// See spec §6.1.
//
// Contract:
//   - Score MUST never return NaN, Inf, or negative values.
//   - Score MAY omit candidates with no contribution; the normalizer treats
//     missing entries as zero. Emptiness is normal during cold-start, not an error.
//   - Precompute is allowed to be a no-op for stateless signals (e.g. trending,
//     recency) — the orchestrator calls it unconditionally.
type SignalModule interface {
	// ID returns the stable identifier (e.g. SignalID("s1")). Used for
	// logging, admin debug page columns, and weight registry keys.
	ID() SignalID

	// Precompute runs the heavy per-user step. Called from cron jobs and
	// the on-write debouncer in later phases. May be a no-op.
	Precompute(ctx context.Context, userID UserID) error

	// Score returns raw scores for each candidate in pool. Candidates with
	// no signal contribution may be omitted from the returned map; the
	// normalizer treats missing entries as zero. Signals that don't depend
	// on user state (e.g. S3 trending, S4 recency) ignore userID.
	Score(ctx context.Context, userID UserID, candidates []AnimeID) (map[AnimeID]RawScore, error)
}
