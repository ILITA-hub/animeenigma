package repo

import (
	stderrors "errors"
	"time"

	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Default + cap on the list endpoint.
const (
	defaultListLimit = 20
	maxListLimit     = 100
)

// NotificationRepository wraps all user_notifications DB access.
// One *gorm.DB handle; v1.0 has no per-request transactional needs because
// every operation is either a single statement or a tightly-scoped pair
// (list rows + count) that does not need transactional consistency.
type NotificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository constructs the repo.
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// ListStatus filters the List response. Only "unread" and "all" are
// supported in v1.0 — the design doc reserves "dismissed" for a future
// "history" tab.
type ListStatus string

const (
	// ListUnread returns active (not dismissed) AND unread.
	ListUnread ListStatus = "unread"
	// ListAll returns active (not dismissed); read state is irrelevant.
	ListAll ListStatus = "all"
)

// List returns up to `limit` notifications for `userID`, filtered by
// `status`, ordered by `created_at DESC`. Also returns the absolute counts
// (unread + total active) the bell badge needs without a second round-trip.
//
// All queries scope to user_id from the caller's JWT claims; no leak path
// exists for cross-user reads.
func (r *NotificationRepository) List(
	ctx context.Context,
	userID string,
	status ListStatus,
	limit, offset int,
) (rows []domain.UserNotification, unreadCount int64, total int64, err error) {
	if limit <= 0 {
		limit = defaultListLimit
	} else if limit > maxListLimit {
		limit = maxListLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Base query: active, non-invalidated rows for this user, filtered to
	// still-relevant new_episode rows (other types pass through).
	base := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("user_id = ? AND dismissed_at IS NULL AND invalidated_at IS NULL", userID).
		Where(relevanceReadClause())

	// Branch on status — the predicate is additive on top of `base`.
	rowsQuery := base.Session(&gorm.Session{})
	if status == ListUnread {
		rowsQuery = rowsQuery.Where("read_at IS NULL")
	}

	if err := rowsQuery.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, 0, apperrors.Wrap(err, apperrors.CodeInternal, "list notifications")
	}

	// Absolute "total active" — always counted off the un-filtered base
	// so the UI can show "X total, Y unread" regardless of the current
	// filter.
	if err := base.Session(&gorm.Session{}).
		Count(&total).Error; err != nil {
		return nil, 0, 0, apperrors.Wrap(err, apperrors.CodeInternal, "count notifications total")
	}

	// Unread count is also relative to the active set.
	if err := base.Session(&gorm.Session{}).
		Where("read_at IS NULL").
		Count(&unreadCount).Error; err != nil {
		return nil, 0, 0, apperrors.Wrap(err, apperrors.CodeInternal, "count notifications unread")
	}

	return rows, unreadCount, total, nil
}

// Get returns a single notification by id, scoped to user.
// Returns errors.NotFound (404) if no row exists for this id+user; this
// shape avoids id-enumeration leaks (cross-user reads look identical to
// "doesn't exist").
func (r *NotificationRepository) Get(
	ctx context.Context,
	userID, id string,
) (*domain.UserNotification, error) {
	var n domain.UserNotification
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&n).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("notification")
	}
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "get notification")
	}
	return &n, nil
}

// UnreadCount returns the count of active+unread notifications for a user.
// Cheap — backed by idx_user_unread partial index.
// Applies the same relevance + invalidated_at filter as List so the bell
// badge count always matches the dropdown list.
func (r *NotificationRepository) UnreadCount(
	ctx context.Context,
	userID string,
) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("user_id = ? AND read_at IS NULL AND dismissed_at IS NULL AND invalidated_at IS NULL", userID).
		Where(relevanceReadClause()).
		Count(&n).Error; err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "count unread notifications")
	}
	return n, nil
}

