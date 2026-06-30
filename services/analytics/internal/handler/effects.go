package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

// maxEffectBatch bounds how many effect rows a single /internal/effects POST
// may ingest, mirroring collect.go's defensive caps (T-02-VAL). Anything past
// this in the array is dropped.
const maxEffectBatch = 1000

// EffectsHandler ingests batches of backend effect rows (egress / db / cache)
// produced by the libs/tracing producer. It reuses the same Sink interface as
// CollectHandler so the existing in-process batcher is the single write path.
//
// This route is registered ONLY at /internal/effects in the analytics router,
// beside /internal/erase — the gateway never proxies /internal/* (T-02-INT,
// Docker-network-only).
type EffectsHandler struct {
	sink Sink
}

// NewEffectsHandler builds an EffectsHandler over the shared batcher Sink.
func NewEffectsHandler(sink Sink) *EffectsHandler {
	return &EffectsHandler{sink: sink}
}

// wireEffect mirrors the libs/tracing producer's JSON effect shape. user_id is
// carried in the producer payload (it rode a PRIVATE ctx value on the BE side,
// never W3C baggage — T-02-PII).
type wireEffect struct {
	Origin     string `json:"origin"`
	Operation  string `json:"operation"`
	EffectKind string `json:"effect_kind"`
	TargetKind string `json:"target_kind"`
	Target     string `json:"target"`
	Provider   string `json:"provider"`
	UserID     string `json:"user_id"`
	AnimeID    string `json:"anime_id"`
	TraceID    string `json:"trace_id"`
	Status     int    `json:"status"`
	Requests   int    `json:"requests"`
	BytesIn    int    `json:"bytes_in"`
	BytesOut   int    `json:"bytes_out"`
	DurationMS int    `json:"duration_ms"`
	RowCount   int    `json:"row_count"`
}

type wireEffectBatch struct {
	Effects []wireEffect `json:"effects"`
}

func (h *EffectsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 256 KB cap mirrors collect.go (T-02-VAL). A truncated body fails JSON
	// parse → 400, which is the desired reject for oversize payloads.
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var batch wireEffectBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	effects := batch.Effects
	if len(effects) > maxEffectBatch {
		effects = effects[:maxEffectBatch]
	}

	for _, we := range effects {
		kind := we.EffectKind
		if kind == "" {
			kind = "egress"
		}
		requests := we.Requests
		if requests <= 0 {
			requests = 1
		}
		ev := domain.Event{
			EventID:    uuid.NewString(),
			ReceivedAt: now,
			Timestamp:  now,
			UserID:     we.UserID,

			Origin:     we.Origin,
			Operation:  we.Operation,
			EffectKind: kind,
			TargetKind: we.TargetKind,
			Target:     we.Target,
			Provider:   we.Provider,
			Source:     "be",
			Accuracy:   "exact",
			AnimeID:    we.AnimeID,
			TraceID:    we.TraceID,

			Requests:   requests,
			BytesIn:    we.BytesIn,
			BytesOut:   we.BytesOut,
			DurationMS: we.DurationMS,
			RowCount:   we.RowCount,
		}
		h.sink.Enqueue(ev)
	}

	w.WriteHeader(http.StatusNoContent)
}
