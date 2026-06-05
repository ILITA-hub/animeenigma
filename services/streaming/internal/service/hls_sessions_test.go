package service

import (
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// fakeSink is a capturing EffectSink for tests (no testify in this service).
type fakeSink struct {
	mu      sync.Mutex
	effects []tracing.Effect
}

func (f *fakeSink) Record(e tracing.Effect) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.effects = append(f.effects, e)
}

func (f *fakeSink) snapshot() []tracing.Effect {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tracing.Effect, len(f.effects))
	copy(out, f.effects)
	return out
}

// newTestSessions builds an aggregator with an injectable clock and idle window
// but WITHOUT starting the background reaper, so tests drive flushing manually
// via flushIdle(now) for determinism.
func newTestSessions(sink tracing.EffectSink, idle time.Duration, cap int, now func() time.Time) *HLSSessions {
	s := NewHLSSessions(sink, idle, cap)
	s.now = now
	return s
}

// TestHLSSessionAggregation: N segment observations under one sess token
// aggregate into EXACTLY ONE Effect after the idle window — requests=N,
// bytes summed, duration = lastSeen-firstSeen, Host+Provider carried.
func TestHLSSessionAggregation(t *testing.T) {
	sink := &fakeSink{}
	base := time.Unix(1_000_000, 0)
	cur := base
	s := newTestSessions(sink, 45*time.Second, 1000, func() time.Time { return cur })

	s.Mint("tok-A", "cdn.example.com", "miruro", "streaming GET /hls", "user-1")

	// 4 segment GETs, advancing the clock between each.
	for i := 0; i < 4; i++ {
		cur = base.Add(time.Duration(i) * time.Second)
		s.Observe("tok-A", "cdn.example.com", 1000, 900)
	}
	lastSeen := cur

	// Not yet idle → no flush.
	cur = lastSeen.Add(10 * time.Second)
	s.flushIdle(cur)
	if got := len(sink.snapshot()); got != 0 {
		t.Fatalf("session still active should not flush; got %d effects", got)
	}

	// Past the idle window → exactly one aggregated row.
	cur = lastSeen.Add(46 * time.Second)
	s.flushIdle(cur)

	eff := sink.snapshot()
	if len(eff) != 1 {
		t.Fatalf("expected exactly ONE aggregated effect, got %d", len(eff))
	}
	e := eff[0]
	if e.Requests != 4 {
		t.Errorf("Requests = %d, want 4 (one row per session, not per segment)", e.Requests)
	}
	if e.BytesIn != 4000 {
		t.Errorf("BytesIn = %d, want 4000 (summed)", e.BytesIn)
	}
	if e.BytesOut != 3600 {
		t.Errorf("BytesOut = %d, want 3600 (summed)", e.BytesOut)
	}
	if e.DurationMS != 3000 {
		t.Errorf("DurationMS = %d, want 3000 (lastSeen-firstSeen)", e.DurationMS)
	}
	if e.Host != "cdn.example.com" {
		t.Errorf("Host = %q, want cdn.example.com", e.Host)
	}
	if e.Provider != "miruro" {
		t.Errorf("Provider = %q, want miruro", e.Provider)
	}
	if e.EffectKind != "egress" {
		t.Errorf("EffectKind = %q, want egress", e.EffectKind)
	}
	if e.UserID != "user-1" {
		t.Errorf("UserID = %q, want user-1 (captured at Mint)", e.UserID)
	}
	if e.Operation != "streaming GET /hls" {
		t.Errorf("Operation = %q, want 'streaming GET /hls'", e.Operation)
	}
}

// TestHLSReaperEviction: idle session flushed AND deleted; active session not
// flushed; concurrent distinct tokens produce distinct rows.
func TestHLSReaperEviction(t *testing.T) {
	sink := &fakeSink{}
	base := time.Unix(2_000_000, 0)
	cur := base
	s := newTestSessions(sink, 30*time.Second, 1000, func() time.Time { return cur })

	s.Observe("idle-tok", "host-a.com", 500, 400)
	idleFirst := cur

	cur = base.Add(5 * time.Second)
	s.Observe("active-tok", "host-b.com", 700, 600)

	// Advance so idle-tok is idle but active-tok is still fresh.
	cur = idleFirst.Add(31 * time.Second) // active-tok last seen at base+5s, only 26s idle
	s.flushIdle(cur)

	eff := sink.snapshot()
	if len(eff) != 1 {
		t.Fatalf("expected only the idle session flushed, got %d", len(eff))
	}
	if eff[0].Host != "host-a.com" {
		t.Errorf("flushed wrong session host = %q, want host-a.com", eff[0].Host)
	}
	if s.len() != 1 {
		t.Errorf("idle session should be deleted; map len = %d, want 1 (active remains)", s.len())
	}

	// Now make active-tok idle → second distinct row.
	cur = cur.Add(60 * time.Second)
	s.flushIdle(cur)
	eff = sink.snapshot()
	if len(eff) != 2 {
		t.Fatalf("expected two distinct rows, got %d", len(eff))
	}
	if s.len() != 0 {
		t.Errorf("all sessions should be flushed; map len = %d, want 0", s.len())
	}
}

// TestHLSSessionMapBounded: exceeding the cap evicts the oldest (no unbounded
// growth).
func TestHLSSessionMapBounded(t *testing.T) {
	sink := &fakeSink{}
	base := time.Unix(3_000_000, 0)
	cur := base
	const cap = 3
	s := newTestSessions(sink, 45*time.Second, cap, func() time.Time { return cur })

	// Insert cap+2 distinct sessions, advancing the clock so insertion order
	// equals age order.
	for i := 0; i < cap+2; i++ {
		cur = base.Add(time.Duration(i) * time.Second)
		s.Observe(tokN(i), hostN(i), 100, 100)
	}

	if s.len() > cap {
		t.Fatalf("map exceeded cap: len = %d, cap = %d", s.len(), cap)
	}
}

// TestHLSGracefulFlush: Stop() flushes all open sessions (D-06).
func TestHLSGracefulFlush(t *testing.T) {
	sink := &fakeSink{}
	base := time.Unix(4_000_000, 0)
	cur := base
	s := newTestSessions(sink, 45*time.Second, 1000, func() time.Time { return cur })
	s.Start()

	s.Observe("g1", "g-host-1.com", 10, 20)
	s.Observe("g2", "g-host-2.com", 30, 40)

	s.Stop() // must flush both open sessions regardless of idle window

	eff := sink.snapshot()
	if len(eff) != 2 {
		t.Fatalf("Stop() should flush ALL open sessions; got %d, want 2", len(eff))
	}
	if s.len() != 0 {
		t.Errorf("Stop() should empty the map; len = %d", s.len())
	}
}

func tokN(i int) string  { return "tok-" + string(rune('a'+i)) }
func hostN(i int) string { return "host-" + string(rune('a'+i)) + ".com" }
