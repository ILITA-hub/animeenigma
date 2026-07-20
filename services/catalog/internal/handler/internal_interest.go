package handler

// Unified interest bands — content-verify queue (:8101) support.
//
// GET /internal/interest/bands?ongoing_limit=500&top_limit=100&idle_window=100&idle_offset=N
//
// Superset of /internal/verify/membership: ongoing + browse-order top plus the
// idle backfill sub-sources (planned, non-ongoing tail window at idle_offset)
// and idle_total so the caller can wrap its round-robin cursor. Docker-network-
// only: /internal/* is never proxied by the gateway.

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// interestSource is the slice of AnimeRepository the handler needs.
type interestSource interface {
	ListInterestBands(ctx context.Context, ongoingLimit, topLimit, idleWindow, idleOffset int) (repo.InterestBands, error)
}

// InternalInterestHandler serves the banded interest snapshot.
// Docker-network-only: /internal/* is never proxied by the gateway.
type InternalInterestHandler struct {
	src interestSource
	log *logger.Logger
}

// NewInternalInterestHandler constructs the handler.
func NewInternalInterestHandler(src interestSource, log *logger.Logger) *InternalInterestHandler {
	return &InternalInterestHandler{src: src, log: log}
}

// Bands handles GET /internal/interest/bands.
func (h *InternalInterestHandler) Bands(w http.ResponseWriter, r *http.Request) {
	ongoingLimit := queryInt(r, "ongoing_limit", 500, 1, 2000)
	topLimit := queryInt(r, "top_limit", 100, 1, 500)
	idleWindow := queryInt(r, "idle_window", 100, 1, 500)
	idleOffset := queryInt(r, "idle_offset", 0, 0, 1_000_000)
	b, err := h.src.ListInterestBands(r.Context(), ongoingLimit, topLimit, idleWindow, idleOffset)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("interest bands query failed", "error", err)
		}
		http.Error(w, "interest query failed", http.StatusInternalServerError)
		return
	}
	httputil.OK(w, b)
}
