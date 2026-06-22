package videoutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if tr.ResponseHeaderTimeout != 20*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 20s (finding L781)", tr.ResponseHeaderTimeout)
	}
}

// TestProxyStream_ResponseHeaderTimeoutFires is the functional proof for finding
// L781: an upstream that completes TCP+TLS but never sends response headers must
// NOT hang the proxy forever — the transport's ResponseHeaderTimeout aborts the
// upstream request and ProxyStreamCounted returns an error, freeing the slot.
//
// The constructor sets a generous 20s in production; here we shrink the timeout
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
