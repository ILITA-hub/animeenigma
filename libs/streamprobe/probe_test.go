package streamprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// tsSegmentBytes is a minimal MPEG-TS-looking payload: sync byte 0x47
// followed by filler. Enough for sniffSegmentBytes to classify as media.
func tsSegmentBytes() []byte {
	b := make([]byte, 188)
	b[0] = 0x47
	return b
}

// TestProbe_Playable: master returns a master playlist whose first
// variant points at /variant_720.m3u8 (served by the same test server),
// which lists relative segments. Ranged GET on /seg/001.ts returns 206
// with MPEG-TS bytes.
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
		if r.Method != http.MethodGet {
			http.Error(w, "expected ranged GET", 405)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(tsSegmentBytes())
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

// TestProbe_AdDecoy: the segment host answers the ranged GET with 200 +
// PNG magic bytes dressed up with a video/mp2t Content-Type — the exact
// nekostream production poison. The byte sniff (which replaced the retired
// static ad-CDN blocklist) must convict it as ReasonAdDecoy and surface
// the offending host in DecoyHost so callers can cache the verdict, and
// must cost exactly ONE ranged GET on the segment.
func TestProbe_AdDecoy(t *testing.T) {
	var segHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		// A MEDIA playlist (no #EXT-X-STREAM-INF) with one relative segment.
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6.0,\n/seg/001.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&segHits, 1)
		// Content-Type lies (video/mp2t) — only the bytes are trusted.
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(200)
		_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) // PNG magic
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
	tsu, _ := url.Parse(ts.URL)
	if got.DecoyHost != tsu.Hostname() {
		t.Fatalf("DecoyHost=%q; want %q (the poisoned segment host)", got.DecoyHost, tsu.Hostname())
	}
	if atomic.LoadInt32(&segHits) != 1 {
		t.Fatalf("segment received %d hits; want exactly 1 ranged GET", segHits)
	}
	// Sampled must include the poisoned host for diagnostics.
	found := false
	for _, h := range got.Sampled {
		if h == tsu.Hostname() {
			found = true
		}
	}
	if !found {
		t.Fatalf("Sampled=%v; want at least one entry equal to %q", got.Sampled, tsu.Hostname())
	}
}

// TestProbe_AdDecoy_HTML: the segment serves an HTML page (ad landing /
// dead-mirror placeholder) — convicted as ReasonAdDecoy by the byte sniff.
func TestProbe_AdDecoy_HTML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\n/seg/001.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("\n  <!DOCTYPE html><html><body>ads</body></html>"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if got.Reason != ReasonAdDecoy {
		t.Fatalf("Reason=%q; want %q. Result=%+v", got.Reason, ReasonAdDecoy, got)
	}
	if got.DecoyHost == "" {
		t.Fatalf("DecoyHost empty; want the segment host. Result=%+v", got)
	}
}

// TestProbe_UnknownMagic_FailOpen: the segment answers 200 with bytes that
// match neither media nor poison magic. The probe must FAIL OPEN (playable)
// — never brick a weird-but-real CDN on an incomplete magic table.
func TestProbe_UnknownMagic_FailOpen(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\n/seg/001.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if !got.Playable {
		t.Fatalf("Playable=false; want true (unknown magic fails open). Result=%+v", got)
	}
	if got.Reason != ReasonPlayable {
		t.Fatalf("Reason=%q; want %q", got.Reason, ReasonPlayable)
	}
}

