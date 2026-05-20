package transport

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
)

// newMiniRedis starts a miniredis instance and returns a connected *redis.Client.
// Both are torn down via t.Cleanup so each test is hermetic.
func newMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, rdb
}

// passThroughHandler is the inner handler the middleware wraps. We assert
// reachability by inspecting the response status code (200 if the request
// got through, otherwise the middleware blocked / failed).
func passThroughHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// authedRequest builds a request with the given user_id injected into the
// authz context — emulates what JWTValidationMiddleware does before this
// middleware runs.
func authedRequest(t *testing.T, userID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	claims := &authz.Claims{UserID: userID, Username: "u-" + userID, Role: authz.RoleUser}
	return req.WithContext(authz.ContextWithClaims(req.Context(), claims))
}

// TestUserRateLimit_BlocksAfterBurst — burst=10, the 11th rapid request from
// the same user must be 429 with a populated Retry-After header AND the
// existing RATE_LIMITED JSON envelope.
func TestUserRateLimit_BlocksAfterBurst(t *testing.T) {
	_, rdb := newMiniRedis(t)
	log := logger.Default()

	// rate 60/min, burst 10 — the GCRA Allow() call accounts for one event
	// per call, so the first 10 events fit in the burst window and the
	// 11th must hit the limit.
	mw := UserRateLimitMiddleware(rdb, 60, 10, log)
	h := mw(passThroughHandler())

	for i := 1; i <= 10; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authedRequest(t, "user-burst"))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: got status %d, want 200", i, w.Code)
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedRequest(t, "user-burst"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th request: got status %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Errorf("Retry-After header missing on 429 response")
	} else if n, err := strconv.Atoi(got); err != nil || n < 0 {
		t.Errorf("Retry-After header = %q; want a non-negative integer", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	body := w.Body.String()
	if !contains(body, `"code":"RATE_LIMITED"`) {
		t.Errorf("response body = %q; want it to contain RATE_LIMITED code", body)
	}
}

// TestUserRateLimit_Replenishes — after FastForwarding the miniredis clock
// past the bucket refill window, traffic flows again.
func TestUserRateLimit_Replenishes(t *testing.T) {
	mr, rdb := newMiniRedis(t)
	log := logger.Default()

	mw := UserRateLimitMiddleware(rdb, 60, 10, log)
	h := mw(passThroughHandler())

	// Drain the bucket (10 OK + 1 blocked).
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authedRequest(t, "user-refill"))
		if w.Code != http.StatusOK {
			t.Fatalf("drain request %d: got %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedRequest(t, "user-refill"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after drain, got %d", w.Code)
	}

	// 60 req/min == 1 req/sec. Advance the fake clock ~2 minutes — should
	// fully refill the bucket.
	mr.FastForward(2 * time.Minute)

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedRequest(t, "user-refill"))
	if w.Code != http.StatusOK {
		t.Fatalf("after refill: got %d, want 200", w.Code)
	}
}

// TestUserRateLimit_AnonymousSkipsRedis — requests without claims in context
// must bypass the middleware entirely and never touch Redis.
func TestUserRateLimit_AnonymousSkipsRedis(t *testing.T) {
	mr, rdb := newMiniRedis(t)
	log := logger.Default()

	before := mr.TotalConnectionCount()

	mw := UserRateLimitMiddleware(rdb, 60, 10, log)
	h := mw(passThroughHandler())

	// 50 anonymous requests — no claims in context.
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/anime/123", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("anonymous request %d: got %d, want 200", i, w.Code)
		}
	}

	// The middleware must not have opened any new Redis connections for
	// anonymous traffic.
	if got := mr.TotalConnectionCount(); got != before {
		t.Errorf("miniredis TotalConnectionCount delta = %d; want 0 (anonymous traffic must skip Redis)", got-before)
	}
}

// TestUserRateLimit_IncrementsBlockedMetric — the Prometheus counter
// `gateway_rate_limit_user_blocked_total` must increase by exactly the
// number of blocked requests observed in this test.
func TestUserRateLimit_IncrementsBlockedMetric(t *testing.T) {
	_, rdb := newMiniRedis(t)
	log := logger.Default()

	// burst=2 keeps the test tight; 5 calls → 2 OK + 3 blocked.
	mw := UserRateLimitMiddleware(rdb, 60, 2, log)
	h := mw(passThroughHandler())

	before := testutil.ToFloat64(userRateLimitBlockedTotal)

	var blocked int
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authedRequest(t, "user-metric"))
		if w.Code == http.StatusTooManyRequests {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("expected at least one blocked request; got 0")
	}
	after := testutil.ToFloat64(userRateLimitBlockedTotal)
	delta := after - before
	if int(delta) != blocked {
		t.Errorf("gateway_rate_limit_user_blocked_total delta = %v; want %d", delta, blocked)
	}
}

// TestUserRateLimit_FailsOpenOnRedisOutage — if Redis is unreachable, the
// middleware must let the request through (with a logged WARN) rather than
// 500-ing every authenticated request because of a Redis blip.
func TestUserRateLimit_FailsOpenOnRedisOutage(t *testing.T) {
	mr, rdb := newMiniRedis(t)
	log := logger.Default()

	mw := UserRateLimitMiddleware(rdb, 60, 10, log)
	h := mw(passThroughHandler())

	// Sanity check — one successful authenticated request first.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, authedRequest(t, "user-failopen"))
	if w.Code != http.StatusOK {
		t.Fatalf("pre-outage request: got %d, want 200", w.Code)
	}

	// Kill the fake Redis mid-flight. Subsequent Allow() calls will return
	// a connection error from the client.
	mr.Close()

	w = httptest.NewRecorder()
	h.ServeHTTP(w, authedRequest(t, "user-failopen"))
	if w.Code != http.StatusOK {
		t.Errorf("post-outage request: got %d, want 200 (fail-open contract)", w.Code)
	}
}

// contains is the tiny helper this file uses to keep the test code readable
// without pulling strings or regex into the imports.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
