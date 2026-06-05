package tracing

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ILITA-hub/animeenigma/libs/authz"
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

// SeedMiddleware is a chi middleware that enriches the request context with the
// activity-register attribution seam: it seeds origin + a COARSE operation label
// ("service METHOD routePattern", e.g. "catalog GET /api/anime/{id}") into W3C
// baggage, and the authenticated user_id into a PRIVATE, non-propagated ctx
// value (WithUserID). Mount it via r.Use(...).
//
// IMPORTANT chi timing: a Use-middleware runs BEFORE the route tree match
// completes, so chi.RouteContext(r).RoutePattern() is still empty at the top of
// the middleware. We therefore seed origin + user_id eagerly (available
// up-front) and the operation LAZILY: we hand the endpoint a ctx that already
// carries a *RouteContext reference, then seed the operation from
// rc.RoutePattern() in a tiny wrapper that runs once chi has resolved the route
// (i.e. right before the matched endpoint). The recording RoundTripper (Task 3)
// reads baggage at outbound-call time, which is inside the endpoint — after the
// pattern is set — so the operation is present for every effect.
//
// Coarse operation (D-07) uses the route PATTERN, not the concrete path, so
// "/api/anime/42" and "/api/anime/99" collapse to one operation. user_id is
// deliberately NOT seeded into baggage (T-02-PII): baggage propagates on the
// wire to 3rd-party hosts, so PII must stay on the private ctx value only.
func SeedMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// origin rides baggage eagerly; user_id rides the private ctx value.
			ctx = SeedBaggage(ctx, "api", "")
			if uid := authz.UserIDFromContext(ctx); uid != "" {
				ctx = WithUserID(ctx, uid)
			}

			// operation is resolved LAZILY: a chi Use-middleware runs before the
			// route tree match completes, so chi.RouteContext(r).RoutePattern()
			// is still empty here. We stash a resolver (service + method + the
			// chi RouteContext pointer chi fills in as it descends); ReadBaggage
			// / the recording transport call it once they run inside the matched
			// endpoint, where the pattern is populated. The resolved operation is
			// then promoted into wire baggage at that point.
			// Guard nil: a non-chi router yields no RouteContext; pass an
			// explicit nil interface so resolveOperation falls back cleanly.
			var rc patternProvider
			if crc := chi.RouteContext(ctx); crc != nil {
				rc = crc
			}
			ctx = withOperationResolver(ctx, service, r.Method, rc)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
