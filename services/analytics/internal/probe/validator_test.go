package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type fakeProber struct{ err error }

func (f fakeProber) Probe(_ context.Context, _ []byte) error { return f.err }

func newStreamingStub(t *testing.T, masterStatus int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.Query().Get("url")
		switch {
		case masterStatus != 200 && strings.Contains(url, "master"):
			w.WriteHeader(masterStatus)
			w.Write([]byte("blocked"))
		case strings.Contains(url, "master"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n/api/streaming/hls-proxy?url=variant\n"))
		case strings.Contains(url, "variant"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXTINF:4,\n/api/streaming/hls-proxy?url=seg0\n"))
		default:
			w.Write([]byte("BINARYSEGMENTDATA"))
		}
	}))
}

func TestValidator_Playable(t *testing.T) {
	s := newStreamingStub(t, 200)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8", Provider: "p"})
	if got.Reason != streamprobe.ReasonPlayable {
		t.Fatalf("want playable, got %s", got.Reason)
	}
}

func TestValidator_403(t *testing.T) {
	s := newStreamingStub(t, 403)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8"})
	if got.Reason != streamprobe.ReasonStatus403 {
		t.Fatalf("want status_403, got %s", got.Reason)
	}
}

// newMP4Stub serves the upstream as a progressive mp4 (NOT an HLS manifest):
// the first bytes are an ISO-BMFF ftyp box, so the validator must ffprobe the
// fetched head directly instead of trying to walk a manifest chain.
func newMP4Stub(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Write([]byte("\x00\x00\x00\x20ftypisomisom...mp4bytes"))
	}))
}

func TestValidator_ProgressiveMP4Playable(t *testing.T) {
	s := newMP4Stub(t)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{
		MasterURL: "https://video.sibnet.ru/v/abc/5.mp4", Provider: "animejoy-sibnet",
		Exp: "1", Sig: "a", Referer: "https://video.sibnet.ru/",
	})
	if got.Reason != streamprobe.ReasonPlayable {
		t.Fatalf("want playable for decodable mp4, got %s", got.Reason)
	}
}

func TestValidator_ProgressiveMP4DecodeFailed(t *testing.T) {
	s := newMP4Stub(t)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{err: errors.New("no video stream")})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://x/y.mp4", Provider: "animejoy-allvideo"})
	if got.Reason != streamprobe.ReasonDecodeFailed {
		t.Fatalf("want decode_failed for undecodable mp4, got %s", got.Reason)
	}
}

func TestValidator_DecodeFailed(t *testing.T) {
	s := newStreamingStub(t, 200)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{err: errors.New("no video")})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8"})
	if got.Reason != streamprobe.ReasonDecodeFailed {
		t.Fatalf("want decode_failed, got %s", got.Reason)
	}
}

// TestValidator_InnerFetchTransportError verifies that a transport-layer failure
// on an inner hop (variant/segment fetch) maps to ReasonCDNUnreachable, NOT
// ReasonStatus403. The master manifest is served successfully; the variant
// request is handled by hijacking and immediately closing the connection so the
// HTTP client sees a transport error.
func TestValidator_InnerFetchTransportError(t *testing.T) {
	// A single httptest server handles both hops:
	//   - master request  (url param contains "master"): returns a valid HLS manifest
	//     whose variant line is an absolute /api/streaming/ path so proxyURL routes
	//     the inner hop back through this same server.
	//   - variant request (url param contains "variant"): hijacks the connection and
	//     closes it immediately, causing the HTTP client to get a transport error.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.Query().Get("url")
		if strings.Contains(u, "master") {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n/api/streaming/hls-proxy?url=variant\n"))
			return
		}
		// Inner hop: abruptly close the connection to produce a transport error.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("httptest.Server does not support http.Hijacker")
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer s.Close()

	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{
		MasterURL: "https://cdn/master.m3u8",
		Provider:  "p",
	})
	if got.Reason != streamprobe.ReasonCDNUnreachable {
		t.Fatalf("want cdn_unreachable for inner transport error, got %s", got.Reason)
	}
}

// TestValidator_AES128SkipsFFprobe verifies that AES-128 encrypted segments are
// accepted without the video-decode gate. kiwi (miruro's inner provider) serves
// AES-128 HLS; encrypted bytes look like random noise to ffprobe, so the probe
// would always return decode_failed even for healthy streams. The validator now
// detects the #EXT-X-KEY:METHOD=AES-128 tag and trusts reachability instead.
func TestValidator_AES128SkipsFFprobe(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.Query().Get("url")
		switch {
		case strings.Contains(u, "master"):
			// Flat AES-128 playlist (no variant level — kiwi omits #EXT-X-STREAM-INF).
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"/api/streaming/hls-proxy?url=key\"\n#EXTINF:4,\n/api/streaming/hls-proxy?url=seg0\n"))
		default:
			// seg0 — opaque ciphertext (from ffprobe's perspective).
			w.Write([]byte("ENCRYPTEDCIPHERTEXTDATA"))
		}
	}))
	defer s.Close()

	// prober returns an error as it would for encrypted bytes; fix must skip it.
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{err: errors.New("no video stream")})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8", Provider: "miruro", Server: "kiwi"})
	if got.Reason != streamprobe.ReasonPlayable {
		t.Fatalf("want playable for AES-128 stream, got %s", got.Reason)
	}
}

