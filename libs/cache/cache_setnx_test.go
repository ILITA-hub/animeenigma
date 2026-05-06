package cache

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"
)

// These tests exercise SetNX against a real Redis instance.
// libs/cache does not currently vendor miniredis, so we use the
// docker-compose redis at REDIS_HOST:REDIS_PORT (defaults to localhost:6379).
// If redis is unreachable we skip — the production verification at Task 9
// step 8b also exercises the real method.
func newTestCache(t *testing.T) *RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	cfg := Config{Host: host, Port: port, DB: 15} // DB 15 reserved for tests
	c, err := New(cfg)
	if err != nil {
		t.Skipf("redis unreachable at %s:%d (%v); skipping SetNX test", host, port, err)
	}
	// Best-effort flush of test DB so prior aborted runs don't pollute.
	_ = c.client.FlushDB(context.Background()).Err()
	t.Cleanup(func() {
		_ = c.client.FlushDB(context.Background()).Err()
		_ = c.Close()
	})
	return c
}

func TestRedisCache_SetNX_AcquiresOnMissingKey(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	acquired, err := c.SetNX(ctx, "test:setnx:key1", "owner-A", 30*time.Second)
	if err != nil {
		t.Fatalf("SetNX returned error: %v", err)
	}
	if !acquired {
		t.Fatalf("expected acquired=true on missing key, got false")
	}

	var got string
	if err := c.Get(ctx, "test:setnx:key1", &got); err != nil {
		t.Fatalf("Get after SetNX returned error: %v", err)
	}
	if got != "owner-A" {
		t.Fatalf("expected stored value %q, got %q", "owner-A", got)
	}

	// TTL must be in (0, 30s].
	ttl, err := c.client.TTL(ctx, "test:setnx:key1").Result()
	if err != nil {
		t.Fatalf("TTL returned error: %v", err)
	}
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("expected TTL in (0,30s], got %s", ttl)
	}
}

func TestRedisCache_SetNX_NoOverwriteOnExistingKey(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := "test:setnx:key2"

	// First owner takes the lock.
	ok1, err := c.SetNX(ctx, key, "owner-A", 30*time.Second)
	if err != nil || !ok1 {
		t.Fatalf("first SetNX: ok=%v err=%v", ok1, err)
	}
	originalTTL, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL probe: %v", err)
	}

	// Second caller fails to acquire; existing value must be untouched.
	ok2, err := c.SetNX(ctx, key, "owner-B", 60*time.Second)
	if err != nil {
		t.Fatalf("second SetNX returned error: %v", err)
	}
	if ok2 {
		t.Fatalf("expected acquired=false on existing key, got true")
	}

	var got string
	if err := c.Get(ctx, key, &got); err != nil {
		t.Fatalf("Get after failed SetNX: %v", err)
	}
	if got != "owner-A" {
		t.Fatalf("expected value untouched (%q), got %q", "owner-A", got)
	}

	// TTL must NOT have been extended by the failed acquire.
	newTTL, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL re-probe: %v", err)
	}
	if newTTL > originalTTL+time.Second {
		t.Fatalf("TTL was extended by failed SetNX: original=%s now=%s", originalTTL, newTTL)
	}
}

func TestRedisCache_SetNX_StructValueRoundTrip(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	want := payload{Name: "rec", Count: 42}

	acquired, err := c.SetNX(ctx, "test:setnx:struct", want, 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("SetNX(struct): ok=%v err=%v", acquired, err)
	}
	var got payload
	if err := c.Get(ctx, "test:setnx:struct", &got); err != nil {
		t.Fatalf("Get(struct): %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: want=%+v got=%+v", want, got)
	}
}

func TestRedisCache_SetNX_MarshalFailureReturnsError(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	// math.NaN can't be marshalled to JSON — used the same way to test error
	// surfaces in other libs/cache callers.
	acquired, err := c.SetNX(ctx, "test:setnx:nan", math.NaN(), 30*time.Second)
	if err == nil {
		t.Fatalf("expected marshal error, got nil")
	}
	if acquired {
		t.Fatalf("expected acquired=false on marshal error, got true")
	}
}
