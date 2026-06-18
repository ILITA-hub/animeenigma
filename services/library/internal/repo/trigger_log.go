package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
)

// triggerLogRetention bounds the append-only autocache_trigger_log: rows older
// than this are pruned on insert so the cause→effect history self-trims without a
// separate cron. 90 days is generous for an operational dashboard view.
const triggerLogRetention = 90 * 24 * time.Hour

// TriggerLogRepository persists append-only autocache cause→effect records. One
// row per user-driven trigger fire (Logic B / backfill); the dashboard joins it to
// library_episodes/library_jobs by (mal_id, target_episode) to show the resulting
// download. Insert is best-effort from the caller's perspective (a logging miss
// never affects the demand itself).
type TriggerLogRepository struct {
	db *gorm.DB
}

// NewTriggerLogRepository constructs a TriggerLogRepository over the provided DB.
func NewTriggerLogRepository(db *gorm.DB) *TriggerLogRepository {
	return &TriggerLogRepository{db: db}
}

// Insert appends one trigger→effect row and opportunistically prunes rows beyond
// the retention window. ID is set explicitly (uuid) so the insert does not rely on
// the Postgres gen_random_uuid() default (keeps SQLite unit tests working).
// CreatedAt is set explicitly to now() rather than relying on the SQL DEFAULT:
// GORM only omits a zero-value column when it carries a magic name/tag, so without
// this the row would land a 0001-01-01 timestamp (the same nullable-default
// footgun the demand repo guards) and break the dashboard's recency ordering.
func (r *TriggerLogRepository) Insert(ctx context.Context, row *domain.AutocacheTriggerLog) error {
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "insert trigger log")
	}
	// Best-effort prune; a failure here is non-fatal (the row was already written).
	cutoff := time.Now().Add(-triggerLogRetention)
	_ = r.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&domain.AutocacheTriggerLog{}).Error
	return nil
}
