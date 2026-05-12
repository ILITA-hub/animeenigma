package transport

import (
	"net"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// privateOnlyMiddleware refuses requests whose RemoteAddr is not a
// private / loopback IP — defense-in-depth for REVIEW.md WR-10. The
// admin handler trusts the gateway gate (D6 in plan 17-03) AND it lives
// on the docker-internal network, but if a future maintainer accidentally
// changes SERVER_HOST or exposes the port externally, the docker-internal
// IP check here still rejects external traffic.
//
// Inside docker-compose every other container's source IP falls in the
// bridge subnet (172.x.x.x — RFC-1918), so legitimate gateway → scraper
// traffic is accepted. Direct external traffic (a public IP) is rejected
// with 403.
//
// This is intentionally lenient: it does NOT replace the gateway's JWT
// + AdminRoleMiddleware gate — it only guarantees that "this request
// could only have come from a docker-internal sibling".
func privateOnlyMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			ip := net.ParseIP(host)
			if ip == nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
				log.Warnw("scraper.admin: rejected non-private RemoteAddr",
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path,
				)
				httputil.Forbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
	// gate per D6, with the WR-10 private-IP defense-in-depth applied to
	// the admin sub-route only).
	r.Route("/scraper", func(r chi.Router) {
		r.Get("/episodes", scraperHandler.GetEpisodes)
		r.Get("/servers", scraperHandler.GetServers)
		r.Get("/stream", scraperHandler.GetStream)
		r.Get("/health", scraperHandler.GetHealth)
		// REVIEW.md WR-10: even though plan 17-03 D6 documents that this
		// route is gateway-gated, add a private-IP guard so a future
		// SERVER_HOST=0.0.0.0 + accidental port exposure does NOT allow
		// public access to the admin snapshot.
		r.Group(func(r chi.Router) {
			r.Use(privateOnlyMiddleware(log))
			r.Get("/health/admin", scraperHandler.GetAdminHealth)
		})
	})

	return r
}
