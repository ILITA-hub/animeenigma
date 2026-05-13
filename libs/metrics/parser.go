package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ParserRequestsTotal counts parser requests by provider, operation, and status.
	ParserRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parser_requests_total",
			Help: "Total number of parser requests",
		},
		[]string{"provider", "operation", "status"},
	)

	// ParserRequestDuration tracks parser request latency.
	ParserRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "parser_request_duration_seconds",
			Help:    "Parser request latency in seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"provider", "operation"},
	)

	// ParserFallbackTotal counts fallback events (e.g. AnimeLib -> Kodik).
	ParserFallbackTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "parser_fallback_total",
			Help: "Total number of parser fallback events",
		},
		[]string{"from", "to"},
	)

	// PlayabilityCanaryRunsTotal counts scheduler scraper-playability-canary
	// run outcomes per (provider, server, result, reason, anime_slot). Used
	// by the Phase 23 canary job to surface upstream-site regressions within
	// 24h — see services/scheduler/internal/jobs/scraper_playability_canary.go
	// + infra/grafana/alerts/scraper.yaml (Phase 23 Plan 23-03).
	//
	// Label conventions:
	//   provider:   one of registered scraper providers (currently "gogoanime")
	//   server:     normalized embed extractor name (vibeplayer, streamhg,
	//               earnvids). Sentinel value "_unreachable" used when
	//               /scraper/servers itself fails so the alert layer has a
	//               deterministic label value to match.
	//   result:     "pass" | "fail"
	//   reason:     one of libs/streamprobe.Reason values (string identity —
	//               this package does NOT import libs/streamprobe to keep it
	//               dependency-free, see ParserUnplayableTotal note).
	//   anime_slot: one of {anchor_frieren, anchor_one_piece, recent_1,
	//               recent_2, recent_3} — see AnimeSlots() below.
	//
	// Cardinality bound (current):
	//   1 provider × 3 servers × 2 results × 7 reasons × 5 slots = 210 series.
	//   Well within Prometheus default limits.
	//
	// SCRAPER-HEAL-13.
	PlayabilityCanaryRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "playability_canary_runs_total",
			Help: "Total count of scraper playability canary results per (provider, server, result, reason, anime_slot)",
		},
		[]string{"provider", "server", "result", "reason", "anime_slot"},
	)
)

// AnimeSlots returns the canonical literal slot values emitted by the canary
// as the `anime_slot` label of PlayabilityCanaryRunsTotal. Single source of
// truth for the counter's label domain — the canary job, Grafana panels, and
// the alert rules in Plan 23-03 all key off these exact strings.
//
// Per CONTEXT.md D4: literal strings (not numeric indexes) for Grafana panel
// readability. `anchor_frieren` is self-describing; `0` requires a legend
// lookup. The bounded set (5) keeps cardinality predictable.
func AnimeSlots() []string {
	return []string{"anchor_frieren", "anchor_one_piece", "recent_1", "recent_2", "recent_3"}
}

// ObserveParser records parser request metrics. Call with defer:
//
//	defer metrics.ObserveParser("hianime", "get_episodes", time.Now(), &err)
func ObserveParser(provider, operation string, start time.Time, errp *error) {
	duration := time.Since(start).Seconds()
	status := "success"
	if errp != nil && *errp != nil {
		status = "error"
	}
	ParserRequestsTotal.WithLabelValues(provider, operation, status).Inc()
	ParserRequestDuration.WithLabelValues(provider, operation).Observe(duration)
}
