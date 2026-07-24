package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// PasskeyHandler serves the passkey (WebAuthn) registration and usernameless
// login endpoints. cookies is the *AuthHandler (same package) whose unexported
// cookie helpers this handler reuses to mint a session on successful login —
// same pattern as MagicLinkHandler.
type PasskeyHandler struct {
	passkeys *service.PasskeyService
	auth     *service.AuthService
	cookies  *AuthHandler
	log      *logger.Logger
}

func NewPasskeyHandler(passkeys *service.PasskeyService, auth *service.AuthService, cookies *AuthHandler, log *logger.Logger) *PasskeyHandler {
	return &PasskeyHandler{passkeys: passkeys, auth: auth, cookies: cookies, log: log}
}

// beginResponse wraps ceremony options with the ceremony id the client must
// echo back to finish.
type beginResponse struct {
	CeremonyID string `json:"ceremony_id"`
	Options    any    `json:"options"`
}

func (h *PasskeyHandler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	user, err := h.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	opts, id, err := h.passkeys.BeginRegistration(r.Context(), user)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, beginResponse{CeremonyID: id, Options: opts})
}

// RegisterFinish expects ?ceremony=<id>&name=<label> query params and the raw
// WebAuthn attestation JSON as the request body (the library parses r.Body).
func (h *PasskeyHandler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	user, err := h.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	row, err := h.passkeys.FinishRegistration(r.Context(), user, r.URL.Query().Get("ceremony"), r.URL.Query().Get("name"), r)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, row)
}

func (h *PasskeyHandler) LoginBegin(w http.ResponseWriter, r *http.Request) {
	opts, id, err := h.passkeys.BeginLogin(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, beginResponse{CeremonyID: id, Options: opts})
}

// LoginFinish completes a usernameless assertion; on success it mints a full
// session exactly like password/Telegram login (cookies + public response).
func (h *PasskeyHandler) LoginFinish(w http.ResponseWriter, r *http.Request) {
	user, err := h.passkeys.FinishLogin(r.Context(), r.URL.Query().Get("ceremony"), r)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	authResp, err := h.auth.SessionForUser(r.Context(), user, sessionContextFromReq(r))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	h.cookies.setRefreshTokenCookie(w, authResp.RefreshToken)
	h.cookies.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
	httputil.OK(w, authResp.ToPublicResponse())
}

func (h *PasskeyHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	rows, err := h.passkeys.List(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, rows)
}

func (h *PasskeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	if err := h.passkeys.Delete(r.Context(), chi.URLParam(r, "id"), claims.UserID); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "deleted"})
}
