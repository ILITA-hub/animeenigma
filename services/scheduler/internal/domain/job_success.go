package domain

import "time"

// JobSuccess persists the last successful run per scheduler job. The
// scheduler_job_last_success_timestamp gauge is in-memory, so every container
// restart used to wipe it and the scheduler-sync-stale alert (noDataState:
// Alerting) paged P0 on the transient no-data window (AUTO-610/611). Rows are
// loaded at startup to re-seed the gauge, keeping the staleness clock honest
// across restarts.
type JobSuccess struct {
	Job           string    `gorm:"primaryKey;size:64" json:"job"`
	LastSuccessAt time.Time `json:"last_success_at"`
}
