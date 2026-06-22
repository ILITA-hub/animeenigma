package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

// maxPlayerTelemetryBatch caps the number of player events accepted per POST,
// matching the per-batch defensive cap used by other analytics handlers.
const maxPlayerTelemetryBatch = 100

// PlayerTelemetryHandler ingests batched player telemetry (resolve outcomes +
// stalls) from the frontend into the analytics.events table, reusing existing
// columns via the new effect_kind values "player_resolve" and "player_stall".
//
// This route is public (/api/analytics/player-events) and gateway-proxied.
// It shares the same Sink as CollectHandler and EffectsHandler so all rows
// flow through the identical InsertBatch write path.
type PlayerTelemetryHandler struct {
	sink Sink
}

// NewPlayerTelemetryHandler builds a PlayerTelemetryHandler over the shared
// batcher Sink.
func NewPlayerTelemetryHandler(sink Sink) *PlayerTelemetryHandler {
	return &PlayerTelemetryHandler{sink: sink}
}

// wirePlayerEvent is one player telemetry event in the wire batch.
type wirePlayerEvent struct {
	// Kind must be "resolve" or "stall"; other values are silently dropped.
	Kind     string `json:"kind"`
	Provider string `json:"provider"`

	// Resolve fields
	LatencyMS       int    `json:"latency_ms"`
	Outcome         string `json:"outcome"`
	ReachedPlayback bool   `json:"reached_playback"`
	ErrorKind       string `json:"error_kind,omitempty"`

	// Stall fields
	StallMS int `json:"stall_ms"`

	// Shared metadata (can also come from the envelope below). The frontend
	// beacon (playerTelemetry.ts) sends these PER EVENT — a buffered batch can
	// span episode/anime switches — so they are read here first and only fall
	// back to the envelope values when absent.
	AnimeID string `json:"anime_id,omitempty"`
	Episode int    `json:"episode,omitempty"`
	Audio   string `json:"audio,omitempty"`
	Lang    string `json:"lang,omitempty"`
}

// wirePlayerBatch is the top-level wire payload for /api/analytics/player-events.
type wirePlayerBatch struct {
	AnimeID string            `json:"anime_id"`
	Episode int               `json:"episode,omitempty"`
	Audio   string            `json:"audio,omitempty"`
	Lang    string            `json:"lang,omitempty"`
	Events  []wirePlayerEvent `json:"events"`
}

func (h *PlayerTelemetryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024)) // 256 KB cap
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var batch wirePlayerBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	events := batch.Events
	if len(events) > maxPlayerTelemetryBatch {
		events = events[:maxPlayerTelemetryBatch]
	}

	now := time.Now().UTC()

	for _, we := range events {
		// Skip events with empty/unknown provider or unknown kind. The provider
		// becomes the source-ranking GROUP BY target, so it MUST be whitelisted
		// to stop an anonymous beacon injecting ranking rows (audit #2).
		if we.Provider == "" {
			continue
		}
		provider := whitelistProvider(we.Provider)
		if provider == "" {
			continue
		}
		var effectKind string
		var durationMS int
		switch we.Kind {
		case "resolve":
			effectKind = "player_resolve"
			durationMS = we.LatencyMS
		case "stall":
			effectKind = "player_stall"
			durationMS = we.StallMS
		default:
			continue // skip unknown kinds
		}

		// Per-event value first, envelope-level fallback. The FE sends
		// anime_id/episode/audio/lang on each event; older/envelope-style
		// senders (and our smoke tests) put them on the batch.
		audio := we.Audio
		if audio == "" {
			audio = batch.Audio
		}
		lang := we.Lang
		if lang == "" {
			lang = batch.Lang
		}
		animeID := we.AnimeID
		if animeID == "" {
			animeID = batch.AnimeID
		}
		episode := we.Episode
		if episode == 0 {
			episode = batch.Episode
		}

		// Pack contextual dimensions into Properties JSON.
		propMap := map[string]any{
			"outcome":          capString(we.Outcome),
			"reached_playback": we.ReachedPlayback,
			"audio":            audio,
			"lang":             lang,
			"episode":          episode,
		}
		if we.ErrorKind != "" {
			propMap["error_kind"] = we.ErrorKind
		}
		propsBytes, _ := json.Marshal(propMap)

		ev := domain.Event{
			EventID:    uuid.NewString(),
			EventType:  domain.EventTypePlayer,
			Timestamp:  now,
			ReceivedAt: now,

			// Effect dimensions
			EffectKind: effectKind,
			Target:     provider,
			TargetKind: "provider",
			AnimeID:    animeID,
			DurationMS: durationMS,

			// Attribution
			Source:   "fe",
			Origin:   "api",
			Accuracy: "exact",

			Requests:   1,
			Properties: string(propsBytes),
		}
		h.sink.Enqueue(ev)
	}

	w.WriteHeader(http.StatusNoContent)
}
