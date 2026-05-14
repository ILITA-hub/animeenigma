package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// SessionsHandler exposes /api/auth/sessions endpoints.
type SessionsHandler struct {
	authService *service.AuthService
	log         *logger.Logger
}

func NewSessionsHandler(authService *service.AuthService, log *logger.Logger) *SessionsHandler {
	return &SessionsHandler{authService: authService, log: log}
}

func (h *SessionsHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	items, err := h.authService.ListSessions(r.Context(), claims.UserID, claims.SessionID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, items)
}

func (h *SessionsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.authService.RevokeSession(r.Context(), claims.UserID, id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

func (h *SessionsHandler) RevokeOthers(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	n, err := h.authService.RevokeOtherSessions(r.Context(), claims.UserID, claims.SessionID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"revoked": n})
}
