package spotlight

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Aggregator dispatches per-card resolvers concurrently and assembles
// the spotlight response. Phase 1 / Plan 01-01 ships only the skeleton —
// the concurrent fan-out, per-card 800ms deadline, overall 2s budget,
// and snapshot fallback all land in Plan 01-03. Plan 01-02 wires the
// 4 real resolvers; until then `resolvers` is empty and Resolve returns
// an empty Response.
type Aggregator struct {
	cache     cache.Cache
	log       *logger.Logger
	resolvers []Resolver
}

// NewAggregator constructs an Aggregator. resolvers may be nil — the
// constructor normalises it to an empty (non-nil) slice so callers can
// range over Resolvers() without an NPE guard.
func NewAggregator(c cache.Cache, log *logger.Logger, resolvers []Resolver) *Aggregator {
	if resolvers == nil {
		resolvers = []Resolver{}
	}
	return &Aggregator{cache: c, log: log, resolvers: resolvers}
}

// Resolvers returns the configured resolver slice. Exported for tests
// only — the field itself stays unexported per Go convention.
func (a *Aggregator) Resolvers() []Resolver {
	return a.resolvers
}

// Resolve assembles the spotlight Response for the given user (nil =
// anonymous). Plan 01-01 ships this stub returning an empty Response
// with a populated GeneratedAt timestamp.
//
// STUB: Plan 03 replaces this body with the concurrent fan-out
// (per-card 800ms ctx.WithTimeout, overall 2s budget, drop-on-error,
// snapshot fallback via SnapshotKey). HSB-BE-03, HSB-BE-04, HSB-BE-05.
func (a *Aggregator) Resolve(ctx context.Context, userID *string) (*Response, error) {
	return &Response{
		Cards:       []Card{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
