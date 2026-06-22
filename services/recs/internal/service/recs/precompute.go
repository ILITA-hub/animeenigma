package recs

import (
	"context"
	"errors"
	"fmt"
)

// Orchestrator runs Precompute across the registered signal modules.
// Phase 9 ships RunForUser only; Phase 10 adds RunPopulation and the cron +
// on-write entry points. See spec §5.
type Orchestrator struct {
	modules []SignalModule
}

// NewOrchestrator wires the orchestrator with the given modules. Order
// determines invocation order, but errors do not short-circuit — every
// module is given a chance to run, and errors are joined and returned.
func NewOrchestrator(modules []SignalModule) *Orchestrator {
	return &Orchestrator{modules: modules}
}

// RunForUser invokes Precompute on every registered module for the given
// user. Errors from individual modules are collected and returned via
// errors.Join. If no module errors, returns nil.
func (o *Orchestrator) RunForUser(ctx context.Context, userID UserID) error {
	var errs []error
	for _, m := range o.modules {
		if err := m.Precompute(ctx, userID); err != nil {
			errs = append(errs, fmt.Errorf("recs: precompute %q for user %q: %w", m.ID(), userID, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// SharedPrecomputer is an optional interface a SignalModule may implement to
// run a once-per-tick population-scope precompute and stow the result in the
// returned context. Modules that don't need cross-user shared state (the
// common case) don't implement it. S5 implements it to hoist its
// population-scope IDF out of every per-user Precompute (audit L648).
type SharedPrecomputer interface {
	// PrecomputeShared runs the once-per-tick shared step and returns a child
	// context carrying its result. On error it should return the parent ctx
	// unchanged so callers can fall back to per-user inline computation.
	PrecomputeShared(ctx context.Context) (context.Context, error)
}

// BuildSharedContext runs PrecomputeShared on every module that implements
// SharedPrecomputer, chaining the enriched context so a later per-user
// RunForUser reuses the shared state. Modules that don't implement the
// interface are skipped. Errors are joined and returned alongside the
// best-effort enriched context — a failed shared step is non-fatal because
// each module's per-user Precompute falls back to inline computation.
//
// Call this ONCE per cron sweep (before iterating users), not per user.
func (o *Orchestrator) BuildSharedContext(ctx context.Context) (context.Context, error) {
	var errs []error
	for _, m := range o.modules {
		sp, ok := m.(SharedPrecomputer)
		if !ok {
			continue
		}
		next, err := sp.PrecomputeShared(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("recs: shared precompute %q: %w", m.ID(), err))
			continue // keep the (unchanged) ctx; fall through to inline per-user
		}
		ctx = next
	}
	if len(errs) == 0 {
		return ctx, nil
	}
	return ctx, errors.Join(errs...)
}
