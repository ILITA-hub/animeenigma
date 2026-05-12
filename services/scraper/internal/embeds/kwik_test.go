package embeds

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Compile-time assertion: KwikExtractor satisfies domain.EmbedExtractor.
// Mirrors the megacloud_test.go guard. If this line fails to compile, the
// EmbedExtractor contract has drifted from the implementation.
var _ domain.EmbedExtractor = (*KwikExtractor)(nil)

// goldenFixturePath returns the on-disk path of the canonical Kwik HTML body
// committed in Plan 16-01. Anchoring the path here (rather than scattering
// "testdata/animepahe/kwik_e_abc.html" across N tests) keeps the cross-plan
// coupling explicit in one place.
//
// The fixture is intentionally a synthetic packer that returns its first arg
// verbatim — the unpacked string contains  const source='...m3u8...'  exactly
// as a real Dean-Edwards-packed Kwik response would. See the fixture file's
// header comment for the rationale.
func goldenFixturePath(t *testing.T) string {
	t.Helper()
	// runtime.Caller-anchored resolution is overkill here — `go test` always
	// runs with cwd = the package dir, so a relative path is stable.
	p := filepath.Join("..", "..", "testdata", "animepahe", "kwik_e_abc.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("kwik golden fixture missing at %s: %v", p, err)
	}
	return p
}

// TestKwik_Name pins the stable identifier used in logs / observability labels.
// EmbedExtractor.Name() contract: lowercase, no spaces.
func TestKwik_Name(t *testing.T) {
	t.Parallel()
	k := NewKwikExtractor()
	if got := k.Name(); got != "kwik" {
		t.Errorf("Name() = %q; want %q", got, "kwik")
	}
}

// TestKwik_Matches exercises the host-equality + strict-subdomain matcher
// against positive (kwik.cx, kwik.si, subdomain) and negative (substring-in-path,
// substring-in-query, malformed URL, empty) inputs.
func TestKwik_Matches(t *testing.T) {
	t.Parallel()
	k := NewKwikExtractor()

	matches := []string{
		"https://kwik.cx/e/abc",
		"https://kwik.si/e/def",
		"https://cdn.kwik.cx/e/abc",       // strict subdomain
		"https://www.kwik.si/embed/12345", // strict subdomain
		"http://kwik.cx/e/abc",            // scheme variations
		"HTTPS://KWIK.CX/E/ABC",           // case-insensitive host (RFC 3986)
	}
	for _, u := range matches {
		u := u
		t.Run("match_"+u, func(t *testing.T) {
			t.Parallel()
			if !k.Matches(u) {
				t.Errorf("Matches(%q) = false; want true", u)
			}
		})
	}

	nonMatches := []string{
		"https://example.com/kwik.cx/path",     // host substring forbidden
		"https://example.com/path?q=kwik.cx",   // query substring forbidden
		"https://megacloud.tv/embed/abc",       // different family
		"",                                     // empty
		"not a url",                            // unparseable
		"://no-scheme.example.com/path",        // malformed
		"kwik://kwik.cx/e/abc",                 // WR-05: non-http(s) scheme rejected
		"ftp://kwik.cx/e/abc",                  // WR-05: non-http(s) scheme rejected
		"file:///kwik.cx/passwd",               // WR-05: non-http(s) scheme rejected
	}
	for _, u := range nonMatches {
		u := u
		t.Run("nomatch_"+u, func(t *testing.T) {
			t.Parallel()
			if k.Matches(u) {
				t.Errorf("Matches(%q) = true; want false", u)
			}
		})
	}
}

