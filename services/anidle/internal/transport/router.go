package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/handler"
)

func NewRouter(healthHandler *handler.HealthHandler, log *logger.Logger, mc *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(mc.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(middleware.RealIP)

	r.Get("/health", healthHandler.Health)
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	return r
}
