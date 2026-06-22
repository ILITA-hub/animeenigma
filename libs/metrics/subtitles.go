package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SubtitleResolveTotal counts subtitle aggregation resolves per provider+status.
	// status ∈ {"ok","down","empty","unconfigured"}.
	SubtitleResolveTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "catalog_subtitle_resolve_total",
		Help: "Subtitle aggregation resolves by provider and status.",
	}, []string{"provider", "status"})

	// SubtitleResolveDuration is the wall time of a full (non-cached) FetchAll.
	SubtitleResolveDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "catalog_subtitle_resolve_duration_seconds",
		Help:    "Wall time of a non-cached subtitle aggregation resolve.",
		Buckets: prometheus.DefBuckets,
	})

	// SubtitleProviderUp is 1 when the provider answered, 0 when it failed.
	// Unconfigured providers emit NO series (see RecordSubtitleResolve).
	SubtitleProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "catalog_subtitle_provider_up",
		Help: "1 if the subtitle provider answered the last resolve, else 0.",
	}, []string{"provider"})

	// SubtitleTracksReturned counts subtitle tracks merged, per provider.
	SubtitleTracksReturned = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "catalog_subtitle_tracks_returned_total",
		Help: "Subtitle tracks returned by each provider.",
	}, []string{"provider"})
)

// SubtitleProviderOutcome is one provider's result within a single resolve.
type SubtitleProviderOutcome struct {
	Provider string
	Status   string // "ok" | "down" | "empty" | "unconfigured"
	Tracks   int
}

// RecordSubtitleResolve emits metrics for one non-cached FetchAll.
func RecordSubtitleResolve(durationSeconds float64, outcomes []SubtitleProviderOutcome) {
	SubtitleResolveDuration.Observe(durationSeconds)
	for _, o := range outcomes {
		SubtitleResolveTotal.WithLabelValues(o.Provider, o.Status).Inc()
		if o.Tracks > 0 {
			SubtitleTracksReturned.WithLabelValues(o.Provider).Add(float64(o.Tracks))
		}
		switch o.Status {
		case "unconfigured":
			// no gauge — provider is intentionally off, not "down"
		case "down":
			SubtitleProviderUp.WithLabelValues(o.Provider).Set(0)
		default: // ok | empty — provider answered
			SubtitleProviderUp.WithLabelValues(o.Provider).Set(1)
		}
	}
}
