package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ProxyActiveConnections tracks the number of active proxy connections.
	ProxyActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "proxy_active_connections",
			Help: "Number of active proxy connections",
		},
	)

	// ProxyRequestsTotal counts proxy requests by type.
	ProxyRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_requests_total",
			Help: "Total number of proxy requests",
		},
		[]string{"type"},
	)

	// ProxyUpstreamErrors counts upstream errors by status code and domain.
	ProxyUpstreamErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_upstream_errors_total",
			Help: "Total number of upstream proxy errors (non-2xx responses)",
		},
		[]string{"status", "domain"},
	)

	// SubtitleRequestsTotal counts subtitle fetch requests by source and status.
	SubtitleRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subtitle_requests_total",
			Help: "Total number of subtitle fetch requests",
		},
		[]string{"source", "status"},
	)
)
