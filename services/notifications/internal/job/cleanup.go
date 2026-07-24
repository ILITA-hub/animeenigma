package job

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

// DismissedRetentionCleanupJob deletes user_notifications rows whose
// dismissed_at, invalidated_at, or deleted_at is older than `retentionDays`
// (NOTIF-DET-09, default 30). Reaps rows retired by the user (dismissed_at =
// cleared from the bell; deleted_at = binned from history) or by the hourly
// RelevanceInvalidationJob (invalidated_at).
//
// Single DELETE statement with a Go-computed cutoff timestamp (portable
// across Postgres and SQLite; removes the old pgx INTERVAL-encoding
// workaround). Runs nightly via Scheduler ("30 3 * * *") plus on-demand
// via POST /internal/cleanup/run-once.
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
// The cutoff is computed in Go (portable across Postgres + SQLite; also
// retires the old pgx INTERVAL-encoding workaround). Reaps rows retired
// either by the user (dismissed_at) or by the relevance invalidation job
// (invalidated_at).
func (j *DismissedRetentionCleanupJob) Run(ctx context.Context) (int64, error) {
	// Go-computed cutoff (portable across Postgres + SQLite; also retires the
	// old pgx INTERVAL-encoding workaround). Reaps rows retired by the user
	// (dismissed_at / deleted_at) or by the relevance invalidation job
	// (invalidated_at).
	cutoff := time.Now().UTC().AddDate(0, 0, -j.retentionDays)
	const q = `
		DELETE FROM user_notifications
		WHERE (dismissed_at   IS NOT NULL AND dismissed_at   < ?)
		   OR (invalidated_at IS NOT NULL AND invalidated_at < ?)
		   OR (deleted_at      IS NOT NULL AND deleted_at      < ?)
	`
	res := j.db.WithContext(ctx).Exec(q, cutoff, cutoff, cutoff)
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
