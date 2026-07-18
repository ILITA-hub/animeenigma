package handler

import (
	"crypto/rand"
	"encoding/base64"
	stderrors "errors"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

const (
	// oidcBindCookie ties the callback to the browser that ran Start — a
	// callback URL replayed in another browser must not mint a session
	// there (login-CSRF / session fixation).
	oidcBindCookie = "ae_oidc_bind"
	oidcBindPath   = "/api/auth/telegram/oidc"
)

// newBindNonce returns a fresh random value for the bind cookie, same recipe
// as generateRefreshToken (service/auth.go): 32 random bytes, base64url.
func newBindNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// TelegramOIDCHandler serves the browser-facing Telegram OIDC login
// endpoints. Both answer with 302s (never JSON): the browser is mid full-page
// navigation, and the gateway forwards these routes with its no-redirect
// proxy so every Location header reaches the browser verbatim.
type TelegramOIDCHandler struct {
	oidc        *service.TelegramOIDC
	authService *service.AuthService
	cookie      cookieSetter
	log         *logger.Logger
}

// NewTelegramOIDCHandler constructs the handler. Pass the *AuthHandler as
// cookieSetter (same package — unexported methods satisfy the interface).
func NewTelegramOIDCHandler(o *service.TelegramOIDC, a *service.AuthService, cookie cookieSetter, log *logger.Logger) *TelegramOIDCHandler {
	return &TelegramOIDCHandler{oidc: o, authService: a, cookie: cookie, log: log}
}

// Start begins a login attempt: 302 to Telegram's authorization endpoint.
// ?return= is the SPA path to land on after login; it travels server-side in
// the OIDC state, sanitized exactly like the magic-link oldurl.
func (h *TelegramOIDCHandler) Start(w http.ResponseWriter, r *http.Request) {
	returnPath := service.SanitizeOldURL(r.URL.Query().Get("return"))
	nonce, err := newBindNonce()
	if err != nil {
		h.log.Errorw("telegram oidc begin failed", "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "begin_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcBindCookie,
		Value:    nonce,
		Path:     oidcBindPath,
		MaxAge:   300,
		HttpOnly: true,
		Secure:   true,
		// Lax: the callback arrives as a top-level cross-site GET navigation
		// from oauth.telegram.org — Lax still sends the cookie there, Strict
		// would not.
		SameSite: http.SameSiteLaxMode,
	})
	authURL, err := h.oidc.Begin(r.Context(), returnPath, nonce)
	if err != nil {
		h.log.Errorw("telegram oidc begin failed", "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "begin_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback finishes the login: exchanges the code, mints the session via the
// existing LoginWithTelegram, sets the standard auth cookies, and lands the
// user on their return path. Every failure lands on /auth?error=… (retryable).
func (h *TelegramOIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		// User cancelled on Telegram's consent screen (or provider error).
		h.log.Infow("telegram oidc denied", "provider_error", e)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "denied").Inc()
		http.Redirect(w, r, "/auth?error=denied", http.StatusFound)
		return
	}
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}

	// Bind check: the initiating browser set this HttpOnly cookie in Start.
	// No cookie (different browser, expired, or blocked) is indistinguishable
	// from an expired attempt from the user's point of view — same retryable
	// error page.
	bindCookie, err := r.Cookie(oidcBindCookie)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "state_expired").Inc()
		http.Redirect(w, r, "/auth?error=expired", http.StatusFound)
		return
	}

	tgUser, returnPath, err := h.oidc.Complete(r.Context(), state, code, bindCookie.Value)
	if err != nil {
		if stderrors.Is(err, service.ErrOIDCStateExpired) {
			metrics.AuthEventsTotal.WithLabelValues("telegram_login", "state_expired").Inc()
			http.Redirect(w, r, "/auth?error=expired", http.StatusFound)
			return
		}
		h.log.Errorw("telegram oidc complete failed", "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "exchange_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}

	resp, err := h.authService.LoginWithTelegram(r.Context(), tgUser, sessionContextFromReq(r))
	if err != nil {
		h.log.Errorw("telegram oidc login failed", "telegram_id", tgUser.ID, "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "login_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}

	metrics.AuthEventsTotal.WithLabelValues("telegram_login", "success").Inc()
	h.cookie.setRefreshTokenCookie(w, resp.RefreshToken)
	h.cookie.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)
	// One-shot SPA bootstrap marker (same contract as the magic-link bridge,
	// magiclink.go): the JS-visible ae_sso=1 tells main.ts to mint an access
	// token into localStorage via /auth/refresh — without it the SPA renders
	// logged-out despite valid httpOnly cookies, because this login happened
	// outside any XHR the store could observe.
	http.SetCookie(w, &http.Cookie{
		Name:     "ae_sso",
		Value:    "1",
		Path:     "/",
		MaxAge:   60,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	// Bind nonce is single-use like the state it was checked against — clear
	// it now that login succeeded.
	http.SetCookie(w, &http.Cookie{
		Name:     oidcBindCookie,
		Value:    "",
		Path:     oidcBindPath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, returnPath, http.StatusFound)
}
