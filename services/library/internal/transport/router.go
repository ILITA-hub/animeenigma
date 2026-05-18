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

// NewRouter wires the chi router used by the library service. Phase 2
// adds GET /api/library/search under the existing /api/library route
// group; auth is enforced at the gateway (see
// services/gateway/internal/transport/router.go — the /api/library/*
// prefix has JWTValidationMiddleware + AdminRoleMiddleware applied for
// all routes except /health).
//
// The jwtConfig argument is retained even though no authenticated
// routes exist at the library service layer — Phase 3 (LIB-09 extended
// health, etc.) reintroduces server-side auth middleware groups.
func NewRouter(
	healthHandler *handler.HealthHandler,
	searchHandler *handler.SearchHandler,
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

	// API routes. Phase 2 adds /search; Phase 3 will add the job-control
	// group (POST /jobs, etc.) with server-side AuthMiddleware +
	// AdminRoleMiddleware.
	r.Route("/api/library", func(r chi.Router) {
		_ = jwtConfig // silence unused-parameter lint until Phase 3
		r.Get("/health", healthHandler.Health)
		r.Get("/search", searchHandler.Search)
	})

	return r
}
