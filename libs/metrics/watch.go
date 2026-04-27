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

	// ComboOverrideTotal tracks user-initiated combo changes within 30s of player load.
	// One increment per (load_session_id, dimension) at most — frontend composable enforces.
	// Cardinality budget: 6 (tier) × 4 (dimension) × 2 (language) × 2 (anon) × 4 (player) = 384 series.
	// See .planning/phases/01-instrumentation-baseline/01-RESEARCH.md §Pattern 3 + §Pitfall 3.
	ComboOverrideTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "combo_override_total",
			Help: "User overrides of auto-picked combo within 30s of player load",
		},
		[]string{"tier", "dimension", "language", "anon", "player"},
	)

	// ComboResolveTotal is the rate denominator for ComboOverrideTotal.
	// Incremented from the resolver service (services/player/internal/service/preference.go) on
	// every successful resolve outcome — labels match ComboOverrideTotal except no `dimension`.
	// PromQL: rate(combo_override_total[5m]) / rate(combo_resolve_total[5m]).
	ComboResolveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "combo_resolve_total",
			Help: "Successful preference resolution outcomes",
		},
		[]string{"tier", "language", "anon", "player"},
	)
)
