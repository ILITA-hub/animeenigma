package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
	log         *logger.Logger
}

func NewAuthHandler(authService *service.AuthService, log *logger.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		log:         log,
	}
}

// Register handles user registration
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req domain.RegisterRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	// Validate request
	if len(req.Username) < 3 || len(req.Username) > 32 {
		httputil.Error(w, errors.InvalidInput("username must be between 3 and 32 characters"))
		return
	}
	if len(req.Password) < 8 {
		httputil.Error(w, errors.InvalidInput("password must be at least 8 characters"))
		return
	}

	resp, err := h.authService.Register(r.Context(), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.Created(w, resp)
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	resp, err := h.authService.Login(r.Context(), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, resp)
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req domain.RefreshRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	resp, err := h.authService.RefreshToken(r.Context(), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, resp)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req domain.RefreshRequest
	if err := httputil.Bind(r, &req); err != nil {
		// Even if no body, logout is successful
		httputil.NoContent(w)
		return
	}

	_ = h.authService.Logout(r.Context(), req.RefreshToken)
	httputil.NoContent(w)
}
