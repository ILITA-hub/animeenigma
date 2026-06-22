package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
)

// InternalHandler serves /internal/gacha/* — Docker-network-only (the gateway
// never proxies /internal/*). Producer services (player, themes, …) credit
// «Энигмы» here. No auth middleware, same model as notifications' internal
// handler (D-05).
type InternalHandler struct {
	svc *service.WalletService
	log *logger.Logger
}

func NewInternalHandler(svc *service.WalletService, log *logger.Logger) *InternalHandler {
	return &InternalHandler{svc: svc, log: log}
}

// CreditRequest is the body of POST /internal/gacha/credit.
type CreditRequest struct {
	UserID string `json:"user_id"`
	Amount int64  `json:"amount"`
	Reason string `json:"reason"`
	Ref    string `json:"ref"`
}

// Credit handles POST /internal/gacha/credit. Idempotent on (user_id, reason,
// ref). Returns { "applied": bool } — applied=false means a duplicate ref or
// disabled service, which producers treat as success (non-fatal).
func (h *InternalHandler) Credit(w http.ResponseWriter, r *http.Request) {
	var req CreditRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	// ref is the idempotency key: the (user_id, reason, ref) dedup index is
	// PARTIAL (WHERE ref <> ''), so an empty ref silently bypasses dedup and
	// double-credits on producer retries. Require it (audit medium #18).
	if req.UserID == "" || req.Reason == "" || req.Ref == "" {
		httputil.BadRequest(w, "user_id, reason and ref (idempotency key) are required")
		return
	}
	applied, err := h.svc.Credit(r.Context(), req.UserID, req.Amount, req.Reason, req.Ref)
	if err != nil {
		h.log.Errorw("internal credit failed",
			"user_id", req.UserID, "reason", req.Reason, "ref", req.Ref, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"applied": applied})
}

// Health handles GET /internal/health.
func (h *InternalHandler) Health(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}
