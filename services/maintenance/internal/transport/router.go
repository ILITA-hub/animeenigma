package transport

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ReportSubmitFunc is called when a report arrives via HTTP.
type ReportSubmitFunc func(report domain.ReportRequest)

// NewRouter builds the maintenance HTTP surface. reportsToken, when non-empty,
// requires the X-Maintenance-Token shared secret on /api/reports (the player
// service is the sole legitimate caller). When empty the endpoint stays open
// for backward compatibility — the daemon logs a WARN at startup so the open
// posture is never silent.
func NewRouter(submitReport ReportSubmitFunc, submitAlert AlertEventFunc, webhookUser, webhookPass, reportsToken string) *chi.Mux {
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

	r.Post("/api/reports", buildReportsHandler(submitReport, reportsToken))

	return r
}

// buildReportsHandler composes the /api/reports handler, wrapping it in the
// shared-secret gate when reportsToken is set. Extracted (metrics-free) so the
// open-vs-gated decision is unit-testable without the global Prometheus
// registry NewRouter pulls in.
func buildReportsHandler(submitReport ReportSubmitFunc, reportsToken string) http.HandlerFunc {
	h := reportsHandler(submitReport)
	if reportsToken != "" {
		h = reportsTokenAuth(reportsToken, h)
	}
	return h
}

// reportsTokenAuth gates a handler behind the X-Maintenance-Token shared
// secret. The player service (the only legitimate /api/reports caller) sends
// the token; an unauthenticated POST is a direct autonomous-fix trigger, so a
// wrong or missing token is rejected before the report reaches the work queue.
func reportsTokenAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	want := []byte(token)
	return func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("X-Maintenance-Token"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
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
