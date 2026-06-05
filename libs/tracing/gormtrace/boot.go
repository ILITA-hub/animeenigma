package gormtrace

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"gorm.io/gorm"
)

// defaultColdStartThresholdMS is the static cold-start db_read P95 threshold (A1)
// used until the plan-05 daily refresher installs live per-(operation, table)
// thresholds. A SELECT slower than this is recorded as a db_read effect when no
// learned threshold exists for its (operation, table) yet.
const defaultColdStartThresholdMS = 50

// defaultRefreshInterval is the read_thresholds snapshot cadence. The gate is a
// coarse cost-ledger threshold, not a real-time value, so a few minutes of
// staleness is fine and keeps Redis load negligible (Pitfall 4).
const defaultRefreshInterval = 5 * time.Minute

// WireDBEffects is the one-call boot wiring for the DB-effect plane shared by
// every GORM service (plan 06). It:
//
//  1. builds a ReadGate with the static cold-start threshold (A1),
//  2. registers the db_write (always) + db_read (P95-gated) GORM after-callbacks
//     against sink (the process Producer — pass tracing.GlobalSink()),
//  3. starts a ThresholdRefresher that snapshots the read_thresholds Redis hash
//     into the gate off the query hot path (D-03, plan 05).
//
// It returns a stop func that halts the refresher (call via defer at boot). When
// sink is nil (no Producer installed) or reader is nil, the corresponding step is
// skipped and a no-op stop is returned — so a service booted without a sink is
// simply unrecorded rather than crashing.
//
// CRITICAL (D-16 / T-03-09): never call WireDBEffects in services/analytics — the
// sink's own DB reads would self-amplify. This is enforced by NOT wiring it in
// the analytics main.go (the boot wiring is the only place this constraint lives).
func WireDBEffects(ctx context.Context, db *gorm.DB, sink tracing.EffectSink, reader HashReader) (stop func(), err error) {
	noop := func() {}
	if db == nil || sink == nil {
		return noop, nil
	}

	gate := NewReadGate(defaultColdStartThresholdMS)
	if err := RegisterEffectCallbacks(db, sink, gate); err != nil {
		return noop, err
	}

	if reader == nil {
		return noop, nil
	}

	refresher := NewThresholdRefresher(reader, gate, defaultRefreshInterval)
	refresher.Start(ctx)
	return refresher.Stop, nil
}
