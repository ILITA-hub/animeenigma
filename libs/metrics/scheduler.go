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
)
