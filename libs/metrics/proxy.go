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

	// ProxyEdgeRotations counts solodcdn edge-rotation attempts (AUTO-562,
	// Layer A of the playback self-healing design): when a p<N>.solodcdn.com
	// edge answers >=500, the HLS proxy retries the identical path on a sibling
	// edge. Labeled by the failed edge (from), the sibling tried (to), and the
	// outcome ("success" = sibling served <400; "fail" = sibling also >=400;
	// "error" = transport error reaching the sibling). As a CounterVec it emits
	// no series until first use, so it stays absent from services that never
	// rotate (only the streaming service does).
	ProxyEdgeRotations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_edge_rotations_total",
			Help: "Total solodcdn edge-rotation attempts by from-edge, to-edge, and outcome",
		},
		[]string{"from", "to", "outcome"},
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
