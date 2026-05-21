package job

import (
	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

// DismissedRetentionCleanupJob deletes user_notifications rows whose
// dismissed_at is older than `retentionDays` (NOTIF-DET-09, default 30).
//
// Single DELETE statement, parameterised on retentionDays to avoid SQL
// injection. Runs nightly via Scheduler ("30 3 * * *") plus on-demand via
// POST /internal/cleanup/run-once.
//
// Idempotent: an empty match set returns 0 deletions without error.
type DismissedRetentionCleanupJob struct {
	db            *gorm.DB
	retentionDays int
	log           *logger.Logger
}

// NewDismissedRetentionCleanupJob constructs the job. retentionDays must be
// positive; 0 or negative falls back to 30.
func NewDismissedRetentionCleanupJob(db *gorm.DB, retentionDays int, log *logger.Logger) *DismissedRetentionCleanupJob {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	return &DismissedRetentionCleanupJob{db: db, retentionDays: retentionDays, log: log}
}

// Run executes the DELETE. Returns the rows-affected count for logging /
// metrics / the admin endpoint's JSON response.
//
// IMPORTANT: pgx (the Postgres driver this service uses) refuses to encode
// an int parameter into a text-shaped slot, which broke an earlier
// `(? || ' days')::interval` formulation. The `INTERVAL '1 day' * N` form
// keeps the parameter as a plain integer so pgx is happy AND the math is
// done server-side. Caught during SC6 of the Phase 2 verification gauntlet.
func (j *DismissedRetentionCleanupJob) Run(ctx context.Context) (int64, error) {
	const q = `
		DELETE FROM user_notifications
		WHERE dismissed_at IS NOT NULL
		  AND dismissed_at < NOW() - (INTERVAL '1 day' * ?)
	`
	res := j.db.WithContext(ctx).Exec(q, j.retentionDays)
	if res.Error != nil {
		return 0, apperrors.Wrap(res.Error, apperrors.CodeInternal, "retention cleanup delete")
	}
	if j.log != nil {
		j.log.Infow("cleanup retention completed",
			"deleted", res.RowsAffected,
			"retention_days", j.retentionDays,
		)
	}
	return res.RowsAffected, nil
}
