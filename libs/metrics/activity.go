package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SearchRequestsTotal counts search requests by source.
	SearchRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "search_requests_total",
			Help: "Total number of anime search requests",
		},
		[]string{"source"},
	)

	// EpisodeStreamRequestsTotal counts episode stream requests by provider.
	EpisodeStreamRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "episode_stream_requests_total",
			Help: "Total number of episode stream requests",
		},
		[]string{"provider"},
	)

	// WatchProgressSavesTotal counts watch progress save operations.
	WatchProgressSavesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "watch_progress_saves_total",
			Help: "Total number of watch progress saves",
		},
	)

	// WatchlistOperationsTotal counts watchlist operations by type.
	WatchlistOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watchlist_operations_total",
			Help: "Total number of watchlist operations",
		},
		[]string{"operation"},
	)
)
