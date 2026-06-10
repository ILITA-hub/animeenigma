package repo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"gorm.io/gorm"
)

// seedFeedbackNotif inserts a feedback_* notification row directly.
func seedFeedbackNotif(t *testing.T, db *gorm.DB, userID, ntype, dedupeKey string, readAt *time.Time) string {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Exec(
		`INSERT INTO user_notifications (user_id,type,dedupe_key,payload,read_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		userID, ntype, dedupeKey, `{"report_id":"r1","status":"created"}`, readAt, now, now,
	).Error; err != nil {
		t.Fatalf("seed feedback notif: %v", err)
	}
	var id string
	if err := db.Raw(`SELECT id FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupeKey).Scan(&id).Error; err != nil {
		t.Fatalf("fetch seeded id: %v", err)
	}
	return id
}

func invalidatedAt(t *testing.T, db *gorm.DB, id string) *time.Time {
	t.Helper()
	var ts sql.NullTime
	if err := db.Raw(`SELECT invalidated_at FROM user_notifications WHERE id=?`, id).Scan(&ts).Error; err != nil {
		t.Fatalf("fetch invalidated_at: %v", err)
	}
	if !ts.Valid {
		return nil
	}
	return &ts.Time
}

// TestInvalidateUnreadByDedupeKeys covers the feedback triage supersede rule:
// unread rows matching the keys get tombstoned; read rows and other users'
// rows are untouched; empty key list is a no-op.
func TestInvalidateUnreadByDedupeKeys(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)
	ctx := context.Background()

	readTime := time.Now().UTC().Add(-time.Hour)
	unreadID := seedFeedbackNotif(t, db, "u1", "feedback_created", "feedback:r1:created", nil)
	readID := seedFeedbackNotif(t, db, "u1", "feedback_in_progress", "feedback:r1:in_progress", &readTime)
	otherUserID := seedFeedbackNotif(t, db, "u2", "feedback_created", "feedback:r1:created", nil)
	unrelatedID := seedFeedbackNotif(t, db, "u1", "feedback_created", "feedback:r2:created", nil)

	n, err := r.InvalidateUnreadByDedupeKeys(ctx, "u1",
		[]string{"feedback:r1:created", "feedback:r1:in_progress"})
	if err != nil {
		t.Fatalf("InvalidateUnreadByDedupeKeys: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1 (only the unread matching row)", n)
	}

	if invalidatedAt(t, db, unreadID) == nil {
		t.Errorf("unread matching row should be invalidated")
	}
	if invalidatedAt(t, db, readID) != nil {
		t.Errorf("already-read row must NOT be invalidated")
	}
	if invalidatedAt(t, db, otherUserID) != nil {
		t.Errorf("other user's row must NOT be invalidated")
	}
	if invalidatedAt(t, db, unrelatedID) != nil {
		t.Errorf("different report's row must NOT be invalidated")
	}

	// Idempotent: second call affects nothing.
	n, err = r.InvalidateUnreadByDedupeKeys(ctx, "u1",
		[]string{"feedback:r1:created", "feedback:r1:in_progress"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if n != 0 {
		t.Errorf("second call rows affected = %d, want 0", n)
	}

	// Empty keys → no-op, no error.
	if n, err = r.InvalidateUnreadByDedupeKeys(ctx, "u1", nil); err != nil || n != 0 {
		t.Errorf("empty keys: n=%d err=%v, want 0,nil", n, err)
	}
}