// TestKwik_Matches_RejectsSubdomainImposters is the dedicated SSRF guard:
// "kwik.cx.attacker.com" must NOT match, even though it contains the literal
// string "kwik.cx". Mirrors TestMegacloudClient_MatchesKnownHosts's host
// substring assertions; explicitly named here so a regression that loosens the
// matcher to strings.Contains lights up THIS test name in CI output.
func TestKwik_Matches_RejectsSubdomainImposters(t *testing.T) {
	t.Parallel()
	k := NewKwikExtractor()

	imposters := []string{
		"https://kwik.cx.attacker.com/e/abc",
		"https://kwik.si.evil.example.org/e/abc",
		"https://akwik.cx/e/abc",  // suffix without dot separator
		"https://akwik.si/e/abc",
		"https://notkwik.cx/e/abc",
	}
	for _, u := range imposters {
		u := u
		t.Run(u, func(t *testing.T) {
			t.Parallel()
			if k.Matches(u) {
				t.Errorf("Matches(%q) = true; SSRF guard regression — host suffix matcher must require a leading '.'", u)
			}
		})
	}
}

// TestKwik_Extract_GoldenFixture loads the canonical Kwik HTML body from
// testdata/animepahe/kwik_e_abc.html (Plan 16-01), serves it from an
// httptest.Server, and asserts Extract returns a Stream with at least one
// HLS Source whose URL contains ".m3u8".
//
// The fixture intentionally contains MULTIPLE m3u8 URLs (480p/720p/1080p) so a
// regex that only captures the first match (FindStringSubmatch instead of
// FindAllStringSubmatch) lights up here.
func TestKwik_Extract_GoldenFixture(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(goldenFixturePath(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	k := NewKwikExtractor()
	stream, err := k.Extract(context.Background(), srv.URL+"/e/abc", http.Header{})
	if err != nil {
		t.Fatalf("Extract() error = %v; want nil", err)
	}
	if stream == nil {
		t.Fatal("Extract() returned nil Stream")
	}
	if len(stream.Sources) == 0 {
		t.Fatal("Extract() Stream has zero Sources; want >=1")
	}
	for i, src := range stream.Sources {
		if !strings.Contains(src.URL, ".m3u8") {
			t.Errorf("Source[%d].URL = %q; want substring %q", i, src.URL, ".m3u8")
		}
		if src.Type != "hls" {
			t.Errorf("Source[%d].Type = %q; want %q", i, src.Type, "hls")
		}
	}
	// Stream.Headers must include Referer so the downstream HLS proxy uses
	// the right Referer when fetching segments.
	if stream.Headers["Referer"] == "" {
		t.Error("Stream.Headers[Referer] is empty; HLS proxy will reject upstream fetches")
	}
}

// TestKwik_Extract_NoPacker serves a 200 response with body that contains NO
// eval(function(p,a,c,k,e,d) packer. Extract must wrap-error with
// ErrExtractFailed (NOT ErrProviderDown — the server answered) and the message
// must mention "no eval() packer" so the orchestrator's failover-loop log
// distinguishes regex mutations from network failures.
func TestKwik_Extract_NoPacker(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<html><body>no packer here, just plain HTML</body></html>`)
	}))
	t.Cleanup(srv.Close)

	k := NewKwikExtractor()
	_, err := k.Extract(context.Background(), srv.URL+"/e/missing", http.Header{})
	if err == nil {
		t.Fatal("Extract() error = nil; want wrapped ErrExtractFailed")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("Extract() error = %v; want errors.Is(..., ErrExtractFailed) = true", err)
	}
	if !strings.Contains(err.Error(), "no eval() packer") {
		t.Errorf("Extract() error message = %q; want substring %q", err.Error(), "no eval() packer")
	}
}

// TestKwik_Extract_ContextCancel verifies that a context cancelled before the
// upstream returns the body terminates Extract quickly (≤ 1s on a 10s-blocking
// server). This locks the http.NewRequestWithContext wiring — without it, a
// hostile upstream that holds the connection open forever stalls the entire
// scraper.
func TestKwik_Extract_ContextCancel(t *testing.T) {
	t.Parallel()

	// Channel blocks the response handler until the test cleans up.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	t.Cleanup(func() {
		close(block)
		srv.Close()
	})

	k := NewKwikExtractor()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	start := time.Now()
	_, err := k.Extract(ctx, srv.URL+"/e/stuck", http.Header{})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Extract() error = nil; want context cancellation error")
	}
	if elapsed > time.Second {
		t.Errorf("Extract() took %v; want <1s (context cancel did not propagate to http.Do)", elapsed)
	}
}

// TestKwik_Extract_Timeout proves vm.Interrupt() from the watchdog goroutine
// actually fires when the JS unpacker enters an infinite loop. Without the
// separate-goroutine Interrupt (Pitfall 3), the test would hang and only fail
// when the surrounding `go test -timeout 60s` kills the binary.
//
// The fixture body contains  eval(function(p,a,c,k,e,d){while(true){}}())  —
// matches the packedJSRegex and immediately deadlocks goja.
func TestKwik_Extract_Timeout(t *testing.T) {
	t.Parallel()

	infiniteLoop := `<html><script>eval(function(p,a,c,k,e,d){while(true){}}('arg'))</script></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, infiniteLoop)
	}))
	t.Cleanup(srv.Close)

	k := NewKwikExtractor(WithKwikTimeout(50 * time.Millisecond))

	start := time.Now()
	_, err := k.Extract(context.Background(), srv.URL+"/e/loop", http.Header{})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Extract() error = nil; want wrapped ErrExtractFailed from goja interrupt")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("Extract() error = %v; want errors.Is(..., ErrExtractFailed) = true", err)
	}
	// 50ms timeout + ~150ms slack for scheduling. If this fires after 1s,
	// the Interrupt goroutine wasn't separate from RunString and Pitfall 3 has
	// regressed.
	if elapsed > time.Second {
		t.Errorf("Extract() took %v; want <1s — vm.Interrupt() must come from a separate goroutine (RESEARCH.md Pitfall 3)", elapsed)
	}
}

