package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WebSocketConnectionsActive tracks the number of active WebSocket connections.
	WebSocketConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	// WebSocketMessagesTotal counts WebSocket messages by direction.
	WebSocketMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_messages_total",
			Help: "Total number of WebSocket messages",
		},
		[]string{"direction"},
	)
)
