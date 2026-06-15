package handler

import (
	"context"
	"net/http"
	"time"
)

// playerRankingRecomputer is the analytics entry point the endpoint drives
// (service.PlayerRankingService.Recompute). Interface so the test injects a fake.
type playerRankingRecomputer interface {
	Recompute(ctx context.Context) error
}

// PlayerRankingHandler triggers the daily provider-reliability recompute +
// Redis publish. Registered ONLY at /internal/player-ranking/recompute beside
// the other /internal routes (the gateway never proxies /internal/*). The
// scheduler (no ClickHouse connection) POSTs here on its daily cron.
type PlayerRankingHandler struct {
	svc playerRankingRecomputer
}

func NewPlayerRankingHandler(svc playerRankingRecomputer) *PlayerRankingHandler {
	return &PlayerRankingHandler{svc: svc}
}

// ServeHTTP runs one recompute (body ignored). 204 on success; 500 on error. A
// bounded timeout caps the two ClickHouse aggregates.
func (h *PlayerRankingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := h.svc.Recompute(ctx); err != nil {
		http.Error(w, "recompute failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