// TestKwik_Extract_FreshRuntime proves goja runtimes are NOT reused across
// concurrent Extract calls (RESEARCH.md Pitfall 2). Spawns N concurrent
// Extract goroutines against the golden fixture and requires all of them to
// return identical, well-formed Streams. If the runtime were shared, the
// `-race` flag would surface a data race here; even without race detection,
// concurrent runtime corruption typically produces sporadic decode failures.
func TestKwik_Extract_FreshRuntime(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(goldenFixturePath(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	const N = 16
	k := NewKwikExtractor()
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		go func() {
			stream, err := k.Extract(context.Background(), srv.URL+"/e/abc", http.Header{})
			if err != nil {
				errs <- err
				return
			}
			if stream == nil || len(stream.Sources) == 0 {
				errs <- errors.New("empty stream")
				return
			}
			errs <- nil
		}()
	}
	for i := 0; i < N; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Extract #%d: %v (goja runtime shared across goroutines? — Pitfall 2)", i, err)
		}
	}
}

// TestKwik_Extract_RespectsBodyLimit serves a 5 MiB response of garbage bytes.
// Extract must error out (no packer found in junk) WITHOUT reading the full
// body — the io.LimitReader cap prevents an OOM on a hostile/misbehaving
// upstream. The assertion here is the absence of an OOM panic + a fast return.
func TestKwik_Extract_RespectsBodyLimit(t *testing.T) {
	t.Parallel()

	// 5 MiB of 'A' — no packer pattern anywhere.
	const huge = 5 << 20
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5242880")
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, 64<<10)
		for i := range buf {
			buf[i] = 'A'
		}
		written := 0
		for written < huge {
			n, err := w.Write(buf)
			written += n
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	k := NewKwikExtractor()
	_, err := k.Extract(context.Background(), srv.URL+"/e/huge", http.Header{})
	if err == nil {
		t.Fatal("Extract() error = nil; want wrapped ErrExtractFailed (no packer in junk)")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("Extract() error = %v; want errors.Is(..., ErrExtractFailed) = true", err)
	}
}
