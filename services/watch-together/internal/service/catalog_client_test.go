// Package service — catalog_client_test.go covers the WT-STATE-02 catalog
// validation client.
//
// All tests stand up an in-process httptest.NewServer and inject its URL into
// NewCatalogClient. The handler tracks an atomic call counter so cache
// hit/miss assertions are exact rather than timing-based.
//
// The TTL is verified by overriding the client's `now` clock via the
// test-only SetClockForTest hook — keeps the test runtime <1s even though
// the production TTL is 5s.
package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// catalogResponse is the wire envelope the catalog endpoint produces (mirrors
// libs/httputil.JSON's {success,data,error} shape). Defined here so the tests
// can construct it without coupling to the catalog package.
type catalogResponse struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   *catalogResponseError  `json:"error,omitempty"`
}

type catalogResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// newOKHandler returns an httptest.Server handler that always responds 200 OK
// with the given (valid, reason) pair wrapped in the success envelope. Atomic
// counter `hits` increments on every request so callers can assert cache
// behavior.
func newOKHandler(t *testing.T, valid bool, reason string, hits *int64) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(catalogResponse{
			Success: true,
			Data: map[string]interface{}{
				"valid":  valid,
				"reason": reason,
			},
		})
	})
}

func TestCatalogClient_ValidateEpisode_HappyValid(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(newOKHandler(t, true, "", &hits))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	res, err := c.ValidateEpisode(context.Background(), "12345", "kodik", "1", "anirise", "sub")
	if err != nil {
		t.Fatalf("ValidateEpisode error = %v, want nil", err)
	}
	if !res.Valid {
		t.Errorf("Valid = false, want true")
	}
	if res.Reason != "" {
		t.Errorf("Reason = %q, want \"\"", res.Reason)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("HTTP hits = %d, want 1", got)
	}
}

func TestCatalogClient_ValidateEpisode_HappyInvalid(t *testing.T) {
	// Valid=false is a domain answer, NOT a transport error. The error
	// channel is reserved for network/protocol failures.
	var hits int64
	srv := httptest.NewServer(newOKHandler(t, false, "EPISODE_UNAVAILABLE", &hits))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	res, err := c.ValidateEpisode(context.Background(), "12345", "kodik", "999", "anirise", "sub")
	if err != nil {
		t.Fatalf("ValidateEpisode error = %v, want nil for logical-invalid", err)
	}
	if res.Valid {
		t.Errorf("Valid = true, want false")
	}
	if res.Reason != "EPISODE_UNAVAILABLE" {
		t.Errorf("Reason = %q, want EPISODE_UNAVAILABLE", res.Reason)
	}
}

func TestCatalogClient_ValidateEpisode_500Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(catalogResponse{
			Success: false,
			Error:   &catalogResponseError{Code: "INTERNAL", Message: "db is down"},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	res, err := c.ValidateEpisode(context.Background(), "12345", "kodik", "1", "t", "sub")
	if err == nil {
		t.Fatal("expected non-nil error for 500 response")
	}
	if res.Valid {
		t.Errorf("Valid = true on error path, want zero value (false)")
	}
}

func TestCatalogClient_ValidateEpisode_400Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(catalogResponse{
			Success: false,
			Error:   &catalogResponseError{Code: "INVALID_INPUT", Message: "missing player"},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	_, err := c.ValidateEpisode(context.Background(), "12345", "", "", "", "")
	if err == nil {
		t.Fatal("expected non-nil error for 400 response")
	}
	if !strings.Contains(err.Error(), "catalog rejected") {
		t.Errorf("error = %v, want substring %q", err, "catalog rejected")
	}
}

func TestCatalogClient_ValidateEpisode_PositiveCacheHit(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(newOKHandler(t, true, "", &hits))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	ctx := context.Background()

	// First call -> HTTP miss + cache insert.
	if _, err := c.ValidateEpisode(ctx, "12345", "kodik", "1", "anirise", "sub"); err != nil {
		t.Fatalf("call 1 error = %v", err)
	}
	// Second call within TTL -> cache hit, MUST NOT hit HTTP.
	if _, err := c.ValidateEpisode(ctx, "12345", "kodik", "1", "anirise", "sub"); err != nil {
		t.Fatalf("call 2 error = %v", err)
	}

	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("HTTP hits = %d, want 1 (second call should be cached)", got)
	}
}

func TestCatalogClient_ValidateEpisode_CacheTTLExpires(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(newOKHandler(t, true, "", &hits))
	t.Cleanup(srv.Close)

	// Override the clock so we can fast-forward past the 5s TTL deterministically.
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	now := base
	c := NewCatalogClient(srv.URL, logger.Default())
	c.SetClockForTest(func() time.Time { return now })

	ctx := context.Background()
	if _, err := c.ValidateEpisode(ctx, "12345", "kodik", "1", "anirise", "sub"); err != nil {
		t.Fatalf("call 1 error = %v", err)
	}

	// Advance past the 5s TTL.
	now = base.Add(6 * time.Second)

	if _, err := c.ValidateEpisode(ctx, "12345", "kodik", "1", "anirise", "sub"); err != nil {
		t.Fatalf("call 2 error = %v", err)
	}

	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Errorf("HTTP hits = %d, want 2 (cache should have expired)", got)
	}
}

