// ws_proxy.go — dedicated WebSocket reverse-proxy for the watch-together
// service (workstream watch-together v1.0 Phase 1 Plan 01.7).
//
// Why a dedicated proxy?
//
// The existing ProxyService.Forward path (internal/service/proxy.go)
// deliberately strips RFC 7230 §6.1 hop-by-hop headers — including
// Upgrade, Connection, Transfer-Encoding, etc. That's the RIGHT behaviour
// for normal HTTP proxying (it neuters request-smuggling primitives and
// hides backend-only headers from the client). But the WebSocket
// handshake REQUIRES Upgrade: websocket + Connection: Upgrade to reach
// the backend verbatim, so we cannot reuse Forward.
//
// httputil.NewSingleHostReverseProxy in stdlib Go 1.12+ handles the WS
// upgrade dance correctly: it detects Connection: Upgrade and hijacks
// the underlying TCP socket, copying bytes bidirectionally without
// further HTTP-level interpretation. We rely on that behaviour and only
// customise:
//
//   - Director — set req.Host to the target host so the backend sees a
//     well-formed Host header.
//   - FlushInterval = -1 — flush every write immediately so streaming
//     WS frames don't get buffered in the response writer.
//   - ErrorHandler — log + 502 on dial failure instead of letting the
//     reverse proxy's default behaviour (silent 502) hide the cause.
//
// Auth divergence (preserved from services/watch-together/internal/handler/
// websocket.go): the WS endpoint sits OUTSIDE JWTValidationMiddleware in the
// gateway router. Browsers cannot set Authorization: Bearer on a WS upgrade
// (the Sec-WebSocket-* handshake is strict), so the watch-together service
// validates the JWT itself from a ?token= query param. The gateway just
// forwards the upgrade transparently.
package transport

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// newWSProxy returns an http.HandlerFunc that reverse-proxies WebSocket
// Upgrade requests to targetBaseURL. The target is the watch-together
// service base URL (e.g. "http://watch-together:8091"); the inbound
// request path + query are forwarded verbatim so /api/watch-together/ws
// remains /api/watch-together/ws at the backend.
//
// targetBaseURL MUST be parseable as http:// or https://. We do NOT
// accept ws:// or wss:// here — Go's httputil reverse proxy speaks HTTP
// and upgrades the underlying socket only after the handshake completes,
// so the URL scheme stays http(s). The wire protocol is then upgraded
// in place.
//
// Returns an error if targetBaseURL is unparseable. Caller wires this at
// router-build time (NewRouterWithCleanup) so a misconfiguration fails
// fast at startup instead of on the first WS upgrade attempt.
func newWSProxy(targetBaseURL string, log *logger.Logger) (http.HandlerFunc, error) {
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		return nil, fmt.Errorf("watch-together ws proxy: parse target %q: %w", targetBaseURL, err)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("watch-together ws proxy: target %q has no host", targetBaseURL)
	}

	rp := httputil.NewSingleHostReverseProxy(target)

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		// SingleHostReverseProxy's default Director leaves req.Host as the
		// inbound Host (i.e. animeenigma.ru). The backend's chi router
		// doesn't care about Host, but a few middlewares do (e.g. CORS),
		// so we set it to the target host explicitly for cleanliness.
		req.Host = target.Host
		// Strip the Cookie header before forwarding. Unlike the standard
		// ProxyService.Forward path, this reverse proxy copies request headers
		// verbatim — so it would otherwise leak the browser's auth cookies
		// (access_token at Path=/, and refresh_token which is also Path=/ so
		// it can reach the gateway on /admin) to the watch-together service.
		// The WS endpoint authenticates via the ?token= query param (see the
		// package doc above), so it needs no cookies. Drop them.
		req.Header.Del("Cookie")
		// IP-provenance attestation (CWE-290): like the standard Forward path,
		// this reverse proxy copies request headers verbatim, so a client-
		// supplied True-Client-IP / X-Forwarded-For / X-Real-IP would otherwise
		// reach watch-together — which mounts chi middleware.RealIP — letting the
		// client choose the IP its access logs key on. Strip them and re-assert a
		// single gateway-attested X-Real-IP from the validated peer address.
		service.AttestForwardedClientIP(req.Header, req.RemoteAddr)
	}

	// FlushInterval=-1 flushes immediately after every write. Mandatory for
	// streaming WS frames — without it, ResponseWriter buffers frames in
	// the response body and the client doesn't see them until the buffer
	// flushes (which for a long-lived WS may be… never).
	rp.FlushInterval = -1

	// ErrorHandler runs when the reverse proxy fails to dial the upstream
	// or the upstream returns an error before the WS handshake completes.
	// Default behaviour returns 502 silently with no logging; we add a
	// structured log entry so ops can diagnose backend-down events without
	// digging into the watch-together service logs.
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Errorw("watch-together ws proxy error",
			"path", r.URL.Path,
			// The WS upgrade carries the caller's access token in
			// ?token=<JWT> (browsers can't set Authorization on a WS
			// upgrade — see the package doc). Logging the raw query would
			// write a live, unexpired bearer credential into the gateway
			// error log (CWE-532), where anyone with log access could
			// replay it. Mask the token but keep room and other non-secret
			// params, which are useful for diagnosing backend-down events.
			"query", redactWSQuery(r.URL.RawQuery),
			"target", target.String(),
			"error", err,
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}

	return rp.ServeHTTP, nil
}

// redactWSQuery returns rawQuery with the value of any `token` parameter
// masked, leaving all other params (e.g. room) intact for debugging. It is
// used to keep the watch-together WS access token out of the gateway logs.
// If the query cannot be parsed, the whole query is dropped rather than
// risk emitting an unmasked token.
func redactWSQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "[redacted: unparseable query]"
	}
	if _, ok := values["token"]; !ok {
		return rawQuery
	}
	values.Set("token", "REDACTED")
	return values.Encode()
}
