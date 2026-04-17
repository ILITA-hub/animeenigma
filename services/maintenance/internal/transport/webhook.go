package transport

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// AlertEventFunc is called when a Grafana webhook arrives.
// The callback MUST be non-blocking (push to workChan and return).
type AlertEventFunc func(payload domain.GrafanaWebhookPayload)

func webhookHandler(submitAlert AlertEventFunc, webhookUser, webhookPass string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != webhookUser || pass != webhookPass {
			webhookErrors.WithLabelValues("auth").Inc()
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var payload domain.GrafanaWebhookPayload
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			webhookErrors.WithLabelValues("parse").Inc()
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		if len(payload.Alerts) == 0 {
			webhookErrors.WithLabelValues("parse").Inc()
			http.Error(w, `{"error":"no alerts in payload"}`, http.StatusBadRequest)
			return
		}

		webhooksReceived.WithLabelValues(payload.Status).Inc()
		submitAlert(payload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}
