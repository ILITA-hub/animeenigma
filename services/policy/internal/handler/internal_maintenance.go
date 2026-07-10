package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// InternalMaintenanceHandler is the Docker-network + host-loopback surface a
// routine uses to read its gate and write back status. NOT gateway-proxied.
type InternalMaintenanceHandler struct {
	svc *service.MaintenanceService
	log *logger.Logger
}

func NewInternalMaintenanceHandler(svc *service.MaintenanceService, log *logger.Logger) *InternalMaintenanceHandler {
	return &InternalMaintenanceHandler{svc: svc, log: log}
}

type gateResponse struct {
	Enabled  bool                `json:"enabled"`
	Settings domain.SettingsJSON `json:"settings"`
}

func (h *InternalMaintenanceHandler) Gate(w http.ResponseWriter, r *http.Request) {
	row, err := h.svc.Gate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, gateResponse{Enabled: row.Enabled, Settings: row.Settings})
}

type statusRequest struct {
	OK        bool       `json:"ok"`
	Summary   string     `json:"summary"`
	NextRunAt *time.Time `json:"next_run_at"`
}

func (h *InternalMaintenanceHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	if err := h.svc.SetStatus(r.Context(), id, req.OK, req.Summary, req.NextRunAt); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"id": id})
}
