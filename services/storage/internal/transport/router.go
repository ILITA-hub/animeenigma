// Package transport wires the chi HTTP router for the storage service.
//
// Every route beyond the standard /health + /metrics pair lives under
// /internal/storage/* — this service has no gateway route (Docker-network
// internal only, per docs/superpowers/specs/2026-07-10-storage-service-design.md),
// so there is no JWT/auth middleware to wire, unlike watch-together/policy.
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/handler"
)

// NewRouter builds the chi router for the storage service.
//
// Route shape:
//
//	GET  /health                              — public liveness
//	GET  /metrics                              — Prometheus scrape target
//	POST /internal/storage/ingest-urls
//	POST /internal/storage/download-urls
//	POST /internal/storage/move
//	POST /internal/storage/copy
//	DELETE /internal/storage/prefix
//	GET  /internal/storage/list
//	GET  /internal/storage/base-urls
//	GET  /internal/storage/health              — probes both backends
func NewRouter(h *handler.StorageHandler, log *logger.Logger, metricsCollector *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	// Same middleware order as watch-together/policy so request-id, metrics,
	// and logs line up across services in Grafana.
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Public liveness. Accepts GET (curl-style probes) and HEAD (BusyBox
	// `wget --spider`, the default in our docker healthcheck stanzas).
	healthHandler := func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	}
	r.Get("/health", healthHandler)
	r.Head("/health", healthHandler)

	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// Docker-network-only — the gateway does not proxy /internal/*.
	r.Route("/internal/storage", func(r chi.Router) {
		r.Post("/ingest-urls", h.IngestURLs)
		r.Post("/download-urls", h.DownloadURLs)
		r.Post("/move", h.Move)
		r.Post("/copy", h.Copy)
		r.Delete("/prefix", h.DeletePrefix)
		r.Get("/list", h.List)
		r.Get("/base-urls", h.BaseURLs)
		r.Get("/health", h.Health)
	})

	return r
}
