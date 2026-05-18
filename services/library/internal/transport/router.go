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

// NewRouter wires the chi router used by the library service. Phase 1 is
// scaffold-only: /health, /metrics, and an empty /api/library subroute. The
// jwtConfig argument is retained even though no authenticated routes exist
// yet — Phase 3 (LIB-03/04) reintroduces auth middleware when it adds the
// protected job-control endpoints (POST /api/library/jobs, etc.).
func NewRouter(
	healthHandler *handler.HealthHandler,
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

	// Health check.
	r.Get("/health", healthHandler.Health)

	// Prometheus metrics endpoint.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// API routes. Phase 1 leaves this empty on purpose — Phase 2 (LIB-03/04/04b)
	// appends search + ingest endpoints; Phase 3 adds the job-control group with
	// AuthMiddleware + AdminRoleMiddleware. The jwtConfig parameter is kept on
	// the constructor signature so those phases don't need to rewire main.go.
	r.Route("/api/library", func(r chi.Router) {
		_ = jwtConfig // silence unused-parameter lint until Phase 3
	})

	return r
}
