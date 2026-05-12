// window.go — sliding-window failure counter for the liveness probe.
//
// Per RESEARCH §Pattern 2 + SCRAPER-OBS-02:
//   - A gauge flips DOWN only after `failureThreshold` failures within
//     `failureWindow`.
//   - A single success resets the window to empty and flips back UP.
//   - The counter is per-(provider, stage); each ProbeRunner owns one
//     windowSet bundling one window per canonical stage.
//
// Clock injection: callers MUST pass `now` explicitly into RecordFailure —
// the probe drives this from a test-overridable now() function so unit
// tests can advance virtual time deterministically (RESEARCH P-09).
package health

import (
	"sync"
	"time"
)

const (
	// failureThreshold is the number of failures that must accumulate within
	// failureWindow before the gauge flips to 0. Per SCRAPER-OBS-02:
	// "3 failures within 15 minutes".
	failureThreshold = 3

	// failureWindow is the rolling lookback in which failureThreshold is
	// evaluated. Entries older than this are pruned on every RecordFailure
	// call (lazy cleanup — cheaper than a background sweeper).
	failureWindow = 15 * time.Minute
)

// window is a per-(provider, stage) sliding-window failure counter.
//
// On every failure: prune entries older than failureWindow, append `now`,
// flip isDown=true if >= failureThreshold entries remain in the window.
// On every success: reset to empty + isDown=false.
//
// Caller passes `now` explicitly (do NOT call time.Now() inside) so unit
// tests can drive the threshold deterministically (RESEARCH P-09).
type window struct {
	mu       sync.Mutex
	failures []time.Time
	isDown   bool
}

// RecordFailure adds a failure timestamp; returns true if the gauge should
// be 0 (i.e. >= failureThreshold failures remain in the sliding window).
func (w *window) RecordFailure(now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := now.Add(-failureWindow)
	pruned := w.failures[:0]
	for _, t := range w.failures {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	w.failures = append(pruned, now)
	if len(w.failures) >= failureThreshold {
		w.isDown = true
	}
	return w.isDown
}

// RecordSuccess resets the window; returns false (gauge = 1).
// A single success is sufficient to clear a previously-down stage —
// the SCRAPER-OBS-02 contract is asymmetric on purpose: failures
// accumulate, successes are decisive.
func (w *window) RecordSuccess() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failures = w.failures[:0]
	w.isDown = false
	return false
}

// IsDown returns the current down/up state without mutation. Useful for
// tests that need to assert state without recording an event.
func (w *window) IsDown() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.isDown
}

// windowSet bundles one window per canonical stage. Constructed once per
// provider in the ProbeRunner; keyed by stage name.
type windowSet struct {
	byStage map[string]*window
}

func newWindowSet() *windowSet {
	out := &windowSet{byStage: make(map[string]*window, len(AllStages))}
	for _, s := range AllStages {
		out.byStage[s] = &window{}
	}
	return out
}

// Record dispatches to RecordFailure or RecordSuccess based on `ok`.
// Returns the resulting isDown state (true = gauge should be 0).
// If the stage is unknown (programmer error), returns false to fail-open.
func (s *windowSet) Record(stage string, now time.Time, ok bool) bool {
	w := s.byStage[stage]
	if w == nil {
		return false
	}
	if ok {
		return w.RecordSuccess()
	}
	return w.RecordFailure(now)
}

// IsDown returns the current down/up state for a stage without recording an
// event. Returns false (up) for unknown stages.
func (s *windowSet) IsDown(stage string) bool {
	w := s.byStage[stage]
	if w == nil {
		return false
	}
	return w.IsDown()
}
