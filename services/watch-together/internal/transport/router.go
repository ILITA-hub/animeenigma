// Package transport wires the chi HTTP router for the watch-together
// service. Phase 1 plan progression:
//
//	01.1 — /health + /metrics (this file's original form)
//	01.4 — /api/watch-together/rooms (added in this iteration)
//	01.5 — /api/watch-together/ws    (lands next)
//
// AuthMiddleware is exported so this file and the 01.5 WS handler share
// one JWT-validation path (mirrors services/notifications + services/themes —
// every service double-validates the JWT the gateway already checked,
// defence-in-depth).
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/handler"
)

// NewRouter builds the chi router for the watch-together service.
//
// Route shape after Plan 01.4:
//
//	GET    /health                                 — public liveness
//	GET    /metrics                                — Prometheus scrape target
//	POST   /api/watch-together/rooms               — JWT-protected (01.4)
//	GET    /api/watch-together/rooms/{id}          — JWT-protected (01.4)
//	DELETE /api/watch-together/rooms/{id}          — JWT-protected (01.4)
//
// Plan 01.5 will mount /api/watch-together/ws on the same /api/watch-together
// subtree (so it shares this AuthMiddleware) but the upgrader reads its
// JWT from a query param, not the Authorization header — see the WS plan.
//
// roomHandler may be nil; if so the /rooms routes are NOT mounted. Used by
// unit tests that exercise the middleware stack without the full DI graph.
func NewRouter(
	cfg *config.Config,
	roomHandler *handler.RoomHandler,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
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

	// Public CRUD — Plan 01.4. All behind JWT. Mounted at /api/watch-together
	// so the gateway can proxy the prefix verbatim and the WS endpoint in
	// 01.5 reuses the same AuthMiddleware scope (only difference is that
	// WS reads the JWT from a query param via a custom upgrader).
	if roomHandler != nil {
		r.Route("/api/watch-together", func(r chi.Router) {
			r.Use(AuthMiddleware(cfg.JWT))
			r.Route("/rooms", func(r chi.Router) {
				r.Post("/", roomHandler.Create)
				r.Get("/{id}", roomHandler.Get)
				r.Delete("/{id}", roomHandler.Delete)
			})
			// NOTE(01.5): r.Get("/ws", wsHandler.Upgrade) lands in the next plan.
			// Keep the route group structure as-is so 01.5 only adds a single
			// line inside this closure.
		})
	}

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
