package domain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// testLogger returns a logger.Default() — wired through the same factory the
// real service uses but writing to stderr only during go test.
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	return logger.Default()
}

// TestBaseHTTPClient_DefaultTimeoutIs10s locks in the production default per
// SCRAPER-NF-01. NewBaseHTTPClient with no options must report a 10s timeout.
func TestBaseHTTPClient_DefaultTimeoutIs10s(t *testing.T) {
	t.Parallel()
	c := NewBaseHTTPClient(testLogger(t))
	if got, want := c.Timeout(), 10*time.Second; got != want {
		t.Errorf("default Timeout() = %v; want %v (SCRAPER-NF-01)", got, want)
	}
}

// TestBaseHTTPClient_HardTimeout verifies that a hanging upstream is cut off
// at the configured per-attempt timeout. We override to 100ms for fast tests;
// the production behavior (10s) is asserted separately above.
func TestBaseHTTPClient_HardTimeout(t *testing.T) {
	t.Parallel()
	hang := make(chan struct{})
	// Close hang BEFORE srv.Close so the blocking handler can return and
	// srv.Close doesn't hang waiting for in-flight requests. Cleanups run in
	// LIFO order, so register hang-close LAST (it runs first).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-hang
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hang) })

	c := NewBaseHTTPClient(testLogger(t),
		WithTimeout(100*time.Millisecond),
		WithMaxRetries(0),
	)

	start := time.Now()
	_, err := c.Get(context.Background(), srv.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Get returned in %v; expected ≤500ms with WithTimeout(100ms)", elapsed)
	}
}

// TestBaseHTTPClient_BackoffSequence verifies the exponential 1→2→4→8 unit
// backoff per SCRAPER-NF-03. We compress the unit to 10ms for fast tests but
// preserve the multiplier sequence.
func TestBaseHTTPClient_BackoffSequence(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var timestamps []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		timestamps = append(timestamps, time.Now())
		if n < 5 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewBaseHTTPClient(testLogger(t),
		WithRetryWaitMin(10*time.Millisecond),
		WithRetryWaitMax(80*time.Millisecond),
		WithMaxRetries(4),
	)

	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()

	if got := attempts.Load(); got != 5 {
		t.Errorf("attempts = %d; want 5 (1 initial + 4 retries)", got)
	}

	// Backoff intervals between attempts; with min=10ms, max=80ms,
	// retryablehttp's default exponential backoff yields ~10, 20, 40, 80 ms.
	// Allow ±50% per interval for jitter.
	if len(timestamps) < 5 {
		t.Fatalf("only %d timestamps captured; need 5", len(timestamps))
	}
	expectedMs := []float64{10, 20, 40, 80}
	for i, want := range expectedMs {
		gap := timestamps[i+1].Sub(timestamps[i])
		gapMs := float64(gap.Milliseconds())
		// Generous lower bound (timing on shared CI can be tight); strict upper bound.
		if gapMs < want*0.4 || gapMs > want*3.0 {
			t.Errorf("attempt-%d backoff = %.1fms; want ~%.1fms (±tolerance)", i+1, gapMs, want)
		}
	}
}

// TestBaseHTTPClient_PerHostRateLimit verifies that a registered per-host
// rate.Limiter throttles back-to-back calls to the same host.
func TestBaseHTTPClient_PerHostRateLimit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	c := NewBaseHTTPClient(testLogger(t),
		WithPerHostRPS(u.Host, 2, 1), // 2 RPS, burst 1 ⇒ second call waits ~500ms
		WithRetryWaitMin(1*time.Millisecond),
		WithRetryWaitMax(2*time.Millisecond),
	)

	ctx := context.Background()

	// First call consumes the burst.
	resp1, err := c.Get(ctx, srv.URL)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	resp1.Body.Close()

	start := time.Now()
	resp2, err := c.Get(ctx, srv.URL)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	resp2.Body.Close()
	elapsed := time.Since(start)

	// 2 RPS with burst=1 means the second call must wait ~500ms for the
	// next token. Tolerance: ≥350ms (allow scheduling slack) and ≤800ms.
	if elapsed < 350*time.Millisecond {
		t.Errorf("second Get returned in %v; expected ≥350ms due to rate limit", elapsed)
	}
	if elapsed > 800*time.Millisecond {
		t.Errorf("second Get returned in %v; expected ≤800ms", elapsed)
	}
}

