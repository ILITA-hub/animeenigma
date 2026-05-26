// Package transport wires the chi HTTP router for the watch-together
// service. Phase 1 plan progression:
//
//	01.1 — /health + /metrics (this file's original form)
//	01.4 — /api/watch-together/rooms (REST CRUD behind AuthMiddleware)
//	01.5 — /api/watch-together/ws    (WebSocket upgrade, auth via ?token=)
//
// IMPORTANT route group structure: the /ws endpoint is NOT inside the
// AuthMiddleware-wrapped subgroup. Browsers can't set custom headers on
// WS upgrades (the Sec-WebSocket-* handshake is strict), so the WS
// handler validates the JWT from a query param itself. Mounting it under
// the same AuthMiddleware as /rooms would 401 every WS upgrade attempt.
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
// Route shape after Plan 01.5:
//
//	GET    /health                                 — public liveness
//	GET    /metrics                                — Prometheus scrape target
//	GET    /api/watch-together/ws                  — WS upgrade (auth via ?token=)
//	POST   /api/watch-together/rooms               — JWT-protected (01.4)
//	GET    /api/watch-together/rooms/{id}          — JWT-protected (01.4)
//	DELETE /api/watch-together/rooms/{id}          — JWT-protected (01.4)
//
// The /ws endpoint sits OUTSIDE the AuthMiddleware-wrapped subgroup because
// browsers can't set Authorization: Bearer on a WS upgrade (see package doc).
// The WS handler validates the JWT itself from the ?token= query param.
//
// roomHandler / wsHandler may each be nil; if so the corresponding routes
// are NOT mounted. Used by unit tests that exercise the middleware stack
// without the full DI graph.
func NewRouter(
	cfg *config.Config,
	roomHandler *handler.RoomHandler,
	wsHandler *handler.WebSocketHandler,
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

	// /api/watch-together subtree with SPLIT auth handling:
	//   - /ws    → no chi-level AuthMiddleware (handler does its own JWT check
	//              from ?token= because browsers can't set Authorization on
	//              the WS upgrade handshake).
	//   - /rooms → wrapped in AuthMiddleware as before (REST clients can set
	//              standard Authorization: Bearer headers).
	r.Route("/api/watch-together", func(r chi.Router) {
		// 01.5 — WS upgrade. Mounted OUTSIDE the AuthMiddleware group.
		if wsHandler != nil {
			r.Get("/ws", wsHandler.Upgrade)
		}

		// 01.4 — REST CRUD. Mounted INSIDE the AuthMiddleware group.
		if roomHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(cfg.JWT))
				r.Route("/rooms", func(r chi.Router) {
					r.Post("/", roomHandler.Create)
					r.Get("/{id}", roomHandler.Get)
					r.Delete("/{id}", roomHandler.Delete)
				})
			})
		}
	})

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
