package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Login brute-force throttle: after loginMaxFails failures within the window a
// username is locked; a successful login clears the counter. Keyed
// case-insensitively. (audit medium #6)
func TestLoginThrottle(t *testing.T) {
	c := newFakeCache()
	s := NewAuthService(nil, nil, c, authz.JWTConfig{}, "", time.Hour, logger.Default())
	s.loginMaxFails = 3
	s.loginFailWindow = time.Minute
	ctx := context.Background()
	const u = "victim"

	if s.loginLocked(ctx, u) {
		t.Fatal("should not be locked initially")
	}
	s.recordLoginFailure(ctx, u)
	s.recordLoginFailure(ctx, u)
	if s.loginLocked(ctx, u) {
		t.Fatal("should not be locked at 2/3 failures")
	}
	s.recordLoginFailure(ctx, u) // 3rd failure -> at threshold
	if !s.loginLocked(ctx, u) {
		t.Fatal("should be locked at 3/3 failures")
	}
	if !s.loginLocked(ctx, "VICTIM") {
		t.Fatal("lockout key must be case-insensitive")
	}

	s.clearLoginFailures(ctx, u)
	if s.loginLocked(ctx, u) {
		t.Fatal("a successful login (clear) must reset the lockout")
	}
}

// A zero/negative threshold disables the throttle (never locks).
func TestLoginThrottle_Disabled(t *testing.T) {
	c := newFakeCache()
	s := NewAuthService(nil, nil, c, authz.JWTConfig{}, "", time.Hour, logger.Default())
	s.loginMaxFails = 0
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		s.recordLoginFailure(ctx, "u")
	}
	if s.loginLocked(ctx, "u") {
		t.Fatal("threshold 0 must disable the lockout")
	}
}

// The throttle key is a fixed-width sha256-hex digest of the normalized
// username, so no caller-controlled length ever reaches Redis (audit F13).
func TestLoginFailKey_FixedWidth(t *testing.T) {
	const prefix = "auth:loginfail:"
	short := loginFailKey("a")
	long := loginFailKey(strings.Repeat("x", 100_000))

	if !strings.HasPrefix(short, prefix) || !strings.HasPrefix(long, prefix) {
		t.Fatalf("key must keep the %q prefix", prefix)
	}
	if len(short) != len(long) {
		t.Fatalf("key width must not depend on username length: %d vs %d", len(short), len(long))
	}
	if got := len(short) - len(prefix); got != 64 { // sha256 hex = 64 chars
		t.Fatalf("digest width = %d, want 64 (sha256 hex)", got)
	}
	// Case-insensitive + whitespace-trimmed, matching the pre-hash behaviour.
	if loginFailKey("Victim") != loginFailKey("  victim ") {
		t.Fatal("key must be normalized (lower + trim)")
	}
}

// --- real-Redis tests (skipped when Redis is unreachable) --------------------

// newRealCache returns a *cache.RedisCache against the test DB (15), or skips
// the test if Redis is unreachable — mirroring libs/cache's SetNX tests. Tests
// use unique keys (uniqueSuffix) so they don't need to FlushDB and never
// collide with other suites sharing DB 15.
func newRealCache(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	c, err := cache.New(cache.Config{Host: host, Port: port, DB: 15})
	if err != nil {
		t.Skipf("redis unreachable at %s:%d (%v); skipping", host, port, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func uniqueSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// Pins the F17 invariant: after any recordLoginFailure that touches Redis, the
// throttle key has a POSITIVE TTL — a lockout can never become permanent from a
// missed EXPIRE (the atomic INCR+PEXPIRE guarantees the key never exists
// without a TTL).
func TestLoginThrottle_RedisKeyAlwaysHasTTL(t *testing.T) {
	c := newRealCache(t)
	s := NewAuthService(nil, nil, c, authz.JWTConfig{}, "", time.Hour, logger.Default())
	s.loginMaxFails = 3
	s.loginFailWindow = time.Minute
	ctx := context.Background()
	u := "ttl-victim-" + uniqueSuffix(t)
	key := loginFailKey(u)
	t.Cleanup(func() { _ = c.Delete(ctx, key) })

	// Record several failures; the TTL must be present (and positive) after
	// every one, not just the first.
	for i := 1; i <= 3; i++ {
		s.recordLoginFailure(ctx, u)

		ttl, err := c.Client().PTTL(ctx, key).Result()
		if err != nil {
			t.Fatalf("PTTL: %v", err)
		}
		// PTTL returns -2 (no key) or -1 (no TTL) as non-positive durations.
		if ttl <= 0 {
			t.Fatalf("after failure %d the throttle key must have a positive TTL, got %v", i, ttl)
		}
		n, err := c.Client().Get(ctx, key).Int()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if n != i {
			t.Fatalf("counter = %d, want %d", n, i)
		}
	}

	if !s.loginLocked(ctx, u) {
		t.Fatal("should be locked at 3/3 failures")
	}
}

// The counter is atomic: concurrent failures must ALL count. The old
// Get-then-Set read-modify-write lost increments under this race (audit F17).
func TestLoginThrottle_ConcurrentFailuresAllCount(t *testing.T) {
	c := newRealCache(t)
	s := NewAuthService(nil, nil, c, authz.JWTConfig{}, "", time.Hour, logger.Default())
	s.loginMaxFails = 1_000_000 // effectively never lock; we only count
	s.loginFailWindow = time.Minute
	ctx := context.Background()
	u := "race-victim-" + uniqueSuffix(t)
	key := loginFailKey(u)
	t.Cleanup(func() { _ = c.Delete(ctx, key) })

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			s.recordLoginFailure(ctx, u)
		}()
	}
	wg.Wait()

	got, err := c.Client().Get(ctx, key).Int()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != n {
		t.Fatalf("counter = %d, want %d (atomic INCR must not lose increments)", got, n)
	}
}
