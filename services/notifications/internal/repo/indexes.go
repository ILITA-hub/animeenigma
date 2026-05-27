package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"gorm.io/gorm"
)

// EnsureIndexes creates partial indexes on user_notifications that GORM
// AutoMigrate cannot express. MUST be called immediately after
// db.AutoMigrate(&UserNotification{}). Safe to call on every boot
// (idempotent for uk_user_dedupe; idx_user_unread is dropped-and-recreated
// so its predicate is always current).
//
// Two indexes created:
//
//  1. uk_user_dedupe (UNIQUE) — enforces "one active row per
//     (user_id, dedupe_key)". Partial on `dismissed_at IS NULL` so a
//     dismissed row does not block a future re-fire of the same logical
//     notification (design doc §Dedupe rules). Intentionally does NOT
//     exclude invalidated rows — Upsert revival relies on the conflict
//     target matching tombstoned (invalidated but not dismissed) rows.
//  2. idx_user_unread — powers the hot read path
//     (list / unread-count / bell-poll). Index sorts by created_at DESC
//     so LIMIT-N queries are an index scan with no sort step. Predicate
//     is tightened to `dismissed_at IS NULL AND invalidated_at IS NULL`
//     to match the read-path base predicate (Task 10). DROP+CREATE is
//     required because IF NOT EXISTS will NOT change an existing
//     index's predicate.
func EnsureIndexes(ctx context.Context, db *gorm.DB) error {
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_user_dedupe
		 ON user_notifications (user_id, dedupe_key)
		 WHERE dismissed_at IS NULL`,
		// Tightened to also exclude invalidated rows, matching the read-path
		// base predicate (dismissed_at IS NULL AND invalidated_at IS NULL).
		// DROP+CREATE because IF NOT EXISTS won't change an existing
		// predicate. Safe + idempotent on this small table.
		`DROP INDEX IF EXISTS idx_user_unread`,
		`CREATE INDEX IF NOT EXISTS idx_user_unread
		 ON user_notifications (user_id, created_at DESC)
		 WHERE dismissed_at IS NULL AND invalidated_at IS NULL`,
	}

	for _, sql := range stmts {
		if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
			return errors.Wrap(err, errors.CodeInternal, "create notifications partial index")
		}
	}
	return nil
}
