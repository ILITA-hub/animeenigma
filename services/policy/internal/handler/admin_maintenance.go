package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// AdminMaintenanceHandler is the admin CRUD surface for maintenance routines
// (JWT + admin gated at the router; the gateway re-applies both). Nested under
// /api/admin/policy/maintenance so it reuses the existing policy admin proxy group.
type AdminMaintenanceHandler struct {
	svc *service.MaintenanceService
	log *logger.Logger
}

func NewAdminMaintenanceHandler(svc *service.MaintenanceService, log *logger.Logger) *AdminMaintenanceHandler {
	return &AdminMaintenanceHandler{svc: svc, log: log}
}

type listRoutinesResponse struct {
	Routines []domain.MaintenanceRoutine `json:"routines"`
}

func (h *AdminMaintenanceHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, listRoutinesResponse{Routines: rows})
}

type setRoutineRequest struct {
	Enabled  bool                `json:"enabled"`
	Settings domain.SettingsJSON `json:"settings"`
}

func (h *AdminMaintenanceHandler) SetRoutine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req setRoutineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	if err := h.svc.SetRoutine(r.Context(), id, req.Enabled, req.Settings); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"id": id})
}
