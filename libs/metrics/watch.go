package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WatchEpisodesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watch_episodes_total",
			Help: "Total episodes marked as watched",
		},
		[]string{"player", "language", "watch_type"},
	)

	WatchActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "watch_active_sessions",
			Help: "Number of currently active watch sessions",
		},
	)

	WatchSessionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "watch_session_duration_seconds",
			Help:    "Duration of watch sessions in seconds",
			Buckets: prometheus.ExponentialBuckets(60, 2, 10), // 1min to ~17hrs
		},
		[]string{"player", "language"},
	)

	TranslationSelectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "translation_selections_total",
			Help: "Total translation selections by users",
		},
		[]string{"player", "language", "watch_type", "translation_title"},
	)

	PreferenceResolutionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "preference_resolution_total",
			Help: "Total preference resolution outcomes by tier",
		},
		[]string{"tier"},
	)

	PreferenceFallbackTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "preference_fallback_total",
			Help: "Total preference fallback triggers by tier and context",
		},
		[]string{"tier", "language", "watch_type"},
	)
)
