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
