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

// RoomsActive is the live gauge of currently-active watch-together rooms.
// Bumped (+1) by RoomService.Create after a successful CreateRoom; bumped
// (-1) by RoomService.Delete and GraceManager.fire after a successful
// teardown. The Grafana panel in Plan 05.6 reads this as the "active rooms"
// stat — a gauge, not a counter, because it must reflect current state at
// scrape time, not cumulative creations.
//
// Plain Gauge — no labels. Per Phase 1 cardinality philosophy, we resist
// the temptation to break out by player / anime / host. The aggregate is
// what the panel needs; per-X breakdowns are out of scope for v1.0.
var RoomsActive = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "wt_rooms_active",
	Help: "Currently active watch-together rooms (incremented on Create, decremented on Delete or grace fire)",
})

// MembersPerRoom is the distribution of member counts observed at every
// member:joined broadcast. Each observation is the room's CURRENT member
// count when that particular join landed — so a room going 1→2→3 members
// over its lifetime contributes 3 observations (1, 2, 3).
//
// Buckets: 1..10. The 10-bucket layout matches the WT-NF-02 10-member
// capacity cap exactly, so the histogram has zero overflow at the cap and
// the Grafana panel can heat-map the distribution cleanly. No +Inf bucket
// problem — Prometheus's auto +Inf will register zero observations because
// the post-AddMember+capacity-check path enforces n <= 10.
var MembersPerRoom = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "wt_members_per_room",
	Help:    "Distribution of member count observed at member:joined broadcast time (current room size after the join lands)",
	Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
})

// ChatMessagesPerRoom is the final chat message count observed at room
// teardown (either explicit Delete or grace fire). Captures the activity
// level of the room over its entire lifetime as a single observation.
//
// Buckets: 0, 1, 5, 10, 25, 50, 100. The 100 bucket is the LIST cap
// (LTRIM 0 99 per Phase 1) — anything reported as 100 means the room was
// at full chat saturation at teardown, AND the LIST was capped to 100
// during life, so older messages were already evicted. The 0 bucket is
// meaningful (quick test rooms with no chat) — empty rooms still observe.
var ChatMessagesPerRoom = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "wt_chat_messages_per_room",
	Help:    "Final chat message count observed at room teardown (Delete or grace fire) — captures lifetime activity level",
	Buckets: []float64{0, 1, 5, 10, 25, 50, 100},
})

// SessionDurationSeconds is the total wall-clock duration a room existed,
// observed at teardown (Delete or grace fire). Computed as
// time.Since(room.CreatedAt) at the teardown moment.
//
// Buckets anchored to the practical lifecycle of co-watch sessions:
//
//	60s    — test rooms
//	300s   — quickie (5min)
//	900s   — one episode (15min)
//	1800s  — movie short (30min)
//	3600s  — typical session (1h)
//	7200s  — full movie (2h)
//	14400s — binge (4h+)
//
// Reading the histogram: p50 should land around 1800-3600s for a healthy
// product; a p50 < 300s suggests rooms aren't sticking (UX issue).
var SessionDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "wt_session_duration_seconds",
	Help:    "Total seconds a room existed (Create → Delete). Observed at teardown (Delete or grace fire)",
	Buckets: []float64{60, 300, 900, 1800, 3600, 7200, 14400},
})

// PersistentDriftTotal counts every persistent-drift error emitted by the
// DriftEngine (drift > 5s for 5 consecutive ticks), labelled by whether
// the affected user is the room's host or a regular member. The Grafana
// panel uses this to answer "is drift concentrated on hosts (likely
// network) or members (likely client-side)?".
//
// Label cardinality bounded to 2: "host" | "member". The label is set by
// the caller in inbound.go (handleTimeTick) by comparing the room's
// host_user_id to the affected userID — no string from user input ever
// enters the label.
var PersistentDriftTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "wt_persistent_drift_total",
		Help: "Persistent-drift errors emitted by the drift engine, labelled by user role (host|member)",
	},
	[]string{"user_role"}, // "host" | "member"
)

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
		RoomsActive,
		MembersPerRoom,
		ChatMessagesPerRoom,
		SessionDurationSeconds,
		PersistentDriftTotal,
	)
}
