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
