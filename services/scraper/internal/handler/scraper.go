// Package handler implements the HTTP handlers for the scraper service.
//
// Phase 15 plan 03 ships:
//   - GetEpisodes / GetServers / GetStream: 503 stubs with a stable JSON
//     contract {"error":"not-yet-implemented","phase":15}.
//   - GetHealth: 200 with the orchestrator's live HealthSnapshot under
//     data.providers (httputil.Response wrapper).
//
// Phase 16+ replaces the three 503 stubs with real implementations that call
// orchestrator.ListEpisodes / ListServers / GetStream.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// ScraperHandler is the HTTP handler for /scraper/* routes. Construction
// takes the orchestrator (for HealthSnapshot today, business calls in Phase 16+)
// and the logger.
type ScraperHandler struct {
	svc *service.Orchestrator
	log *logger.Logger
}

// NewScraperHandler builds a ScraperHandler.
func NewScraperHandler(svc *service.Orchestrator, log *logger.Logger) *ScraperHandler {
	return &ScraperHandler{svc: svc, log: log}
}

// notYetImplemented writes the canonical 503 JSON body. This is the contract
// the catalog plan-04 thin client and frontend gracefully degrade against —
// keep the shape stable.
//
// We intentionally bypass httputil.Response wrapping for this stub so the
// shape is "as-spec'd in the plan" (top-level error + phase) rather than
// nested under {success:false, error:{code,message,details}}.
func notYetImplemented(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	body := map[string]any{"error": "not-yet-implemented", "phase": 15}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// Best-effort; the client has already received headers + status.
		logger.Default().Errorw("failed to encode 503 stub body", "error", err)
	}
}

// GetEpisodes handles GET /scraper/episodes. Phase 15: 503 stub.
func (h *ScraperHandler) GetEpisodes(w http.ResponseWriter, r *http.Request) {
	notYetImplemented(w)
}

// GetServers handles GET /scraper/servers. Phase 15: 503 stub.
func (h *ScraperHandler) GetServers(w http.ResponseWriter, r *http.Request) {
	notYetImplemented(w)
}

// GetStream handles GET /scraper/stream. Phase 15: 503 stub.
func (h *ScraperHandler) GetStream(w http.ResponseWriter, r *http.Request) {
	notYetImplemented(w)
}

// GetHealth handles GET /scraper/health. Returns the orchestrator's live
// HealthSnapshot keyed by provider name. Phase 15: zero providers → empty
// map; Phase 16+ populates as providers register.
func (h *ScraperHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	snap := h.svc.HealthSnapshot(r.Context())
	httputil.OK(w, map[string]any{"providers": snap})
}
