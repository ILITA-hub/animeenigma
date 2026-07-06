package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// PublicFlagsHandler serves the per-user visibility feed for the SPA. JWT is
// optional — anonymous callers resolve to everyone-flags only. Fail-open.
type PublicFlagsHandler struct {
	svc *service.PolicyService
	log *logger.Logger
}

func NewPublicFlagsHandler(svc *service.PolicyService, log *logger.Logger) *PublicFlagsHandler {
	return &PublicFlagsHandler{svc: svc, log: log}
}

func (h *PublicFlagsHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	role := string(authz.RoleFromContext(r.Context()))
	mine, err := h.svc.ResolveForUser(r.Context(), userID, role)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, mine)
}
