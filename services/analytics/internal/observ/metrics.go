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
	// EventsFlushFailed counts events/identities LOST because a store write
	// failed during flush, by op (insert|identity). Previously these failures
	// only logged, so a store outage silently dropped clickstream data with no
	// metric signal — alert on rate(analytics_events_flush_failed_total) > 0.
	EventsFlushFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_events_flush_failed_total",
		Help: "Events/identities lost because a store write failed during flush, by op.",
	}, []string{"op"})
	// FEErrorsReceived counts frontend errors accepted at
	// /api/analytics/client-errors, labelled by the whitelisted `kind`
	// (js/unhandledrejection/vue/http/player/suppressed/cap/other).
	FEErrorsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_fe_errors_total",
		Help: "Frontend errors accepted at /api/analytics/client-errors, by kind.",
	}, []string{"kind"})
)
