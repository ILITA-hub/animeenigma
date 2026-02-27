package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DBPoolOpenConnections tracks the number of open database connections.
	DBPoolOpenConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_pool_open_connections",
			Help: "Number of open database connections",
		},
	)

	// DBPoolIdleConnections tracks the number of idle database connections.
	DBPoolIdleConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_pool_idle_connections",
			Help: "Number of idle database connections",
		},
	)

	// DBPoolWaitTotal tracks total number of connections waited for.
	DBPoolWaitTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "db_pool_wait_total",
			Help: "Total number of connections waited for",
		},
	)

	// DBPoolWaitDurationTotal tracks total time spent waiting for connections.
	DBPoolWaitDurationTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "db_pool_wait_duration_seconds_total",
			Help: "Total time spent waiting for database connections in seconds",
		},
	)
)