// TestValidator_UsesNativeProxyPath is the regression test for the deploy bug:
// the probe calls streaming DIRECTLY, so it must hit the native /api/v1/hls-proxy
// route, not the public /api/streaming/hls-proxy path the gateway rewrites. This
// stub serves ONLY the native route (404 elsewhere, exactly like prod) and emits
// child URLs as the public path the real proxy rewrites them to — the validator
// must remap those children to the native route too, or the variant/segment hops
// 404 and the verdict is a false empty_response.
func TestValidator_UsesNativeProxyPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/hls-proxy" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found\n"))
			return
		}
		u := r.URL.Query().Get("url")
		switch {
		case strings.Contains(u, "master"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n/api/streaming/hls-proxy?url=variant\n"))
		case strings.Contains(u, "variant"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:4,\n/api/streaming/hls-proxy?url=seg0\n"))
		default:
			_, _ = w.Write([]byte("BINARYSEGMENTDATA"))
		}
	}))
	defer s.Close()

	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{
		MasterURL: "http://minio:9000/raw-library/x/master.m3u8", Provider: "ae", Exp: "1", Sig: "a",
	})
	if got.Reason != streamprobe.ReasonPlayable {
		t.Fatalf("want playable via native /api/v1/hls-proxy path; got %s", got.Reason)
	}
}

// TestValidator_RemapsMaskedChildToNativeRoute is the regression test for the
// masked-token double-proxy bug (found 2026-07-12 while recovering miruro):
// proxyURL only remapped the LEGACY /api/streaming/hls-proxy child prefix to
// the native route — it never learned about the Track A opaque masked-token
// prefix (/api/streaming/m/<token>/<leaf>), live since 2026-07-11. A masked
// child fell through to the generic `url=` wrapper and got double-proxied: a
// bare relative path (no host) passed as sourceURL to the legacy endpoint,
// which is deterministically rejected by the domain allowlist. Since masked
// children are always-on, this hit the variant/segment hop of EVERY HLS
// provider's health probe on EVERY tick, misreported as ReasonEmptyResponse
// regardless of real CDN health — the root cause behind several providers'
// (miruro, animepahe, nineanime, gogoanime) false "down" flips since 07-11.
func TestValidator_RemapsMaskedChildToNativeRoute(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/hls-proxy":
			u := r.URL.Query().Get("url")
			if strings.Contains(u, "master") {
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:4,\n/api/streaming/m/FAKETOKEN/segment-1.ts?sess=abc\n"))
				return
			}
			// A double-proxied masked child lands here pre-fix — mirror the real
			// proxy's DomainNotAllowedError (502) for a bare relative sourceURL.
			w.WriteHeader(http.StatusBadGateway)
		case strings.HasPrefix(r.URL.Path, "/api/v1/m/"):
			if r.URL.Path != "/api/v1/m/FAKETOKEN/segment-1.ts" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte("BINARYSEGMENTDATA"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer s.Close()

	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{
		MasterURL: "http://minio:9000/raw-library/x/master.m3u8", Provider: "miruro", Server: "kiwi",
	})
	if got.Reason != streamprobe.ReasonPlayable {
		t.Fatalf("want playable via native masked route /api/v1/m/...; got %s", got.Reason)
	}
}

// okProber is a VideoProber that always accepts (decode gate off for the test).
type okProber struct{}

func (okProber) Probe(_ context.Context, _ []byte) error { return nil }

func TestValidatePopulatesMetrics(t *testing.T) {
	const master = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\nvariant.m3u8\n"
	const variant = "#EXTM3U\n#EXTINF:5.0,\nseg-1.ts\n"
	segment := strings.Repeat("A", 200000) // 200 KB "segment"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("url")
		switch {
		case strings.HasSuffix(raw, "master.m3u8"):
			_, _ = w.Write([]byte(master))
		case strings.HasSuffix(raw, "variant.m3u8"):
			_, _ = w.Write([]byte(variant))
		default: // seg-1.ts
			_, _ = w.Write([]byte(segment))
		}
	}))
	defer srv.Close()

	v := NewHTTPValidator(srv.URL, srv.Client(), okProber{})
	rs := ResolvedStream{Provider: "miruro", MasterURL: "https://cdn.example.test/master.m3u8"}
	got := v.Validate(context.Background(), rs)

	if !got.Playable() {
		t.Fatalf("expected playable, got reason=%q", got.Reason)
	}
	if got.ManifestMs < 0 || got.SegmentBytes != int64(len(segment)) {
		t.Fatalf("bad measures: ManifestMs=%d SegmentBytes=%d", got.ManifestMs, got.SegmentBytes)
	}
	if got.CDNHost != "cdn.example.test" {
		t.Fatalf("CDNHost = %q, want cdn.example.test", got.CDNHost)
	}
	if got.Quality != "1080p" {
		t.Fatalf("Quality = %q, want 1080p", got.Quality)
	}
}
