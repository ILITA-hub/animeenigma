package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
)

// WalletHandler serves the authenticated wallet endpoint.
type WalletHandler struct {
	svc *service.WalletService
	log *logger.Logger
}

func NewWalletHandler(svc *service.WalletService, log *logger.Logger) *WalletHandler {
	return &WalletHandler{svc: svc, log: log}
}

// GetWallet handles GET /api/gacha/wallet. Returns the caller's wallet,
// creating it (and granting the starter bonus) on first access. User identity
// comes from the JWT claims the AuthMiddleware put on the context.
func (h *WalletHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	wallet, err := h.svc.GetOrCreate(r.Context(), claims.UserID)
	if err != nil {
		h.log.Errorw("get wallet failed", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, wallet)
}
