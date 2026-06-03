package tracing

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// opsBypass are paths that must never produce trace spans — health checks and
// the Prometheus scrape would otherwise flood Tempo with no diagnostic value.
var opsBypass = map[string]struct{}{
	"/health":  {},
	"/healthz": {},
	"/metrics": {},
}

// isWebSocketUpgrade reports whether r is a WS handshake. otelhttp wraps the
// ResponseWriter via httpsnoop, which DOES preserve http.Hijacker, so the
// upgrade would not 500 — but a span that begins on the upgrade request and
// then times out against the long-lived WebSocket connection would pollute
// Tempo with misleading duration/status. So bypass WS upgrades entirely.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// HTTPMiddleware returns a net/http middleware that continues (or starts) a
// server span per request, using the globally-registered W3C propagator that
// New() installs. Span name is "METHOD /path". When tracing is disabled the
// global provider is a no-op, so wrapping is always safe and effectively free.
//
// Wrap at the http.Server.Handler level so it applies uniformly regardless of
// a service's internal router:
//
//	srv := &http.Server{Handler: tracing.HTTPMiddleware("catalog")(router)}
func HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		instrumented := otelhttp.NewHandler(
			next, service,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, skip := opsBypass[r.URL.Path]; skip || isWebSocketUpgrade(r) {
				next.ServeHTTP(w, r)
				return
			}
			instrumented.ServeHTTP(w, r)
		})
	}
}
