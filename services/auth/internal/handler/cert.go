package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// CertHandler serves client-cert issuance/listing/revocation, the mTLS-vhost
// handshake-login endpoint, and its one-time-token consume step. cookies is
// the *AuthHandler (same package) whose unexported cookie helpers this
// handler reuses to mint a session — same pattern as MagicLinkHandler.
type CertHandler struct {
	certs   *service.CertService
	auth    *service.AuthService
	users   *service.UserService
	cookies *AuthHandler
	log     *logger.Logger
}

func NewCertHandler(certs *service.CertService, auth *service.AuthService, users *service.UserService, cookies *AuthHandler, log *logger.Logger) *CertHandler {
	return &CertHandler{certs: certs, auth: auth, users: users, cookies: cookies, log: log}
}

// CAPem serves the CA certificate (public). The host setup step curls this
// into /etc/nginx/certs/ae-user-ca.pem.
func (h *CertHandler) CAPem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(h.certs.CAPEM())
}

// CAInfo returns the platform CA subject + fingerprints (settings-modal
// display, so users can verify the OS trust prompt when importing the .p12).
func (h *CertHandler) CAInfo(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, h.certs.CAInfo())
}

type issueCertRequest struct {
	Name string `json:"name"`
}

func (h *CertHandler) Issue(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	var req issueCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		httputil.Error(w, errors.InvalidInput("invalid body"))
		return
	}
	user, err := h.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	resp, err := h.certs.IssueCertificate(r.Context(), user, req.Name)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, resp)
}

func (h *CertHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	rows, err := h.certs.ListCertificates(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, rows)
}

func (h *CertHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	if err := h.certs.RevokeCertificate(r.Context(), chi.URLParam(r, "id"), claims.UserID); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "revoked"})
}

// UpdateAutoLogin toggles User.CertAutoLogin (settings modal toggle).
func (h *CertHandler) UpdateAutoLogin(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	var req domain.UpdateCertAutoLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, errors.InvalidInput("invalid body"))
		return
	}
	if err := h.users.UpdateCertAutoLogin(r.Context(), claims.UserID, req.Enabled); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"cert_auto_login": req.Enabled})
}

// HandshakeLogin is the mTLS-vhost endpoint (root mux — NOT proxied by the
// gateway; only cert.animeenigma.org's nginx location can reach it from
// outside the Docker network). Returns a one-time token for
// POST /api/auth/cert/consume on the main origin.
//
// All failures except the auto_login_disabled case render through the
// uniform httputil.Error(errors.Unauthorized) path — a foreign/unknown/
// revoked/expired cert must be externally indistinguishable so a probing
// client can't learn which failure mode it hit.
func (h *CertHandler) HandshakeLogin(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.HandshakeCertLogin(
		r.Context(),
		r.Header.Get("X-AE-Cert-Verify"),
		r.Header.Get("X-AE-Cert-PEM"),
		h.certs,
	)
	if err != nil {
		if err == service.ErrCertAutoLoginDisabled {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"reason": "auto_login_disabled"})
			return
		}
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"token": token})
}

type consumeCertRequest struct {
	Token string `json:"token"`
}

// Consume exchanges a one-time cert-login token for a normal session (cookies
// on the main origin) — the terminal step of cert auto-login.
func (h *CertHandler) Consume(w http.ResponseWriter, r *http.Request) {
	var req consumeCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, errors.InvalidInput("invalid body"))
		return
	}
	authResp, err := h.auth.ConsumeCertLoginToken(r.Context(), req.Token, sessionContextFromReq(r))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	h.cookies.setRefreshTokenCookie(w, authResp.RefreshToken)
	h.cookies.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
	httputil.OK(w, authResp.ToPublicResponse())
}
