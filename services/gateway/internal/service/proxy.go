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
	serviceURLs      config.ServiceURLs
	client           *http.Client
	noRedirectClient *http.Client
	// streamClient carries large/long-lived bodies (HLS/MP4/image-proxy) and
	// deliberately has NO total http.Client.Timeout — a fixed total timeout
	// covers the whole exchange including the streamed body read, so it
	// truncated long playback bodies at 15s (audit finding L466). TTFB is
	// still bounded by the transport's ResponseHeaderTimeout; the body is
	// bounded by the request context + the streaming router's WriteTimeout.
	streamClient *http.Client
	// scraperJSONClient serves the /api/anime/{id}/scraper/* JSON routes
	// (episodes/servers/stream/health). These bodies are small, but discovery
	// for a cold engine=browser provider (animepahe/gogoanime/miruro/
	// nineanime — a Camoufox Turnstile solve) can legitimately take up to
	// catalog's own SCRAPER_TIMEOUT (40s, itself sized to the scraper's 35s
	// BrowserProviderTimeout). The plain `client`'s 15s cap sits OUTSIDE both
	// of those already-fixed budgets and was never raised to match them, so
	// real users hit a gateway-side 500 ("context deadline exceeded") on any
	// cold resolve over 15s even though catalog/scraper would have succeeded
	// — reproduced live 2026-07-10 on a fully-recovered animepahe. 45s gives
	// the 40s inner budget a margin, same rationale as streamClient's split
	// from the plain client above (small body, just a slow one).
	scraperJSONClient *http.Client
	log               *logger.Logger
}

func NewProxyService(serviceURLs config.ServiceURLs, log *logger.Logger) *ProxyService {
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	transport := tracing.WrapTransport(baseTransport)

	// streamTransport reuses the same dialer/pool tuning but adds a
	// ResponseHeaderTimeout so a hung upstream can't park a streaming
	// connection forever waiting for the first byte. Crucially it is paired
	// with a client that has NO total Timeout, so the body itself streams
	// without a hard 15s cap. Cloned (not shared) so setting the header
	// timeout here never affects the API-JSON client above.
	streamBase := baseTransport.Clone()
	streamBase.ResponseHeaderTimeout = 15 * time.Second
	streamTransport := tracing.WrapTransport(streamBase)

	return &ProxyService{
		serviceURLs: serviceURLs,
		client: &http.Client{
			Timeout: 15 * time.Second,
			// tracing.WrapTransport injects the active span's W3C traceparent
			// into the forwarded request so downstream services continue the
			// same trace. This is the core FE→BE propagation fix — the gateway
			// previously propagated nothing. No-op when tracing is disabled.
			Transport: transport,
		},
		// noRedirectClient is identical to client but does NOT follow redirects.
		// Used for magic-link bridge routes (/magic-link-generate,
		// /magic-link-login) so that 302 responses from the auth service reach
		// the browser unmodified instead of being chased server-side (which
		// would swallow the redirect and try to fetch the external .org URL).
		noRedirectClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		streamClient: &http.Client{
			// No total Timeout — the streamed body must not be truncated.
			Transport: streamTransport,
		},
		scraperJSONClient: &http.Client{
			Timeout:   45 * time.Second,
			Transport: transport,
		},
		log: log,
	}
}

// Forward forwards the request to the appropriate service
func (s *ProxyService) Forward(r *http.Request, service string) (*http.Response, error) {
	return s.forwardWith(s.client, r, service)
}

// ForwardStream forwards the request to the appropriate service using the
// streamClient — the client WITHOUT a total http.Client.Timeout (audit finding
// L466). Use this for routes whose response body is a large or long-lived
// stream (HLS playlists/segments, MP4 restream, image-proxy) so the body read
// is bounded by the request context + the streaming router's WriteTimeout
// rather than truncated at the 15s API timeout. TTFB is still bounded by the
// transport's ResponseHeaderTimeout.
func (s *ProxyService) ForwardStream(r *http.Request, service string) (*http.Response, error) {
	return s.forwardWith(s.streamClient, r, service)
}

// ForwardScraperJSON forwards the request using the scraperJSONClient (45s)
// instead of the plain 15s client. Use for the /api/anime/{id}/scraper/*
// JSON routes, whose discovery can ride a cold engine=browser provider solve
// well past 15s even on a healthy provider.
func (s *ProxyService) ForwardScraperJSON(r *http.Request, service string) (*http.Response, error) {
	return s.forwardWith(s.scraperJSONClient, r, service)
}