// MarkRead sets read_at on a single notification. No-op on rows already
// read (the WHERE clause filters them out). Returns errors.NotFound if no
// row exists for this id+user (or if the row was already read — same
// "looks like not-found" shape, by design).
func (r *NotificationRepository) MarkRead(
	ctx context.Context,
	userID, id string,
) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", id, userID).
		Update("read_at", time.Now())
	if res.Error != nil {
		return apperrors.Wrap(res.Error, apperrors.CodeInternal, "mark notification read")
	}
	if res.RowsAffected == 0 {
		// Disambiguate "doesn't exist for this user" from "already read":
		// only return NotFound when the row truly doesn't exist for this
		// user. If it exists but was already read, this is a no-op success.
		if _, err := r.Get(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}

// MarkAllRead bulk-sets read_at on every active+unread row for a user.
// Returns the count of rows updated so the caller can reply with
// `{updated: N}`.
func (r *NotificationRepository) MarkAllRead(
	ctx context.Context,
	userID string,
) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("user_id = ? AND read_at IS NULL AND dismissed_at IS NULL", userID).
		Update("read_at", time.Now())
	if res.Error != nil {
		return 0, apperrors.Wrap(res.Error, apperrors.CodeInternal, "mark all notifications read")
	}
	return res.RowsAffected, nil
}

// Dismiss soft-removes a notification from the user's active set by
// stamping dismissed_at. Once dismissed, the partial uk_user_dedupe index
// no longer covers this row, so a future Upsert with the same dedupe_key
// is free to insert a fresh row.
func (r *NotificationRepository) Dismiss(
	ctx context.Context,
	userID, id string,
) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("id = ? AND user_id = ? AND dismissed_at IS NULL", id, userID).
		Update("dismissed_at", time.Now())
	if res.Error != nil {
		return apperrors.Wrap(res.Error, apperrors.CodeInternal, "dismiss notification")
	}
	if res.RowsAffected == 0 {
		// Same disambiguation pattern as MarkRead — distinguishes truly
		// missing rows from already-dismissed ones (success no-op).
		if _, err := r.Get(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}

// Click sets clicked_at to NOW the first time a notification is clicked.
// Subsequent clicks are no-ops (the WHERE clause filters them out).
func (r *NotificationRepository) Click(
	ctx context.Context,
	userID, id string,
) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("id = ? AND user_id = ? AND clicked_at IS NULL", id, userID).
		Update("clicked_at", time.Now())
	if res.Error != nil {
		return apperrors.Wrap(res.Error, apperrors.CodeInternal, "click notification")
	}
	if res.RowsAffected == 0 {
		if _, err := r.Get(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}

// Upsert is the producer path: insert a fresh notification, or update the
// existing active one for the same (user_id, dedupe_key). Matches the
// partial uk_user_dedupe index — dismissed rows do not block a new insert.
//
// On UPDATE the payload is replaced AND read_at is cleared so the user
// sees a fresh unread notification when the underlying state changes
// (e.g. episode 14 → 15 for the same combo).
//
// Returns the resulting row (re-fetched after the UPSERT so the caller
// gets the canonical state).
func (r *NotificationRepository) Upsert(
	ctx context.Context,
	userID, ntype, dedupeKey string,
	payload []byte,
) (*domain.UserNotification, error) {
	now := time.Now()

	row := &domain.UserNotification{
		UserID:    userID,
		Type:      ntype,
		DedupeKey: dedupeKey,
		Payload:   datatypes.JSON(payload),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// ON CONFLICT (user_id, dedupe_key) WHERE dismissed_at IS NULL DO UPDATE
	// matches the partial uk_user_dedupe index. Postgres needs
	// `TargetWhere` so the conflict resolver picks the same index.
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "dedupe_key"},
		},
		TargetWhere: clause.Where{
			Exprs: []clause.Expression{
				gorm.Expr("dismissed_at IS NULL"),
			},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"payload":    datatypes.JSON(payload),
			"updated_at": now,
			"read_at":    gorm.Expr("NULL"),
			"type":       ntype, // future-proof: payload + type co-evolve
		}),
	}).Create(row).Error

	if res != nil {
		return nil, apperrors.Wrap(res, apperrors.CodeInternal, "upsert notification")
	}

	// Re-fetch by the natural key so we return the canonical post-UPSERT row
	// (Postgres' ON CONFLICT DO UPDATE doesn't fill `row.ID` reliably on
	// updates across all driver versions).
	var out domain.UserNotification
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND dedupe_key = ? AND dismissed_at IS NULL",
			userID, dedupeKey).
		First(&out).Error; err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "fetch upserted notification")
	}
	return &out, nil
}