// TestSniffSegmentBytes: table-driven contract for the magic-byte sniffer.
func TestSniffSegmentBytes(t *testing.T) {
	ftyp := append([]byte{0x00, 0x00, 0x00, 0x20}, []byte("ftypisom")...)
	moof := append([]byte{0x00, 0x00, 0x00, 0x18}, []byte("moof")...)
	cases := []struct {
		name string
		in   []byte
		want segmentVerdict
	}{
		{"mpeg-ts sync", tsSegmentBytes(), segmentMedia},
		{"fmp4 ftyp", ftyp, segmentMedia},
		{"fmp4 moof", moof, segmentMedia},
		{"ebml webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0x01}, segmentMedia},
		{"id3-prefixed", []byte("ID3\x04\x00"), segmentMedia},
		{"aac adts", []byte{0xFF, 0xF1, 0x50, 0x80}, segmentMedia},
		{"png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}, segmentPoison},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, segmentPoison},
		{"gif", []byte("GIF89a"), segmentPoison},
		{"html doctype", []byte("<!DOCTYPE html><html>"), segmentPoison},
		{"html tag after whitespace+bom", append(append([]byte{}, utf8BOM...), []byte("  \n<HTML>")...), segmentPoison},
		{"empty", nil, segmentUnknown},
		{"unknown magic", []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}, segmentUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sniffSegmentBytes(tc.in); got != tc.want {
				t.Fatalf("sniffSegmentBytes(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
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

// TestProbe_Segment_403: master + variant 200 m3u8, but the ranged GET on
// the first segment returns 403 → status_403.
func TestProbe_Segment_403(t *testing.T) {
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

// TestProbe_SegmentRangedGETFirst: the first-segment check must issue a
// ranged GET as its PRIMARY probe (HEAD produced false negatives on
// HEAD-hostile CDNs — finding L718) and must NOT send any HEAD when the
// GET succeeds.
func TestProbe_SegmentRangedGETFirst(t *testing.T) {
	var headHits, getHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "playable_master.m3u8"))
	})
	mux.HandleFunc("/variant_720.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "playable_variant.m3u8"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			atomic.AddInt32(&headHits, 1)
			w.WriteHeader(200)
		case http.MethodGet:
			atomic.AddInt32(&getHits, 1)
			if r.Header.Get("Range") == "" {
				t.Errorf("primary GET must carry a Range header")
			}
			w.WriteHeader(http.StatusPartialContent) // 206
			_, _ = w.Write(tsSegmentBytes())
		default:
			http.Error(w, "unexpected", 400)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if !got.Playable {
		t.Fatalf("Playable=false; want true. Result=%+v", got)
	}
	if atomic.LoadInt32(&getHits) != 1 {
		t.Fatalf("GET hits=%d; want 1 (ranged GET is the primary probe)", getHits)
	}
	if atomic.LoadInt32(&headHits) != 0 {
		t.Fatalf("HEAD hits=%d; want 0 (no HEAD when the ranged GET succeeds)", headHits)
	}
}

// TestProbe_SegmentGETHostile_HEADFallback: a GET-hostile segment host
// answers the ranged GET with 405 but a bare HEAD with 200 — the probe
// must fall back to HEAD and classify Playable (fail-open, no bytes).
func TestProbe_SegmentGETHostile_HEADFallback(t *testing.T) {
	var headHits, getHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\n/seg/001.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			atomic.AddInt32(&getHits, 1)
			w.WriteHeader(http.StatusMethodNotAllowed) // 405 — GET-hostile
		case http.MethodHead:
			atomic.AddInt32(&headHits, 1)
			w.WriteHeader(200)
		default:
			http.Error(w, "unexpected", 400)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	got := Probe(context.Background(), ts.URL+"/master.m3u8", nil)
	if !got.Playable {
		t.Fatalf("Playable=false; want true (405-on-GET must fall back to HEAD). Result=%+v", got)
	}
	if atomic.LoadInt32(&getHits) != 1 || atomic.LoadInt32(&headHits) != 1 {
		t.Fatalf("GET hits=%d HEAD hits=%d; want 1 and 1", getHits, headHits)
	}
}

// TestProbe_SegmentGET_SignedExpired: the segment's ranged GET returns 403
// and the URL carries an expired `?e=<epoch>` — routed through classify403
// → signed_url_expired (must still be distinguished from generic 403s).
func TestProbe_SegmentGET_SignedExpired(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		// Media playlist whose single segment is an absolute URL with an expired
		// signed-URL epoch query param (so classify403 sees the `?e=` token).
		seg := srv.URL + "/seg/001.ts?e=1000000000"
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\n" + seg + "\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg/001.ts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403) // both HEAD and GET return 403
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	got := Probe(context.Background(), srv.URL+"/master.m3u8", nil)
	if got.Reason != ReasonSignedURLExpired {
		t.Fatalf("Reason=%q; want %q (segment GET 403 must still classify signed-url-expired). Result=%+v", got.Reason, ReasonSignedURLExpired, got)
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
	// decode_failed / invalid_video are part of the public Reason enum but are
	// NOT emitted by the Probe() primitive itself — they are classified by
	// higher-level consumers (the analytics playability validator, which decodes
	// the actual video bytes). The HLS probe in this package never returns them,
	// so there is no TestProbe_* case to map; they are covered by the consumer's
	// own tests, not this primitive's.
	consumerEmitted := map[Reason]bool{
		ReasonDecodeFailed: true,
		ReasonInvalidVideo: true,
	}
	for _, r := range AllReasons() {
		if consumerEmitted[r] {
			continue
		}
		if _, ok := covered[r]; !ok {
			t.Fatalf("Reason %q not covered by any TestProbe_* test", r)
		}
	}
}
