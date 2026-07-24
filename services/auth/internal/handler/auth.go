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
	// refreshTokenMaxAge is effectively "never" for the browser cookie. The DB
	// session is non-expiring (revoke-only), and we re-set this cookie on every
	// refresh so it keeps sliding ~10 years out and never ages out client-side.
	refreshTokenMaxAge    = 10 * 365 * 24 * time.Hour
	accessTokenCookieName = "access_token"

	// telegramNonceCookieName holds the one-time browser-binding nonce for the
	// Telegram deep-link / QR login. DeepLink sets it (HttpOnly); CheckDeepLink
	// requires it so a leaked deep-link token cannot be redeemed by a browser
	// that never held the cookie. Scoped to /api/auth so it rides only the
	// deeplink + check requests. Must match the name re-forwarded by the gateway
	// proxy for the auth service (services/gateway/internal/service/proxy.go).
	telegramNonceCookieName = "tg_deeplink_nonce"
	// telegramNonceMaxAge matches the deep-link token TTL (cache.TTLTelegramAuth
	// = 5min); a stale nonce is worthless once the token expires.
	telegramNonceMaxAge = 5 * time.Minute
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

// sameSiteMode resolves the configured SameSite policy string to its
// http.SameSite value (defaulting to Lax). Shared by every Set-Cookie helper
// below so the policy is defined in exactly one place.
func (h *AuthHandler) sameSiteMode() http.SameSite {
	switch h.cookieConfig.SameSite {
	case "Strict":
		return http.SameSiteStrictMode
	case "None":
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (h *AuthHandler) setRefreshTokenCookie(w http.ResponseWriter, token string) {
	sameSite := h.sameSiteMode()

	http.SetCookie(w, &http.Cookie{
		Name:  refreshTokenCookieName,
		Value: token,
		// Path "/" (not "/api/auth") so the gateway's AdminSessionRefreshMiddleware
		// can read it on /admin/* and transparently renew the short-lived access
		// token for browser-driven admin tools (Grafana), which run outside the
		// Vue SPA and so can't trigger the SPA's /api/auth/refresh interceptor.
		// The token stays HttpOnly+Secure, and the gateway strips the Cookie
		// header before forwarding to any downstream service (proxy.go
		// copyForwardHeaders + ws_proxy.go Director), so widening the path does
		// not expose the refresh token to backends.
		Path:     "/",
		Domain:   h.cookieConfig.Domain,
		MaxAge:   int(refreshTokenMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieConfig.Secure,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) clearRefreshTokenCookie(w http.ResponseWriter) {
	// Clear at BOTH paths: the current cookie is set at "/", but sessions
	// created before the path-widening deploy hold a cookie at "/api/auth".
	// A cookie is keyed by (name, path, domain), so a single "/" deletion
	// would not remove the legacy "/api/auth" cookie, leaving an orphaned
	// 30-day credential in the browser. Emit a deletion for each path.
	for _, path := range []string{"/", "/api/auth"} {
		http.SetCookie(w, &http.Cookie{
			Name:     refreshTokenCookieName,
			Value:    "",
			Path:     path,
			Domain:   h.cookieConfig.Domain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.cookieConfig.Secure,
		})
	}
}

func (h *AuthHandler) setAccessTokenCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	sameSite := h.sameSiteMode()

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

func (h *AuthHandler) setTelegramNonceCookie(w http.ResponseWriter, nonce string) {
	sameSite := h.sameSiteMode()

	http.SetCookie(w, &http.Cookie{
		Name:  telegramNonceCookieName,
		Value: nonce,
		// Scoped to /api/auth: the browser only needs to present it on the
		// /telegram/check poll (and it is minted on /telegram/deeplink). Unlike
		// the refresh cookie there is no admin-path reason to widen this to "/".
		Path:     "/api/auth",
		Domain:   h.cookieConfig.Domain,
		MaxAge:   int(telegramNonceMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieConfig.Secure,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) clearTelegramNonceCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     telegramNonceCookieName,
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

	// Validate request. httputil.Bind does NOT run the struct validate tags,
	// so enforce the policy explicitly via the domain validators (single
	// source of truth): alphanum+_- username and an 8–72 byte password (72 is
	// bcrypt's input limit; longer inputs used to surface as a generic 500).
	if err := domain.ValidateUsername(req.Username); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := domain.ValidatePassword(req.Password); err != nil {
		httputil.Error(w, err)
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
		// Distinguish brute-force lockouts (429) from ordinary failures so the
		// throttle is observable in Grafana (audit medium #6).
		status := "error"
		if appErr, ok := errors.IsAppError(err); ok && appErr.Code == errors.CodeRateLimited {
			status = "lockout"
		}
		metrics.AuthEventsTotal.WithLabelValues("login", status).Inc()
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
	resp, err := h.authService.RefreshToken(r.Context(), req, sc)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("refresh_token", "error").Inc()
		// Clear invalid cookie
		h.clearRefreshTokenCookie(w)
		httputil.Error(w, err)
		return
	}

	// Non-rotating: the refresh token is unchanged. Re-set the same cookie value
	// so its 10-year max-age slides forward and the browser never drops it.
	h.setRefreshTokenCookie(w, cookie.Value)

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
// It also sets a one-time HttpOnly browser-binding nonce cookie: the same
// browser must present it when polling CheckDeepLink, so a leaked deep-link
// token cannot be redeemed from a different browser (vector A). The requesting
// client (IP + User-Agent) is captured into the pending session so the bot's
// Confirm-login prompt can show where the login was requested from (vector B).
func (h *AuthHandler) DeepLink(w http.ResponseWriter, r *http.Request) {
	sc := sessionContextFromReq(r)
	resp, nonce, err := h.authService.CreateDeepLinkToken(r.Context(), h.telegramConfig.BotName, sc)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.setTelegramNonceCookie(w, nonce)
	httputil.OK(w, resp)
}

// CheckDeepLink polls the status of a deep link auth token.
func (h *AuthHandler) CheckDeepLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		httputil.Error(w, errors.InvalidInput("token is required"))
		return
	}

	// Browser-binding nonce (vector A): present only if this is the browser
	// that minted the token. Missing cookie → empty string, which the service
	// treats as a non-match for any bound session.
	var bindingNonce string
	if c, err := r.Cookie(telegramNonceCookieName); err == nil {
		bindingNonce = c.Value
	}

	sc := sessionContextFromReq(r)
	checkResp, authResp, err := h.authService.CheckDeepLinkToken(r.Context(), token, bindingNonce, sc)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// If confirmed, set auth cookies
	if authResp != nil {
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "success").Inc()
		h.setRefreshTokenCookie(w, authResp.RefreshToken)
		h.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
		// One-time nonce: the token is consumed, so retire the binding cookie.
		h.clearTelegramNonceCookie(w)
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

