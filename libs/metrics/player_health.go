package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// PlayerHealthUp indicates whether a player/parser is reachable (1=up, 0=down).
	PlayerHealthUp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "player_health_up",
			Help: "Whether a player source is reachable (1=up, 0=down)",
		},
		[]string{"player"},
	)

	// PlayerHealthCheckDuration tracks health check latency per player.
	PlayerHealthCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "player_health_check_duration_seconds",
			Help:    "Player health check duration in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"player"},
	)

	// PlayerHealthLastCheck tracks the timestamp of the last health check per player.
	PlayerHealthLastCheck = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "player_health_last_check_timestamp",
			Help: "Unix timestamp of last player health check",
		},
		[]string{"player"},
	)
)
