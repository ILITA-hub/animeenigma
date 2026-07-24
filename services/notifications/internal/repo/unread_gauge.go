package repo

import (
	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/gorm"
)

// UnreadGaugeRepository serves the active-unread COUNT(*) for the metrics
// goroutine (NOTIF-NF-01 → notifications_active_unread_gauge).
//
// Backed by the idx_user_unread partial index (tightened predicate):
//
//	CREATE INDEX idx_user_unread ON user_notifications (user_id, created_at DESC)
//	  WHERE dismissed_at IS NULL AND invalidated_at IS NULL AND deleted_at IS NULL;
//
// so the COUNT runs against a slim partial-index scan even with millions of
// historical-but-dismissed rows. Invalidated (tombstoned) AND user-deleted rows
// are excluded so the gauge stays consistent with the per-user badge counts
// shown in the UI.
type UnreadGaugeRepository struct {
	db *gorm.DB
}

// NewUnreadGaugeRepository constructs the repo.
func NewUnreadGaugeRepository(db *gorm.DB) *UnreadGaugeRepository {
	return &UnreadGaugeRepository{db: db}
}

// ActiveUnreadCount returns the total active + unread notification count
// across all users.
func (r *UnreadGaugeRepository) ActiveUnreadCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("dismissed_at IS NULL AND read_at IS NULL AND invalidated_at IS NULL AND deleted_at IS NULL").
		Count(&n).Error
	if err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "active unread count")
	}
	return n, nil
}
