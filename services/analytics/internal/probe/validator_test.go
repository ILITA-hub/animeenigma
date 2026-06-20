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
