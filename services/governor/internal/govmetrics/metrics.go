// Package govmetrics holds the governor's singleton Prometheus metrics.
// Deliberately NOT in libs/metrics: plain (non-Vec) promauto metrics
// auto-register in every binary that imports the shared package and would
// export as a permanent 0 from ~20 services, turning the dashboard's
// authoritative ae_degradation_level into a 5-way ambiguous series set.
// Only the governor emits these; only the governor links this package.
package govmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DegradationLevel is the authoritative published degradation level
	// (0 Normal / 1 Elevated / 2 Critical) after hysteresis and override.
	DegradationLevel = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ae_degradation_level",
			Help: "Published platform degradation level (0 normal, 1 elevated, 2 critical).",
		},
	)

	// DegradationScore is the authoritative smoothed pressure score (0.00-1.00)
	// after asymmetric EWMA and override pinning.
	DegradationScore = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ae_degradation_score",
			Help: "Published continuous pressure score (0.00 normal .. 1.00 critical), smoothed.",
		},
	)

	// DegradationReasonActive marks which signals currently justify the
	// published level. Label universe is fixed and bounded: breach signals
	// (psi_cpu_some, psi_io_full, psi_mem_full, mem_available) at severity
	// elevated|critical, plus synthetic info reasons (manual_override,
	// prometheus_unreachable).
	DegradationReasonActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ae_degradation_reason_active",
			Help: "1 when the (signal, severity) reason is active for the published degradation level.",
		},
		[]string{"signal", "severity"},
	)

	// GovernorTransitionsTotal counts published level changes by destination.
	GovernorTransitionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "governor_transitions_total",
			Help: "Degradation level transitions by destination level.",
		},
		[]string{"to_level"},
	)

	// GovernorEvalFailuresTotal counts Prometheus polls that failed.
	GovernorEvalFailuresTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "governor_eval_failures_total",
			Help: "Failed governor evaluation ticks (Prometheus unreachable or bad response).",
		},
	)

	// SignalStaleness is the age (seconds) of the freshest pressure sample at
	// the last successful poll: time() - timestamp(ae:pressure_score:preview).
	// Rises when node-exporter scrape or rule evaluation lags under load — the
	// blind spot a slow-but-succeeding Prometheus would otherwise hide.
	SignalStaleness = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ae_governor_signal_staleness_seconds",
			Help: "Age of the freshest pressure sample at last poll (seconds); high => scrape/rule-eval lagging.",
		},
	)

	// GovernorStaleTicksTotal counts ticks where the signal was too old to trust
	// and the governor held the level instead of acting on stale data.
	GovernorStaleTicksTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "governor_stale_ticks_total",
			Help: "Ticks where the pressure signal exceeded the staleness budget and the level was held.",
		},
	)

	// EgressUplinkFraction is uplink egress ÷ configured capacity
	// (GOVERNOR_UPLINK_MBPS). 0 when uplink governance is disabled or idle.
	EgressUplinkFraction = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ae_governor_egress_uplink_fraction",
			Help: "Uplink egress as a fraction of configured capacity (0 when disabled/idle).",
		},
	)
)
