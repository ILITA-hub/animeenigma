package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouter_StreamingInternalNotProxied asserts the gateway does NOT proxy
// /api/streaming/internal/* to the streaming service. That subtree contains the
// stream-token minter (streaming /api/v1/internal/token, service-to-service);
// proxying it through the public gateway exposed an unauthenticated token
// minter to the internet. Per the platform convention, every /internal/*
// endpoint is Docker-network-only and MUST NOT be gateway-proxied.
func TestRouter_StreamingInternalNotProxied(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/api/streaming/internal/token"},
		{http.MethodGet, "/api/streaming/internal/token"},
		{http.MethodGet, "/api/streaming/internal/anything/else"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "10.0.0.30:1234"
		rec := httptest.NewRecorder()
		gw.router.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Fatalf("%s %s was proxied (status=%d) — streaming /internal/* MUST NOT be gateway-proxied", tc.method, tc.path, rec.Code)
		}
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: expected 404/405 (no route); got %d (body=%q)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}
