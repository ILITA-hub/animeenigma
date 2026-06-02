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
//	GET  /metrics                  (public, prom format)
//	POST /api/analytics/collect    (public — anonymous users tracked; gateway
//	                                forwards the full path UNCHANGED, same as
//	                                every other service serves /api/<name>/...)
//	POST /internal/erase           (internal — gateway never proxies /internal/*)
func NewRouter(
	collect *handler.CollectHandler,
	admin *handler.AdminHandler,
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

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// The gateway forwards the full request path unchanged, so the public
	// ingestion route must be the full /api/analytics/collect (mirrors how
	// notifications serves /api/notifications/...). /internal/* is hit
	// directly inside the Docker network and is never gateway-proxied.
	r.Post("/api/analytics/collect", collect.ServeHTTP)
	r.Post("/internal/erase", admin.Erase)

	return r
}
