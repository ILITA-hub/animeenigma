package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
)

type HistoryHandler struct {
	historyService *service.HistoryService
	log            *logger.Logger
}

func NewHistoryHandler(historyService *service.HistoryService, log *logger.Logger) *HistoryHandler {
	return &HistoryHandler{
		historyService: historyService,
		log:            log,
	}
}

// GetWatchHistory returns user's watch history
func (h *HistoryHandler) GetWatchHistory(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	history, err := h.historyService.GetWatchHistory(r.Context(), claims.UserID, 100)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, history)
}
