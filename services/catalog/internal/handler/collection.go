package handler

// Phase 17 (UX-33) — editorial collections HTTP layer.

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

type CollectionHandler struct {
	svc *service.CollectionService
	log *logger.Logger
}

func NewCollectionHandler(svc *service.CollectionService, log *logger.Logger) *CollectionHandler {
	return &CollectionHandler{svc: svc, log: log}
}

// ListPublic: GET /api/collections?limit=N (default 12, max 50).
// Returns published collections only.
func (h *CollectionHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	limit := 12
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 50 {
		limit = 50
	}
	collections, err := h.svc.ListPublished(r.Context(), limit)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if collections == nil {
		collections = []*domain.Collection{}
	}
	httputil.OK(w, collections)
}

// GetBySlug: GET /api/collections/{slug}. Drafts return 404.
func (h *CollectionHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httputil.BadRequest(w, "slug is required")
		return
	}
	c, err := h.svc.GetBySlug(r.Context(), slug)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, c)
}

// ListAdmin: GET /api/admin/collections. Returns drafts + published.
func (h *CollectionHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	collections, err := h.svc.ListAdmin(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if collections == nil {
		collections = []*domain.Collection{}
	}
	httputil.OK(w, collections)
}

// GetAdmin: GET /api/admin/collections/{id}. Returns the row (drafts ok).
func (h *CollectionHandler) GetAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	c, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, c)
}

// Create: POST /api/admin/collections.
func (h *CollectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateCollectionRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if req.Title == "" {
		httputil.BadRequest(w, "title is required")
		return
	}
	createdBy := callerUserID(r)
	c, err := h.svc.Create(r.Context(), &req, createdBy)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	h.log.Infow("collection created", "id", c.ID, "slug", c.Slug, "created_by", createdBy)
	httputil.Created(w, c)
}

// Update: PUT /api/admin/collections/{id}. Partial — only non-nil fields apply.
func (h *CollectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	var req domain.UpdateCollectionRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	c, err := h.svc.Update(r.Context(), id, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, c)
}

// Delete: DELETE /api/admin/collections/{id}. Soft-delete.
func (h *CollectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

// AddItem: POST /api/admin/collections/{id}/items.
func (h *CollectionHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	var req domain.AddCollectionItemRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if req.AnimeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}
	item, err := h.svc.AddItem(r.Context(), id, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.Created(w, item)
}

// RemoveItem: DELETE /api/admin/collections/{id}/items/{animeId}.
func (h *CollectionHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	animeID := chi.URLParam(r, "animeId")
	if id == "" || animeID == "" {
		httputil.BadRequest(w, "id and animeId are required")
		return
	}
	if err := h.svc.RemoveItem(r.Context(), id, animeID); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

// callerUserID extracts the caller's UUID from JWT claims. Returns ""
// when claims are missing — the admin middleware on the route would
// already have rejected unauthenticated traffic, so an empty value here
// indicates a wiring bug rather than a real anon caller.
func callerUserID(r *http.Request) string {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		return ""
	}
	return claims.UserID
}
