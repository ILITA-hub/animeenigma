package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// StaffHandler serves the anime staff/crew endpoint.
type StaffHandler struct {
	svc *service.StaffService
	log *logger.Logger
}

func NewStaffHandler(svc *service.StaffService, log *logger.Logger) *StaffHandler {
	return &StaffHandler{svc: svc, log: log}
}

// GetAnimeStaff handles GET /api/anime/{animeId}/staff.
func (h *StaffHandler) GetAnimeStaff(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	list, err := h.svc.GetAnimeStaff(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, list)
}
