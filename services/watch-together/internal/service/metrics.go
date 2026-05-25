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

// init registers RoomCreateTotal onto the default Prometheus registry so the
// service-wide /metrics handler in cmd/watch-together-api/main.go surfaces
// it automatically. Mirrors the explicit-MustRegister pattern from
// internal/hub/metrics.go so a grep for `MustRegister` in this service
// finds every counter.
func init() {
	prometheus.MustRegister(RoomCreateTotal)
}
