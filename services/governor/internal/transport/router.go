// Package transport wires the governor's chi router: health, metrics, and the
// read-only degradation status endpoint.
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/handler"
)

// NewRouter builds the governor HTTP surface.
func NewRouter(statusH *handler.StatusHandler, log *logger.Logger, metricsCollector *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(middleware.RealIP)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Read-only current state (level, reasons, raw signals, override).
	// Docker-network + host-debug only; the gateway does not route here.
	r.Get("/api/degradation/status", statusH.ServeHTTP)

	return r
}
