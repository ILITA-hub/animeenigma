package cache

import (
	"context"
	"testing"
	"time"
)

// Exercises the DegradationWatcher against the real docker-compose Redis
// (same pattern as cache_setnx_test.go — skip when unreachable, DB 15).

func TestDegradationWatcher_NilSafety(t *testing.T) {
	var w *DegradationWatcher
	if w.Level() != 0 || w.ShouldShed(1) {
		t.Fatal("nil watcher must read as level 0 / never shed")
	}
	w.Start(context.Background()) // must not panic

	w2 := NewDegradationWatcher(nil, time.Second)
	w2.Start(context.Background()) // nil cache: no-op loop
	if w2.Level() != 0 {
		t.Fatal("nil-cache watcher must read as level 0")
	}
}

func TestDegradationWatcher_ReadsAndFailsOpen(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	client := c.Client()
	t.Cleanup(func() { client.Del(ctx, DegradationLevelKey) })

	w := NewDegradationWatcher(c, time.Second)

	// Missing key -> 0.
	client.Del(ctx, DegradationLevelKey)
	w.refresh(ctx)
	if got := w.Level(); got != 0 {
		t.Fatalf("missing key: want 0, got %d", got)
	}

	// Published level -> read.
	client.Set(ctx, DegradationLevelKey, "2", time.Minute)
	w.refresh(ctx)
	if got := w.Level(); got != 2 {
		t.Fatalf("want 2, got %d", got)
	}
	if !w.ShouldShed(1) || !w.ShouldShed(2) {
		t.Fatal("level 2 must shed at min 1 and 2")
	}

	// Garbage value -> fail open.
	client.Set(ctx, DegradationLevelKey, "banana", time.Minute)
	w.refresh(ctx)
	if got := w.Level(); got != 0 {
		t.Fatalf("garbage value: want 0, got %d", got)
	}

	// Out-of-range -> fail open.
	client.Set(ctx, DegradationLevelKey, "7", time.Minute)
	w.refresh(ctx)
	if got := w.Level(); got != 0 {
		t.Fatalf("out of range: want 0, got %d", got)
	}
}
