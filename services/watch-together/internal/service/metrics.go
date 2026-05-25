// Package service holds the watch-together domain logic. The RoomService is
// the single mutation surface for room lifecycle (Create / Get / Delete);
// handlers in internal/handler/ delegate every Redis write through here so
// validation + metrics live in one place.
//
// Service-level Prometheus metrics (matching the catalog set in
// .planning/workstreams/watch-together/phases/01-backend-foundation/01-CONTEXT.md
// §Metrics):
//
//   - wt_room_create_total — bumped on every successful POST /rooms
//
// WebSocket / hub metrics live in internal/hub/metrics.go (separate file so
// the bell-domain metric stays decoupled from the connection-domain metrics).
package service

import "github.com/prometheus/client_golang/prometheus"

// RoomCreateTotal counts every successful room creation. Bumped from
// RoomService.Create after the repo CreateRoom call returns nil; failures
// before the persist do NOT increment the counter.
//
// No labels — the counter is intentionally low-cardinality. Per-anime /
// per-player breakdowns are out of scope for v1.0 (would push label
// cardinality into the hundreds-of-thousands range as the catalog grows).
var RoomCreateTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "wt_room_create_total",
	Help: "Total watch-together rooms created via POST /rooms",
})

// DriftCorrectionsTotal counts every drift correction emitted by the
// DriftEngine (see sync.go), labelled by severity:
//
//	soft       — 1.5s < drift <= 5s (single playback:correction nudge)
//	hard       — drift > 5s         (single playback:correction nudge,
//	                                  member's consecutive-hard counter ++)
//	persistent — 5th consecutive hard drift (error:PERSISTENT_DRIFT, no
//	                                  further corrections for this member
//	                                  until Reset/reconnect)
//
// Phase 1 Grafana panel (WT-NF-06) ingests this for the "drift corrections
// per minute" stat. Label cardinality is bounded to 3 — well inside the
// safe range.
var DriftCorrectionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "wt_drift_corrections_total",
		Help: "Drift corrections emitted by the watch-together drift engine, by severity",
	},
	[]string{"severity"}, // "soft" | "hard" | "persistent"
)

// RateLimitedTotal counts every inbound message rejected by the per-user
// in-process token bucket (WT-NF-02). Labelled by message type so the
// Grafana row can break out seek-rate-limits vs chat-rate-limits.
var RateLimitedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "wt_ws_rate_limited_total",
		Help: "Inbound watch-together messages rejected by the per-user rate limiter, by type",
	},
	[]string{"type"},
)

// ChatMessagesTotal counts every chat:message that survives the cap +
// rate-limit gates and is broadcast to the room. Tracks both the persisted
// LIST entry AND the wire fanout (1:1 relationship).
var ChatMessagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "wt_chat_messages_total",
	Help: "Chat messages broadcast (excludes over-cap and rate-limited rejections)",
})

// ReactionsTotal counts every chat:reaction broadcast. Reactions are
// ephemeral (no Redis LIST append) but still bumps a counter so the Phase 5
// Grafana panel can size the activity overlay properly.
var ReactionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "wt_reactions_total",
	Help: "Emoji reactions broadcast to watch-together rooms (not persisted)",
})

// init registers every metric onto the default Prometheus registry so the
// service-wide /metrics handler in cmd/watch-together-api/main.go surfaces
// them automatically. Mirrors the explicit-MustRegister pattern from
// internal/hub/metrics.go so a grep for `MustRegister` in this service
// finds every counter.
func init() {
	prometheus.MustRegister(
		RoomCreateTotal,
		DriftCorrectionsTotal,
		RateLimitedTotal,
		ChatMessagesTotal,
		ReactionsTotal,
	)
}
