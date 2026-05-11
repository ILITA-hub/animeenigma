package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the scraper service.
//
// Phase 15 plan 01: only the operational endpoints (/health, /metrics) are
// registered. Plan 03 will extend this router with /scraper/{episodes,
// servers,stream} routes once the orchestrator + handlers exist.
//
// The middleware chain mirrors the catalog service so a request through
// scraper has the same logging shape, request-ID propagation, and
// Prometheus metric labels.
func NewRouter(cfg *config.Config, log *logger.Logger, metricsCollector *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	return r
}
