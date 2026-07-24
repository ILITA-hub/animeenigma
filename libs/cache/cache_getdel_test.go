package cache

import (
	"context"
	"testing"
	"time"
)

// These tests exercise GetDel against a real Redis instance (same
// unreachable-skip pattern as cache_setnx_test.go's newTestCache).

func TestRedisCache_GetDel_ReturnsValueAndDeletesKey(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := "test:getdel:key1"

	if err := c.Set(ctx, key, "payload-A", 30*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got string
	if err := c.GetDel(ctx, key, &got); err != nil {
		t.Fatalf("GetDel returned error: %v", err)
	}
	if got != "payload-A" {
		t.Fatalf("expected value %q, got %q", "payload-A", got)
	}

	exists, err := c.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatalf("expected key to be deleted after GetDel, but it still exists")
	}
}

func TestRedisCache_GetDel_MissingKeyReturnsErrNotFound(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	var got string
	err := c.GetDel(ctx, "test:getdel:missing", &got)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRedisCache_GetDel_ConcurrentCallsOnlyOneSucceeds(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := "test:getdel:race"

	if err := c.Set(ctx, key, "one-time", 30*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	const workers = 20
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			var got string
			results <- c.GetDel(ctx, key, &got)
		}()
	}

	successes := 0
	for i := 0; i < workers; i++ {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful GetDel under concurrency, got %d", successes)
	}
}

func TestRedisCache_GetDel_StructValueRoundTrip(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	want := payload{Name: "rec", Count: 42}
	key := "test:getdel:struct"

	if err := c.Set(ctx, key, want, 30*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got payload
	if err := c.GetDel(ctx, key, &got); err != nil {
		t.Fatalf("GetDel(struct): %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: want=%+v got=%+v", want, got)
	}
}
