package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type ListHandler struct {
	listService *service.ListService
	log         *logger.Logger
}

func NewListHandler(listService *service.ListService, log *logger.Logger) *ListHandler {
	return &ListHandler{
		listService: listService,
		log:         log,
	}
}

// GetUserList returns user's anime list
func (h *ListHandler) GetUserList(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	status := r.URL.Query().Get("status")

	list, err := h.listService.GetUserList(r.Context(), claims.UserID, status)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, list)
}

// UpdateListEntry updates an anime list entry
func (h *ListHandler) UpdateListEntry(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateListRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	entry, err := h.listService.UpdateListEntry(r.Context(), claims.UserID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, entry)
}

// DeleteListEntry removes an anime from user's list
func (h *ListHandler) DeleteListEntry(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	err := h.listService.DeleteListEntry(r.Context(), claims.UserID, animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.NoContent(w)
}
