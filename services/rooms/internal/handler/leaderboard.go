package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
)

type LeaderboardHandler struct {
	leaderboardService *service.LeaderboardService
	log                *logger.Logger
}

func NewLeaderboardHandler(leaderboardService *service.LeaderboardService, log *logger.Logger) *LeaderboardHandler {
	return &LeaderboardHandler{
		leaderboardService: leaderboardService,
		log:                log,
	}
}

// GetLeaderboard returns the global leaderboard
func (h *LeaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	leaderboard, err := h.leaderboardService.GetGlobalLeaderboard(r.Context(), 100)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, leaderboard)
}
