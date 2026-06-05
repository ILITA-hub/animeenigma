package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// fakeSink captures recorded effects for assertions. Record is non-blocking
// (matches the EffectSink contract).
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

// ctxWithOp returns a ctx carrying a coarse baggage operation for D-10 reads.
func ctxWithOp(op string) context.Context {
	return tracing.SeedBaggage(context.Background(), "api", op)
}

// TestAggregatorMissThenHitTwoRows: miss then hit on the same key-class yields
// two distinct classified cache rows after a forced flush (AR-EFFECT-02).
func TestAggregatorMissThenHitTwoRows(t *testing.T) {
	sink := &fakeSink{}
	agg := NewCacheAggregator(sink, 0, 0)
	ctx := ctxWithOp("catalog GET /api/anime/{id}")

	agg.Observe(ctx, "anime:detail", "miss")
	agg.Observe(ctx, "anime:detail", "hit")
	agg.flushAll()

	got := sink.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 cache rows, got %d: %+v", len(got), got)
	}
	for _, e := range got {
		if e.EffectKind != "cache" {
			t.Errorf("EffectKind = %q, want cache", e.EffectKind)
		}
		if e.Target != "anime:detail" {
			t.Errorf("Target = %q, want anime:detail", e.Target)
		}
		if e.TargetKind != "key_class" {
			t.Errorf("TargetKind = %q, want key_class", e.TargetKind)
		}
		if e.Requests != 1 {
			t.Errorf("Requests = %d, want 1", e.Requests)
		}
		if e.Operation == "" {
			t.Errorf("Operation must never be empty")
		}
	}
}

// TestAggregatorSums: N repeats of the same (key_class,result,op) sum into ONE
// row with Requests=N — aggregation, not one-row-per-op (D-05).
func TestAggregatorSums(t *testing.T) {
	sink := &fakeSink{}
	agg := NewCacheAggregator(sink, 0, 0)
	ctx := ctxWithOp("catalog GET /api/anime/{id}")

	const n = 50
	for i := 0; i < n; i++ {
		agg.Observe(ctx, "anime:list", "hit")
	}
	agg.flushAll()

	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 summed row, got %d: %+v", len(got), got)
	}
	if got[0].Requests != n {
		t.Fatalf("Requests = %d, want %d", got[0].Requests, n)
	}
}

// TestAggregatorCoarseOpNeverEmpty: the coarse op is read from ctx baggage at
// Observe-time (D-10); a ctx with no baggage op still yields a non-empty op
// (the origin fallback) — never empty.
func TestAggregatorCoarseOpNeverEmpty(t *testing.T) {
	sink := &fakeSink{}
	agg := NewCacheAggregator(sink, 0, 0)

	// Baggage with origin only, no operation.
	ctx := tracing.SeedBaggage(context.Background(), "scheduled_job(refresh)", "")
	agg.Observe(ctx, "anime:top", "hit")
	agg.flushAll()

	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].Operation == "" {
		t.Fatalf("Operation must fall back to a non-empty value, got empty")
	}
}

// TestAggregatorBounded: Observe beyond maxEntries evicts the oldest; the map
// never exceeds maxEntries (T-03-10 clone of evictIfFullLocked).
func TestAggregatorBounded(t *testing.T) {
	sink := &fakeSink{}
	const cap = 4
	agg := NewCacheAggregator(sink, 0, cap)
	ctx := ctxWithOp("op")

	// Force many distinct counter keys via distinct ops (key includes op).
	for i := 0; i < 100; i++ {
		c := ctxWithOp("op-" + itoa(i))
		agg.Observe(c, "anime:detail", "hit")
		if got := agg.len(); got > cap {
			t.Fatalf("map size %d exceeded cap %d", got, cap)
		}
	}
	_ = ctx
}

// TestAggregatorStopFlushes: Stop() flushes all outstanding counters before
// returning (graceful drain) and is safe to call once.
func TestAggregatorStopFlushes(t *testing.T) {
	sink := &fakeSink{}
	agg := NewCacheAggregator(sink, 10*time.Millisecond, 0)
	ctx := ctxWithOp("op")

	agg.Observe(ctx, "search", "miss")
	agg.Start()
	agg.Stop()

	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("Stop() must flush outstanding counters, got %d rows", len(got))
	}
	// Second Stop must not panic.
	agg.Stop()
}

// TestAggregatorNilSinkNoOp: a nil sink makes Observe a no-op (mirrors the
// HLSSessions sink==nil guard) so cache-less paths need no aggregator.
func TestAggregatorNilSinkNoOp(t *testing.T) {
	agg := NewCacheAggregator(nil, 0, 0)
	agg.Observe(ctxWithOp("op"), "anime:detail", "hit")
	agg.flushAll()
	// No panic, no sink to assert — reaching here is the assertion.
	agg.Stop()
}
