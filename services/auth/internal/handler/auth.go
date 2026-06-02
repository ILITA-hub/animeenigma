package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

const (
	refreshTokenCookieName = "refresh_token"
	// refreshTokenMaxAge MUST match service.SessionTTL — the cookie expiring
	// before the DB session leaves an orphaned row that the user can't reclaim.
	refreshTokenMaxAge    = 30 * 24 * time.Hour
	accessTokenCookieName = "access_token"
)

// clientIP returns the best-effort client IP without the port.
// chi's middleware.RealIP rewrites RemoteAddr based on X-Real-IP /
// X-Forwarded-For when present, but the result still includes the
// trailing :port from net.JoinHostPort.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}

func sessionContextFromReq(r *http.Request) service.SessionContext {
	return service.SessionContext{
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	}
}

type AuthHandler struct {
	authService    *service.AuthService
	cookieConfig   config.CookieConfig
	telegramConfig config.TelegramConfig
	log            *logger.Logger
}

func NewAuthHandler(authService *service.AuthService, cookieConfig config.CookieConfig, telegramConfig config.TelegramConfig, log *logger.Logger) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		cookieConfig:   cookieConfig,
		telegramConfig: telegramConfig,
		log:            log,
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

func (h *AuthHandler) setAccessTokenCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	sameSite := http.SameSiteLaxMode
	switch h.cookieConfig.SameSite {
	case "Strict":
		sameSite = http.SameSiteStrictMode
	case "None":
		sameSite = http.SameSiteNoneMode
	}

	// Calculate MaxAge from expiration time
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(w, &http.Cookie{
		Name:     accessTokenCookieName,
		Value:    token,
		Path:     "/",
		Domain:   h.cookieConfig.Domain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieConfig.Secure,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) clearAccessTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessTokenCookieName,
		Value:    "",
		Path:     "/",
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

	sc := sessionContextFromReq(r)
	resp, err := h.authService.Register(r.Context(), &req, sc)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("register", "error").Inc()
		httputil.Error(w, err)
		return
	}

	metrics.AuthEventsTotal.WithLabelValues("register", "success").Inc()

	// Set refresh token as httpOnly cookie
	h.setRefreshTokenCookie(w, resp.RefreshToken)

	// Set access token as httpOnly cookie for direct browser navigation
	h.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)

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

	sc := sessionContextFromReq(r)
	resp, err := h.authService.Login(r.Context(), &req, sc)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("login", "error").Inc()
		httputil.Error(w, err)
		return
	}

	metrics.AuthEventsTotal.WithLabelValues("login", "success").Inc()

	// Set refresh token as httpOnly cookie
	h.setRefreshTokenCookie(w, resp.RefreshToken)

	// Set access token as httpOnly cookie for direct browser navigation
	h.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)

	// Return response without refresh token in body
	httputil.OK(w, resp.ToPublicResponse())
}

// GuestSession mints an ephemeral guest identity for joining a Watch Together
// room via invite link. Public (no auth). Returns {access_token, expires_at,
// user:{id,username,role:"guest"}} with NO refresh cookie — guests re-mint a
// fresh token client-side when this one nears expiry. The guest JWT carries
// RoleGuest, which the gateway rejects everywhere except the Watch Together
// routes (see gateway BlockGuestRoleMiddleware).
func (h *AuthHandler) GuestSession(w http.ResponseWriter, r *http.Request) {
	resp, err := h.authService.GuestSession(r.Context())
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("guest", "error").Inc()
		httputil.Error(w, err)
		return
	}
	metrics.AuthEventsTotal.WithLabelValues("guest", "success").Inc()
	httputil.OK(w, resp)
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	// Read refresh token from cookie
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		httputil.Error(w, errors.Unauthorized("refresh token not found"))
		return
	}

	sc := sessionContextFromReq(r)
	req := &domain.RefreshRequest{RefreshToken: cookie.Value}
	resp, rotated, err := h.authService.RefreshToken(r.Context(), req, sc)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("refresh_token", "error").Inc()
		// Clear invalid cookie
		h.clearRefreshTokenCookie(w)
		httputil.Error(w, err)
		return
	}

	// Only set a new refresh-token cookie when the token was actually rotated.
	// On the grace path (rotated=false), the existing cookie remains valid.
	if rotated {
		h.setRefreshTokenCookie(w, resp.RefreshToken)
	}

	// Set access token as httpOnly cookie for direct browser navigation
	h.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)

	// Return response without refresh token in body
	httputil.OK(w, resp.ToPublicResponse())
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Try to get refresh token from cookie
	if cookie, err := r.Cookie(refreshTokenCookieName); err == nil {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}

	// Clear the cookies
	h.clearRefreshTokenCookie(w)
	h.clearAccessTokenCookie(w)
	httputil.NoContent(w)
}

// DeepLink creates a new deep link auth token and returns the Telegram bot URL.
func (h *AuthHandler) DeepLink(w http.ResponseWriter, r *http.Request) {
	resp, err := h.authService.CreateDeepLinkToken(r.Context(), h.telegramConfig.BotName)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, resp)
}

// CheckDeepLink polls the status of a deep link auth token.
func (h *AuthHandler) CheckDeepLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		httputil.Error(w, errors.InvalidInput("token is required"))
		return
	}

	sc := sessionContextFromReq(r)
	checkResp, authResp, err := h.authService.CheckDeepLinkToken(r.Context(), token, sc)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// If confirmed, set auth cookies
	if authResp != nil {
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "success").Inc()
		h.setRefreshTokenCookie(w, authResp.RefreshToken)
		h.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
	}

	httputil.OK(w, checkResp)
}

// GenerateApiKey creates a new API key for the authenticated user
func (h *AuthHandler) GenerateApiKey(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	apiKey, err := h.authService.GenerateApiKey(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, &domain.ApiKeyResponse{ApiKey: apiKey})
}

// RevokeApiKey removes the API key for the authenticated user
func (h *AuthHandler) RevokeApiKey(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	if err := h.authService.RevokeApiKey(r.Context(), claims.UserID); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.NoContent(w)
}

// HasApiKey checks whether the authenticated user has an API key
func (h *AuthHandler) HasApiKey(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}

	has, err := h.authService.HasApiKey(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]bool{"has_api_key": has})
}

// ResolveApiKey is an internal endpoint that validates an API key and returns claims.
// This is called by the gateway to resolve ak_ tokens.
func (h *AuthHandler) ResolveApiKey(w http.ResponseWriter, r *http.Request) {
	var req domain.ResolveApiKeyRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.ApiKey == "" {
		httputil.Error(w, errors.InvalidInput("api_key is required"))
		return
	}

	claims, err := h.authService.ResolveApiKey(r.Context(), req.ApiKey)
	if err != nil {
		httputil.Unauthorized(w)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"user_id":  claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
	})
}

