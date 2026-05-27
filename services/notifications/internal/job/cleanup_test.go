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
