package handler

import (
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

// Incident is one row in the system-status response. v0.1 always surfaces
// at most ONE incident sourced from env (SYSTEM_BANNER_ACTIVE +
// SYSTEM_BANNER_MESSAGE). Future phases will back this with a real ops
// pipeline read.
type Incident struct {
	ID       string    `json:"id"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Since    time.Time `json:"since"`
}

// SystemStatusResponse is the response shape. Empty incidents slice when
// no banner is active. The frontend banner component renders nothing when
// the slice is empty, so the contract stays stable across config flips.
type SystemStatusResponse struct {
	Incidents []Incident `json:"incidents"`
}

type SystemStatusHandler struct {
	cfg *config.Config
	// started is the gateway process start time, used as the Incident.Since
	// floor. v0.1 has no real "since" — the env-backed incident is treated
	// as always-on since gateway boot.
	started time.Time
}

func NewSystemStatusHandler(cfg *config.Config) *SystemStatusHandler {
	return &SystemStatusHandler{cfg: cfg, started: time.Now().UTC()}
}

// GetStatus serves GET /api/system/status. Public, no JWT required.
// Phase 11 / UX-24.
func (h *SystemStatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	resp := SystemStatusResponse{Incidents: []Incident{}}
	if h.cfg.SystemBannerActive && h.cfg.SystemBannerMessage != "" {
		resp.Incidents = append(resp.Incidents, Incident{
			ID:       "env-active",
			Severity: "incident",
			Title:    h.cfg.SystemBannerMessage,
			Since:    h.started,
		})
	}
	httputil.OK(w, resp)
}
