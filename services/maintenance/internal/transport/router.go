package transport

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// ReportSubmitFunc is called when a report arrives via HTTP.
type ReportSubmitFunc func(report domain.ReportRequest)

func NewRouter(submitReport ReportSubmitFunc) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	collector := metrics.NewCollector("maintenance")
	r.Use(collector.Middleware)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Handle("/metrics", metrics.Handler())

	r.Post("/api/reports", func(w http.ResponseWriter, r *http.Request) {
		var report domain.ReportRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&report); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		if report.PlayerType == "" || report.AnimeName == "" {
			http.Error(w, `{"error":"player_type and anime_name required"}`, http.StatusBadRequest)
			return
		}
		submitReport(report)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	})

	return r
}
