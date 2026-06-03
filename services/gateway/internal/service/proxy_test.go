package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

// TestProxyService_Forward_StripsHopByHopHeaders — BLK-02 regression. The
// gateway MUST NOT forward RFC 7230 §6.1 hop-by-hop headers verbatim to the
// upstream service. Forwarding Transfer-Encoding / TE / Connection /
// Proxy-Authorization is a request-smuggling primitive; forwarding Cookie
// leaks the auth-service refresh-token cookie to backend services.
func TestProxyService_Forward_StripsHopByHopHeaders(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Copy so the channel receiver sees a stable snapshot.
		h := r.Header.Clone()
		gotHeaders <- h
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.Header.Set("Connection", "close")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Te", "trailers")
	req.Header.Set("Trailer", "Expires")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Proxy-Authenticate", "Basic")
	req.Header.Set("Proxy-Authorization", "Basic Zm9vOmJhcg==")
	req.Header.Set("Cookie", "refresh_token=secret_value; session=xyz")
	// And a legitimate end-to-end header that MUST pass through.
	req.Header.Set("X-Request-ID", "test-req-id")

	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	forbidden := []string{
		"Connection", "Keep-Alive", "Te", "Trailer", "Transfer-Encoding",
		"Upgrade", "Proxy-Authenticate", "Proxy-Authorization", "Cookie",
	}
	for _, h := range forbidden {
		if got := headers.Get(h); got != "" {
			t.Errorf("backend received hop-by-hop header %s = %q; must be stripped", h, got)
		}
	}
	if got := headers.Get("X-Request-ID"); got != "test-req-id" {
		t.Errorf("backend lost legitimate end-to-end header X-Request-ID = %q; want test-req-id", got)
	}
}

// TestProxyService_Forward_HonoursConnectionHeaderList — BLK-02 regression
// for the request-smuggling sub-case. A client sending
// `Connection: Authorization, X-Forwarded-For` MUST cause those headers to
// also be stripped from the upstream request (RFC 7230 §6.1).
func TestProxyService_Forward_HonoursConnectionHeaderList(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	// Client claims these are hop-by-hop via the Connection list.
	req.Header.Set("Connection", "X-Smuggled, X-Forwarded-For")
	req.Header.Set("X-Smuggled", "attacker_value")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	// A legitimate header NOT named in Connection must still pass.
	req.Header.Set("X-Request-ID", "ok")

	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	for _, h := range []string{"X-Smuggled", "X-Forwarded-For"} {
		if got := headers.Get(h); got != "" {
			t.Errorf("backend received Connection-named header %s = %q; must be stripped", h, got)
		}
	}
	if headers.Get("X-Request-ID") != "ok" {
		t.Errorf("legitimate X-Request-ID was dropped; got %q", headers.Get("X-Request-ID"))
	}
}

// TestProxyService_Forward_PreservesAuthorization — BLK-02 regression.
// JWTValidationMiddleware uses r.Header.Set("Authorization", ...) so
// exactly ONE Authorization value reaches Forward. That value MUST be
// forwarded to the backend so protected routes can authenticate the user.
// Authorization is NOT a hop-by-hop header per RFC 7230.
func TestProxyService_Forward_PreservesAuthorization(t *testing.T) {
	t.Parallel()
	gotAuth := make(chan []string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth <- r.Header.Values("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.Header.Set("Authorization", "Bearer gateway_minted_jwt")

	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	got := <-gotAuth
	if len(got) != 1 {
		t.Fatalf("backend received %d Authorization values; want 1", len(got))
	}
	if got[0] != "Bearer gateway_minted_jwt" {
		t.Errorf("backend Authorization = %q; want %q", got[0], "Bearer gateway_minted_jwt")
	}
}

// newTestProxy constructs a ProxyService wired to a config with the given
// scraper URL. The other service URLs are left as zero-value strings — tests
// in this file only exercise the scraper case.
func newTestProxy(scraperURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		ScraperService: scraperURL,
	}, logger.Default())
}

