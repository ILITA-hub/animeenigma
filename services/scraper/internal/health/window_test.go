package health

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

// TestWindow_ThreeFailuresWithin15Min_FlipsDown — record 3 failures within
// the 15-min window; assert isDown flips to true on the third (threshold).
func TestWindow_ThreeFailuresWithin15Min_FlipsDown(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	w := &window{}

	if got := w.RecordFailure(t0); got {
		t.Fatalf("after 1 failure isDown = true; want false")
	}
	if got := w.RecordFailure(t0.Add(1 * time.Minute)); got {
		t.Fatalf("after 2 failures isDown = true; want false")
	}
	if got := w.RecordFailure(t0.Add(2 * time.Minute)); !got {
		t.Fatalf("after 3 failures isDown = false; want true")
	}
}

// TestWindow_TwoFailuresWithin15Min_StaysUp — 2 failures < threshold; isDown
// stays false.
func TestWindow_TwoFailuresWithin15Min_StaysUp(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	w := &window{}

	w.RecordFailure(t0)
	w.RecordFailure(t0.Add(5 * time.Minute))
	if w.IsDown() {
		t.Fatalf("after 2 failures isDown = true; want false")
	}
}

// TestWindow_FailuresSpreadOver16Min_DoNotTriggerThreshold — t, t+8, t+17:
// after the prune at t+17, only the t+8 and t+17 entries remain in the
// 15-min window. The t-entry is dropped, so threshold is not met.
func TestWindow_FailuresSpreadOver16Min_DoNotTriggerThreshold(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	w := &window{}

	w.RecordFailure(t0)
	w.RecordFailure(t0.Add(8 * time.Minute))
	if got := w.RecordFailure(t0.Add(17 * time.Minute)); got {
		t.Fatalf("3 failures spread over 17min should NOT trigger threshold (t-entry pruned); isDown = true; want false")
	}
	if w.IsDown() {
		t.Fatalf("IsDown = true; want false")
	}
}

// TestWindow_SuccessAfterDown_RestoresUp — flip down, then a single success
// resets the slice and flips back up.
func TestWindow_SuccessAfterDown_RestoresUp(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	w := &window{}

	w.RecordFailure(t0)
	w.RecordFailure(t0.Add(1 * time.Minute))
	w.RecordFailure(t0.Add(2 * time.Minute))
	if !w.IsDown() {
		t.Fatalf("setup failed: isDown should be true after 3 failures")
	}

	if got := w.RecordSuccess(); got {
		t.Fatalf("RecordSuccess returned true; want false (gauge = 1)")
	}
	if w.IsDown() {
		t.Fatalf("after success isDown = true; want false")
	}
	if len(w.failures) != 0 {
		t.Fatalf("after success len(failures) = %d; want 0", len(w.failures))
	}
}

// TestWindow_RaceFreeConcurrent — 50 goroutines mixing failures/successes
// under -race exits clean.
func TestWindow_RaceFreeConcurrent(t *testing.T) {
	t.Parallel()
	w := &window{}
	t0 := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), 0))
			for j := 0; j < 100; j++ {
				now := t0.Add(time.Duration(rng.IntN(60)) * time.Second)
				if rng.IntN(2) == 0 {
					w.RecordFailure(now)
				} else {
					w.RecordSuccess()
				}
			}
		}(i)
	}
	wg.Wait()
	// We don't assert specific final state — just that the run completes
	// without a -race violation.
}
