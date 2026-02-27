package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// AuthEventsTotal counts authentication events by event type and status.
	AuthEventsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_events_total",
			Help: "Total number of authentication events",
		},
		[]string{"event", "status"},
	)
)
