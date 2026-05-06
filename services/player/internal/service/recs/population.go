package recs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// populationCache is the minimal cache surface PopulationOrchestrator depends
// on. *libs/cache.RedisCache satisfies this interface. We define it locally so
// recs doesn't import libs/cache at build time and tests can swap a fake.
//
// (libs/cache.Cache has a wider surface than this; we narrow it to what the
// orchestrator actually uses. Go interface-from-the-consumer pattern.)
type populationCache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Exists(ctx context.Context, key string) (bool, error)
}

// LastComputedKey is the cross-process cache-buster key that records the most
// recent successful population precompute tick. Anonymous response cache
// readers consult this key to decide whether to invalidate the served payload.
const LastComputedKey = "recs:popsignal:lastcomputed"

// lastComputedTTL keeps the cache-buster timestamp around long enough that a
// stale cron tick can be detected by any reader. 24h is well past the
// 60-minute cadence so the key never naturally expires under normal operation.
const lastComputedTTL = 24 * time.Hour

// PopulationOrchestrator runs Precompute across all population-scope signal
// modules on a fixed cadence (60 minutes in production; tests use shorter
// intervals). It is intentionally a separate type from the per-user
// Orchestrator (precompute.go) — the population scope ignores userID, and the
// failure semantics are different (population failures must NOT crash the
// service; stale signals continue serving until the next successful tick).
//
// Spec ref: docs/superpowers/specs/2026-05-03-rec-engine-design.md §5.
type PopulationOrchestrator struct {
	modules []SignalModule
	cache   populationCache
	log     *logger.Logger
}

// NewPopulationOrchestrator wires the orchestrator with population-scope
// modules (S3, S4 in Phase 10), a Redis-backed cache (or any populationCache),
// and a structured logger.
func NewPopulationOrchestrator(modules []SignalModule, cache populationCache, log *logger.Logger) *PopulationOrchestrator {
	return &PopulationOrchestrator{modules: modules, cache: cache, log: log}
}

// RunOnce invokes Precompute on every registered module. Errors are joined
// (not short-circuited) so a single failing signal does not block the others.
// The cache-buster timestamp is written even on partial failure: stale signals
// continue serving until the next successful tick (per CONTEXT.md decision
// "stale signals continue serving until next successful run").
//
// Returns nil if all modules succeeded, or a joined error otherwise.
// Caller (Start, or production main) is responsible for logging the error.
func (p *PopulationOrchestrator) RunOnce(ctx context.Context) error {
	var errs []error
	for _, m := range p.modules {
		if err := m.Precompute(ctx, ""); err != nil {
			errs = append(errs, fmt.Errorf("recs: population precompute %q: %w", m.ID(), err))
		}
	}

	// Always write the cache-buster timestamp — stale-but-recent reads are
	// preferable to "no signal at all" until the next tick succeeds.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := p.cache.Set(ctx, LastComputedKey, now, lastComputedTTL); err != nil {
		errs = append(errs, fmt.Errorf("recs: write %s: %w", LastComputedKey, err))
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// Start spawns a goroutine that fires RunOnce immediately (boot tick) and then
// once every `interval` thereafter. Cancelling ctx exits the goroutine.
//
// A failing tick is logged via p.log.Errorw and the goroutine continues. This
// is the success-criterion #5 contract: cron failure does NOT crash the
// service; the next tick still runs.
func (p *PopulationOrchestrator) Start(ctx context.Context, interval time.Duration) {
	go func() {
		// Boot tick — populate signals within seconds of redeploy so the
		// trending row works on cold start without the user waiting an hour.
		if err := p.RunOnce(ctx); err != nil {
			p.log.Errorw("population precompute failed (boot tick)", "error", err)
		} else {
			p.log.Infow("population precompute boot tick complete")
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				p.log.Infow("population precompute cron stopped")
				return
			case <-ticker.C:
				if err := p.RunOnce(ctx); err != nil {
					p.log.Errorw("population precompute failed (tick)", "error", err)
					// Do NOT return / panic — continue ticking. Per
					// success criterion #5: stale signals continue serving.
					continue
				}
				p.log.Infow("population precompute tick complete")
			}
		}
	}()
}
