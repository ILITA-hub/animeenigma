// Package handler — rec_events.go: POST /api/events/rec.
//
// Phase 14 (REC-EVAL-01). Public telemetry endpoint that increments the
// rec_click_total / rec_watched_total Prometheus counters AND persists a
// rec_events row for ad-hoc Postgres analysis. JWT-OPTIONAL — anonymous
// trending-row CTR data is valid (CONTEXT.md §C4).
//
// Design constraint: telemetry must NEVER break a click. DB insert failures
// are logged but the HTTP response remains 200 so the calling frontend gets
// a clean ack and the localStorage-correlated rec_watched emit pipeline keeps
// working even when Postgres is degraded.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// recEventsRepoSurface is the narrow Insert-only interface the handler
// depends on. *repo.RecEventsRepository satisfies this in production;
// tests inject a fake.
type recEventsRepoSurface interface {
	Insert(ctx context.Context, ev *domain.RecEvent) error
}

// RecEventsHandler serves POST /api/events/rec.
type RecEventsHandler struct {
	repo recEventsRepoSurface
	log  *logger.Logger
}

// NewRecEventsHandler wires the handler with its dependencies.
func NewRecEventsHandler(repo recEventsRepoSurface, log *logger.Logger) *RecEventsHandler {
	return &RecEventsHandler{repo: repo, log: log}
}

// recEventBody is the JSON request shape for POST /api/events/rec.
type recEventBody struct {
	EventType      string  `json:"event_type"`
	AnimeID        string  `json:"anime_id"`
	SignalID       string  `json:"signal_id"`
	Pinned         bool    `json:"pinned"`
	PinSource      *string `json:"pin_source,omitempty"`
	PinSeedAnimeID *string `json:"pin_seed_anime_id,omitempty"`
	SourceRoute    *string `json:"source_route,omitempty"`
	Rank           *int    `json:"rank,omitempty"`
}

// PostRecEvent persists one rec_click or rec_watched event and increments the
// matching Prometheus counter. JWT is optional via OptionalAuthMiddleware
// (mounted at the router layer); when claims are present, the row carries
// user_id, otherwise user_id is NULL.
func (h *RecEventsHandler) PostRecEvent(w http.ResponseWriter, r *http.Request) {
	var body recEventBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.BadRequest(w, "invalid body")
		return
	}
	if body.EventType != "rec_click" && body.EventType != "rec_watched" {
		httputil.BadRequest(w, "event_type must be rec_click or rec_watched")
		return
	}
	if body.AnimeID == "" || body.SignalID == "" {
		httputil.BadRequest(w, "anime_id and signal_id are required")
		return
	}

	pinnedLabel := strconv.FormatBool(body.Pinned)
	switch body.EventType {
	case "rec_click":
		metrics.RecClickTotal.WithLabelValues(body.SignalID, pinnedLabel).Inc()
	case "rec_watched":
		metrics.RecWatchedTotal.WithLabelValues(body.SignalID, pinnedLabel).Inc()
	}

	ev := &domain.RecEvent{
		EventType:      body.EventType,
		AnimeID:        body.AnimeID,
		SignalID:       body.SignalID,
		Pinned:         body.Pinned,
		PinSource:      body.PinSource,
		PinSeedAnimeID: body.PinSeedAnimeID,
		SourceRoute:    body.SourceRoute,
		Rank:           body.Rank,
	}
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil && claims.UserID != "" {
		uid := claims.UserID
		ev.UserID = &uid
	}
	if err := h.repo.Insert(r.Context(), ev); err != nil {
		// Telemetry must NEVER break a click — log and continue.
		h.log.Errorw("rec_events insert failed (non-fatal)",
			"error", err, "event_type", body.EventType, "signal_id", body.SignalID)
	}

	httputil.OK(w, map[string]bool{"ok": true})
}
