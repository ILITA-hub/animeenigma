package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// TestProxyService_Forward_StripsForgedClientIPProvenance — F31 (CWE-290)
// regression. An unauthenticated client MUST NOT be able to choose the IP
// identity a backend's chi middleware.RealIP observes. Every client-supplied
// IP-provenance header (True-Client-IP, X-Forwarded-For, X-Real-IP,
// CF-Connecting-IP, Forwarded) must be stripped from the upstream request and
// exactly one — X-Real-IP — re-asserted from the gateway's validated
// r.RemoteAddr, so the backend keys its scraper quota / session / audit records
// on the gateway-attested address, not the attacker's.
func TestProxyService_Forward_StripsForgedClientIPProvenance(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	// Attacker forges every IP-provenance header with the same bogus address.
	const forged = "6.6.6.6"
	req.Header.Set("True-Client-IP", forged)
	req.Header.Set("X-Forwarded-For", forged+", 10.0.0.2")
	req.Header.Set("X-Real-IP", forged)
	req.Header.Set("CF-Connecting-IP", forged)
	req.Header.Set("Forwarded", "for="+forged)
	// The gateway's RealClientIP middleware already put the validated peer here
	// (bare IP, no port — the post-middleware shape).
	req.RemoteAddr = "203.0.113.7"

	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	// The two headers chi RealIP prefers over X-Real-IP, plus the CDN/RFC ones,
	// must not carry the attacker value at all.
	for _, h := range []string{"True-Client-IP", "X-Forwarded-For", "CF-Connecting-IP", "Forwarded"} {
		if got := headers.Get(h); strings.Contains(got, forged) {
			t.Errorf("backend received forged %s = %q; attacker IP must be stripped", h, got)
		}
	}
	// Exactly one attested address is asserted, and it is the gateway's
	// validated r.RemoteAddr — never the attacker's forged value.
	if got := headers.Get("X-Real-IP"); got != "203.0.113.7" {
		t.Errorf("backend received X-Real-IP = %q; want the gateway-attested 203.0.113.7 (not the forged %s)", got, forged)
	}
}

// TestProxyService_Forward_ConveysRealClientIPToBackend — F31 companion. The
// fix must not blind the backend: a normal request still conveys the real
// client IP. The gateway derives it from r.RemoteAddr (here the fail-safe
// host:port TCP-peer form) and re-asserts it as X-Real-IP for the backend's
// chi middleware.RealIP.
func TestProxyService_Forward_ConveysRealClientIPToBackend(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.RemoteAddr = "198.51.100.23:5555"

	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Real-IP"); got != "198.51.100.23" {
		t.Errorf("backend received X-Real-IP = %q; want the real client 198.51.100.23 (host of r.RemoteAddr)", got)
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

// newTestStreamingProxy points the streaming service URL at a test backend.
func newTestStreamingProxy(streamingURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		StreamingService: streamingURL,
	}, logger.Default())
}

// slowStreamBackend writes a header + first chunk immediately, sleeps past the
// 15s API-client total timeout, then writes a final chunk. A client with a
// fixed total Timeout truncates mid-body; a streaming client (header-timeout
// only) reads it all.
func slowStreamBackend(t *testing.T, sleep time.Duration, head, tail string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("backend ResponseWriter is not a Flusher")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, head)
		fl.Flush()
		time.Sleep(sleep)
		_, _ = io.WriteString(w, tail)
		fl.Flush()
	}))
}

// TestProxyService_ForwardStream_NoTotalTimeout — audit finding L466. A
// gateway-routed HLS/MP4/image stream body must NOT be truncated by a fixed
// 15s http.Client.Timeout. ForwardStream uses a client with no total timeout
// (header timeout only), so a body that streams slowly past 15s arrives whole.
func TestProxyService_ForwardStream_NoTotalTimeout(t *testing.T) {
	t.Parallel()
	const head, tail = "FIRST", "SECOND"
	backend := slowStreamBackend(t, 16*time.Second, head, tail)
	defer backend.Close()

	p := newTestStreamingProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/streaming/hls-proxy", nil)
	resp, err := p.ForwardStream(req, "streaming")
	if err != nil {
		t.Fatalf("ForwardStream: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read streamed body: %v (got %q)", err, body)
	}
	if got := string(body); got != head+tail {
		t.Errorf("streamed body = %q; want %q (body truncated — total timeout still capping the stream)", got, head+tail)
	}
}

