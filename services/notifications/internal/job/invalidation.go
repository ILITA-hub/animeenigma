package job

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"gorm.io/gorm"
)

// RelevanceInvalidationJob stamps invalidated_at on active new_episode
// notifications that are no longer relevant — the user is no longer
// 'watching' the anime, or has caught up to the advertised latest episode
// (anime-level, any combo). Runs hourly right after the detector (so it sees
// freshly upserted rows) via Scheduler.
//
// The read path (repo.List / repo.UnreadCount) already hides these between
// runs using the same predicate; this job persists the state so storage is
// reclaimable (retention cleanup) and the hot-read partial index stays tight.
//
// Idempotent: invalidated_at IS NULL in the WHERE means already-tombstoned
// rows are skipped.
type RelevanceInvalidationJob struct {
	db  *gorm.DB
	log *logger.Logger
}

// NewRelevanceInvalidationJob constructs the job.
func NewRelevanceInvalidationJob(db *gorm.DB, log *logger.Logger) *RelevanceInvalidationJob {
	return &RelevanceInvalidationJob{db: db, log: log}
}

// Run executes the single UPDATE. Returns rows-affected for logging/metrics/
// the (future) admin endpoint.
func (j *RelevanceInvalidationJob) Run(ctx context.Context) (int64, error) {
	q := `UPDATE user_notifications SET invalidated_at = ?
	      WHERE dismissed_at IS NULL
	        AND invalidated_at IS NULL
	        AND ` + repo.NotRelevantClause()

	res := j.db.WithContext(ctx).Exec(q, time.Now().UTC())
	if res.Error != nil {
		return 0, apperrors.Wrap(res.Error, apperrors.CodeInternal, "relevance invalidation update")
	}
	if res.RowsAffected > 0 {
		NotificationsStaleInvalidatedTotal.Add(float64(res.RowsAffected))
	}
	if j.log != nil {
		j.log.Infow("relevance invalidation completed", "invalidated", res.RowsAffected)
	}
	return res.RowsAffected, nil
}
