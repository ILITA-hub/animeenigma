package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Phase 14 (REC-EVAL-02) — recs engine click + watched counters.
//
// RecClickTotal and RecWatchedTotal are the two counter primitives the eval
// pipeline ships. The Per-signal CTR (1h rate) Grafana panel computes its
// metric on demand via:
//
//   rate(rec_watched_total[1h]) / rate(rec_click_total[1h])  by signal_id
//
// — there is intentionally NO separate stored rec_signal_ctr metric (avoids
// a custom metric type and works with the standard Prometheus rate function).
//
// Cardinality is bounded: signal_id is one of the closed set {s1, s2, s3,
// s4, s5, s6_pin}; pinned is the literal "true" or "false" string. Total
// series count is at most 6 × 2 = 12 per counter — small and safe.
//
// Anime ID is intentionally NOT a label (would explode cardinality at ~3.5k
// anime). Per-anime CTR breakdown is recovered via Postgres queries against
// the rec_events table; ad-hoc analysis tooling is deferred to v2.1 per
// CONTEXT.md §reference_data #3.
var (
	// RecClickTotal counts user clicks on cards in any recs row, labeled by
	// the click-time top contributor signal_id (or "s6_pin" for the S6 pin)
	// and whether the card was the pin (pinned="true"|"false").
	RecClickTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rec_click_total",
			Help: "Total clicks on cards in recs rows, labeled by signal_id and whether the card was the S6 pin (Phase 14, REC-EVAL-02).",
		},
		[]string{"signal_id", "pinned"},
	)

	// RecWatchedTotal counts auto-mark events for anime that originated from
	// a rec_click click within the last hour (correlated client-side via
	// localStorage.recentRecClicks). Anonymous events are valid.
	RecWatchedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rec_watched_total",
			Help: "Total auto-mark events for anime that originated from a rec_click click within the last hour, labeled by signal_id and pinned (Phase 14, REC-EVAL-02).",
		},
		[]string{"signal_id", "pinned"},
	)

	// RecsCronLastSuccessUnixtime records the wall-clock time (unix seconds) of
	// the most recent SUCCESSFUL tick for each recs cron. A frozen / hung cron
	// stops advancing this gauge, so `time() - recs_cron_last_success_unixtime`
	// in Grafana surfaces a stalled ticker. The `cron` label is the closed set
	// {population, user, co_occurrence} — bounded cardinality (audit L641).
	RecsCronLastSuccessUnixtime = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "recs_cron_last_success_unixtime",
			Help: "Unix timestamp (seconds) of the last successful recs cron tick, labeled by cron {population,user,co_occurrence} (audit L641).",
		},
		[]string{"cron"},
	)
)
