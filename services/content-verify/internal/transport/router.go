package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/handler"
)

func NewRouter(h *handler.VerifyHandler, log *logger.Logger, collector *metrics.Collector) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	if collector != nil {
		r.Use(collector.Middleware)
	}
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Handle("/metrics", metrics.Handler())
	if h != nil {
		r.Get("/internal/verify/verdicts", h.Verdicts)
		r.Post("/internal/verify/hint", h.Hint)
		r.Get("/internal/verify/queue", h.Queue)
	}
	return r
}
