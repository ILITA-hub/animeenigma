package metrics

import "github.com/prometheus/client_golang/prometheus/promauto"
import "github.com/prometheus/client_golang/prometheus"

var (
	ProbeProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_provider_up",
		Help: "Per-provider playability verdict: 1 up, 0.5 degraded, 0 down.",
	}, []string{"provider"})

	ProbeRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "probe_runs_total",
		Help: "Playability probe results per (provider, slot, server, result, reason).",
	}, []string{"provider", "slot", "server", "result", "reason"})

	ProbeLastRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "probe_last_run_timestamp",
		Help: "Unix timestamp of the last completed probe run.",
	})
)
