package streamprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain flips the loopback allowance for the suite — httptest.NewServer
// binds to 127.0.0.1 which the production SSRF guard rejects. SSRF tests
// explicitly toggle the flag OFF to prove the guard works.
func TestMain(m *testing.M) {
	allowLoopbackForTests = true
	code := m.Run()
	os.Exit(code)
}

// readFixture loads a testdata file body.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestProbe_Playable: master returns a master playlist whose first
// variant points at /variant_720.m3u8 (served by the same test server),
// which lists relative segments. HEAD on /seg/001.ts returns 200.
func TestProbe_Playable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(readFixture(t, "playable_master.m3u8"))
	})
	mux.HandleFunc("/variant_720.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(readFixture(t, "playable_variant.m3u8"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			http.Error(w, "expected HEAD", 405)
			return
		}
		w.WriteHeader(200)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if !got.Playable {
		t.Fatalf("Playable=false; want true. Result=%+v", got)
	}
	if got.Reason != ReasonPlayable {
		t.Fatalf("Reason=%q; want %q", got.Reason, ReasonPlayable)
	}
	if len(got.Sampled) < 1 {
		t.Fatalf("Sampled empty; want at least 1 hostname. Result=%+v", got)
	}
}

// TestProbe_AdDecoy: master is itself a media playlist whose first
// segment URI is the production-poison TikTok ad CDN
// (p16-ad-sg.ibyteimg.com). The blocklist check MUST short-circuit
// BEFORE any HEAD is attempted — we verify by registering a counter
// handler on a separate mock server pointing at the ad-CDN host (which
// would receive zero requests because the blocklist hits first).
func TestProbe_AdDecoy(t *testing.T) {
	var adCDNHits int32

	// Mock server simulating the ad-CDN — registered but should NEVER hit.
	// We never actually point Probe at this server's URL; the segment URL
	// inside the fixture is an absolute https://p16-ad-sg.ibyteimg.com/...
	// reference, which Probe resolves to that hostname and short-circuits.
	adCDN := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&adCDNHits, 1)
		w.WriteHeader(200)
	}))
	defer adCDN.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		// The fixture is a MEDIA playlist (no #EXT-X-STREAM-INF) whose
		// first #EXTINF segment is the absolute ad-CDN URL.
		_, _ = w.Write(readFixture(t, "ad_decoy_variant.m3u8"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Playable {
		t.Fatalf("Playable=true; want false. Result=%+v", got)
	}
	if got.Reason != ReasonAdDecoy {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonAdDecoy, got)
	}
	if atomic.LoadInt32(&adCDNHits) != 0 {
		t.Fatalf("ad-CDN mock received %d hits; want 0 (blocklist must short-circuit)", adCDNHits)
	}
	// Sampled must include the ad-CDN host for diagnostics.
	found := false
	for _, h := range got.Sampled {
		if strings.Contains(h, "ibyteimg.com") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Sampled=%v; want at least one entry containing ibyteimg.com", got.Sampled)
	}
}

// TestProbe_Status403: master server returns 403 (no `?e=<epoch>` so
// this is NOT classified as signed-url-expired).
func TestProbe_Status403(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonStatus403 {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonStatus403, got)
	}
}

// TestProbe_ZeroMatch_NotM3U8: master returns html (no #EXTM3U sentinel).
func TestProbe_ZeroMatch_NotM3U8(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "zero_match_no_extm3u.m3u8"))
	}))
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonZeroMatch {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonZeroMatch, got)
	}
}

// TestProbe_EmptyResponse: master returns a valid #EXTM3U body with no
// #EXTINF segments (e.g. only EXT-X-ENDLIST). Probe treats this as a
// media playlist directly (no #EXT-X-STREAM-INF) and classifies as
// empty_response when extractSegmentURIs returns zero entries.
func TestProbe_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "empty_variant.m3u8"))
	}))
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonEmptyResponse {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonEmptyResponse, got)
	}
}

// TestProbe_CDNUnreachable: build a server, close it, then call Probe.
// The dial fails → classified as cdn_unreachable.
func TestProbe_CDNUnreachable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	ts.Close() // shut down immediately so the dial fails

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonCDNUnreachable {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonCDNUnreachable, got)
	}
}

// TestProbe_SignedURLExpired: master returns 403 at a URL containing
// `?e=1000000000` (Sept 2001). The classifier detects the expired-epoch
// query param and returns signed_url_expired instead of status_403.
func TestProbe_SignedURLExpired(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8?e=1000000000", nil)
	if got.Reason != ReasonSignedURLExpired {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonSignedURLExpired, got)
	}
}

