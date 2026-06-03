// Package metrics — scraper provider health collectors (Phase 17 + Phase 21).
//
// Five collectors:
//   - ProviderHealthUp:        gauge{provider, stage}, 0|1, written by the
//     liveness probe after 3-of-15min threshold logic.
//   - ProviderProbeLastTick:   gauge{provider}, Unix ts; heartbeat for the
//     absent()-style dead-probe alert (RESEARCH P-07).
//   - ParserZeroMatchTotal:    counter{provider, selector}, missing today —
//     required by SCRAPER-NF-04. Selector label MUST
//     be a short stable identifier (NOT raw CSS) — see
//     RESEARCH Pitfall P-02 (cardinality bomb).
//   - ParserUnplayableTotal:   counter{provider, server, reason}, every
//     playability-gate fail in GetStream increments. reason MUST be one of
//     the libs/streamprobe.ReasonEnum values (string identity, no import).
//     SCRAPER-HEAL-06.
//   - ParserAdDecoyTotal:      counter{provider, server}, dedicated subset
//     of ParserUnplayableTotal with reason="ad_decoy". Kept as a separate
//     counter so the ScraperAdDecoySurge alert (Phase 23) can fire on a
//     simple non-zero rate without label-matching. SCRAPER-HEAL-06.
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

	// ParserUnplayableTotal counts playability-gate failures observed inside
	// a provider's GetStream path. The `reason` label MUST be one of the 7
	// typed values defined in libs/streamprobe/reason.go (string identity —
	// this package does not import libs/streamprobe to keep libs/metrics
	// dependency-free for downstream consumers and to avoid a cyclic
	// potential).
	//
	// Cardinality bound: 7 reasons × |providers| × |servers| ≈ 7 × 3 × 5 =
	// ~100 series. server label values are normalized embed names
	// (vibeplayer, streamhg, earnvids) from the embed registry — bounded
	// set, NOT raw URLs. SCRAPER-HEAL-06.
	ParserUnplayableTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parser_unplayable_total",
			Help: "Total count of playability-gate failures per (provider, server, reason). reason is one of libs/streamprobe.Reason values.",
		},
		[]string{"provider", "server", "reason"},
	)

	// ParserAdDecoyTotal counts the subset of ParserUnplayableTotal where
	// reason == "ad_decoy" — a dedicated counter so the Prometheus alert
	// rule "ScraperAdDecoySurge" (Phase 23) can fire on a simple non-zero
	// rate without label-matching. SCRAPER-HEAL-06.
	ParserAdDecoyTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parser_ad_decoy_total",
			Help: "Total count of playability-gate ad-decoy classifications per (provider, server). Subset of parser_unplayable_total with reason='ad_decoy'.",
		},
		[]string{"provider", "server"},
	)

	// ProviderEnabled is the config-driven management gauge: 1 = enabled
	// (registered in the failover chain), 0 = disabled. Emitted for ALL known
	// providers so disabled ones remain visible in Grafana. Source:
	// scraper-providers.yaml. ISS-023.
	ProviderEnabled = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_enabled",
			Help: "Whether a scraper provider is enabled in the failover chain (1=enabled, 0=disabled), per scraper-providers.yaml",
		},
		[]string{"provider"},
	)

	// ProviderInfo is an info-style gauge (always 1) carrying per-provider
	// management metadata (reason, description) for the Grafana table. Values
	// change only on a config edit + scraper restart, so cardinality is bounded
	// (~7 providers). ISS-023.
	ProviderInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_info",
			Help: "Info gauge (always 1) exposing per-provider management metadata (reason, description) for Grafana",
		},
		[]string{"provider", "reason", "description"},
	)
)
