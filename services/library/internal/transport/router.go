package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter wires the chi router used by the library service. Phase
// 2 adds GET /api/library/search and Phase 3 adds the /api/library/
// jobs group (POST/GET/GET-by-id/DELETE) under the existing
// /api/library route group; auth is enforced at the gateway (see
// services/gateway/internal/transport/router.go — the /api/library/*
// prefix has JWTValidationMiddleware + AdminRoleMiddleware applied
// for all routes except /health).
//
// The jwtConfig argument is retained for forward compat — Phase 4+
// may want server-side enforcement on a subset of routes.
func NewRouter(
	healthHandler *handler.HealthHandler,
	searchHandler *handler.SearchHandler,
	jobsHandler *handler.JobsHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	// Standard middleware chain (mirrors services/themes/internal/transport/router.go).
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check — exposed at /health for direct probes (make health,
	// docker healthcheck) and at /api/library/health so the gateway can
	// forward `/api/library/health` verbatim without path rewriting.
	r.Get("/health", healthHandler.Health)

	// Prometheus metrics endpoint.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// API routes. Phase 2 adds /search; Phase 3 adds the job-control
	// group. Gateway-side admin gate covers all /api/library/*
	// non-/health routes (services/gateway/internal/transport/router.go).
	r.Route("/api/library", func(r chi.Router) {
		_ = jwtConfig // retained for forward compat (Phase 4+)
		r.Get("/health", healthHandler.Health)
		r.Get("/search", searchHandler.Search)
		if jobsHandler != nil {
			r.Post("/jobs", jobsHandler.Create)
			r.Get("/jobs", jobsHandler.List)
			r.Get("/jobs/{id}", jobsHandler.Get)
			r.Delete("/jobs/{id}", jobsHandler.Delete)
		}
	})

	return r
}