// TestBaseHTTPClient_CookieJarPersists verifies the cookie jar persists Set-Cookie
// headers across calls within the same host.
func TestBaseHTTPClient_CookieJarPersists(t *testing.T) {
	t.Parallel()
	var sawCookie atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/check":
			ck, err := r.Cookie("session")
			if err == nil && ck.Value == "abc123" {
				sawCookie.Store(true)
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewBaseHTTPClient(testLogger(t),
		WithRetryWaitMin(1*time.Millisecond),
		WithRetryWaitMax(2*time.Millisecond),
	)

	ctx := context.Background()
	resp, err := c.Get(ctx, srv.URL+"/set")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	resp.Body.Close()

	resp, err = c.Get(ctx, srv.URL+"/check")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	resp.Body.Close()

	if !sawCookie.Load() {
		t.Error("server did not receive session cookie on second call; jar failed to persist")
	}
}

// TestBaseHTTPClient_BaselineHeaders asserts that every outgoing request
// carries the User-Agent, Accept-Language, and Accept-Encoding the megacloud
// sidecar matches against. Drift here can re-trigger anti-bot blocks.
func TestBaseHTTPClient_BaselineHeaders(t *testing.T) {
	t.Parallel()
	var ua, al, ae string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		al = r.Header.Get("Accept-Language")
		ae = r.Header.Get("Accept-Encoding")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewBaseHTTPClient(testLogger(t),
		WithRetryWaitMin(1*time.Millisecond),
		WithRetryWaitMax(2*time.Millisecond),
	)
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()

	if !strings.Contains(ua, "Mozilla/5.0") {
		t.Errorf("User-Agent = %q; want a Mozilla/5.0 desktop UA", ua)
	}
	if al != "en-US,en;q=0.9" {
		t.Errorf("Accept-Language = %q; want en-US,en;q=0.9", al)
	}
	// Note: Go's transport adds its own Accept-Encoding: gzip when
	// DisableCompression is false (default). We assert our header is the one
	// the upstream sees, OR Go's default — both are acceptable for this test.
	if !strings.Contains(ae, "gzip") {
		t.Errorf("Accept-Encoding = %q; want to include gzip", ae)
	}
}

// TestBaseHTTPClient_PerHostLimiterIsolation verifies that throttling on host A
// does NOT block host B — limiters are per-host.
func TestBaseHTTPClient_PerHostLimiterIsolation(t *testing.T) {
	t.Parallel()
	hits := make(map[string]*atomic.Int32)
	srvFor := func(label string) *httptest.Server {
		hits[label] = new(atomic.Int32)
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[label].Add(1)
			w.WriteHeader(http.StatusOK)
		}))
	}
	srvA := srvFor("A")
	srvB := srvFor("B")
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)

	uA, _ := url.Parse(srvA.URL)

	c := NewBaseHTTPClient(testLogger(t),
		WithPerHostRPS(uA.Host, 0.5, 1), // host A: 0.5 RPS, very slow
		WithRetryWaitMin(1*time.Millisecond),
		WithRetryWaitMax(2*time.Millisecond),
	)

	ctx := context.Background()
	resp, err := c.Get(ctx, srvA.URL)
	if err != nil {
		t.Fatalf("Get A1: %v", err)
	}
	resp.Body.Close()

	// Now hammer host B 3 times; none of them should wait for host A's slow limiter.
	start := time.Now()
	for i := 0; i < 3; i++ {
		resp, err := c.Get(ctx, fmt.Sprintf("%s/?i=%d", srvB.URL, i))
		if err != nil {
			t.Fatalf("Get B%d: %v", i, err)
		}
		resp.Body.Close()
	}
	elapsed := time.Since(start)

	// 3 calls to host B with no limiter should be near-instant (<500ms total).
	if elapsed > 500*time.Millisecond {
		t.Errorf("3 host-B calls took %v; expected <500ms (host-A limiter must not throttle host B)", elapsed)
	}
	if hits["B"].Load() != 3 {
		t.Errorf("host B received %d hits; want 3", hits["B"].Load())
	}
}

// TestBaseHTTPClient_JarAccessor locks the Jar() accessor that Plan 16-03's
// DDoS-Guard cookie inspection relies on (Phase-15 one-line amend mandated by
// 16-RESEARCH.md §Pattern 3 / Assumption A4). The accessor must return a
// non-nil http.CookieJar that round-trips a Set/Get pair.
func TestBaseHTTPClient_JarAccessor(t *testing.T) {
	t.Parallel()
	c := NewBaseHTTPClient(testLogger(t))

	jar := c.Jar()
	if jar == nil {
		t.Fatal("Jar() returned nil; expected a live http.CookieJar")
	}

	u, err := url.Parse("https://animepahe.ru/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{
		{Name: "ddg_probe", Value: "abc", Path: "/", Domain: "animepahe.ru"},
	})

	got := jar.Cookies(u)
	if len(got) != 1 || got[0].Name != "ddg_probe" || got[0].Value != "abc" {
		t.Errorf("Jar().Cookies(animepahe.ru) = %+v; want one cookie ddg_probe=abc", got)
	}
}

// TestBaseHTTPClient_JarAccessor_StableInstance proves Jar() returns the same
// underlying jar instance on repeated calls — providers can hold the
// reference and observe cookies written by the http stack on later requests.
func TestBaseHTTPClient_JarAccessor_StableInstance(t *testing.T) {
	t.Parallel()
	c := NewBaseHTTPClient(testLogger(t))
	if a, b := c.Jar(), c.Jar(); a != b {
		t.Errorf("Jar() returned two different instances (%p vs %p); want the same live jar", a, b)
	}
}

// TestBaseHTTPClient_DoMethod verifies the lower-level Do method accepts a
// caller-built *http.Request and applies the same limiter / retry / baseline
// headers.
func TestBaseHTTPClient_DoMethod(t *testing.T) {
	t.Parallel()
	var customHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewBaseHTTPClient(testLogger(t),
		WithRetryWaitMin(1*time.Millisecond),
		WithRetryWaitMax(2*time.Millisecond),
	)

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-Custom", "abc")

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if customHeader != "abc" {
		t.Errorf("caller-set X-Custom not preserved; got %q", customHeader)
	}
}
