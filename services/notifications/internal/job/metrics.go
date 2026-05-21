package job

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Phase 2 v1.0 Notifications Engine — NOTIF-NF-01. Six Prometheus series the
// detector + cleanup + unread-gauge poller publish. Names + labels MUST match
// REQUIREMENTS.md exactly — Grafana dashboards in v1.1 will query these
// literal names.
//
// `promauto` registers into the default registry so they appear at
// :8090/metrics without explicit MustRegister calls. Pattern matches
// libs/metrics/scheduler.go.
var (
	// NotificationsCreatedTotal counts notifications successfully UPSERTed
	// by Phase 2's detector (and any future producer). Labels:
	//   type     — notification type (v1.0 only "new_episode")
	//   producer — "detector" | "<future-producer>"
	NotificationsCreatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_created_total",
			Help: "Notifications successfully UPSERTed (Phase 2 detector + future producers).",
		},
		[]string{"type", "producer"},
	)

	// NotificationsDetectorRunsTotal counts detector cron-tick or manual
	// run outcomes. Labels:
	//   outcome — "success" | "partial" | "failed"
	NotificationsDetectorRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_detector_runs_total",
			Help: "Detector run count, labelled by outcome (success|partial|failed).",
		},
		[]string{"outcome"},
	)

	// NotificationsDetectorDurationSeconds tracks per-run wall-clock
	// duration. Buckets match libs/metrics/scheduler.go for cross-service
	// Grafana consistency.
	NotificationsDetectorDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "notifications_detector_duration_seconds",
			Help:    "Detector run duration in seconds.",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
	)

	// NotificationsDetectorCombosScanned is the gauge of "active hot
	// combos" the most recent detector run observed (DISTINCT join across
	// watch_history + anime_list + animes filtered to status=watching/
	// ongoing). Useful for capacity planning.
	NotificationsDetectorCombosScanned = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "notifications_detector_combos_scanned",
			Help: "Number of active hot combos observed by the latest detector run.",
		},
	)

	// NotificationsDetectorParserFailuresTotal counts per-combo parser
	// failures (any non-NotFound error from the catalog HTTP client).
	// Labels:
	//   player — kodik | animelib | <future>
	NotificationsDetectorParserFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_detector_parser_failures_total",
			Help: "Per-combo parser failures encountered by the detector, labelled by player.",
		},
		[]string{"player"},
	)

	// NotificationsActiveUnreadGauge tracks the live count of active +
	// unread notifications across all users. Polled every 5 minutes by
	// the unread-gauge goroutine (NOTIF-NF-01 + design-doc §Telemetry).
	NotificationsActiveUnreadGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "notifications_active_unread_gauge",
			Help: "Active + unread notification rows across all users (polled every 5m).",
		},
	)
)
