package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	jobHandler *handler.JobHandler,
	taskHandler *handler.TaskHandler,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
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

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Job management routes (should be protected in production)
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/status", jobHandler.GetJobStatus)
			r.Post("/shikimori-sync", jobHandler.TriggerShikimoriSync)
			r.Post("/cleanup", jobHandler.TriggerCleanup)
		})

		// Task management routes for MAL export
		r.Route("/tasks", func(r chi.Router) {
			// Export job management
			r.Post("/anime-load", taskHandler.CreateExportJob)
			r.Post("/anime-load/tasks", taskHandler.CreateTasks)
			r.Get("/anime-load/status/{exportId}", taskHandler.GetExportJobStatus)
			r.Delete("/anime-load/{malId}", taskHandler.DeletePendingTask)

			// Worker status
			r.Get("/worker/status", taskHandler.GetWorkerStatus)
		})
	})

	return r
}
