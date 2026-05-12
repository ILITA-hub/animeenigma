package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the scraper service.
//
// Phase 15 plan 03 extends plan 01's router with the /scraper/* business
// routes routed to the ScraperHandler. The operational /health and /metrics
// endpoints remain at the root level — `/scraper/health` is the
// orchestrator-aware health endpoint, NOT to be confused with `/health`
// which is the docker-compose healthcheck target.
func NewRouter(
	scraperHandler *handler.ScraperHandler,
	cfg *config.Config,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	// REVIEW.md WR-02: CORS middleware intentionally omitted. The scraper is
	// a backend-to-backend service (bound to 127.0.0.1:8088, called only by
	// catalog) — it is never hit directly from a browser, so an
	// `Access-Control-Allow-Origin: *` header would be both unnecessary and
	// silently permissive if the bind address changes in the future.
	r.Use(middleware.RealIP)

	// Service liveness (docker-compose healthcheck target). Distinct from
	// /scraper/health which returns the per-provider orchestrator snapshot.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Prometheus exposition.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Scraper business routes — Phase 15 ships 503 stubs for the first three
	// and a live HealthSnapshot for the fourth. Phase 17 Plan 03 adds the
	// admin debug endpoint (gateway-gated; this handler trusts the gateway
	// gate per D6).
	r.Route("/scraper", func(r chi.Router) {
		r.Get("/episodes", scraperHandler.GetEpisodes)
		r.Get("/servers", scraperHandler.GetServers)
		r.Get("/stream", scraperHandler.GetStream)
		r.Get("/health", scraperHandler.GetHealth)
		r.Get("/health/admin", scraperHandler.GetAdminHealth) // Plan 17-03
	})

	return r
}
