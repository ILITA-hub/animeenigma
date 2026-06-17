package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SchedulerJobExecutionsTotal counts scheduler job executions by job and status.
	SchedulerJobExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scheduler_job_executions_total",
			Help: "Total number of scheduler job executions",
		},
		[]string{"job", "status"},
	)

	// SchedulerJobDuration tracks scheduler job duration.
	SchedulerJobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "scheduler_job_duration_seconds",
			Help:    "Scheduler job execution duration in seconds",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"job"},
	)

	// SchedulerJobLastSuccess tracks the timestamp of the last successful job execution.
	SchedulerJobLastSuccess = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scheduler_job_last_success_timestamp",
			Help: "Unix timestamp of last successful scheduler job execution",
		},
		[]string{"job"},
	)

	// AutocachePredictedBytes is the Phase-11 (OBS-05) daily storage-need heuristic.
	// It carries the `library_autocache_` prefix DELIBERATELY — even though it is
	// emitted by the scheduler (the shared-DB owner that can run the Logic A watcher
	// join), not the library — so OBS-05's Grafana table can union it with the
	// library-exposed library_autocache_budget_bytes in a single query. Cardinality
	// is {component}-only (exactly 2 series: ongoing, nextep); it MUST NEVER be
	// labelled per-anime (CONTEXT pitfall — a per-anime breakdown is deferred to v2).
	AutocachePredictedBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "library_autocache_predicted_bytes",
			Help: "Predicted RAW-pool bytes needed per heuristic component (ongoing|nextep). Emitted by the scheduler so OBS-05 can join it with the library-exposed library_autocache_budget_bytes.",
		},
		[]string{"component"},
	)
)
