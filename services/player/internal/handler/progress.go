package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type ProgressHandler struct {
	progressService *service.ProgressService
	log             *logger.Logger
}

func NewProgressHandler(progressService *service.ProgressService, log *logger.Logger) *ProgressHandler {
	return &ProgressHandler{
		progressService: progressService,
		log:             log,
	}
}

// UpdateProgress updates watch progress for an episode
func (h *ProgressHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateProgressRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.Player != "" && !domain.ValidateCombo(req.Player, req.Language, req.WatchType) {
		httputil.Error(w, errors.InvalidInput("invalid combo fields: player, language, or watch_type"))
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	progress, err := h.progressService.UpdateProgress(r.Context(), claims.UserID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	metrics.WatchProgressSavesTotal.Inc()
	httputil.OK(w, progress)
}

// GetProgress returns watch progress for an anime
func (h *ProgressHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	progress, err := h.progressService.GetProgress(r.Context(), claims.UserID, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, progress)
}

// MarkDropOff is the drop-off beacon endpoint. Receives a navigator.sendBeacon
// payload from the player when the user closes the page mid-episode. Records
// where the user stopped so Phase 6 Tier 2 inference and future analytics can
// distinguish "completed" from "abandoned at minute X". Phase 5 (G-01).
//
// Beacons typically arrive with Content-Type "text/plain;charset=UTF-8" or
// no header at all — httputil.Bind tolerates both because it only inspects
// the body when present.
func (h *ProgressHandler) MarkDropOff(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	var req domain.DropOffRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if req.EpisodeNumber <= 0 {
		httputil.Error(w, errors.InvalidInput("episode_number must be > 0"))
		return
	}
	if req.Progress < 0 {
		httputil.Error(w, errors.InvalidInput("progress must be >= 0"))
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	if err := h.progressService.MarkDropOff(r.Context(), claims.UserID, animeID, &req); err != nil {
		// Beacon endpoints can't surface errors to the user (the page is
		// already gone). Log and return success so the browser doesn't
		// retry — there's nothing the client can do about the failure.
		h.log.Errorw("failed to record dropoff",
			"user_id", claims.UserID,
			"anime_id", animeID,
			"episode", req.EpisodeNumber,
			"progress_secs", req.Progress,
			"error", err,
		)
	}
	metrics.WatchProgressSavesTotal.Inc()
	httputil.OK(w, map[string]bool{"recorded": true})
}
