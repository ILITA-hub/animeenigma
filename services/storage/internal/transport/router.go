// Package transport wires the chi HTTP router for the storage service.
//
// Every route beyond the standard /health + /metrics pair lives under
// /internal/storage/* — this service has no gateway route (Docker-network
// internal only, per docs/superpowers/specs/2026-07-10-storage-service-design.md),
// so there is no JWT/auth middleware to wire, unlike watch-together/policy.
package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/handler"
)

// copyPath is the one long-running route exempt from the short per-request
// timeout: a cross-backend prefix copy streams a whole (potentially multi-GB)
// episode before it can respond, which legitimately takes minutes.
const copyPath = "/internal/storage/copy"

// ScopedTimeout wraps next so every route EXCEPT /internal/storage/copy keeps
// an effective `short` response ceiling (503 via http.TimeoutHandler past it),
// while the copy route passes through untouched and is bounded only by the
// server's WriteTimeout. This lets main.go raise the server-wide WriteTimeout
// for the copy route without letting a hung List/ingest/delete request hold
// its connection open for the same 20 minutes.
func ScopedTimeout(next http.Handler, short time.Duration) http.Handler {
	shortHandler := http.TimeoutHandler(next, short,
		`{"success":false,"error":{"code":"TIMEOUT","message":"request exceeded the storage service response deadline"}}`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == copyPath {
			next.ServeHTTP(w, r)
			return
		}
		shortHandler.ServeHTTP(w, r)
	})
}

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
