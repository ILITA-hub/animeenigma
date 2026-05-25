// Package transport wires the chi HTTP router for the watch-together
// service. Phase 1 Plan 01.1 only exposes /health + /metrics — REST room
// lifecycle endpoints land in 01.4, WebSocket upgrade in 01.5.
//
// AuthMiddleware is exported even though 01.1 doesn't use it — downstream
// plans (01.4) wrap the /api/watch-together/* subtree with it. Mirrors the
// project convention of double-validating the JWT the gateway already
// checked (defence-in-depth; same shape as services/notifications and
// services/themes).
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the watch-together service.
//
// Route shape for Plan 01.1:
//
//	GET /health   — public liveness probe
//	GET /metrics  — Prometheus scrape target
//
// Subsequent plans extend this: 01.4 mounts /rooms; 01.5 mounts /ws.
func NewRouter(cfg *config.Config, log *logger.Logger, metricsCollector *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	// Middleware stack — same order as services/notifications + services/themes
	// so request-id, metrics, and logs line up across services in Grafana.
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Public liveness.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Prometheus metrics endpoint.
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// Touching cfg here is a no-op for Plan 01.1 — keeps the parameter wired
	// so downstream plans (01.4, 01.5) can drop in handlers without changing
	// the constructor signature.
	_ = cfg

	return r
}

// AuthMiddleware validates JWT access tokens and populates the request context
// with claims. Copied verbatim from services/notifications and services/themes
// (project convention — every service double-validates the JWT the gateway
// already checked, defence-in-depth).
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}

			claims, err := jwtManager.ValidateAccessToken(token)
			if err != nil {
				httputil.Unauthorized(w)
				return
			}

			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
