package transport

import "github.com/prometheus/client_golang/prometheus"

var webhooksReceived = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "maintenance_grafana_webhooks_received_total",
		Help: "Grafana alert webhook deliveries processed by status",
	},
	[]string{"status"},
)

var webhookErrors = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "maintenance_grafana_webhook_errors_total",
		Help: "Grafana alert webhook errors by reason",
	},
	[]string{"reason"},
)

func init() {
	prometheus.MustRegister(webhooksReceived, webhookErrors)
}
