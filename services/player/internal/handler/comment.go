package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// CommentHandler exposes the four comment endpoints:
//
//	GET    /api/anime/{animeId}/comments
//	POST   /api/anime/{animeId}/comments
//	PATCH  /api/anime/{animeId}/comments/{commentId}
//	DELETE /api/anime/{animeId}/comments/{commentId}
//
// Plan 03: real implementations. Plan 04 wires them into the chi router
// in transport/router.go so they become live HTTP endpoints.
type CommentHandler struct {
	commentService *service.CommentService
	log            *logger.Logger
}

// NewCommentHandler wires a CommentHandler against the service layer.
func NewCommentHandler(s *service.CommentService, log *logger.Logger) *CommentHandler {
	return &CommentHandler{commentService: s, log: log}
}

// CreateComment handles POST /api/anime/{animeId}/comments.
//
// Auth required (the route group applies AuthMiddleware in plan 04).
// Returns 201 on success, 400 on empty/long body, 401 on missing claims,
// 429 on rate-limit, 500 on persistence error.
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "animeId is required")
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	var req domain.CreateCommentRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	c, err := h.commentService.CreateComment(r.Context(), claims.UserID, claims.Username, animeID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.Created(w, c)
}

// UpdateComment handles PATCH /api/anime/{animeId}/comments/{commentId}.
//
// Auth required. Owner-or-admin only — non-owner non-admin returns 403.
// Admin override happens at the service layer; the frontend hides the
// pencil for admins on non-owned comments (CONTEXT.md decision).
func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "animeId") // accepted for symmetry; service keys on commentId
	commentID := chi.URLParam(r, "commentId")
	if commentID == "" {
		httputil.BadRequest(w, "commentId is required")
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	var req domain.UpdateCommentRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	c, err := h.commentService.UpdateComment(r.Context(), claims.UserID, commentID, authz.IsAdmin(r.Context()), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, c)
}

// DeleteComment handles DELETE /api/anime/{animeId}/comments/{commentId}.
//
// Auth required. Owner OR admin — non-owner non-admin returns 403.
// Soft-deletes via gorm.DeletedAt; subsequent list queries exclude the
// row. Returns 204 on success.
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "animeId") // accepted for symmetry; service keys on commentId
	commentID := chi.URLParam(r, "commentId")
	if commentID == "" {
		httputil.BadRequest(w, "commentId is required")
		return
	}

	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	if err := h.commentService.DeleteComment(r.Context(), claims.UserID, commentID, authz.IsAdmin(r.Context())); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

// ListComments handles GET /api/anime/{animeId}/comments?cursor=&limit=.
//
// Public — no auth. Limit defaults to 50, capped at 100.
func (h *CommentHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "animeId is required")
		return
	}

	q := r.URL.Query()
	cursor := q.Get("cursor")
	limit := 0 // service applies the default + cap
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	resp, err := h.commentService.ListComments(r.Context(), animeID, cursor, limit)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, resp)
}
