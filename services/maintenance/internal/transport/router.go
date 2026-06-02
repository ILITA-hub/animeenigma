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

func NewRouter(submitReport ReportSubmitFunc, submitAlert AlertEventFunc, webhookUser, webhookPass string) *chi.Mux {
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

	if submitAlert != nil && webhookPass != "" {
		r.Post("/api/grafana-webhook", webhookHandler(submitAlert, webhookUser, webhookPass))
	}

	r.Post("/api/reports", reportsHandler(submitReport))

	return r
}

// reportsHandler accepts user-submitted reports from the player service: per-player
// error reports (which carry anime context) and footer "Обратная связь" feedback
// (player_type=feedback, no anime context).
func reportsHandler(submitReport ReportSubmitFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var report domain.ReportRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&report); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		if report.PlayerType == "" {
			http.Error(w, `{"error":"player_type required"}`, http.StatusBadRequest)
			return
		}
		// anime_name is only meaningful for per-player error reports. Footer
		// feedback carries no anime context, so requiring it here silently
		// dropped every feedback message before it could reach Telegram.
		if report.PlayerType != "feedback" && report.AnimeName == "" {
			http.Error(w, `{"error":"anime_name required"}`, http.StatusBadRequest)
			return
		}
		submitReport(report)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}
