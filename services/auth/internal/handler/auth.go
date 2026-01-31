package handler

import (
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

const (
	refreshTokenCookieName = "refresh_token"
	refreshTokenMaxAge     = 7 * 24 * time.Hour
)

type AuthHandler struct {
	authService  *service.AuthService
	cookieConfig config.CookieConfig
	log          *logger.Logger
}

func NewAuthHandler(authService *service.AuthService, cookieConfig config.CookieConfig, log *logger.Logger) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		cookieConfig: cookieConfig,
		log:          log,
	}
}

func (h *AuthHandler) setRefreshTokenCookie(w http.ResponseWriter, token string) {
	sameSite := http.SameSiteLaxMode
	switch h.cookieConfig.SameSite {
	case "Strict":
		sameSite = http.SameSiteStrictMode
	case "None":
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    token,
		Path:     "/api/auth",
		Domain:   h.cookieConfig.Domain,
		MaxAge:   int(refreshTokenMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieConfig.Secure,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) clearRefreshTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     "/api/auth",
		Domain:   h.cookieConfig.Domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieConfig.Secure,
	})
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

	// Set refresh token as httpOnly cookie
	h.setRefreshTokenCookie(w, resp.RefreshToken)

	// Return response without refresh token in body
	httputil.Created(w, resp.ToPublicResponse())
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

	// Set refresh token as httpOnly cookie
	h.setRefreshTokenCookie(w, resp.RefreshToken)

	// Return response without refresh token in body
	httputil.OK(w, resp.ToPublicResponse())
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	// Read refresh token from cookie
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		httputil.Error(w, errors.Unauthorized("refresh token not found"))
		return
	}

	req := &domain.RefreshRequest{RefreshToken: cookie.Value}
	resp, err := h.authService.RefreshToken(r.Context(), req)
	if err != nil {
		// Clear invalid cookie
		h.clearRefreshTokenCookie(w)
		httputil.Error(w, err)
		return
	}

	// Set new refresh token cookie
	h.setRefreshTokenCookie(w, resp.RefreshToken)

	// Return response without refresh token in body
	httputil.OK(w, resp.ToPublicResponse())
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Try to get refresh token from cookie
	if cookie, err := r.Cookie(refreshTokenCookieName); err == nil {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}

	// Clear the cookie
	h.clearRefreshTokenCookie(w)
	httputil.NoContent(w)
}
