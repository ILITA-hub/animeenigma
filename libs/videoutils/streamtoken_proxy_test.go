package videoutils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A preauth call (token already decoded by the handler) must serve a host
// that is NOT in the static allowlist and carries NO exp/sig query params.
func TestProxyPreauthCounted_BypassesAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("SEGMENTDATA"))
	}))
	defer upstream.Close()

	proxy := NewVideoProxy(DefaultProxyConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/m/sometoken/seg-1.ts", nil)
	rec := httptest.NewRecorder()

	_, _, err := proxy.ProxyPreauthCounted(req.Context(), upstream.URL+"/seg-1.ts", "", rec, req)
	if err != nil {
		t.Fatalf("preauth proxy failed: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "SEGMENTDATA") {
		t.Fatal("upstream body not forwarded")
	}
}

// The plain path must still enforce the gate (no regression).
func TestProxyWithRefererCounted_GateStillEnforced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer upstream.Close()

	proxy := NewVideoProxy(DefaultProxyConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-proxy?url="+upstream.URL, nil)
	rec := httptest.NewRecorder()

	_, _, err := proxy.ProxyWithRefererCounted(req.Context(), upstream.URL+"/x.ts", "", rec, req)
	if err == nil {
		t.Fatal("unsigned non-allowlisted host must be rejected on the legacy path")
	}
}
