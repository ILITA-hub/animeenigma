package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SyncJobsStartedTotal counts sync jobs started by source.
	SyncJobsStartedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_jobs_started_total",
			Help: "Total number of sync jobs started",
		},
		[]string{"source"},
	)

	// SyncJobsTotal counts completed sync jobs by source and status.
	SyncJobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_jobs_total",
			Help: "Total number of completed sync jobs",
		},
		[]string{"source", "status"},
	)

	// SyncJobDurationSeconds tracks duration of sync jobs by source.
	SyncJobDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sync_job_duration_seconds",
			Help:    "Sync job duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"source"},
	)

	// SyncJobEntriesTotal counts individual entries processed by source and result.
	SyncJobEntriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_job_entries_total",
			Help: "Total number of individual entries processed during sync",
		},
		[]string{"source", "result"},
	)
)
