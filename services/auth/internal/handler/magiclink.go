package handler

import (
	"net/http"
	"net/url"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// cookieSetter is satisfied by *AuthHandler (reuses setRefreshTokenCookie /
// setAccessTokenCookie). Defined to avoid duplicating cookie logic.
type cookieSetter interface {
	setRefreshTokenCookie(w http.ResponseWriter, token string)
	setAccessTokenCookie(w http.ResponseWriter, token string, expiresAt time.Time)
}

// MagicLinkHandler serves the cross-domain SSO bridge endpoints. targetBase is
// the canonical .org base (e.g. https://animeenigma.org) that Generate redirects to.
type MagicLinkHandler struct {
	authService *service.AuthService
	cookie      cookieSetter
	targetBase  string
	log         *logger.Logger
}

// NewMagicLinkHandler constructs a MagicLinkHandler. Pass the *AuthHandler as
// the cookieSetter — it lives in the same package so unexported methods satisfy
// the interface.
func NewMagicLinkHandler(authService *service.AuthService, cookie cookieSetter, targetBase string, log *logger.Logger) *MagicLinkHandler {
	return &MagicLinkHandler{authService: authService, cookie: cookie, targetBase: targetBase, log: log}
}

// Generate (served on .ru): reads the refresh_token cookie, mints a one-time
// token, and 302s to <targetBase>/magic-link-login?oldurl=&token=. Anonymous
// callers are redirected straight to <targetBase><oldurl> (no token).
func (h *MagicLinkHandler) Generate(w http.ResponseWriter, r *http.Request) {
	oldurl := service.SanitizeOldURL(r.URL.Query().Get("oldurl"))
	var token string
	if c, err := r.Cookie(refreshTokenCookieName); err == nil {
		token, _ = h.authService.MintMagicToken(r.Context(), c.Value)
	}
	dest := h.targetBase + oldurl
	if token != "" {
		dest = h.targetBase + "/magic-link-login?oldurl=" + url.QueryEscape(oldurl) + "&token=" + url.QueryEscape(token)
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// Login (served on .org): consumes the token, sets .org session cookies, 302s to
// oldurl. Any failure lands the user anonymously on oldurl (never an error page).
func (h *MagicLinkHandler) Login(w http.ResponseWriter, r *http.Request) {
	oldurl := service.SanitizeOldURL(r.URL.Query().Get("oldurl"))
	token := r.URL.Query().Get("token")
	if token != "" {
		if resp, err := h.authService.ConsumeMagicToken(r.Context(), token, sessionContextFromReq(r)); err == nil && resp != nil {
			h.cookie.setRefreshTokenCookie(w, resp.RefreshToken)
			h.cookie.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)
			metrics.AuthEventsTotal.WithLabelValues("magic_link", "success").Inc()
		} else {
			metrics.AuthEventsTotal.WithLabelValues("magic_link", "error").Inc()
		}
	}
	http.Redirect(w, r, oldurl, http.StatusFound)
}
