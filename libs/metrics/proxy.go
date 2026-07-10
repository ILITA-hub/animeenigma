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

	// ProxyEdgeAttemptSeconds is the latency of EVERY solodcdn edge attempt
	// (including the nominal one), labeled by edge and outcome ("ok", "http4xx",
	// "http5xx", "dial_error", "timeout"). It exposes the METRICS behind edge
	// selection — e.g. how long a cold edge took before answering, or how long a
	// timeout burned before rotating. Buckets stretch to 60s to capture the 45s
	// response-header window. HistogramVec => no series until first use.
	ProxyEdgeAttemptSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "proxy_edge_attempt_seconds",
			Help:    "Latency of each solodcdn edge attempt by edge and outcome",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 45, 60},
		},
		[]string{"edge", "outcome"},
	)

	// ProxyEdgeSelected counts which solodcdn edge ultimately served a playable
	// (<400) response, labeled by edge — the DECISION, for "what share of traffic
	// each edge carries". CounterVec => no series until first use.
	ProxyEdgeSelected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_edge_selected_total",
			Help: "Total playable responses served per solodcdn edge",
		},
		[]string{"edge"},
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
