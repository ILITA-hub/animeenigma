package transport

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// newCountingAuthServer stands in for the auth service's
// /internal/resolve-api-key endpoint, incrementing a hit counter per request
// and returning the supplied status + a fixed claim body on 200.
func newCountingAuthServer(t *testing.T, status int) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"user_id":"u1","username":"alice","role":"user"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestAPIKeyCache_HitWithinTTL — audit finding L473. Two sequential resolves of
// the same ak_ token within the TTL must produce exactly ONE upstream POST.
// Today every call round-trips (no cache exists); this is the red anchor.
func TestAPIKeyCache_HitWithinTTL(t *testing.T) {
	srv, hits := newCountingAuthServer(t, http.StatusOK)

	now := time.Unix(0, 0)
	cache := newAPIKeyCache(resolveApiKey, func() time.Time { return now })

	const token = "ak_hotkey"
	c1, err := cache.resolveCached(srv.URL, token)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	c2, err := cache.resolveCached(srv.URL, token)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("upstream hit count = %d; want 1 (second call should hit cache)", got)
	}
	if c1.UserID != "u1" || c2.UserID != "u1" {
		t.Errorf("resolved UserID = %q/%q; want u1/u1", c1.UserID, c2.UserID)
	}
	if c1.Username != "alice" || c1.Role != authz.Role("user") {
		t.Errorf("resolved claims = %+v; want username=alice role=user", c1)
	}
}

// TestAPIKeyCache_ExpiryReHitsUpstream — advancing the injected clock past the
// TTL forces the next resolve to re-hit upstream (counter == 2).
func TestAPIKeyCache_ExpiryReHitsUpstream(t *testing.T) {
	srv, hits := newCountingAuthServer(t, http.StatusOK)

	now := time.Unix(0, 0)
	cache := newAPIKeyCache(resolveApiKey, func() time.Time { return now })

	const token = "ak_hotkey"
	if _, err := cache.resolveCached(srv.URL, token); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// Still within TTL → cache hit, no new upstream call.
	now = now.Add(apiKeyCacheTTL - time.Second)
	if _, err := cache.resolveCached(srv.URL, token); err != nil {
		t.Fatalf("within-TTL resolve: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("hit count after within-TTL call = %d; want 1", got)
	}
	// Past TTL → cache expired, re-hits upstream.
	now = now.Add(2 * time.Second)
	if _, err := cache.resolveCached(srv.URL, token); err != nil {
		t.Fatalf("post-TTL resolve: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("hit count after post-TTL call = %d; want 2 (cache should have expired)", got)
	}
}

// TestAPIKeyCache_DoesNotCacheFailures — a resolve that returns 401 (revoked
// key) is NOT cached, so a retry re-hits upstream and a key that is later
// re-authorized recovers without waiting out a TTL.
func TestAPIKeyCache_DoesNotCacheFailures(t *testing.T) {
	srv, hits := newCountingAuthServer(t, http.StatusUnauthorized)

	now := time.Unix(0, 0)
	cache := newAPIKeyCache(resolveApiKey, func() time.Time { return now })

	const token = "ak_revoked"
	if _, err := cache.resolveCached(srv.URL, token); err == nil {
		t.Fatal("first resolve of a 401 key: want error, got nil")
	}
	if _, err := cache.resolveCached(srv.URL, token); err == nil {
		t.Fatal("retry resolve of a 401 key: want error, got nil")
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("hit count = %d; want 2 (failures must not be cached)", got)
	}
}

// TestAPIKeyCache_DistinctKeysIsolated — different raw keys must not collide;
// each is resolved upstream once.
func TestAPIKeyCache_DistinctKeysIsolated(t *testing.T) {
	srv, hits := newCountingAuthServer(t, http.StatusOK)

	now := time.Unix(0, 0)
	cache := newAPIKeyCache(resolveApiKey, func() time.Time { return now })

	if _, err := cache.resolveCached(srv.URL, "ak_one"); err != nil {
		t.Fatalf("resolve ak_one: %v", err)
	}
	if _, err := cache.resolveCached(srv.URL, "ak_two"); err != nil {
		t.Fatalf("resolve ak_two: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("hit count = %d; want 2 (two distinct keys)", got)
	}
}

// TestAPIKeyCache_ReturnedClaimsAreCopies — mutating the returned claims (the
// middlewares set SessionID per request) must not corrupt the shared cache
// entry that the next request reads.
func TestAPIKeyCache_ReturnedClaimsAreCopies(t *testing.T) {
	srv, _ := newCountingAuthServer(t, http.StatusOK)

	now := time.Unix(0, 0)
	cache := newAPIKeyCache(resolveApiKey, func() time.Time { return now })

	const token = "ak_hotkey"
	first, err := cache.resolveCached(srv.URL, token)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	first.SessionID = "ak-mutated-by-caller"

	second, err := cache.resolveCached(srv.URL, token) // served from cache
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if second.SessionID != "" {
		t.Errorf("cached claim leaked caller mutation: SessionID = %q; want empty", second.SessionID)
	}
}
