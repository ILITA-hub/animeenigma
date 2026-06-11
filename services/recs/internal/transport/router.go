package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/handler"
)

func NewRouter(
	recsHandler *handler.RecsHandler,
	adminRecsHandler *handler.AdminRecsHandler,
	recEventsHandler *handler.RecEventsHandler,
	internalHintHandler *handler.InternalHintHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	// Middleware — mirrors player's stack order exactly.
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check — mirrors player's httputil.OK shape.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Prometheus metrics endpoint.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Docker-network-only producer endpoint — the gateway does NOT proxy
	// /internal/* (same rule as notifications' /internal/notifications).
	r.Post("/internal/recs/recompute-hint", internalHintHandler.PostRecomputeHint)

	r.Route("/api", func(r chi.Router) {
		// Phase 10: anonymous trending recs row.
		// MUST live OUTSIDE a protected /users group — OptionalAuthMiddleware
		// so JWTs decode if present but anonymous callers pass through.
		// URL contract: gateway proxies /api/users/recs verbatim to recs:8094.
		r.Route("/users/recs", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/", recsHandler.GetRecs)
		})

		// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug + force-recompute.
		// AuthMiddleware first (401 on missing/invalid JWT), AdminRoleMiddleware
		// second (403 on non-admin role). Defense-in-depth: the gateway also
		// applies the same gates.
		r.Route("/admin/recs", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Get("/{user_id}", adminRecsHandler.GetAdminRecs)
			r.Post("/{user_id}/recompute", adminRecsHandler.ForceRecompute)
		})

		// Phase 14 (REC-EVAL-01): public telemetry endpoint. JWT-OPTIONAL —
		// anonymous trending CTR data is valid. The handler increments
		// Prometheus counters AND persists a rec_events row.
		r.Route("/events", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Post("/rec", recEventsHandler.PostRecEvent)
		})
	})

	return r
}

// AuthMiddleware validates JWT tokens and attaches claims to the request context.
// Returns 401 when the token is missing or invalid.
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