// TestProxyService_Forward_APIClientTruncatesSlowBody is the control: the
// existing 15s API-JSON client (Forward) MUST cut off a body that streams past
// its total timeout. This proves the timeout split is real — the streaming
// client behaves differently from the API client.
func TestProxyService_Forward_APIClientTruncatesSlowBody(t *testing.T) {
	t.Parallel()
	const head, tail = "FIRST", "SECOND"
	backend := slowStreamBackend(t, 16*time.Second, head, tail)
	defer backend.Close()

	p := newTestStreamingProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/streaming/hls-proxy", nil)
	resp, err := p.Forward(req, "streaming")
	if err != nil {
		// A transport-level deadline before any response is also acceptable
		// evidence the API client refuses the slow body.
		return
	}
	defer resp.Body.Close()

	_, readErr := io.ReadAll(resp.Body)
	if readErr == nil {
		t.Errorf("API client read the full slow body without error; expected the 15s total Timeout to truncate it")
	}
}

// newTestCatalogProxy points the catalog service URL at a test backend.
func newTestCatalogProxy(catalogURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		CatalogService: catalogURL,
	}, logger.Default())
}

// TestProxyService_ForwardScraperJSON_SurvivesPast15s — 2026-07-10 finding:
// a cold engine=browser scraper provider (animepahe/gogoanime/miruro/
// nineanime) discovery call can legitimately take longer than 15s even on a
// HEALTHY provider (catalog's own SCRAPER_TIMEOUT is already 40s), but the
// plain 15s API client sat one layer further out and was never raised to
// match — real users got a gateway 500 on any cold resolve over 15s.
// ForwardScraperJSON (the /api/anime/{id}/scraper/* route family) must
// tolerate a body/response that takes longer than 15s.
func TestProxyService_ForwardScraperJSON_SurvivesPast15s(t *testing.T) {
	t.Parallel()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(16 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true,"data":{"episodes":[]}}`)
	}))
	defer backend.Close()

	p := newTestCatalogProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/x/scraper/episodes", nil)
	resp, err := p.ForwardScraperJSON(req, "catalog")
	if err != nil {
		t.Fatalf("ForwardScraperJSON: %v (a 16s-slow but healthy backend must not be treated as down)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}
}

// TestProxyService_Forward_APIClientTruncatesSlowScraperBody is the control:
// the existing 15s API-JSON client (Forward, used by every OTHER /api/anime/*
// route) MUST still fail a response past its total timeout. Proves the split
// is real and scoped to ForwardScraperJSON, not a blanket timeout raise.
func TestProxyService_Forward_APIClientTruncatesSlowScraperBody(t *testing.T) {
	t.Parallel()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(16 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestCatalogProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/x", nil)
	_, err := p.Forward(req, "catalog")
	if err == nil {
		t.Error("Forward succeeded against a 16s-slow backend; expected the 15s total Timeout to fire")
	}
}

// newTestRoomsProxy points the rooms service URL at a test backend.
func newTestRoomsProxy(roomsURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		RoomsService: roomsURL,
	}, logger.Default())
}

// TestProxyService_PathRewrite_Rooms asserts the gateway rewrites the public
// /api/rooms/... prefix to the rooms service's actual mount /api/v1/rooms/...
// (audit finding L753). The rooms service only mounts routes under /api/v1, so
// without this rewrite the inbound /api/rooms/* arrives verbatim and 404s.
func TestProxyService_PathRewrite_Rooms(t *testing.T) {
	t.Parallel()
	gotPath := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestRoomsProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/rooms/abc", nil)
	resp, err := p.Forward(req, "rooms")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if got := <-gotPath; got != "/api/v1/rooms/abc" {
		t.Errorf("rooms backend received path = %q; want /api/v1/rooms/abc", got)
	}
}

// TestProxyService_PathRewrite_RoomsRoot asserts the bare /api/rooms collection
// path also rewrites to /api/v1/rooms (the trailing-slash form covers the
// {roomId} subroutes; this covers the no-trailing-slash collection endpoint).
func TestProxyService_PathRewrite_RoomsRoot(t *testing.T) {
	t.Parallel()
	gotPath := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestRoomsProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
	resp, err := p.Forward(req, "rooms")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if got := <-gotPath; got != "/api/v1/rooms" {
		t.Errorf("rooms backend received path = %q; want /api/v1/rooms", got)
	}
}

// TestProxyService_PathRewrite_GameRooms asserts the /api/game/rooms/... family
// the SPA's gameApi actually calls (frontend/web/src/api/client.ts) is rewritten
// onto the rooms service's /api/v1/rooms/... mount (audit finding L753). Without
// this, joining a co-watch game room 404s end to end.
func TestProxyService_PathRewrite_GameRooms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"/api/game/rooms", "/api/v1/rooms"},
		{"/api/game/rooms/abc", "/api/v1/rooms/abc"},
		{"/api/game/rooms/abc/join", "/api/v1/rooms/abc/join"},
		{"/api/game/rooms/abc/leave", "/api/v1/rooms/abc/leave"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			gotPath := make(chan string, 1)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath <- r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			p := newTestRoomsProxy(backend.URL)
			req := httptest.NewRequest(http.MethodGet, c.in, nil)
			resp, err := p.Forward(req, "rooms")
			if err != nil {
				t.Fatalf("Forward: %v", err)
			}
			defer resp.Body.Close()

			if got := <-gotPath; got != c.want {
				t.Errorf("rooms backend received path = %q; want %q", got, c.want)
			}
		})
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

// newTestUpscalerProxy points the upscaler service URL at a test backend.
func newTestUpscalerProxy(upscalerURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		UpscalerService: upscalerURL,
	}, logger.Default())
}

// TestProxyService_Upscaler_InjectsGatewayInternalHeader asserts that forwarding
// a request to the "upscaler" service sets X-Gateway-Internal: "1" on the
// outbound request. This is the positive case: an admin request without any
// client-supplied header still receives the injected value.
func TestProxyService_Upscaler_InjectsGatewayInternalHeader(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestUpscalerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/jobs", nil)
	resp, err := p.Forward(req, "upscaler")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	got := headers.Get("X-Gateway-Internal")
	if got != "1" {
		t.Errorf("upscaler backend received X-Gateway-Internal = %q; want \"1\"", got)
	}
}

// TestProxyService_Upscaler_StripsForgedGatewayInternalHeader asserts the
// strip-then-set security property: a client-supplied X-Gateway-Internal header
// is stripped and replaced with the gateway-trusted value. A rogue client
// sending any X-Gateway-Internal value cannot control what the upscaler sees.
func TestProxyService_Upscaler_StripsForgedGatewayInternalHeader(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestUpscalerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/jobs", nil)
	// Attacker tries to supply any header value — gateway must wipe and replace.
	req.Header.Set("X-Gateway-Internal", "attacker-forged-value")

	resp, err := p.Forward(req, "upscaler")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	got := headers.Get("X-Gateway-Internal")
	// The backend must see exactly "1", not the attacker's value.
	if got != "1" {
		t.Errorf("upscaler backend received X-Gateway-Internal = %q; want \"1\" (forged value must be overwritten)", got)
	}
}

// TestProxyService_NonUpscaler_DoesNotInjectGatewayInternalHeader asserts that
// the X-Gateway-Internal injection is scoped to the "upscaler" service only —
// other services MUST NOT receive this header (defence-in-depth isolation).
func TestProxyService_NonUpscaler_DoesNotInjectGatewayInternalHeader(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Use the scraper (non-upscaler) service as the target.
	p := newTestProxy(backend.URL) // newTestProxy wires ScraperService
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Gateway-Internal"); got != "" {
		t.Errorf("scraper backend received X-Gateway-Internal = %q; must be absent for non-upscaler services", got)
	}
}

// TestProxyService_Upscaler_InjectsAdminIDFromClaims (I1): the upscaler proxy
// must inject X-Admin-ID from the authenticated JWT subject (claims.UserID) so
// the upscaler's remote-shell audit log attributes actions to the real admin.
func TestProxyService_Upscaler_InjectsAdminIDFromClaims(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestUpscalerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/workers/w1/shell", nil)
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{
		UserID:   "admin-uuid-123",
		Username: "alice",
		Role:     authz.RoleAdmin,
	}))

	resp, err := p.Forward(req, "upscaler")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Admin-ID"); got != "admin-uuid-123" {
		t.Errorf("upscaler backend received X-Admin-ID = %q; want %q (JWT subject)", got, "admin-uuid-123")
	}
}

// TestProxyService_Upscaler_StripsForgedAdminID (I1): a client-supplied X-Admin-ID
// must be stripped and replaced with the JWT-derived identity — an admin must not
// be able to spoof another admin in the non-repudiation audit trail.
func TestProxyService_Upscaler_StripsForgedAdminID(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestUpscalerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/workers/w1/shell", nil)
	// Attacker tries to impersonate another admin in the audit trail.
	req.Header.Set("X-Admin-ID", "victim-admin")
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{
		UserID: "attacker-uuid",
		Role:   authz.RoleAdmin,
	}))

	resp, err := p.Forward(req, "upscaler")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Admin-ID"); got != "attacker-uuid" {
		t.Errorf("upscaler backend received X-Admin-ID = %q; want %q (forged value must be overwritten with JWT subject)", got, "attacker-uuid")
	}
}

// TestProxyService_Upscaler_StripsAdminIDWhenNoClaims (I1): with no authenticated
// claims in context, any client-supplied X-Admin-ID must still be stripped (never
// forwarded), so a forged header can never reach the audit log unverified.
func TestProxyService_Upscaler_StripsAdminIDWhenNoClaims(t *testing.T) {
	t.Parallel()
	gotHeaders := make(chan http.Header, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestUpscalerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/workers/w1/shell", nil)
	req.Header.Set("X-Admin-ID", "forged-no-auth")

	resp, err := p.Forward(req, "upscaler")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	headers := <-gotHeaders
	if got := headers.Get("X-Admin-ID"); got != "" {
		t.Errorf("upscaler backend received X-Admin-ID = %q; want empty (forged header must be stripped when no claims)", got)
	}
}

// newTestPlayerProxy points the player service URL at a test backend.
func newTestPlayerProxy(playerURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		PlayerService: playerURL,
	}, logger.Default())
}

// upstreamTarget captures what a backend actually receives on the wire: the
// escaped path, the decoded path, the query, and the raw request-target line.
type upstreamTarget struct {
	escapedPath string
	path        string
	rawQuery    string
	requestURI  string
}

// recordUpstream spins up a backend that records the inbound request target and
// returns it over a channel — this is exactly what the upstream service sees.
func recordUpstream(t *testing.T) (*httptest.Server, <-chan upstreamTarget) {
	t.Helper()
	got := make(chan upstreamTarget, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- upstreamTarget{
			escapedPath: r.URL.EscapedPath(),
			path:        r.URL.Path,
			rawQuery:    r.URL.RawQuery,
			requestURI:  r.RequestURI,
		}
		w.WriteHeader(http.StatusOK)
	})) //nolint:bodyclose
	return srv, got
}

// TestProxyService_Forward_PreservesEscapedPath is the CWE-436 regression
// (route/authorization confusion). chi authorizes on the ESCAPED path, so a
// percent-encoded '/', '?' or '#' inside a matched {param}/* segment stays a
// single opaque segment for the gateway's route+middleware decision. The
// upstream request MUST carry that same escaping — forwarding the DECODED path
// let http.NewRequestWithContext re-split the URL on the injected special (an
// encoded '?' truncated the path to a DIFFERENT upstream route than chi
// authorized), sidestepping FeatureGate/JWT/guest-block/rate-limit middleware.
//
// The attacker segment "VICTIM%2Fshowcase%3Fz" matches the unauthenticated
// /users/{userId}/watchlist/public route (no literal '/' in the raw segment),
// but the OLD decoded-path forward emitted GET /api/users/VICTIM/showcase to
// player, serving the public-showcase handler FeatureGate was meant to withhold.
func TestProxyService_Forward_PreservesEscapedPath(t *testing.T) {
	t.Parallel()
	backend, got := recordUpstream(t)
	defer backend.Close()

	const target = "/api/users/VICTIM%2Fshowcase%3Fz/watchlist/public"
	p := newTestPlayerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	// Sanity: chi would have matched on this escaped form (RawPath set).
	if req.URL.EscapedPath() != target {
		t.Fatalf("test setup: inbound EscapedPath = %q; want %q", req.URL.EscapedPath(), target)
	}

	resp, err := p.Forward(req, "player")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	up := <-got
	// The backend must receive the SAME opaque segment chi matched — the
	// %2F/%3F stay encoded, not split into extra path segments or a query.
	if up.escapedPath != target {
		t.Errorf("backend escaped path = %q; want %q (encoded specials must survive end-to-end)", up.escapedPath, target)
	}
	if up.requestURI != target {
		t.Errorf("backend request-target = %q; want %q (chi-authorized route must equal served route)", up.requestURI, target)
	}
	// The injected '?' MUST NOT have become a real query string.
	if up.rawQuery != "" {
		t.Errorf("backend raw query = %q; want empty (an encoded '?' must not truncate the path into a query)", up.rawQuery)
	}
	// Decoded, the backend sees the single opaque segment (literal '/'+'?' from
	// %2F/%3F), NOT the truncated /api/users/VICTIM/showcase the old code served.
	const wantDecoded = "/api/users/VICTIM/showcase?z/watchlist/public"
	if up.path != wantDecoded {
		t.Errorf("backend decoded path = %q; want %q", up.path, wantDecoded)
	}
}

// TestProxyService_Forward_NormalPathByteIdentical pins the no-regression half:
// a request with NO percent-encoded specials in its path (plus a real query)
// must forward a byte-identical upstream request-target to what the old
// targetURL+r.URL.Path+"?"+RawQuery concatenation produced — routing for the
// normal 99% case is unchanged by the escaping-preservation fix.
func TestProxyService_Forward_NormalPathByteIdentical(t *testing.T) {
	t.Parallel()
	backend, got := recordUpstream(t)
	defer backend.Close()

	const target = "/api/users/abc123/watchlist/public?foo=bar&baz=1"
	p := newTestPlayerProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, target, nil)

	resp, err := p.Forward(req, "player")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	up := <-got
	if up.requestURI != target {
		t.Errorf("backend request-target = %q; want %q (normal path must forward byte-identically)", up.requestURI, target)
	}
	if up.escapedPath != "/api/users/abc123/watchlist/public" {
		t.Errorf("backend escaped path = %q; want /api/users/abc123/watchlist/public", up.escapedPath)
	}
	if up.rawQuery != "foo=bar&baz=1" {
		t.Errorf("backend raw query = %q; want foo=bar&baz=1 (query must still forward)", up.rawQuery)
	}
}

// TestProxyService_Forward_PreservesEscapedPathThroughRewrite asserts the
// escaping-preservation survives a prefix-rewrite service too: the fixed
// /api/streaming/ prefix is rewritten to /api/v1/ while an encoded special in
// the trailing (param/wildcard) segment stays encoded end-to-end.
func TestProxyService_Forward_PreservesEscapedPathThroughRewrite(t *testing.T) {
	t.Parallel()
	backend, got := recordUpstream(t)
	defer backend.Close()

	p := newTestStreamingProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/streaming/hls-proxy/a%2Fb%3Fc", nil)

	resp, err := p.Forward(req, "streaming")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	up := <-got
	const want = "/api/v1/hls-proxy/a%2Fb%3Fc"
	if up.escapedPath != want {
		t.Errorf("backend escaped path = %q; want %q (prefix rewritten AND encoded specials preserved)", up.escapedPath, want)
	}
	if up.rawQuery != "" {
		t.Errorf("backend raw query = %q; want empty", up.rawQuery)
	}
}
