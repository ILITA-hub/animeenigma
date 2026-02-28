package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ActivityHandler struct {
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

func NewActivityHandler(activityRepo *repo.ActivityRepository, log *logger.Logger) *ActivityHandler {
	return &ActivityHandler{
		activityRepo: activityRepo,
		log:          log,
	}
}

// GetFeed returns the public activity feed.
func (h *ActivityHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	before := r.URL.Query().Get("before")

	events, hasMore, err := h.activityRepo.GetFeed(r.Context(), limit, before)
	if err != nil {
		h.log.Errorw("failed to get activity feed", "error", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"events":   events,
		"has_more": hasMore,
	})
}
