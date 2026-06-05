package handler

import (
	"context"
	"net/http"
	"time"
)

// readThresholdRecomputer is the analytics service entry point the recompute
// endpoint drives (service.ReadThresholdService.Recompute). Kept as an
// interface so the handler test injects a fake without ClickHouse/Redis.
type readThresholdRecomputer interface {
	Recompute(ctx context.Context) error
}

// ReadThresholdHandler triggers the daily db_read P95 recompute + Redis publish.
//
// It is registered ONLY at /internal/read-thresholds/recompute in the analytics
// router, beside /internal/effects and /internal/erase — the gateway never
// proxies /internal/* (T-02-INT / T-03-15, Docker-network-only). The scheduler
// service (which has no ClickHouse connection) POSTs here on its daily cron.
type ReadThresholdHandler struct {
	svc readThresholdRecomputer
}

// NewReadThresholdHandler builds the handler over the recompute service.
func NewReadThresholdHandler(svc readThresholdRecomputer) *ReadThresholdHandler {
	return &ReadThresholdHandler{svc: svc}
}

// ServeHTTP runs one recompute. The body is ignored (the trigger carries no
// payload); a bounded timeout caps the ClickHouse query so a slow CH cannot
// pin the request. Success → 204; a compute/publish error → 500.
func (h *ReadThresholdHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := h.svc.Recompute(ctx); err != nil {
		http.Error(w, "recompute failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
