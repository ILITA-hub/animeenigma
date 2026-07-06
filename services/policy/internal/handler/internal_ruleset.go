package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// InternalRulesetHandler serves the compact ruleset the gateway caches.
// Reachable only from the Docker network (gateway does NOT proxy /internal/*).
type InternalRulesetHandler struct {
	svc *service.PolicyService
	log *logger.Logger
}

func NewInternalRulesetHandler(svc *service.PolicyService, log *logger.Logger) *InternalRulesetHandler {
	return &InternalRulesetHandler{svc: svc, log: log}
}

func (h *InternalRulesetHandler) GetRuleset(w http.ResponseWriter, r *http.Request) {
	rs, err := h.svc.Ruleset(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, rs)
}
