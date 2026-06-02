package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// Eraser is the erasure port (implemented by repo functions wrapped at
// wire-up time).
type Eraser interface {
	EraseByUserID(ctx context.Context, userID string) error
	EraseByAnonymousID(ctx context.Context, anonymousID string) error
}

type AdminHandler struct{ eraser Eraser }

func NewAdminHandler(e Eraser) *AdminHandler { return &AdminHandler{eraser: e} }

type eraseReq struct {
	UserID      string `json:"user_id"`
	AnonymousID string `json:"anonymous_id"`
}

// Erase implements the right-to-erasure path (spec §5).
func (h *AdminHandler) Erase(w http.ResponseWriter, r *http.Request) {
	var req eraseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid json")
		return
	}
	switch {
	case req.UserID != "":
		if err := h.eraser.EraseByUserID(r.Context(), req.UserID); err != nil {
			httputil.Error(w, err)
			return
		}
	case req.AnonymousID != "":
		if err := h.eraser.EraseByAnonymousID(r.Context(), req.AnonymousID); err != nil {
			httputil.Error(w, err)
			return
		}
	default:
		httputil.BadRequest(w, "user_id or anonymous_id is required")
		return
	}
	httputil.OK(w, map[string]string{"status": "erased"})
}
