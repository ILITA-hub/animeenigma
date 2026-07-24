package job

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

// insertNotifWithState inserts a bare notification row with explicit
// dismissed_at / invalidated_at timestamps (nil => NULL).
func insertNotifWithState(t *testing.T, db *gorm.DB, id string, dismissedAt, invalidatedAt *time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO user_notifications (id,user_id,type,dedupe_key,payload,dismissed_at,invalidated_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		id, "u1", "new_episode", "dk-"+id, `{}`, dismissedAt, invalidatedAt,
	).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func rowExists(t *testing.T, db *gorm.DB, id string) bool {
	t.Helper()
	var n int64
	_ = db.Raw(`SELECT COUNT(*) FROM user_notifications WHERE id=?`, id).Scan(&n).Error
	return n == 1
}

func Test_Retention_ReapsOldDismissedAndInvalidated(t *testing.T) {
	db := testDB(t) // from detector_test.go
	old := time.Now().UTC().AddDate(0, 0, -40)
	recent := time.Now().UTC().AddDate(0, 0, -1)

	insertNotifWithState(t, db, "old-dismissed", &old, nil)
	insertNotifWithState(t, db, "old-invalidated", nil, &old)
	insertNotifWithState(t, db, "recent-invalidated", nil, &recent)
	insertNotifWithState(t, db, "active", nil, nil)

	job := NewDismissedRetentionCleanupJob(db, 30, logger.Default())
	deleted, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("want 2 deleted, got %d", deleted)
	}
	if rowExists(t, db, "old-dismissed") || rowExists(t, db, "old-invalidated") {
		t.Fatalf("old rows should be deleted")
	}
	if !rowExists(t, db, "recent-invalidated") || !rowExists(t, db, "active") {
		t.Fatalf("recent/active rows must survive")
	}
}

// insertNotifDeleted inserts a bare notification row with an explicit
// deleted_at timestamp (the "binned from history" state).
func insertNotifDeleted(t *testing.T, db *gorm.DB, id string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO user_notifications (id,user_id,type,dedupe_key,payload,deleted_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		id, "u1", "new_episode", "dk-"+id, `{}`, deletedAt,
	).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
}

// Test_Retention_ReapsOldDeleted: rows the user binned from the history modal
// (deleted_at) are reaped on the same retention window as dismissed /
// invalidated rows; recent deletions survive until the window elapses.
func Test_Retention_ReapsOldDeleted(t *testing.T) {
	db := testDB(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	recent := time.Now().UTC().AddDate(0, 0, -1)

	insertNotifDeleted(t, db, "old-deleted", &old)
	insertNotifDeleted(t, db, "recent-deleted", &recent)

	job := NewDismissedRetentionCleanupJob(db, 30, logger.Default())
	deleted, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("want 1 deleted, got %d", deleted)
	}
	if rowExists(t, db, "old-deleted") {
		t.Fatalf("old deleted row should be reaped")
	}
	if !rowExists(t, db, "recent-deleted") {
		t.Fatalf("recent deleted row must survive the retention window")
	}
}