func TestCatalogClient_ValidateEpisode_NegativeNotCached(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(newOKHandler(t, false, "EPISODE_UNAVAILABLE", &hits))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	ctx := context.Background()

	if _, err := c.ValidateEpisode(ctx, "12345", "kodik", "999", "anirise", "sub"); err != nil {
		t.Fatalf("call 1 error = %v", err)
	}
	// Within TTL but negative — MUST re-check so a fixed catalog state
	// propagates immediately.
	if _, err := c.ValidateEpisode(ctx, "12345", "kodik", "999", "anirise", "sub"); err != nil {
		t.Fatalf("call 2 error = %v", err)
	}

	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Errorf("HTTP hits = %d, want 2 (negative results must not be cached)", got)
	}
}

func TestCatalogClient_ValidateEpisode_URLConstruction(t *testing.T) {
	var (
		gotPath  string
		gotQuery url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(catalogResponse{
			Success: true,
			Data:    map[string]interface{}{"valid": true, "reason": ""},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())
	if _, err := c.ValidateEpisode(context.Background(),
		"12345", "ourenglish", "ep-3", "trans-7", "sub"); err != nil {
		t.Fatalf("ValidateEpisode error = %v", err)
	}

	wantPath := "/internal/anime/12345/episodes/validate"
	if gotPath != wantPath {
		t.Errorf("request path = %q, want %q", gotPath, wantPath)
	}
	if got, want := gotQuery.Get("player"), "ourenglish"; got != want {
		t.Errorf("query.player = %q, want %q", got, want)
	}
	if got, want := gotQuery.Get("episode_id"), "ep-3"; got != want {
		t.Errorf("query.episode_id = %q, want %q", got, want)
	}
	if got, want := gotQuery.Get("translation_id"), "trans-7"; got != want {
		t.Errorf("query.translation_id = %q, want %q", got, want)
	}
	if got, want := gotQuery.Get("watch_type"), "sub"; got != want {
		t.Errorf("query.watch_type = %q, want %q", got, want)
	}
}

func TestCatalogClient_ValidateEpisode_ContextCancelled(t *testing.T) {
	// Verify that a cancelled parent context surfaces immediately as an
	// error (rather than hanging until the 3s client timeout) — proves
	// http.NewRequestWithContext wires ctx into the request properly.
	//
	// We use a handler that sleeps long enough to outlast the test's
	// patience; ctx.Cancel must abort the in-flight request.
	blocker := make(chan struct{})
	t.Cleanup(func() { close(blocker) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := NewCatalogClient(srv.URL, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.ValidateEpisode(ctx, "12345", "kodik", "1", "anirise", "sub")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	// Must abort well before the 3s client timeout would fire.
	if elapsed >= time.Second {
		t.Errorf("ctx-cancel took %s, want <1s", elapsed)
	}
}

// TestCatalogClient_EvictExpired purges only entries past their TTL (audit #32).
func TestCatalogClient_EvictExpired(t *testing.T) {
	c := NewCatalogClient("http://catalog.invalid", logger.Default())
	defer c.Stop()
	base := time.Now()
	c.SetClockForTest(func() time.Time { return base })

	c.mu.Lock()
	c.cache["fresh"] = cachedValidation{result: ValidateResult{Valid: true}, expireAt: base.Add(time.Minute)}
	c.cache["stale"] = cachedValidation{result: ValidateResult{Valid: true}, expireAt: base.Add(-time.Minute)}
	c.mu.Unlock()

	c.evictExpired()

	c.mu.Lock()
	_, freshOK := c.cache["fresh"]
	_, staleOK := c.cache["stale"]
	n := len(c.cache)
	c.mu.Unlock()

	if !freshOK {
		t.Error("fresh entry must survive eviction")
	}
	if staleOK {
		t.Error("expired entry must be evicted")
	}
	if n != 1 {
		t.Errorf("cache size = %d, want 1", n)
	}
}
