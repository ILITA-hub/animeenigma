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

	// ProbeProviderStatus is an info-style gauge (value always 1) carrying the
	// per-provider rollup verdict as labels, so the playback dashboard table can
	// render Provider | Status | Reason directly. Reset() each run (the probe
	// reports the COMPLETE provider set each run) to avoid stale label series.
	ProbeProviderStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_provider_status",
		Help: "Per-provider playability rollup as labels (value always 1).",
	}, []string{"provider", "status", "reason"})

	// ProbeSubtitleProviderUp is the ACTIVE subtitle probe verdict per provider:
	// 1 up, 0.5 degraded, 0 down. Distinct from the passive
	// catalog_subtitle_provider_up (which is driven by real resolve traffic).
	// Reset() each run so a provider that drops out of the probe set is not left
	// with a stale series.
	ProbeSubtitleProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_subtitle_provider_up",
		Help: "Active subtitle-probe verdict per provider: 1 up, 0.5 degraded, 0 down.",
	}, []string{"provider"})

	// ProbeSubtitleLatencySeconds is the last active-probe ping latency per provider.
	ProbeSubtitleLatencySeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_subtitle_latency_seconds",
		Help: "Last active subtitle-probe ping latency per provider, in seconds.",
	}, []string{"provider"})

	// ProbeSubtitleLastRun is the unix timestamp of the last completed subtitle probe run.
	ProbeSubtitleLastRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "probe_subtitle_last_run_timestamp",
		Help: "Unix timestamp of the last completed active subtitle probe run.",
	})
)
