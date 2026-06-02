// Package observ holds analytics Prometheus metrics, auto-registered to the
// default registry that /metrics serves.
package observ

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_events_received_total",
		Help: "Clickstream events accepted at /collect.",
	})
	EventsDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_events_dropped_total",
		Help: "Clickstream events dropped because the in-process buffer was full.",
	})
)
