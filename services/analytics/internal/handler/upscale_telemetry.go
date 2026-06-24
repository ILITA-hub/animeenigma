package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// maxUpscaleTelemetryBatch bounds how many telemetry rows a single
// /internal/upscale-telemetry POST may ingest. Mirrors maxEffectBatch.
const maxUpscaleTelemetryBatch = 1000

// UpscaleTelemetryStore is the write interface required by UpscaleTelemetryHandler.
// ClickHouseStore satisfies this interface via InsertUpscaleTelemetry.
type UpscaleTelemetryStore interface {
	InsertUpscaleTelemetry(ctx context.Context, rows []repo.UpscaleTelemetryRow) error
}

// UpscaleTelemetryHandler ingests batches of per-worker GPU telemetry rows
// forwarded by the upscaler service (which acts as the trust boundary — the
// untrusted worker never touches analytics directly).
//
// This route is registered ONLY at /internal/upscale-telemetry in the
// analytics router — Docker-network-only, NEVER gateway-proxied (CD-15,
// mirrors /internal/effects isolation).
type UpscaleTelemetryHandler struct {
	store UpscaleTelemetryStore
	log   *logger.Logger
}

// NewUpscaleTelemetryHandler builds an UpscaleTelemetryHandler backed by store.
func NewUpscaleTelemetryHandler(store UpscaleTelemetryStore) *UpscaleTelemetryHandler {
	return &UpscaleTelemetryHandler{store: store, log: logger.Default()}
}

func (h *UpscaleTelemetryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 256 KB cap mirrors collect.go and effects.go defensive caps.
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var rows []repo.UpscaleTelemetryRow
	if err := json.Unmarshal(body, &rows); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Cap array length — anything past maxUpscaleTelemetryBatch is dropped.
	if len(rows) > maxUpscaleTelemetryBatch {
		rows = rows[:maxUpscaleTelemetryBatch]
	}

	// InsertUpscaleTelemetry is a direct CH write (no batcher needed for the
	// low-frequency telemetry path — batching happens on the upscaler side).
	// We return 202 regardless so the upscaler's fire-and-forget producer
	// doesn't retry on transient failures; but we log the error so a persistent
	// CH write failure is NOT silent.
	if err := h.store.InsertUpscaleTelemetry(r.Context(), rows); err != nil {
		h.log.Warnw("upscale_telemetry: CH insert failed (rows dropped)", "error", err, "rows", len(rows))
	}
	w.WriteHeader(http.StatusAccepted)
}
