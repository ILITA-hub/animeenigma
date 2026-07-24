package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-chi/chi/v5"
)

// newTestOGHandler wires an OpenGraphHandler against a stub catalog that returns
// HTTP 200 for every /api/anime/<id> request, so any id an attacker sends would
// "resolve" — exactly the condition the DoS relies on. The returned counter
// records how many upstream fetches actually happened.
func newTestOGHandler(t *testing.T) (*OpenGraphHandler, *int64) {
	t.Helper()
	var calls int64
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"` + strings.TrimPrefix(r.URL.Path, "/api/anime/") + `","name":"Test Anime"}}`))
	}))
	t.Cleanup(catalog.Close)

	h := NewOpenGraphHandler(catalog.URL, catalog.URL, catalog.URL, "https://example.test", logger.Default())
	return h, &calls
}

// serveAnime drives ServeAnime through a chi router so chi.URLParam resolves.
func serveAnime(h *OpenGraphHandler, id string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Get("/og/anime/{animeId}", h.ServeAnime)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/og/anime/"+id, nil))
	return rec
}

const validUUID = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

// TestServeAnimeValidIDCachesOnce proves the legitimate path is intact: a real
// bot request for a valid anime renders, caches exactly one entry, and a second
// request for the same canonical id is served from cache (no second fetch).
func TestServeAnimeValidIDCachesOnce(t *testing.T) {
	h, calls := newTestOGHandler(t)

	if rec := serveAnime(h, validUUID); rec.Code != http.StatusOK {
		t.Fatalf("first request: code = %d", rec.Code)
	}
	if got := h.cache.Len(); got != 1 {
		t.Fatalf("after valid request cache len = %d, want 1", got)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}

	// Second request for the same id must be a cache hit — no new fetch.
	if rec := serveAnime(h, validUUID); rec.Code != http.StatusOK {
		t.Fatalf("second request: code = %d", rec.Code)
	}
	if got := h.cache.Len(); got != 1 {
		t.Fatalf("after repeat request cache len = %d, want 1", got)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Fatalf("upstream calls after cache hit = %d, want 1", got)
	}

	// Valid prefixed forms are also accepted and cached.
	serveAnime(h, "shiki_12345")
	serveAnime(h, "mal_36726")
	if got := h.cache.Len(); got != 3 {
		t.Fatalf("cache len after prefixed ids = %d, want 3", got)
	}
}

// TestServeAnimeInvalidIDsNotCached proves (a): non-canonical / oversized keys
// never enter the cache and fall back to the default page — even though the
// stub catalog would answer 200 for any of them.
func TestServeAnimeInvalidIDsNotCached(t *testing.T) {
	h, calls := newTestOGHandler(t)

	bad := []string{
		"notauuid",
		"shiki_",                             // no digits
		"shiki_notanumber",                   // non-digit
		"shiki_1234567890123",                // 13 digits, over the length bound
		"mal_abc",                            // non-digit
		strings.Repeat("a", 5000),            // oversized key attempt
		validUUID + "extra",                  // UUID with trailing junk
		"3f2504e0-4f89-41d3-9a0c-0305e82c33", // truncated UUID
	}
	for _, id := range bad {
		rec := serveAnime(h, id)
		if rec.Code != http.StatusOK { // serveDefault still returns 200 HTML
			t.Fatalf("id %q: code = %d, want 200 (default page)", id, rec.Code)
		}
	}
	if got := h.cache.Len(); got != 0 {
		t.Fatalf("cache len after invalid ids = %d, want 0", got)
	}
	if got := atomic.LoadInt64(calls); got != 0 {
		t.Fatalf("upstream calls for invalid ids = %d, want 0 (no fetch)", got)
	}
}

// TestServeAnimeCaseVariantUUIDsCollapse proves (b): the 2^hexletters case
// spellings of one UUID collapse to a single cache entry and a single fetch.
func TestServeAnimeCaseVariantUUIDsCollapse(t *testing.T) {
	h, calls := newTestOGHandler(t)

	variants := []string{
		validUUID,
		strings.ToUpper(validUUID),
		"3F2504e0-4f89-41D3-9a0c-0305E82C3301",
		"3f2504E0-4F89-41d3-9A0C-0305e82c3301",
	}
	for _, v := range variants {
		if rec := serveAnime(h, v); rec.Code != http.StatusOK {
			t.Fatalf("variant %q: code = %d", v, rec.Code)
		}
	}
	if got := h.cache.Len(); got != 1 {
		t.Fatalf("case-variant UUIDs cache len = %d, want 1", got)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Fatalf("case-variant UUIDs upstream calls = %d, want 1", got)
	}
}

// TestServeUserInvalidAndCaseCollapse mirrors the anime checks on the user route.
func TestServeUserInvalidAndCaseCollapse(t *testing.T) {
	h, _ := newTestOGHandler(t)
	r := chi.NewRouter()
	r.Get("/og/user/{publicId}", h.ServeUser)
	serve := func(id string) int {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/og/user/"+id, nil))
		return rec.Code
	}

	// Invalid shapes are not cached.
	for _, id := range []string{"ab", "with-dash", strings.Repeat("z", 40), "under_score"} {
		serve(id)
	}
	if got := h.cache.Len(); got != 0 {
		t.Fatalf("user cache len after invalid ids = %d, want 0", got)
	}

	// Case-variant UUIDs collapse to one key on the user route too.
	serve(validUUID)
	serve(strings.ToUpper(validUUID))
	if got := h.cache.Len(); got != 1 {
		t.Fatalf("user case-variant cache len = %d, want 1", got)
	}
}

// TestOGCacheCapBounded proves (c): the LRU never exceeds its cap regardless of
// how many distinct keys arrive, and evicts least-recently-used entries.
func TestOGCacheCapBounded(t *testing.T) {
	const cap = 8
	c := newOGCache(cap)
	for i := 0; i < 100000; i++ {
		c.Store(fmt.Sprintf("key-%d", i), &ogCacheEntry{
			html:      []byte("x"),
			expiresAt: time.Now().Add(time.Hour),
		})
		if got := c.Len(); got > cap {
			t.Fatalf("after %d stores cache len = %d, exceeds cap %d", i+1, got, cap)
		}
	}
	if got := c.Len(); got != cap {
		t.Fatalf("final cache len = %d, want %d", got, cap)
	}
	// The oldest keys must have been evicted; only the most recent cap remain.
	if _, ok := c.Load("key-0"); ok {
		t.Fatalf("key-0 should have been evicted")
	}
	if _, ok := c.Load("key-99999"); !ok {
		t.Fatalf("key-99999 (most recent) should be present")
	}
}

// TestOGCacheConcurrent hammers the cache from many goroutines; run under -race
// it proves the LRU stays concurrency-safe and never exceeds its cap.
func TestOGCacheConcurrent(t *testing.T) {
	const cap = 32
	c := newOGCache(cap)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				key := fmt.Sprintf("g%d-k%d", g, i%128)
				c.Store(key, &ogCacheEntry{html: []byte("x"), expiresAt: time.Now().Add(time.Hour)})
				c.Load(key)
				if i%50 == 0 {
					c.purgeExpired(time.Now())
				}
			}
		}(g)
	}
	wg.Wait()
	if got := c.Len(); got > cap {
		t.Fatalf("concurrent cache len = %d, exceeds cap %d", got, cap)
	}
}

// TestOGCachePurgeExpired confirms the folded-in sweeper reclaims TTL-expired
// entries while keeping live ones.
func TestOGCachePurgeExpired(t *testing.T) {
	c := newOGCache(16)
	now := time.Now()
	c.Store("live", &ogCacheEntry{html: []byte("x"), expiresAt: now.Add(time.Hour)})
	c.Store("dead", &ogCacheEntry{html: []byte("x"), expiresAt: now.Add(-time.Minute)})

	c.purgeExpired(now)

	if _, ok := c.Load("dead"); ok {
		t.Fatalf("expired entry should have been purged")
	}
	if _, ok := c.Load("live"); !ok {
		t.Fatalf("live entry should survive purge")
	}
}
