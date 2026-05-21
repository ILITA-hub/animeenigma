package transport

// Workstream hero-spotlight v1.0 Phase 3 (Plan 03-04) — defense-in-depth
// regression test for the player internal-list endpoint.
//
// Plan 03-01 added `GET /internal/users/{id}/list` on the PLAYER service so
// the catalog spotlight aggregator can read a user's anime_list from inside
// the Docker network. This route is deliberately NOT proxied by the gateway:
//
//   - The route handles user-scoped data with NO JWT (the trust boundary is
//     the Docker network — internal-only by convention, matching player's
//     /internal/resolve-api-key, catalog's /internal/cache/invalidate/raw,
//     and catalog's /internal/anime/{id}/episodes).
//   - The gateway's /api/users/* group is JWT-protected, but a gateway proxy
//     entry for /internal/users/{id}/list would BYPASS that gate because
//     /internal/* is outside /api in the routing tree.
//
// This test pins the absence of any such route. If a future change accidentally
// adds a gateway entry for /internal/users/{id}/list, this test fails loudly.
//
// Threat T-03-15 (info disclosure via gateway proxying /internal/*) — mitigated.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouter_InternalListNotProxied asserts a baseline GET against the
// internal-list path returns 404 (or 405) — i.e. the gateway has no matching
// route. We use the shared testGateway harness so the test exercises the
// SAME router NewRouterWithCleanup builds for production. A 404 means chi
// did not find a registered handler for the path; a 200 or any successful
// proxy response would indicate the route IS proxied — which is the
// regression we are guarding against.
func TestRouter_InternalListNotProxied(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u1/list?status=watching", nil)
	req.RemoteAddr = "10.0.0.20:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	// The gateway must not have a route for /internal/*. chi's default
	// no-match response is 404. Any 2xx here would mean the route IS
	// proxied, which would defeat the security model.
	if rec.Code == http.StatusOK {
		t.Fatalf("gateway routed /internal/users/u1/list — internal-list endpoint MUST NOT be proxied (status=%d body=%q)", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 404/405 for /internal/users/u1/list; got %d (body=%q)", rec.Code, rec.Body.String())
	}

	// Belt-and-suspenders: neither backend may have received a request. If
	// any did, it means a proxy route was registered (and matched something).
	select {
	case got := <-gw.catalogGotURL:
		t.Errorf("catalog backend received unexpected /internal/* request: %q — gateway must not proxy /internal/*", got)
	default:
	}
	select {
	case got := <-gw.scraperGotURL:
		t.Errorf("scraper backend received unexpected /internal/* request: %q — gateway must not proxy /internal/*", got)
	default:
	}
}

// TestRouter_InternalListNotProxied_PostMethod — same defense, POST method.
// Some misguided refactor could add a POST handler that the route ordering
// hides under /api/users/*. Assert the POST flavor too.
func TestRouter_InternalListNotProxied_PostMethod(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodPost, "/internal/users/u1/list", nil)
	req.RemoteAddr = "10.0.0.21:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("gateway routed POST /internal/users/u1/list — internal-list endpoint MUST NOT be proxied (status=%d)", rec.Code)
	}
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 404/405 for POST /internal/users/u1/list; got %d", rec.Code)
	}
}

// TestRouter_InternalListNotProxied_AnyUserID — varies the user_id path
// component (UUIDs, special characters, sub-paths) to prove the absence is
// path-pattern-agnostic, not just for the literal "u1" case.
func TestRouter_InternalListNotProxied_AnyUserID(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"uuid_user", "/internal/users/d1ce41e5-9ace-4b3c-9ab4-ce82a26fb0f0/list"},
		{"escaped_user", "/internal/users/u%2F1/list"},
		{"subpath", "/internal/users/u1/list/extra/segments"},
		{"trailing_slash", "/internal/users/u1/list/"},
		{"with_query", "/internal/users/u1/list?status=planned,postponed&extra=foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := buildTestGatewayRouter(t)
			defer gw.teardown()

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.RemoteAddr = "10.0.0.22:1234"
			rec := httptest.NewRecorder()
			gw.router.ServeHTTP(rec, req)

			if rec.Code == http.StatusOK {
				t.Fatalf("gateway routed %q — internal-list endpoint MUST NOT be proxied for any user id", tc.path)
			}
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 404/405 for %q; got %d", tc.path, rec.Code)
			}
		})
	}
}
