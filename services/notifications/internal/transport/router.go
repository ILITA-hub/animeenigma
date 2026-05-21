package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the notifications service.
//
// Route shape (matches design doc §API and PLAN.md interfaces block):
//
//	GET    /health                         (public)
//	GET    /metrics                        (public, prom format)
//	POST   /internal/notifications         (internal — gateway never proxies)
//	GET    /internal/health                (internal — gateway never proxies)
//	GET    /api/notifications              (JWT)
//	GET    /api/notifications/unread-count (JWT)
//	POST   /api/notifications/mark-all-read(JWT)
//	POST   /api/notifications/{id}/read    (JWT)
//	POST   /api/notifications/{id}/dismiss (JWT)
//	POST   /api/notifications/{id}/click   (JWT)
//
// Literal sub-paths (`mark-all-read`, `unread-count`) are registered BEFORE
// the param sub-paths (`{id}/...`) so chi's resolver does not shadow them
// (R6 in the plan's risks block).
func NewRouter(
	notifHandler *handler.NotificationHandler,
	internalHandler *handler.InternalHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	// Middleware — same stack as services/themes for cross-service
	// consistency (logs, metrics, request-id all line up in Grafana).
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Public health check.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Prometheus metrics endpoint.
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// Internal routes at the root, no middleware (D-05). Reachable only
	// inside the Docker network because the gateway never proxies
	// /internal/*. The Phase 2 detector calls these.
	r.Post("/internal/notifications", internalHandler.CreateNotification)
	r.Get("/internal/health", internalHandler.Health)

	// Public CRUD — all behind JWT.
	r.Route("/api/notifications", func(r chi.Router) {
		r.Use(AuthMiddleware(jwtConfig))

		// Literal routes BEFORE param routes (chi precedence safety).
		r.Get("/", notifHandler.List)
		r.Get("/unread-count", notifHandler.UnreadCount)
		r.Post("/mark-all-read", notifHandler.MarkAllRead)

		// Param routes.
		r.Post("/{id}/read", notifHandler.MarkRead)
		r.Post("/{id}/dismiss", notifHandler.Dismiss)
		r.Post("/{id}/click", notifHandler.Click)
	})

	return r
}

// AuthMiddleware validates JWT access tokens and populates ctx with claims.
// Copied verbatim from services/themes/internal/transport/router.go
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
