package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/go-chi/chi/v5"
)

type ActivityHandler struct {
	activityRepo *repo.ActivityRepository
	followRepo   *repo.FollowRepository
	log          *logger.Logger
}

func NewActivityHandler(activityRepo *repo.ActivityRepository, followRepo *repo.FollowRepository, log *logger.Logger) *ActivityHandler {
	return &ActivityHandler{
		activityRepo: activityRepo,
		followRepo:   followRepo,
		log:          log,
	}
}

// GetFeed returns the public activity feed.
func (h *ActivityHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	limit := parseActivityLimit(r)

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

// GetFollowingFeed returns activity from the current user's subscriptions.
func (h *ActivityHandler) GetFollowingFeed(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	limit := parseActivityLimit(r)
	events, hasMore, err := h.activityRepo.GetFollowingFeed(
		r.Context(), claims.UserID, r.URL.Query().Get("user_id"), limit, r.URL.Query().Get("before"),
	)
	if err != nil {
		h.log.Errorw("failed to get following activity feed", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{"events": events, "has_more": hasMore})
}

func (h *ActivityHandler) ListFollowing(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	users, err := h.followRepo.List(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]interface{}{"users": users})
}

func (h *ActivityHandler) GetFollowStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	following, err := h.followRepo.IsFollowing(r.Context(), claims.UserID, chi.URLParam(r, "userId"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"following": following})
}

func (h *ActivityHandler) Follow(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	targetID := chi.URLParam(r, "userId")
	if targetID == "" || targetID == claims.UserID {
		httputil.Error(w, errors.InvalidInput("cannot follow this user"))
		return
	}
	exists, err := h.followRepo.UserExists(r.Context(), targetID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if !exists {
		httputil.Error(w, errors.NotFound("user"))
		return
	}
	if err := h.followRepo.Follow(r.Context(), claims.UserID, targetID); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"following": true})
}

func (h *ActivityHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	if err := h.followRepo.Unfollow(r.Context(), claims.UserID, chi.URLParam(r, "userId")); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"following": false})
}

func parseActivityLimit(r *http.Request) int {
	limit := 10
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 50 {
		limit = parsed
	}
	return limit
}
