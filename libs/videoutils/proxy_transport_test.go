package videoutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestNewIPv4Transport_ConnectionPoolConfig locks the keep-alive pool sizing on
// the hottest path (per-segment HLS fetch). Finding L417: the cloned default
// transport leaves MaxIdleConnsPerHost at Go's default of 2, which forces a
// re-dial + re-TLS on all but two idle conns to the same CDN host during the
// steady flood of segment GETs. We raise it to 64 while keeping the global
// MaxIdleConns=100 default. Finding L781: the constructor must also bound the
// time-to-first-header phase with ResponseHeaderTimeout so a hung-headers
// upstream cannot pin a proxy slot indefinitely.
func TestNewIPv4Transport_ConnectionPoolConfig(t *testing.T) {
	tr := newIPv4Transport(nil)

	if tr.MaxIdleConnsPerHost != 64 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 64 (finding L417)", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxIdleConns != 100 {
		t.Fatalf("MaxIdleConns = %d, want 100 (Go default, must be preserved)", tr.MaxIdleConns)
	}
	// 45s (raised from 20s, 2026-07-10 Kodik edge-failover design): a solodcdn
	// edge can be slow to first byte while it cold-starts / prepares HLS — we'd
	// rather wait than prematurely abandon a slow-but-alive edge.
	if tr.ResponseHeaderTimeout != 45*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 45s (finding L781; widened for cold-start edges)", tr.ResponseHeaderTimeout)
	}
}

// TestProxyStream_ResponseHeaderTimeoutFires is the functional proof for finding
// L781: an upstream that completes TCP+TLS but never sends response headers must
// NOT hang the proxy forever — the transport's ResponseHeaderTimeout aborts the
// upstream request and ProxyStreamCounted returns an error, freeing the slot.
//
// The constructor sets a generous 45s in production; here we shrink the timeout
// on the same transport to keep the test fast while still exercising the real
// client.Do error path through ProxyStreamCounted.
func TestProxyStream_ResponseHeaderTimeoutFires(t *testing.T) {
	// Backend accepts the request but blocks without ever writing headers.
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release // never write headers until released; simulates a hung upstream
	}))
	defer upstream.Close()
	defer close(release)

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	// Shrink the header timeout on the live transport so the test resolves fast
	// (production value asserted separately in the constructor test above).
	if tr, ok := proxy.client.Transport.(*http.Transport); ok {
		tr.ResponseHeaderTimeout = 300 * time.Millisecond
	} else {
		t.Fatalf("proxy transport is not *http.Transport")
	}

	sourceURL := upstream.URL + "/playlist.m3u8"
	exp, sig := signProvenance(sourceURL, time.Now())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	done := make(chan error, 1)
	go func() {
		_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a header-timeout error from a hung-headers upstream, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ProxyWithRefererCounted hung: ResponseHeaderTimeout did not abort the upstream request")
	}
}

// TestProxyStream_FastUpstreamSucceeds is the control: a backend that writes
// headers promptly streams normally (the header timeout does not false-positive
// on a healthy fast upstream).
func TestProxyStream_FastUpstreamSucceeds(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\nseg-0.ts\n"))
	}))
	defer upstream.Close()

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	sourceURL := upstream.URL + "/playlist.m3u8"
	exp, sig := signProvenance(sourceURL, time.Now())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	if _, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req); err != nil {
		t.Fatalf("fast upstream should proxy without error, got: %v", err)
	}
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Result().StatusCode)
	}
}

// TestProxyStream_MixedCaseMpegURLContentTypeIsRewritten is the regression
// proof for the okru media-playlist bug: okcdn.ru serves path-style variant
// playlists (no .m3u8 suffix — the quality is baked into the path itself,
// e.g. ".../type/2/video/") with a mixed-case "application/x-mpegURL"
// Content-Type. isM3U8's detection used strings.Contains with an
// all-lowercase needle, which is case-sensitive in Go, so it silently missed
// the capitalized "mpegURL" and streamed the response as opaque bytes — the
// relative segment URIs inside never got rewritten to go through the proxy,
// breaking playback for any client resolving them against the CDN host
// instead of the proxy origin.
func TestProxyStream_MixedCaseMpegURLContentTypeIsRewritten(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No .m3u8 suffix on the path — Content-Type is the only signal.
		w.Header().Set("Content-Type", "application/x-mpegURL")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\nseg-0.ts\n"))
	}))
	defer upstream.Close()

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	sourceURL := upstream.URL + "/expires/1/type/2/video/"
	exp, sig := signProvenance(sourceURL, time.Now())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	if _, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req); err != nil {
		t.Fatalf("proxy should not error, got: %v", err)
	}
	body := rec.Body.String()
	if strings.HasSuffix(strings.TrimSpace(body), "\nseg-0.ts") || strings.TrimSpace(body) == "#EXTM3U\n#EXTINF:6.0,\nseg-0.ts" {
		t.Fatalf("segment URI was not rewritten through the proxy (case-sensitive Content-Type check regression): %q", body)
	}
	if !strings.Contains(body, "/api/streaming/hls-proxy?url=") {
		t.Fatalf("expected rewritten segment to route through the proxy, got: %q", body)
	}
}
