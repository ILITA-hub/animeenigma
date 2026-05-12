// Package metrics — scraper provider health collectors (Phase 17).
//
// Three collectors:
//   - ProviderHealthUp:        gauge{provider, stage}, 0|1, written by the
//     liveness probe after 3-of-15min threshold logic.
//   - ProviderProbeLastTick:   gauge{provider}, Unix ts; heartbeat for the
//     absent()-style dead-probe alert (RESEARCH P-07).
//   - ParserZeroMatchTotal:    counter{provider, selector}, missing today —
//     required by SCRAPER-NF-04. Selector label MUST
//     be a short stable identifier (NOT raw CSS) — see
//     RESEARCH Pitfall P-02 (cardinality bomb).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ProviderHealthUp is the per-(provider, stage) liveness gauge. The probe
	// runner (Plan 17-02) sets this after each 15-minute tick based on the
	// 3-failures-in-15-min sliding-window threshold.
	ProviderHealthUp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_health_up",
			Help: "Whether a scraper provider stage is up (1=up, 0=down) per the 3-of-15min liveness probe",
		},
		[]string{"provider", "stage"},
	)

	// ProviderProbeLastTick is the per-provider probe heartbeat. Grafana's
	// `absent_over_time(...) > 0` alert uses this to catch the "probe died
	// silently and the gauge is stale 1" case.
	ProviderProbeLastTick = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_probe_last_tick_timestamp",
			Help: "Unix timestamp of the last completed probe tick per provider",
		},
		[]string{"provider"},
	)

	// ParserZeroMatchTotal counts selector-miss events. Selector label MUST
	// be a short stable identifier (e.g. "episode_list_item"), NOT a raw CSS
	// string — see RESEARCH Pitfall P-02 (cardinality bomb).
	ParserZeroMatchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parser_zero_match_total",
			Help: "Total count of HTML/JSON selector-miss events per (provider, selector)",
		},
		[]string{"provider", "selector"},
	)
)