// ForwardNoRedirect forwards the request to the appropriate service without
// following HTTP redirects. The upstream's 3xx response (including Location
// and Set-Cookie headers) is returned to the caller verbatim. Used for the
// magic-link bridge routes so that cross-domain 302s reach the browser.
func (s *ProxyService) ForwardNoRedirect(r *http.Request, service string) (*http.Response, error) {
	return s.forwardWith(s.noRedirectClient, r, service)
}

// forwardWith is the shared implementation for Forward and ForwardNoRedirect.
// It performs path rewrites, header filtering, Grafana SSO injection, and the
// auth-service refresh_token cookie re-forward — the caller selects which
// http.Client to use (redirect-following vs. non-following).
func (s *ProxyService) forwardWith(client *http.Client, r *http.Request, service string) (*http.Response, error) {
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
	case "rooms":
		// The rooms service mounts its REST + WS + leaderboard routes ONLY
		// under /api/v1, so inbound paths must be rewritten or they 404
		// (audit finding L753). Two inbound prefixes reach here:
		//   - /api/game/rooms/...  -> /api/v1/rooms/...  (the family the SPA's
		//     gameApi actually calls — see frontend/web/src/api/client.ts)
		//   - /api/rooms/...       -> /api/v1/rooms/...  (defensive/direct callers)
		// A single prefix Replace per form covers both the bare collection
		// (/api/rooms) and the {roomId} subroutes (/api/rooms/{id}/join).
		switch {
		case strings.HasPrefix(path, "/api/game/rooms"):
			path = strings.Replace(path, "/api/game/rooms", "/api/v1/rooms", 1)
		case strings.HasPrefix(path, "/api/rooms"):
			path = strings.Replace(path, "/api/rooms", "/api/v1/rooms", 1)
		}
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

	// Upscaler admin gate (defense-in-depth): the /api/upscale/* admin routes
	// are only reachable at upscaler:8096 if X-Gateway-Internal is present.
	// The upscaler's requireGatewayInternal middleware checks for a non-empty
	// value; the worker-facing /worker/* routes do NOT go through this branch
	// (they use the ExternalAPIKeyMiddleware group in the gateway router, which
	// calls a different handler — not ProxyToUpscaler → forwardWith).
	//
	// Strip-then-set pattern: Del removes any client-supplied value that survived
	// copyForwardHeaders (copyForwardHeaders passes through arbitrary headers it
	// doesn't know about), then Set injects the gateway-trusted value. This
	// ensures a rogue client cannot forge the header by sending it directly.
	//
	// FOLLOW-UP (Phase 2): replace the static "1" with an HMAC-SHA256 signed
	// token (rotated per deploy) so a leaked Docker-network access cannot
	// trivially impersonate the gateway. The upscaler's requireGatewayInternal
	// will need a matching verifier. For Phase 1 the static value is sufficient
	// because upscaler:8096 is not internet-exposed (Docker-internal only) and
	// the gateway router already enforces JWT + AdminRole before calling this
	// proxy path.
	if service == "upscaler" {
		req.Header.Del("X-Gateway-Internal")
		req.Header.Set("X-Gateway-Internal", "1")

		// Admin audit attribution (non-repudiation): the upscaler's remote-shell
		// handler reads X-Admin-ID for its audit log. Strip any client-supplied
		// value (a client could otherwise spoof another admin in the audit trail),
		// then inject the authenticated identity from the validated JWT subject —
		// the same strip-then-set discipline as X-Gateway-Internal above and the
		// grafana X-Webauth-User pattern. Done post-copyForwardHeaders so a smuggled
		// `Connection: X-Admin-ID` cannot drop the injected value. The /api/upscale/*
		// routes already passed JWTValidationMiddleware + AdminRoleMiddleware, so
		// claims are present and the caller is an admin.
		req.Header.Del("X-Admin-ID")
		if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil && claims.UserID != "" {
			req.Header.Set("X-Admin-ID", claims.UserID)
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
	resp, err := client.Do(req)
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
	case "recs":
		return s.serviceURLs.RecsService, nil
	case "anidle":
		return s.serviceURLs.AnidleService, nil
	case "upscaler":
		return s.serviceURLs.UpscalerService, nil
	case "fanfic":
		return s.serviceURLs.FanficService, nil
	case "policy":
		return s.serviceURLs.PolicyService, nil
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
