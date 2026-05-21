package transport

// Workstream hero-spotlight, v1.0 Phase 1 (HSB-BE-06) — gateway proxy contract
// tests for GET /api/home/spotlight.
//
// These tests pin two invariants that must not regress:
//
//  1. The route is registered inside the /api Route block and forwards to the
//     catalog backend (not player, scraper, web, etc.) via ProxyToCatalog. The
//     catalog backend receives the path unchanged at /api/home/spotlight (the
//     catalog proxy path-rewrite is a no-op — see services/gateway/internal/
//     service/proxy.go Forward(...) switch — so the catalog router can mount
//     the endpoint at the same URL it was called with).
//
//  2. The route is NOT behind JWTValidationMiddleware. A request with NO
//     Authorization header must reach the catalog backend (not bounce off
//     401 at the gateway). Phase 1 is intentionally public; if a future
//     personalization phase needs auth it will be optional-auth on the
//     catalog side, not enforced auth at the gateway.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRouter_Spotlight_ProxiesToCatalog asserts the route is registered and
// the request reaches the catalog backend at the expected path. We re-use the
// shared buildTestGatewayRouter helper (defined in router_test.go) which spins
// up two httptest backends — one for catalog, one for scraper — so we can
// prove the request landed on catalog and NOT on scraper.
func TestRouter_Spotlight_ProxiesToCatalog(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	req.RemoteAddr = "10.0.0.10:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	// Catalog backend MUST receive the request, at the same path.
	select {
	case got := <-gw.catalogGotURL:
		if got != "/api/home/spotlight" {
			t.Errorf("catalog backend received path = %q; want /api/home/spotlight", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("catalog backend never received the request — is the /home/spotlight route registered inside /api?")
	}

	// Scraper backend must NOT have been called.
	select {
	case got := <-gw.scraperGotURL:
		t.Errorf("scraper backend received unexpected request: %q (route should go to catalog, not scraper)", got)
	default:
	}

	// Body should be the catalog stub's success envelope, confirming the
	// gateway streamed the response back to the client unchanged.
	if !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Errorf("response body did not contain catalog stub payload; got %q", rec.Body.String())
	}
}

// TestRouter_Spotlight_NotJWTProtected asserts the route is publicly
// accessible without an Authorization header. If somebody accidentally nests
// the registration inside a JWT-gated r.Group, this test catches it because
// the gateway would return 401 (JWTValidationMiddleware) before the catalog
// backend is ever called.
func TestRouter_Spotlight_NotJWTProtected(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	// Note: NO Authorization header.
	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	req.RemoteAddr = "10.0.0.11:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	// If the route were JWT-gated, we'd see 401 here.
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("/api/home/spotlight returned 401 without Authorization — route must be public (HSB-BE-06)")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 for public proxy passthrough", rec.Code)
	}

	// And we expect the catalog backend to have been reached — proof the
	// request actually traversed the proxy (rather than being short-circuited
	// by some other middleware returning 200).
	select {
	case <-gw.catalogGotURL:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("catalog backend never received the anonymous request — middleware short-circuit?")
	}
}
