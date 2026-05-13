package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
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
// Wave-0 stub: every handler responds 503 via errors.CodeUnavailable so the
// route wiring in plan 04 can attach these without changing signatures. The
// URL params are read so chi compile-time route binding stays honest.
type CommentHandler struct {
	commentService *service.CommentService
	log            *logger.Logger
}

// NewCommentHandler wires a CommentHandler against the service layer.
func NewCommentHandler(s *service.CommentService, log *logger.Logger) *CommentHandler {
	return &CommentHandler{commentService: s, log: log}
}

// CreateComment handles POST /api/anime/{animeId}/comments.
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "animeId")
	httputil.Error(w, errors.New(errors.CodeUnavailable, "comment handler CreateComment: not implemented"))
}

// UpdateComment handles PATCH /api/anime/{animeId}/comments/{commentId}.
func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "animeId")
	_ = chi.URLParam(r, "commentId")
	httputil.Error(w, errors.New(errors.CodeUnavailable, "comment handler UpdateComment: not implemented"))
}

// DeleteComment handles DELETE /api/anime/{animeId}/comments/{commentId}.
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "animeId")
	_ = chi.URLParam(r, "commentId")
	httputil.Error(w, errors.New(errors.CodeUnavailable, "comment handler DeleteComment: not implemented"))
}

// ListComments handles GET /api/anime/{animeId}/comments.
func (h *CommentHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "animeId")
	httputil.Error(w, errors.New(errors.CodeUnavailable, "comment handler ListComments: not implemented"))
}
