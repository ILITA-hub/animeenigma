// Package domain holds the degradation-governor model: the level scale, the
// per-tick verdict extracted from Prometheus, and the published snapshot /
// transition records. Full design:
// docs/superpowers/specs/2026-07-10-graceful-degradation-design.md.
package domain

import (
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

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
	// Aliased to the shared consumer-side constant so producer and consumers
	// can never drift apart.
	RedisLevelKey = cache.DegradationLevelKey
	// RedisScoreKey holds the smoothed pressure score as "0.00".."1.00".
	RedisScoreKey = cache.DegradationScoreKey
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
	// ReasonHeldByHysteresis marks a level that is > 0 purely because the
	// smoothed score has not yet decayed below the quantizer's exit threshold —
	// no instantaneous breach is active. Without it the status/dashboard shows
	// "level 1, reasons: []", which reads as an unexplained degradation.
	ReasonHeldByHysteresis = "held_by_hysteresis"
	// ReasonSignalStale marks a tick where Prometheus answered but the freshest
	// sample is older than the staleness budget (scrape/rule-eval lagging under
	// the very load we are watching). The governor HOLDS the level on such ticks
	// rather than trusting stale data to lower shedding.
	ReasonSignalStale = "signal_stale"
)

// EgressUplinkSignal is the breach-signal name for uplink-egress pressure
// (governor-computed from ae:host_egress:bytes_per_second vs GOVERNOR_UPLINK_MBPS,
// not a Prometheus recording rule). Opt-in: only ever active when an uplink
// capacity is configured.
const EgressUplinkSignal = "egress_uplink"

// BreachSignals is the fixed universe of breach signal names the governor can
// report — the ae:pressure_breach:* recording-rule signals
// (docker/prometheus/rules/degradation.yml) plus the governor-computed
// egress_uplink. Kept as a fixed list so the reason gauge publishes a stable,
// bounded label set (absent = 0, never a stale 1).
var BreachSignals = []string{"psi_cpu_some", "psi_io_full", "psi_mem_full", "mem_available", EgressUplinkSignal}

// InfoReasons is the fixed universe of synthetic (info-severity) reasons the
// governor can publish. Kept here beside BreachSignals so the reason gauge's
// zeroing loop covers EVERY possible reason — a new info reason added without
// updating this list would otherwise leave a stale =1 gauge, the exact hazard
// the fixed-universe design exists to prevent.
var InfoReasons = []string{ReasonManualOverride, ReasonPrometheusUnreachable, ReasonHeldByHysteresis, ReasonSignalStale}

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
	// Score is the raw (pre-smoothing) pressure score: max of
	// ae:pressure_score:preview and the governor-computed egress band.
	Score float64
	// SampleAgeSeconds is the age of the freshest pressure sample at query time
	// (time() - timestamp(ae:pressure_score:preview)). A negative value means
	// "unknown" (the age sub-query failed) and is treated as not-stale.
	SampleAgeSeconds float64
	// EgressFraction is egress ÷ configured uplink (0 when uplink governance is
	// disabled). Published for the dashboard, not folded twice.
	EgressFraction float64
}

// Snapshot is the currently-published state (served on /api/degradation/status).
type Snapshot struct {
	Level       Level              `json:"level"`
	Score       float64            `json:"score"`
	Reasons     []Reason           `json:"reasons"`
	Signals     map[string]float64 `json:"signals,omitempty"`
	Override    *Level             `json:"override,omitempty"`
	Target      Level              `json:"target"`
	UpdatedAt   time.Time          `json:"updated_at"`
	PromHealthy bool               `json:"prometheus_healthy"`
	// SampleAgeSeconds / EgressFraction mirror the Verdict fields for the status
	// endpoint + dashboard (negative age = unknown; 0 egress = disabled/idle).
	SampleAgeSeconds float64 `json:"sample_age_seconds"`
	EgressFraction   float64 `json:"egress_fraction"`
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