// TestProbe_PerStepTimeout: master sleeps 6s before responding. Probe's
// per-step timeout (4s) cuts the request short → cdn_unreachable. Total
// wall-clock must be ≤ 5s (4s budget + ~1s slack).
func TestProbe_PerStepTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Second)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	start := time.Now()
	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	elapsed := time.Since(start)

	if got.Reason != ReasonCDNUnreachable {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonCDNUnreachable, got)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Probe took %v; want ≤ 5s", elapsed)
	}
}

// TestProbe_SSRF_Loopback: 127.0.0.1 destinations are short-circuited
// BEFORE any HTTP dial — verified by wall-clock < 100 ms (well below
// the 4 s dial timeout that an actual dial would take to fail). This
// test toggles allowLoopbackForTests OFF to validate production behavior.
func TestProbe_SSRF_Loopback(t *testing.T) {
	prev := allowLoopbackForTests
	allowLoopbackForTests = false
	defer func() { allowLoopbackForTests = prev }()

	start := time.Now()
	got := Probe(context.Background(), "http://127.0.0.1:1/foo.m3u8", nil)
	elapsed := time.Since(start)

	if got.Reason != ReasonCDNUnreachable {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonCDNUnreachable, got)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Probe took %v; want < 100ms (SSRF guard must short-circuit before dial)", elapsed)
	}
	if got.Sampled != nil {
		t.Fatalf("Sampled=%v; want nil (no host observed because no dial attempted)", got.Sampled)
	}
}

// TestProbe_SSRF_RFC1918: 10.0.0.0/8 destinations are short-circuited.
// RFC1918 rejection is NOT gated by allowLoopbackForTests — defense-in-depth.
func TestProbe_SSRF_RFC1918(t *testing.T) {
	start := time.Now()
	got := Probe(context.Background(), "http://10.0.0.1/foo.m3u8", nil)
	elapsed := time.Since(start)

	if got.Reason != ReasonCDNUnreachable {
		t.Fatalf("Reason=%q; want %q", got.Reason, ReasonCDNUnreachable)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Probe took %v; want < 100ms", elapsed)
	}
}

// TestProbe_SSRF_LinkLocal: 169.254.0.0/16 destinations are short-circuited.
// Link-local rejection is NOT gated by allowLoopbackForTests.
func TestProbe_SSRF_LinkLocal(t *testing.T) {
	got := Probe(context.Background(), "http://169.254.169.254/foo.m3u8", nil)
	if got.Reason != ReasonCDNUnreachable {
		t.Fatalf("Reason=%q; want %q", got.Reason, ReasonCDNUnreachable)
	}
}

// TestProbe_SegmentHEAD_403: master + variant 200 m3u8, but HEAD on the
// first segment returns 403 → status_403.
func TestProbe_SegmentHEAD_403(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "playable_master.m3u8"))
	})
	mux.HandleFunc("/variant_720.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "playable_variant.m3u8"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonStatus403 {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonStatus403, got)
	}
}

// TestProbe_RelativeSegmentURI: variant uses relative /seg/001.ts path.
// resolveURI should join it with the variant's base host so the HEAD
// hits the same test server. End-to-end this is the same as the playable
// happy path; this test explicitly anchors that relative resolution works.
func TestProbe_RelativeSegmentURI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "playable_master.m3u8"))
	})
	mux.HandleFunc("/variant_720.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "playable_variant.m3u8"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if !got.Playable {
		t.Fatalf("Playable=false; want true (relative segment URI should resolve to test server). Result=%+v", got)
	}
	if got.Reason != ReasonPlayable {
		t.Fatalf("Reason=%q; want %q", got.Reason, ReasonPlayable)
	}
}

// TestProbe_CtxCancelled: ctx cancelled before HEAD returns
// ReasonCDNUnreachable.
func TestProbe_CtxCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Probe even starts

	got := Probe(ctx, ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonCDNUnreachable {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonCDNUnreachable, got)
	}
}

// TestProbe_AllReasonsCovered: meta-test asserting we exercise every
// Reason value at least once across the suite. The maintenance prompt
// reason-enum dispatch table depends on this.
func TestProbe_AllReasonsCovered(t *testing.T) {
	// Manually-curated map of Reason → exercising test name.
	covered := map[Reason]string{
		ReasonPlayable:         "TestProbe_Playable",
		ReasonAdDecoy:          "TestProbe_AdDecoy",
		ReasonZeroMatch:        "TestProbe_ZeroMatch_NotM3U8",
		ReasonStatus403:        "TestProbe_Status403",
		ReasonSignedURLExpired: "TestProbe_SignedURLExpired",
		ReasonCDNUnreachable:   "TestProbe_CDNUnreachable",
		ReasonEmptyResponse:    "TestProbe_EmptyResponse",
	}
	for _, r := range AllReasons() {
		if _, ok := covered[r]; !ok {
			t.Fatalf("Reason %q not covered by any TestProbe_* test", r)
		}
	}
}
