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
)

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
