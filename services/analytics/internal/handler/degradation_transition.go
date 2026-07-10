package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// DegradationTransitionStore is the write interface required by
// DegradationTransitionHandler. ClickHouseStore satisfies it via
// InsertDegradationTransition.
type DegradationTransitionStore interface {
	InsertDegradationTransition(ctx context.Context, row repo.DegradationTransitionRow) error
}

// DegradationTransitionHandler ingests governor degradation-level transitions
// — the durable "what changed, when, and why" history behind the Degradation
// Overview dashboard annotations.
//
// This route is registered ONLY at /internal/degradation/transition in the
// analytics router — Docker-network-only, NEVER gateway-proxied (mirrors
// /internal/effects isolation).
type DegradationTransitionHandler struct {
	store DegradationTransitionStore
	log   *logger.Logger
}

// NewDegradationTransitionHandler builds the handler backed by store.
func NewDegradationTransitionHandler(store DegradationTransitionStore) *DegradationTransitionHandler {
	return &DegradationTransitionHandler{store: store, log: logger.Default()}
}

func (h *DegradationTransitionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 64 KB cap: a transition is a handful of reasons + ~a dozen signal values.
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var row repo.DegradationTransitionRow
	if err := json.Unmarshal(body, &row); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if row.FromLevel > 2 || row.ToLevel > 2 {
		http.Error(w, "level out of range", http.StatusBadRequest)
		return
	}
	if row.TS.IsZero() {
		row.TS = time.Now().UTC()
	}
	if err := h.store.InsertDegradationTransition(r.Context(), row); err != nil {
		h.log.Errorw("degradation transition insert failed", "error", err)
		http.Error(w, "insert failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
