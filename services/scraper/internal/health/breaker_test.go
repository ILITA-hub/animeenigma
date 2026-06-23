package health

import (
	"testing"
	"time"
)

// fakeClock is a deterministic time source for breaker tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestBreaker() (*Breaker, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	cache := NewInMemoryHealthCacheWithNow(clk.now)
	b := NewBreakerWithNow(cache, clk.now)
	return b, clk
}

// TestBreaker_TripsAtThreeWithin60s: two wedged errors do NOT trip; the third
// within 60s forces the provider DOWN in the cache.
func TestBreaker_TripsAtThreeWithin60s(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "nineanime"

	b.Record(p, true)
	clk.advance(10 * time.Second)
	b.Record(p, true)
	if !b.cache.IsHealthy(p) {
		t.Fatalf("after 2 wedged errors, IsHealthy = false; want true (below threshold)")
	}
	clk.advance(10 * time.Second)
	b.Record(p, true) // 3rd within 60s -> trip
	if b.cache.IsHealthy(p) {
		t.Errorf("after 3 wedged errors in 60s, IsHealthy = true; want false (tripped)")
	}
}

// TestBreaker_WindowSlides: errors spread > 60s apart never reach 3-in-window.
func TestBreaker_WindowSlides(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "gogoanime"
	b.Record(p, true)
	clk.advance(40 * time.Second)
	b.Record(p, true)
	clk.advance(40 * time.Second) // first error now 80s old -> pruned
	b.Record(p, true)             // only 2 within the trailing 60s
	if !b.cache.IsHealthy(p) {
		t.Errorf("spread-out errors tripped the breaker; want still healthy (window slid)")
	}
}

// TestBreaker_ClearsOnSuccess: a success after a trip writes the provider UP.
func TestBreaker_ClearsOnSuccess(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "nineanime"
	for i := 0; i < 3; i++ {
		b.Record(p, true)
		clk.advance(5 * time.Second)
	}
	if b.cache.IsHealthy(p) {
		t.Fatalf("precondition: breaker should be tripped")
	}
	b.Record(p, false) // success
	if !b.cache.IsHealthy(p) {
		t.Errorf("after success, IsHealthy = false; want true (breaker cleared)")
	}
}

// TestBreaker_HalfOpenAfter120s: 120s after the trip the breaker half-opens
// (provider rejoins for a trial). If still wedged, 3 more within 60s re-trip.
func TestBreaker_HalfOpenAfter120s(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "nineanime"
	for i := 0; i < 3; i++ {
		b.Record(p, true)
		clk.advance(1 * time.Second)
	}
	if b.cache.IsHealthy(p) {
		t.Fatalf("precondition: tripped")
	}
	// Within the 120s closed window, additional wedged errors keep it DOWN.
	clk.advance(30 * time.Second)
	b.Record(p, true)
	if b.cache.IsHealthy(p) {
		t.Errorf("within 120s, IsHealthy = true; want false (still tripped)")
	}
	// Cross the 120s half-open boundary: the next wedged error is the trial that
	// re-opens the window; the provider is briefly healthy again.
	clk.advance(100 * time.Second) // now ~131s past trip
	b.Record(p, true)              // half-open trial -> resets window, writes UP, counts 1
	if !b.cache.IsHealthy(p) {
		t.Errorf("after 120s half-open, IsHealthy = false; want true (trial allowed through)")
	}
	// Two more wedged within 60s re-trip.
	clk.advance(5 * time.Second)
	b.Record(p, true)
	clk.advance(5 * time.Second)
	b.Record(p, true)
	if b.cache.IsHealthy(p) {
		t.Errorf("after re-tripping post-half-open, IsHealthy = true; want false")
	}
}

// TestBreaker_PerProviderIsolation: tripping nineanime does NOT down gogoanime.
func TestBreaker_PerProviderIsolation(t *testing.T) {
	b, clk := newTestBreaker()
	for i := 0; i < 3; i++ {
		b.Record("nineanime", true)
		clk.advance(1 * time.Second)
	}
	if b.cache.IsHealthy("nineanime") {
		t.Fatalf("nineanime should be tripped")
	}
	if !b.cache.IsHealthy("gogoanime") {
		t.Errorf("gogoanime IsHealthy = false; want true (per-provider isolation)")
	}
}
