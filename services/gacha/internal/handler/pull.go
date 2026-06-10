package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
	"github.com/go-chi/chi/v5"
)

// PullHandler serves the authenticated player-facing gacha endpoints:
// POST /banners/{id}/pull, GET /banners, GET /collection.
type PullHandler struct {
	svc *service.PullService
	log *logger.Logger
}

func NewPullHandler(svc *service.PullService, log *logger.Logger) *PullHandler {
	return &PullHandler{svc: svc, log: log}
}

// pullRequest is the body of POST /banners/{id}/pull.
type pullRequest struct {
	Mode string `json:"mode"`
}

// Pull handles POST /api/gacha/banners/{id}/pull. Body: {"mode":"x1"|"x10"}.
func (h *PullHandler) Pull(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid banner id"))
		return
	}
	var req pullRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	res, err := h.svc.Pull(r.Context(), claims.UserID, id, req.Mode)
	if err != nil {
		h.log.Errorw("pull failed", "user_id", claims.UserID, "banner_id", id, "mode", req.Mode, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, res)
}

// Banners handles GET /api/gacha/banners — active banners + the caller's pity.
func (h *PullHandler) Banners(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	views, err := h.svc.ActiveBannersView(r.Context(), claims.UserID)
	if err != nil {
		h.log.Errorw("active banners view failed", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, views)
}

// Collection handles GET /api/gacha/collection — the full album + progress.
func (h *PullHandler) Collection(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	view, err := h.svc.CollectionView(r.Context(), claims.UserID)
	if err != nil {
		h.log.Errorw("collection view failed", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, view)
}
