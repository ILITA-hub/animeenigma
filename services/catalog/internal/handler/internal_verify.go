package handler

// Content-verify queue membership — content-verify service (:8101) support.
//
// GET /internal/verify/membership?ongoing_limit=500&top_limit=100
//
// Returns the anime the content-verify probe queue should track: all visible
// ongoings (for freshness sampling) plus the browse-order top (sort_priority
// DESC, score DESC — mirrors the public browse ranking).
//
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as the other /internal/* endpoints (the
// gateway does not proxy /internal/*).
//
// Response 200: {"success":true,"data":{"ongoing":[...],"top":[...]}}
// Response 500: repo query failure.

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// verifyMembershipSource is the slice of AnimeRepository the handler needs.
type verifyMembershipSource interface {
	ListVerifyMembership(ctx context.Context, ongoingLimit, topLimit int) ([]repo.VerifyMembershipRow, []repo.VerifyMembershipRow, error)
}

// InternalVerifyHandler serves the content-verify queue membership.
// Docker-network-only: /internal/* is never proxied by the gateway.
type InternalVerifyHandler struct {
	src verifyMembershipSource
	log *logger.Logger
}

// NewInternalVerifyHandler constructs the handler.
func NewInternalVerifyHandler(src verifyMembershipSource, log *logger.Logger) *InternalVerifyHandler {
	return &InternalVerifyHandler{src: src, log: log}
}

type verifyMembershipResponse struct {
	Ongoing []repo.VerifyMembershipRow `json:"ongoing"`
	Top     []repo.VerifyMembershipRow `json:"top"`
}

// Membership handles GET /internal/verify/membership.
func (h *InternalVerifyHandler) Membership(w http.ResponseWriter, r *http.Request) {
	ongoingLimit := queryInt(r, "ongoing_limit", 500, 1, 2000)
	topLimit := queryInt(r, "top_limit", 100, 1, 500)
	ongoing, top, err := h.src.ListVerifyMembership(r.Context(), ongoingLimit, topLimit)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("verify membership query failed", "error", err)
		}
		http.Error(w, "membership query failed", http.StatusInternalServerError)
		return
	}
	if ongoing == nil {
		ongoing = []repo.VerifyMembershipRow{}
	}
	if top == nil {
		top = []repo.VerifyMembershipRow{}
	}
	httputil.OK(w, verifyMembershipResponse{Ongoing: ongoing, Top: top})
}

// queryInt parses an integer query param, clamping to [min, max] and falling
// back to def when absent, malformed, or out of range.
func queryInt(r *http.Request, key string, def, min, max int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= min && n <= max {
			return n
		}
	}
	return def
}
