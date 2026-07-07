package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CapabilityPlayabilityPromotionsTotal counts per-title promotions of a
	// degraded provider to active/selectable (Phase B B3/B4).
	CapabilityPlayabilityPromotionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "capability_playability_promotions_total",
		Help: "Total per-title promotions of a degraded provider to selectable via playability index",
	})

	// CapabilityPlayabilityFetchFailuresTotal counts best-effort analytics
	// playability-scores fetches that failed (fed a health-only index).
	CapabilityPlayabilityFetchFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "capability_playability_fetch_failures_total",
		Help: "Total failed analytics playability-scores fetches (index fell back to health-only)",
	})
)
