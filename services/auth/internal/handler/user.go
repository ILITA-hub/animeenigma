package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
	"github.com/go-chi/chi/v5"
)

type UserHandler struct {
	userService *service.UserService
	log         *logger.Logger
}

func NewUserHandler(userService *service.UserService, log *logger.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		log:         log,
	}
}

// GetCurrentUser returns the current authenticated user
func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	user, err := h.userService.GetByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, user)
}

// UpdateCurrentUser updates the current user's profile
func (h *UserHandler) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	var req domain.UpdateUserRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	user, err := h.userService.Update(r.Context(), claims.UserID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, user)
}

// GetUser returns a user's public profile
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "user ID is required")
		return
	}

	user, err := h.userService.GetPublicProfile(r.Context(), userID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, user)
}

// GetUserByPublicID returns a user's public profile by public_id
func (h *UserHandler) GetUserByPublicID(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "publicId")
	if publicID == "" {
		httputil.BadRequest(w, "public ID is required")
		return
	}

	user, err := h.userService.GetPublicProfileByPublicID(r.Context(), publicID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, user)
}

// UpdatePublicID updates the current user's public_id
func (h *UserHandler) UpdatePublicID(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	var req domain.UpdatePublicIDRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if err := h.userService.UpdatePublicID(r.Context(), claims.UserID, req.PublicID); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]string{"public_id": req.PublicID})
}

// UpdatePrivacy updates the current user's public_statuses
func (h *UserHandler) UpdatePrivacy(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	var req domain.UpdatePrivacyRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if err := h.userService.UpdatePublicStatuses(r.Context(), claims.UserID, req.PublicStatuses); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string][]string{"public_statuses": req.PublicStatuses})
}
