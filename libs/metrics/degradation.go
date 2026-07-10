package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Degradation-governor metrics (graceful-degradation Phase 2,
// docs/superpowers/specs/2026-07-10-graceful-degradation-design.md).
// Emitted by services/governor; rendered on the Degradation Overview
// dashboard (uid degradation-overview).
var (
	// DegradationLevel is the authoritative published degradation level
	// (0 Normal / 1 Elevated / 2 Critical) after hysteresis and override.
	DegradationLevel = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ae_degradation_level",
			Help: "Published platform degradation level (0 normal, 1 elevated, 2 critical).",
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
)

// DegradationShed marks the shed intensity each heavy subsystem is currently
// applying (0 = admitting normally, 1 = new-work admission paused at Elevated,
// 2 = refusing at Critical). Emitted by the consumers themselves (library,
// stealth-scraper), not the governor — it reflects what is ACTUALLY shed.
var DegradationShed = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "ae_degradation_shed",
		Help: "Shed intensity currently applied by a heavy subsystem (0 none, 1 paused admission, 2 refusing).",
	},
	[]string{"subsystem"},
)
