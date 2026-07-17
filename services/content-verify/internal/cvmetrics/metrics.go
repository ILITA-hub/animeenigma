// Package cvmetrics holds content-verify's service-local Prometheus metrics.
// Deliberately NOT in libs/metrics: plain promauto metrics auto-register in
// every binary importing the shared package and would export permanent-0
// impostor series from ~20 services.
package cvmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_queue_depth", Help: "Candidates with a non-zero score at the last snapshot.",
	})
	ProbesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_probes_total", Help: "Unit probes by provider and result.",
	}, []string{"provider", "result"}) // result: verified|inconclusive|unreachable|error|synth
	ProbeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "content_verify_probe_duration_seconds",
		Help:    "Wall time of one unit probe (resolve→extract→analyze).",
		Buckets: []float64{5, 10, 20, 30, 40, 50, 60, 90, 120},
	})
	TicksSkippedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_ticks_skipped_total", Help: "Worker ticks that did no probe.",
	}, []string{"reason"}) // reason: degraded|idle|claim_error
	LastProbeTS = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_last_probe_timestamp", Help: "Unix time of the last completed probe.",
	})
	SkipProbesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_skip_probes_total", Help: "Skip-lane (OP/ED) probes by provider and result.",
	}, []string{"provider", "result"}) // result: detected|no_match|pending_fp|unreachable
)
