// Package hub holds the in-process WebSocket connection registry for the
// watch-together service. The Hub maintains per-room connection sets, dispatches
// broadcast / per-recipient envelopes, and bridges to Redis pubsub for the
// forward-compat multi-instance fanout described in
// docs/superpowers/specs/2026-05-25-watch-together-design.md
// §Forward-Compat Pubsub. v1.0 is single-instance so the subscriber is wired but
// drops every message tagged with our own instanceID.
package hub

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metrics for the watch-together WebSocket hub. Per Phase 1 metric
// scope in 01-CONTEXT.md §Metrics:
//
//   - wt_ws_connections_active{room_id}  — gauge of currently-open connections
//   - wt_ws_messages_received_total{type} — inbound counter (bumped by readPump)
//   - wt_ws_messages_sent_total{type}     — outbound counter (bumped by local fanout)
//   - wt_ws_messages_dropped_total        — outbound dropped due to full send buffer
//
// These names MUST NOT change once shipped — Grafana dashboard rows + alert
// rules in Phase 5 query against them. label cardinality of {type} is bounded
// by the ~16 Msg* constants in domain/ws_message.go, well under Prometheus's
// "low cardinality" guideline.
var (
	// Plain Gauge (no room_id label): rooms are ephemeral UUIDs, so a per-room
	// series was created for every room and never deleted on room end —
	// unbounded Prometheus cardinality (audit #25). The only consumer is the
	// dashboard's sum(wt_ws_connections_active), which already discards the
	// label, so an aggregate gauge is an exact, bounded replacement.
	ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "wt_ws_connections_active",
			Help: "Active WebSocket connections across all watch-together rooms",
		},
	)

	MessagesReceivedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wt_ws_messages_received_total",
			Help: "Inbound WebSocket messages by envelope type",
		},
		[]string{"type"},
	)

	MessagesSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wt_ws_messages_sent_total",
			Help: "Outbound WebSocket messages by envelope type (local fanout only)",
		},
		[]string{"type"},
	)

	MessagesDroppedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wt_ws_messages_dropped_total",
			Help: "Outbound WebSocket messages dropped because the recipient's send buffer was full",
		},
	)
)

// init registers every metric onto the default Prometheus registry so the
// service-wide /metrics handler wired up in cmd/watch-together-api/main.go
// (per 01.1 scaffold) automatically surfaces them. promauto would also work
// here, but the plan acceptance criteria require an explicit MustRegister call
// the linter can grep for. The two patterns are functionally equivalent.
func init() {
	prometheus.MustRegister(
		ActiveConnections,
		MessagesReceivedTotal,
		MessagesSentTotal,
		MessagesDroppedTotal,
	)
}
