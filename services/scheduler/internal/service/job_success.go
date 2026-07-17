package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
)

// KnownJobs lists every job name this binary records success for. Startup
// seeding filters persisted rows through it so a renamed/removed job cannot
// resurrect a stale gauge series that would age past the scheduler-sync-stale
// 25h threshold forever.
var KnownJobs = []string{
	"shikimori_sync",
	"cleanup",
	"top_anime_sync",
	"calendar_sync",
	"announcements_sync",
	"playback_probe",
	"read_threshold_recompute",
	"provider_ranking_recompute",
	"subtitle_probe",
	"autocache_logic_a",
	"autocache_prediction",
	"fanfic_daily",
}

// successStore is the narrow persistence surface recordSuccess writes to.
// Nil-safe by contract: an unwired store only skips persistence.
type successStore interface {
	Upsert(ctx context.Context, job string, at time.Time) error
}

// SetSuccessStore wires the job-success persistence repository.
func (s *JobService) SetSuccessStore(st successStore) {
	s.successStore = st
}

// recordSuccess refreshes the last-success freshness gauge and persists the
// timestamp so SeedLastSuccess can restore the series after a restart. One
// clock read feeds both, so the gauge, the DB row, and a post-restart seed
// all agree. Persistence failure is non-fatal: the gauge (the alerting
// signal) is already set; only restart continuity degrades.
func (s *JobService) recordSuccess(ctx context.Context, job string) {
	now := time.Now()
	metrics.SchedulerJobLastSuccess.WithLabelValues(job).Set(float64(now.Unix()))
	if s.successStore == nil {
		return
	}
	if err := s.successStore.Upsert(ctx, job, now); err != nil {
		s.log.Warnw("failed to persist job success timestamp", "job", job, "error", err)
	}
}

// SeedLastSuccess primes scheduler_job_last_success_timestamp from persisted
// rows at startup. Without it the in-memory gauge starts empty after every
// restart and the scheduler-sync-stale alert (noDataState: Alerting) paged P0
// on the transient no-data window (AUTO-610/611). Returns the number of
// series seeded.
func SeedLastSuccess(rows []domain.JobSuccess, log *logger.Logger) int {
	known := make(map[string]bool, len(KnownJobs))
	for _, j := range KnownJobs {
		known[j] = true
	}
	seeded := 0
	for _, row := range rows {
		if !known[row.Job] {
			log.Infow("skipping persisted last-success for unknown job", "job", row.Job)
			continue
		}
		metrics.SchedulerJobLastSuccess.WithLabelValues(row.Job).Set(float64(row.LastSuccessAt.Unix()))
		seeded++
	}
	return seeded
}
