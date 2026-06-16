package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the analytics chi router.
//
//	GET  /health                  (public)
//	HEAD /health                  (public — Docker healthcheck wget --spider)
//	GET  /metrics                  (public, prom format)
//	POST /api/analytics/collect    (public — anonymous users tracked; gateway
//	                                forwards the full path UNCHANGED, same as
//	                                every other service serves /api/<name>/...)
//	POST /api/analytics/client-errors (public — FE error log sink, log-only)
//	POST /api/analytics/player-events (public — player resolve/stall telemetry)
//	POST /internal/effects         (internal — BE egress producer sink)
//	POST /internal/erase           (internal — gateway never proxies /internal/*)
//	POST /internal/read-thresholds/recompute (internal — scheduler daily trigger)
func NewRouter(
	collect *handler.CollectHandler,
	clientError *handler.ClientErrorHandler,
	playerTelemetry *handler.PlayerTelemetryHandler,
	effects *handler.EffectsHandler,
	admin *handler.AdminHandler,
	readThresholds *handler.ReadThresholdHandler,
	log *logger.Logger,
	collector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(collector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	healthHandler := func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	}
	r.Get("/health", healthHandler)
	// The Docker healthcheck uses `wget --spider`, which issues a HEAD request.
	// Without an explicit HEAD route chi returns 405, leaving the container
	// flagged unhealthy even though the service is serving fine. Register HEAD
	// too so the compose healthcheck (and any HEAD probe) passes.
	r.Head("/health", healthHandler)
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// The gateway forwards the full request path unchanged, so the public
	// ingestion route must be the full /api/analytics/collect (mirrors how
	// notifications serves /api/notifications/...). /internal/* is hit
	// directly inside the Docker network and is never gateway-proxied.
	r.Post("/api/analytics/collect", collect.ServeHTTP)
	// /api/analytics/client-errors is the public FE error log sink (log-only,
	// no DB write). Same anonymous trust model as /collect; gateway-proxied.
	r.Post("/api/analytics/client-errors", clientError.ServeHTTP)
	// /api/analytics/player-events ingests frontend player telemetry (resolve
	// outcomes + stalls) into the events table via the shared batcher Sink,
	// using effect_kind "player_resolve" / "player_stall". Gateway-proxied.
	r.Post("/api/analytics/player-events", playerTelemetry.ServeHTTP)
	// /internal/effects ingests BE egress/db/cache effect batches from the
	// libs/tracing producer. Like /internal/erase it lives only here and is
	// never gateway-proxied (Docker-network-only; T-02-INT).
	r.Post("/internal/effects", effects.ServeHTTP)
	r.Post("/internal/erase", admin.Erase)
	// /internal/read-thresholds/recompute triggers the daily db_read P95
	// compute + read_thresholds Redis-hash publish (D-03). The scheduler service
	// (no ClickHouse connection) POSTs here on its daily cron. Docker-network
	// only — never gateway-proxied (T-03-15).
	if readThresholds != nil {
		r.Post("/internal/read-thresholds/recompute", readThresholds.ServeHTTP)
	}

	return r
}
