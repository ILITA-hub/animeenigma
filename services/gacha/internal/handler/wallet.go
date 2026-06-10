package handler

import (
	"net/http"
	"time"

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

// dailyClaimResponse is the JSON shape returned by ClaimDaily.
type dailyClaimResponse struct {
	Claimed bool            `json:"claimed"`
	Amount  int64           `json:"amount,omitempty"`
	Streak  int             `json:"streak,omitempty"`
	Wallet  interface{}     `json:"wallet"`
}

// ClaimDaily handles POST /api/gacha/daily. Issues the player's daily
// «Энигмы» reward with a consecutive-day streak bonus. Idempotent per UTC
// calendar day — calling it a second time on the same day returns
// claimed=false and the current wallet without any writes.
func (h *WalletHandler) ClaimDaily(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	result, err := h.svc.Daily(r.Context(), claims.UserID, time.Now())
	if err != nil {
		h.log.Errorw("daily claim failed", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, dailyClaimResponse{
		Claimed: result.Claimed,
		Amount:  result.Amount,
		Streak:  result.Streak,
		Wallet:  result.Wallet,
	})
}
