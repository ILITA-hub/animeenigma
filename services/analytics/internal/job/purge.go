// Package job holds the analytics background cron jobs.
package job

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Purger deletes events older than a cutoff (implemented by repo).
type Purger interface {
	PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// PurgeJob enforces the retention window (spec §5). Runs in-service (like
// the notifications cleanup cron) rather than the scheduler service, to
// avoid cross-service coupling.
type PurgeJob struct {
	purger        Purger
	retentionDays int
	log           *logger.Logger
}

func NewPurgeJob(p Purger, retentionDays int, log *logger.Logger) *PurgeJob {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	return &PurgeJob{purger: p, retentionDays: retentionDays, log: log}
}

// RunOnce executes a single purge pass and returns the rows deleted.
func (j *PurgeJob) RunOnce(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(j.retentionDays) * 24 * time.Hour)
	n, err := j.purger.PurgeOlderThan(ctx, cutoff)
	if err != nil {
		if j.log != nil {
			j.log.Errorw("analytics purge failed", "error", err)
		}
		return 0, err
	}
	if j.log != nil {
		j.log.Infow("analytics purge complete", "deleted", n, "cutoff", cutoff)
	}
	return n, nil
}
