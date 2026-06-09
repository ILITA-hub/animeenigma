package service

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

// hopByHopHeaders are the headers RFC 7230 §6.1 says a proxy MUST NOT
// forward verbatim. Plus Cookie — backend services have no business with
// the auth-service refresh-token cookie (REVIEW.md BLK-02 concern: the
// cookie leaks to any service in the chain).
//
// Authorization is INTENTIONALLY NOT in this list. JWTValidationMiddleware
// uses r.Header.Set("Authorization", ...) which replaces (not appends) the
// client's original value with a freshly-minted JWT for ak_ API-key auth,
// or leaves a valid JWT untouched. Either way exactly ONE Authorization
// value reaches Forward, and that value is what the backend should see.
//
// Keys are canonical form (http.CanonicalHeaderKey) so case-insensitive
// header comparison works against both r.Header (canonical) and the values
// parsed out of the Connection: <name> handshake (uppercased by us).
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Trailers":            {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Cookie":              {},
}

// copyForwardHeaders copies src → dst while filtering hop-by-hop headers
// (REVIEW.md BLK-02). Honours Connection: <name>, <name>, ... per
// RFC 7230 §6.1 — every header named in Connection is also stripped before
// forwarding (this is the request-smuggling primitive: a client could send
// `Connection: Authorization` to ask the proxy to drop the JWT, then send
// its own header value).
func copyForwardHeaders(dst, src http.Header) {
	// RFC 7230: Connection: close, foo, bar  →  also strip "foo", "bar".
	for _, name := range strings.Split(src.Get("Connection"), ",") {
		name = http.CanonicalHeaderKey(strings.TrimSpace(name))
		if name != "" {
			dst.Del(name)
		}
	}
	for key, values := range src {
		canon := http.CanonicalHeaderKey(key)
		if _, hop := hopByHopHeaders[canon]; hop {
			continue
		}
		// Also defensively skip anything named via Connection (already
		// stripped from dst above, but the src loop would re-add it).
		if isInConnectionList(src, canon) {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

// isInConnectionList reports whether `canon` is named in the comma-separated
// Connection header of src. canon must already be canonical form.
func isInConnectionList(src http.Header, canon string) bool {
	conn := src.Get("Connection")
	if conn == "" {
		return false
	}
	for _, name := range strings.Split(conn, ",") {
		if http.CanonicalHeaderKey(strings.TrimSpace(name)) == canon {
			return true
		}
	}
	return false
}

type ProxyService struct {
	serviceURLs config.ServiceURLs
	client      *http.Client
	log         *logger.Logger
}

func NewProxyService(serviceURLs config.ServiceURLs, log *logger.Logger) *ProxyService {
	return &ProxyService{
		serviceURLs: serviceURLs,
		client: &http.Client{
			Timeout: 15 * time.Second,
			// tracing.WrapTransport injects the active span's W3C traceparent
			// into the forwarded request so downstream services continue the
			// same trace. This is the core FE→BE propagation fix — the gateway
			// previously propagated nothing. No-op when tracing is disabled.
			Transport: tracing.WrapTransport(&http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   3 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			}),
		},
		log:         log,
	}
}

// Forward forwards the request to the appropriate service
func (s *ProxyService) Forward(r *http.Request, service string) (*http.Response, error) {
	targetURL, err := s.getServiceURL(service)
	if err != nil {
		return nil, err
	}

	// Rewrite path for services with different internal paths
	path := r.URL.Path
	switch service {
	case "grafana":
		// /admin/grafana/... -> /admin/grafana/... (Grafana's sub-path, pass through)
		if path == "" || path == "/admin/grafana" {
			path = "/admin/grafana/"
		}
	case "prometheus":
		// /admin/prometheus/... -> /prometheus/... (Prometheus's sub-path)
		path = strings.TrimPrefix(path, "/admin/prometheus")
		if !strings.HasPrefix(path, "/prometheus") {
			path = "/prometheus" + path
		}
	case "scraper":
		// Phase 17 Plan 03: gateway-side admin debug routes.
		//   /api/admin/scraper/health → /scraper/health/admin (the admin
		//     endpoint mounts under the existing /scraper subroute group).
		//   /api/admin/scraper/<other> → /scraper/<other> (generic
		//     fallthrough — strip the /admin segment; no /admin suffix
		//     append because no other admin routes exist yet).
		// Future admin routes get their own explicit branch above the
		// generic strip so the path-rewrite never silently 404s.
		if path == "/api/admin/scraper/health" {
			path = "/scraper/health/admin"
		} else {
			path = strings.Replace(path, "/api/admin/scraper", "/scraper", 1)
		}
	case "streaming":
		// /api/streaming/... -> /api/v1/... (streaming service uses /api/v1)
		path = strings.Replace(path, "/api/streaming/", "/api/v1/", 1)
	}

	// Build target URL with path and query
	fullURL := targetURL + path
	if r.URL.RawQuery != "" {
		fullURL += "?" + r.URL.RawQuery
	}

	// Create new request
	req, err := http.NewRequestWithContext(r.Context(), r.Method, fullURL, r.Body)
	if err != nil {
		return nil, errors.Internal(fmt.Sprintf("create proxy request: %v", err))
	}

	// Copy headers — filter hop-by-hop + Cookie + Authorization per
	// REVIEW.md BLK-02 (RFC 7230 §6.1). Any fresh Authorization for the
	// backend is set by JWTValidationMiddleware on the inbound request
	// BEFORE Forward is called; copyForwardHeaders strips the original
	// client Authorization so the backend never sees two values.
	copyForwardHeaders(req.Header, r.Header)

	// Grafana auth-proxy SSO: the /admin/grafana route already passed
	// JWTValidationMiddleware + AdminRoleMiddleware, so the request carries
	// validated admin claims. Assert that identity to Grafana via X-WEBAUTH-*
	// headers (GF_AUTH_PROXY_*), so an authenticated admin lands in Grafana as
	// Admin with no second login — without reopening the anonymous-Admin hole
	// closed in UA-115.
	//
	// Done HERE, on the final outbound request AFTER copyForwardHeaders, so no
	// client-controlled header can influence it: Del wipes any client-supplied
	// value (canonical keys — Go canonicalizes inbound r.Header, which
	// copyForwardHeaders re-adds verbatim), and because this runs post-copy a
	// smuggled `Connection: X-WEBAUTH-USER` cannot drop the injected value.
	// Grafana additionally only trusts these headers from the gateway's network
	// (GF_AUTH_PROXY_WHITELIST). Every request reaching here is an admin, so
	// asserting Role=Admin is correct.
	if service == "grafana" {
		req.Header.Del("X-Webauth-User")
		req.Header.Del("X-Webauth-Role")
		if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil && claims.Username != "" {
			req.Header.Set("X-Webauth-User", claims.Username)
			req.Header.Set("X-Webauth-Role", "Admin")
		}
	}

	// For auth service only: selectively re-forward the refresh_token
	// cookie so that /api/auth/refresh and /api/auth/logout can read it.
	// Other services MUST NOT receive browser cookies (REVIEW.md BLK-02).
	if service == "auth" {
		if c, err := r.Cookie("refresh_token"); err == nil {
			req.AddCookie(c)
		}
	}

	// Forward request. On a transport error Go's client returns resp==nil —
	// EXCEPT the rare redirect-policy case where a non-nil response accompanies
	// the error. If the upstream actually produced a response, relay it (status
	// + headers, including Set-Cookie) rather than letting the handler
	// synthesize a 500 that would silently drop the upstream's rotated
	// refresh-token cookie.
	resp, err := s.client.Do(req)
	if err != nil {
		if resp != nil {
			return resp, nil
		}
		return nil, errors.Internal(fmt.Sprintf("forward request: %v", err))
	}

	return resp, nil
}

func (s *ProxyService) getServiceURL(service string) (string, error) {
	switch strings.ToLower(service) {
	case "auth":
		return s.serviceURLs.AuthService, nil
	case "catalog":
		return s.serviceURLs.CatalogService, nil
	case "player":
		return s.serviceURLs.PlayerService, nil
	case "rooms":
		return s.serviceURLs.RoomsService, nil
	case "scraper":
		return s.serviceURLs.ScraperService, nil
	case "streaming":
		return s.serviceURLs.StreamingService, nil
	case "themes":
		return s.serviceURLs.ThemesService, nil
	case "library":
		return s.serviceURLs.LibraryService, nil
	case "notifications":
		return s.serviceURLs.NotificationsService, nil
	case "gacha":
		// workstream gacha (Лудка), Phase 1 — REST passthrough to gacha:8093.
		return s.serviceURLs.GachaService, nil
	case "analytics":
		return s.serviceURLs.AnalyticsService, nil
	case "watch-together":
		// workstream watch-together, v1.0 Phase 1 — REST passthrough only.
		// The WebSocket /ws endpoint is NOT routed through this Forward path:
		// copyForwardHeaders deliberately strips RFC 7230 §6.1 hop-by-hop
		// headers (Upgrade, Connection, etc.) which would kill the WS
		// handshake. WS upgrades go through transport/ws_proxy.go instead.
		return s.serviceURLs.WatchTogetherService, nil
	case "grafana":
		return s.serviceURLs.GrafanaService, nil
	case "prometheus":
		return s.serviceURLs.PrometheusService, nil
	case "web":
		return s.serviceURLs.WebService, nil
	default:
		return "", errors.NotFound("service")
	}
}