// TestProxyService_GetServiceURL_Scraper asserts the "scraper" case routes to
// ServiceURLs.ScraperService.
func TestProxyService_GetServiceURL_Scraper(t *testing.T) {
	t.Parallel()
	p := newTestProxy("http://scraper:8088")
	got, err := p.getServiceURL("scraper")
	if err != nil {
		t.Fatalf("getServiceURL: %v", err)
	}
	if got != "http://scraper:8088" {
		t.Errorf("getServiceURL(scraper) = %q; want http://scraper:8088", got)
	}
}

// TestProxyService_PathRewrite_AdminHealth asserts the explicit rewrite for
// the admin health endpoint: /api/admin/scraper/health → /scraper/health/admin.
//
// We exercise this by spinning up a backend httptest.Server that records the
// inbound URL.Path; the Forward call routes through the rewrite block and
// the recorded path is what the scraper service would actually see.
func TestProxyService_PathRewrite_AdminHealth(t *testing.T) {
	t.Parallel()
	gotPath := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if got := <-gotPath; got != "/scraper/health/admin" {
		t.Errorf("backend received path = %q; want /scraper/health/admin", got)
	}
}

// TestProxyService_PathRewrite_OtherAdminScraper asserts the generic
// fallthrough for unknown admin/scraper subpaths: the /admin segment is
// stripped but no /admin suffix is appended. Today only /health has an
// explicit rewrite; this test pins the fallthrough so a future second admin
// endpoint slots in deterministically.
func TestProxyService_PathRewrite_OtherAdminScraper(t *testing.T) {
	t.Parallel()
	gotPath := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/other", nil)
	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	got := <-gotPath
	if got != "/scraper/other" {
		t.Errorf("backend received path = %q; want /scraper/other", got)
	}
	// Defensive: ensure no /admin segment slipped through.
	if strings.Contains(got, "/admin") {
		t.Errorf("backend received path = %q; must not contain /admin", got)
	}
}

// newTestGrafanaProxy points the grafana service URL at a test backend.
func newTestGrafanaProxy(grafanaURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		GrafanaService: grafanaURL,
	}, logger.Default())
}

// TestProxyService_Forward_GrafanaAuthProxy_OverwritesForgedIdentity is the
// UA-115 forgery guard. The gateway asserts the authenticated admin's identity
// to Grafana via X-WEBAUTH-* (auth-proxy SSO). A client MUST NOT be able to
// spoof that identity: any client-supplied X-WEBAUTH-* is wiped and replaced
// with the value from the validated JWT claims, on the final outbound request.
func TestProxyService_Forward_GrafanaAuthProxy_OverwritesForgedIdentity(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestGrafanaProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/admin/grafana/api/user", nil)
	// Attacker tries to spoof identity + privilege via client headers.
	req.Header.Set("X-Webauth-User", "attacker")
	req.Header.Set("X-Webauth-Role", "Viewer")
	// The gateway's admin middleware put the real, validated claims in context.
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{
		Username: "realadmin",
		Role:     authz.RoleAdmin,
	}))

	resp, err := p.Forward(req, "grafana")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Webauth-User"); got != "realadmin" {
		t.Errorf("Grafana received X-Webauth-User = %q; want realadmin (forged value must be overwritten)", got)
	}
	if got := headers.Get("X-Webauth-Role"); got != "Admin" {
		t.Errorf("Grafana received X-Webauth-Role = %q; want Admin", got)
	}
}

// TestProxyService_Forward_GrafanaAuthProxy_NoClaimsStripsForgedHeader: with no
// claims in context (should never happen past AdminRoleMiddleware, but defense
// in depth), a client-supplied X-WEBAUTH-* is stripped and NOT replaced — so no
// identity is ever asserted from client input alone.
func TestProxyService_Forward_GrafanaAuthProxy_NoClaimsStripsForgedHeader(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestGrafanaProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/admin/grafana/api/user", nil)
	req.Header.Set("X-Webauth-User", "attacker")

	resp, err := p.Forward(req, "grafana")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Webauth-User"); got != "" {
		t.Errorf("Grafana received X-Webauth-User = %q with no claims; forged header must be stripped", got)
	}
}
