package job

import (
	"context"
	"strconv"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

func seedNotifJob(t *testing.T, db *gorm.DB, userID, animeID string, latestEp int) string {
	t.Helper()
	payload := `{"anime_id":"` + animeID + `","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":` + strconv.Itoa(latestEp) + `,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`
	dedupe := "new_episode:" + animeID + ":kodik:ru:sub:1"
	if err := db.Exec(
		`INSERT INTO user_notifications (user_id,type,dedupe_key,payload,created_at,updated_at)
		 VALUES (?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		userID, "new_episode", dedupe, payload).Error; err != nil {
		t.Fatalf("seed notif: %v", err)
	}
	var id string
	_ = db.Raw(`SELECT id FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupe).Scan(&id).Error
	return id
}

func invalidatedAtNull(t *testing.T, db *gorm.DB, id string) bool {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM user_notifications WHERE id=? AND invalidated_at IS NULL`, id).
		Scan(&n).Error; err != nil {
		t.Fatalf("check invalidated: %v", err)
	}
	return n == 1
}

func Test_InvalidationJob_TombstonesStaleKeepsRelevant(t *testing.T) {
	db := testDB(t) // from detector_test.go (same package)

	// Relevant: watching + behind -> must stay active.
	seedList(t, db, "u1", "anime-keep", "watching")
	seedWatch(t, db, "u1", "anime-keep", "kodik", "ru", "sub", "1", 5)
	keepID := seedNotifJob(t, db, "u1", "anime-keep", 7)

	// Stale (caught up): watching + watched latest -> tombstone.
	seedList(t, db, "u1", "anime-caught", "watching")
	seedWatch(t, db, "u1", "anime-caught", "kodik", "ru", "sub", "1", 7)
	caughtID := seedNotifJob(t, db, "u1", "anime-caught", 7)

	// Stale (dropped): not watching -> tombstone.
	seedList(t, db, "u1", "anime-drop", "dropped")
	dropID := seedNotifJob(t, db, "u1", "anime-drop", 7)

	job := NewRelevanceInvalidationJob(db, logger.Default())
	count, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("invalidation run: %v", err)
	}
	if count != 2 {
		t.Fatalf("want 2 tombstoned, got %d", count)
	}
	if !invalidatedAtNull(t, db, keepID) {
		t.Fatalf("relevant row was wrongly invalidated")
	}
	if invalidatedAtNull(t, db, caughtID) {
		t.Fatalf("caught-up row was not invalidated")
	}
	if invalidatedAtNull(t, db, dropID) {
		t.Fatalf("dropped row was not invalidated")
	}
}

func Test_InvalidationJob_Idempotent(t *testing.T) {
	db := testDB(t)
	seedList(t, db, "u1", "anime-drop", "dropped")
	_ = seedNotifJob(t, db, "u1", "anime-drop", 7)

	job := NewRelevanceInvalidationJob(db, logger.Default())
	first, _ := job.Run(context.Background())
	second, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if first != 1 {
		t.Fatalf("want first=1, got %d", first)
	}
	if second != 0 {
		t.Fatalf("want second=0 (already invalidated), got %d", second)
	}
}
