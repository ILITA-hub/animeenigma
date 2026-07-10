// Package domain holds the degradation-governor model: the level scale, the
// per-tick verdict extracted from Prometheus, and the published snapshot /
// transition records. Full design:
// docs/superpowers/specs/2026-07-10-graceful-degradation-design.md.
package domain

import "time"

// Level is the platform-wide degradation level. Heavy subsystems (library
// torrent/encode, Camoufox warming, scheduler heavy crons) consume it via
// Redis and shed NEW work admission at Elevated and above.
type Level int

const (
	LevelNormal   Level = 0
	LevelElevated Level = 1
	LevelCritical Level = 2
)

// Redis keys. Consumers MUST fail open: a missing/expired level key means
// LevelNormal (the governor refreshes the key every tick with a TTL, so a
// dead governor degrades to "no shedding" — never to "everything shed").
const (
	// RedisLevelKey holds the current published level as "0" | "1" | "2".
	RedisLevelKey = "ae:degradation:level"
	// RedisReasonsKey holds the JSON-encoded reasons slice for the current level.
	RedisReasonsKey = "ae:degradation:reasons"
	// RedisOverrideKey, when set to "0" | "1" | "2" (by the owner via
	// bin/degradation-override.sh), pins the published level. The hysteresis
	// machine keeps evaluating in the background so clearing the override
	// snaps back to the computed level instantly.
	RedisOverrideKey = "ae:degradation:override"
)

// Reason severities, published as the `severity` label of
// ae_degradation_reason_active and stored in transition rows.
const (
	SeverityElevated = "elevated"
	SeverityCritical = "critical"
	SeverityInfo     = "info"
)

// Synthetic reason signals (not Prometheus breach signals).
const (
	ReasonManualOverride        = "manual_override"
	ReasonPrometheusUnreachable = "prometheus_unreachable"
)

// BreachSignals is the fixed universe of breach signal names emitted by the
// ae:pressure_breach:* recording rules (docker/prometheus/rules/degradation.yml).
// Kept as a fixed list so the reason gauge publishes a stable, bounded label
// set (absent = 0, never a stale 1).
var BreachSignals = []string{"psi_cpu_some", "psi_io_full", "psi_mem_full", "mem_available"}

// Reason is one active cause of the current level.
type Reason struct {
	Signal   string `json:"signal"`
	Severity string `json:"severity"`
}

// Verdict is what one Prometheus poll says RIGHT NOW (no hysteresis).
type Verdict struct {
	// Target is the instantaneous level implied by the active breaches.
	Target Level
	// Reasons lists the breach signals active at their highest severity.
	Reasons []Reason
	// Signals carries the raw ae:host_* signal values for observability
	// (stored on transitions so "why" survives in ClickHouse).
	Signals map[string]float64
}

// Snapshot is the currently-published state (served on /api/degradation/status).
type Snapshot struct {
	Level       Level              `json:"level"`
	Reasons     []Reason           `json:"reasons"`
	Signals     map[string]float64 `json:"signals,omitempty"`
	Override    *Level             `json:"override,omitempty"`
	Target      Level              `json:"target"`
	UpdatedAt   time.Time          `json:"updated_at"`
	PromHealthy bool               `json:"prometheus_healthy"`
}

// Transition records one published-level change; persisted to ClickHouse
// (analytics.degradation_transitions) via the analytics internal endpoint —
// the durable "what changed, when, and why" history.
type Transition struct {
	TS           time.Time          `json:"ts"`
	FromLevel    Level              `json:"from_level"`
	ToLevel      Level              `json:"to_level"`
	Reasons      []string           `json:"reasons"`
	SignalValues map[string]float64 `json:"signal_values"`
}
