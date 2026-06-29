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

// ShowcaseHandler serves the profile showcase (Steam-style wall):
//
//	GET /api/users/{userId}/showcase   (public read)
//	PUT /api/users/me/showcase         (owner write, JWT)
type ShowcaseHandler struct {
	svc *service.ShowcaseService
	log *logger.Logger
}

// NewShowcaseHandler wires a ShowcaseHandler against the service layer.
func NewShowcaseHandler(s *service.ShowcaseService, log *logger.Logger) *ShowcaseHandler {
	return &ShowcaseHandler{svc: s, log: log}
}

type showcaseResponse struct {
	Blocks  []domain.Block `json:"blocks"`
	Enabled bool           `json:"enabled"`
}

type saveShowcaseRequest struct {
	Blocks  []domain.Block `json:"blocks"`
	Enabled bool           `json:"enabled"`
}

// GetShowcase handles GET /api/users/{userId}/showcase.
//
// Public — no auth required. Returns the showcase blocks for the given user.
// Returns an empty blocks array (not null) when the user has no saved showcase.
func (h *ShowcaseHandler) GetShowcase(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "userId is required")
		return
	}
	blocks, enabled, err := h.svc.GetShowcase(r.Context(), userID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if blocks == nil {
		blocks = []domain.Block{}
	}
	httputil.OK(w, showcaseResponse{Blocks: blocks, Enabled: enabled})
}

// SaveShowcase handles PUT /api/users/me/showcase.
//
// Auth required (the route group applies AuthMiddleware). Owner is resolved
// from JWT claims — the "me" path segment is a convention, not a DB lookup.
// Returns the saved blocks on success.
func (h *ShowcaseHandler) SaveShowcase(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	var req saveShowcaseRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	enabled, err := h.svc.SaveShowcase(r.Context(), claims.UserID, req.Blocks, req.Enabled)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, showcaseResponse{Blocks: req.Blocks, Enabled: enabled})
}
