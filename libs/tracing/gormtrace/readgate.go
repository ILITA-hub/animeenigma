package gormtrace

import "sync/atomic"

// ReadGate decides whether a completed SELECT is slow enough to be fact-rowed as
// a db_read effect (D-02). The trivial-read long tail is never recorded — only a
// read that exceeds its own per-(operation, table) P95 produces a row, keeping
// the cost ledger sparse (D-04).
//
// ShouldRecord runs on the GORM Query after-callback hot path, so the
// implementation MUST be a pure in-memory lookup — never a synchronous Redis or
// HTTP round-trip per query (Pitfall 4 / T-03-07). The daily-P95 producer that
// REFRESHES the snapshot ships in plan 05 and feeds SetSnapshot off-path.
type ReadGate interface {
	// ShouldRecord reports whether a SELECT of operation against table that took
	// durationMS milliseconds should be recorded as a db_read effect.
	ShouldRecord(operation, table string, durationMS int) bool
}

// snapshotGate is the default in-memory ReadGate. It holds an immutable
// threshold map (keyed "operation|table" -> p95_ms) behind an atomic.Value so
// reads are lock-free and SetSnapshot swaps the whole map atomically. A
// (operation, table) with no entry falls back to the static cold-start default.
type snapshotGate struct {
	// snap holds the current map[string]float64 threshold snapshot. Read
	// lock-free via Load; replaced wholesale via Store (SetSnapshot). The stored
	// map is treated as immutable after Store — callers must not mutate it.
	snap atomic.Value // map[string]float64

	// staticDefaultMS is the cold-start threshold used when a (op,table) key is
	// absent from the snapshot (A1 — flagged assumption, defaults to 50ms;
	// exposed via the constructor so it can be tuned without redeploying the
	// gate logic).
	staticDefaultMS int
}

// NewReadGate returns a snapshotGate whose cold-start threshold (used for any
// (operation, table) missing from the snapshot) is staticDefaultMS milliseconds.
// The snapshot starts empty; the plan-05 refresher ticker calls SetSnapshot to
// install live per-(operation, table) P95 thresholds.
func NewReadGate(staticDefaultMS int) *snapshotGate {
	g := &snapshotGate{staticDefaultMS: staticDefaultMS}
	g.snap.Store(map[string]float64{})
	return g
}

// SetSnapshot atomically replaces the threshold snapshot. It is safe to call
// concurrently with ShouldRecord; in-flight reads observe either the old or the
// new map, never a torn one. Called by the plan-05 daily-P95 refresher ticker —
// the ONLY consumer of the (Redis/analytics) refresh source, which is read
// off-path, never by the hot-path callback.
func (g *snapshotGate) SetSnapshot(m map[string]float64) {
	if m == nil {
		m = map[string]float64{}
	}
	g.snap.Store(m)
}

// ShouldRecord performs a pure in-memory lookup: if the "operation|table" key
// has a P95 threshold, record when durationMS exceeds it; otherwise record when
// durationMS exceeds the static cold-start default. No synchronous IO (Pitfall
// 4).
func (g *snapshotGate) ShouldRecord(operation, table string, durationMS int) bool {
	m, _ := g.snap.Load().(map[string]float64)
	if m != nil {
		if p95, ok := m[operation+"|"+table]; ok {
			return float64(durationMS) > p95
		}
	}
	return durationMS > g.staticDefaultMS
}

// compile-time guarantee snapshotGate satisfies ReadGate.
var _ ReadGate = (*snapshotGate)(nil)
