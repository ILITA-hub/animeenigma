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
	// FEErrorsReceived counts frontend errors accepted at
	// /api/analytics/client-errors, labelled by the whitelisted `kind`
	// (js/unhandledrejection/vue/http/player/suppressed/cap/other).
	FEErrorsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_fe_errors_total",
		Help: "Frontend errors accepted at /api/analytics/client-errors, by kind.",
	}, []string{"kind"})
)
