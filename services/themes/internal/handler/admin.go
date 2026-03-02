package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/service"
)

type AdminHandler struct {
	syncService *service.SyncService
	log         *logger.Logger
}

func NewAdminHandler(syncService *service.SyncService, log *logger.Logger) *AdminHandler {
	return &AdminHandler{
		syncService: syncService,
		log:         log,
	}
}

// TriggerSync handles POST /api/themes/admin/sync?year=2026&season=winter
func (h *AdminHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	year := 0
	if yearStr := r.URL.Query().Get("year"); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			year = y
		}
	}
	season := r.URL.Query().Get("season")

	if err := h.syncService.StartSync(year, season); err != nil {
		httputil.BadRequest(w, err.Error())
		return
	}

	status := h.syncService.GetStatus()
	httputil.OK(w, status)
}

// GetSyncStatus handles GET /api/themes/admin/sync/status
func (h *AdminHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	status := h.syncService.GetStatus()
	httputil.OK(w, status)
}
