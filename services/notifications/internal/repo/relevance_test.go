package repo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// relevanceTestDB spins up an in-memory SQLite DB with the notifications
// table (including invalidated_at) plus the read-only source tables the
// relevance predicate joins against. Mirrors job/detector_test.go::testDB
// but uses the PARTIAL unique index so Upsert's ON CONFLICT ... WHERE
// dismissed_at IS NULL conflict target matches (revival test depends on it).
func relevanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE user_notifications (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			dedupe_key TEXT NOT NULL,
			payload TEXT NOT NULL,
			read_at DATETIME,
			dismissed_at DATETIME,
			invalidated_at DATETIME,
			deleted_at DATETIME,
			clicked_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		// Mirrors the production partial predicate (see repo.EnsureIndexes) so
		// the Upsert ON CONFLICT ... WHERE dismissed_at IS NULL AND deleted_at
		// IS NULL conflict target matches (revival + delete-dedupe tests).
		`CREATE UNIQUE INDEX uk_user_dedupe ON user_notifications (user_id, dedupe_key)
		 WHERE dismissed_at IS NULL AND deleted_at IS NULL`,
		`CREATE TABLE anime_list (
			user_id TEXT, anime_id TEXT, status TEXT,
			PRIMARY KEY (user_id, anime_id)
		)`,
		`CREATE TABLE watch_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT, anime_id TEXT, episode_number INTEGER,
			player TEXT, language TEXT, watch_type TEXT, translation_id TEXT
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	return db
}

// seedNotif inserts a new_episode notification row with the given combo +
// latest episode encoded into the JSON payload.
func seedNotif(t *testing.T, db *gorm.DB, userID, animeID string, latestEp int) string {
	t.Helper()
	payload := fmt.Sprintf(
		`{"anime_id":%q,"anime_title":"X","first_unwatched_episode":1,`+
			`"latest_available_episode":%d,"player":"kodik","language":"ru",`+
			`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`,
		animeID, latestEp)
	dedupe := "new_episode:" + animeID + ":kodik:ru:sub:1"
	now := time.Now().UTC()
	if err := db.Exec(
		`INSERT INTO user_notifications (user_id,type,dedupe_key,payload,created_at,updated_at)
		 VALUES (?,?,?,?,?,?)`,
		userID, "new_episode", dedupe, payload, now, now,
	).Error; err != nil {
		t.Fatalf("seed notif: %v", err)
	}
	var id string
	if err := db.Raw(`SELECT id FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupe).Scan(&id).Error; err != nil {
		t.Fatalf("fetch seeded id: %v", err)
	}
	return id
}

func seedList(t *testing.T, db *gorm.DB, userID, animeID, status string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO anime_list (user_id,anime_id,status) VALUES (?,?,?)`,
		userID, animeID, status).Error; err != nil {
		t.Fatalf("seed list: %v", err)
	}
}

func seedWatch(t *testing.T, db *gorm.DB, userID, animeID, player string, ep int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO watch_history (user_id,anime_id,episode_number,player,language,watch_type,translation_id)
		 VALUES (?,?,?,?,?,?,?)`,
		userID, animeID, ep, player, "ru", "sub", "1").Error; err != nil {
		t.Fatalf("seed watch: %v", err)
	}
}

// Test_JSONSQLPortability is a spike: confirms the bundled SQLite driver
// supports the `->>` operator and standard CAST forms the relevance
// predicate relies on. If this fails, the driver is too old and the
// whole approach needs revisiting BEFORE debugging the bigger queries.
func Test_JSONSQLPortability(t *testing.T) {
	db := relevanceTestDB(t)
	seedNotif(t, db, "u1", "anime-1", 7)

	var animeID string
	if err := db.Raw(
		`SELECT payload ->> 'anime_id' FROM user_notifications LIMIT 1`,
	).Scan(&animeID).Error; err != nil {
		t.Fatalf("->> operator failed (SQLite too old?): %v", err)
	}
	if animeID != "anime-1" {
		t.Fatalf("->> returned %q, want anime-1", animeID)
	}

	var ep int
	if err := db.Raw(
		`SELECT CAST(payload ->> 'latest_available_episode' AS INTEGER) FROM user_notifications LIMIT 1`,
	).Scan(&ep).Error; err != nil {
		t.Fatalf("CAST AS INTEGER failed: %v", err)
	}
	if ep != 7 {
		t.Fatalf("CAST returned %d, want 7", ep)
	}
	_ = domain.TypeNewEpisode // confirm domain import is used
}

// helper: build repo + call List(unread) and return (rowCount, unread, total)
func listUnread(t *testing.T, db *gorm.DB, userID string) (int, int64, int64) {
	t.Helper()
	r := NewNotificationRepository(db)
	rows, unread, total, err := r.List(context.Background(), userID, ListUnread, 20, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(rows), unread, total
}

func Test_List_WatchingBehind_Shows(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5) // watched 5, latest 7
	seedNotif(t, db, "u1", "anime-1", 7)
	n, unread, total := listUnread(t, db, "u1")
	if n != 1 || unread != 1 || total != 1 {
		t.Fatalf("want shown (1,1,1), got (%d,%d,%d)", n, unread, total)
	}
}

func Test_List_CaughtUp_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 7) // watched 7 == latest 7
	seedNotif(t, db, "u1", "anime-1", 7)
	n, unread, total := listUnread(t, db, "u1")
	if n != 0 || unread != 0 || total != 0 {
		t.Fatalf("want hidden (0,0,0), got (%d,%d,%d)", n, unread, total)
	}
}

func Test_List_CaughtUpOtherCombo_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "animelib", 7) // watched in a DIFFERENT player
	seedNotif(t, db, "u1", "anime-1", 7)             // notif combo is kodik
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (anime-level), got %d rows", n)
	}
}

func Test_List_Dropped_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "dropped")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (not watching), got %d rows", n)
	}
}

func Test_List_NotInList_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	// no anime_list row at all
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (not in list), got %d rows", n)
	}
}

func Test_List_PartialRange_Shows(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 7) // watched 7, latest 9 -> still behind
	seedNotif(t, db, "u1", "anime-1", 9)
	n, _, _ := listUnread(t, db, "u1")
	if n != 1 {
		t.Fatalf("want shown (newer eps unwatched), got %d rows", n)
	}
}

func Test_List_NeverWatched_Shows(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	// no watch_history rows -> COALESCE(-1) < 7 -> show
	seedNotif(t, db, "u1", "anime-1", 7)
	n, _, _ := listUnread(t, db, "u1")
	if n != 1 {
		t.Fatalf("want shown (never watched), got %d rows", n)
	}
}

func Test_List_Invalidated_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5) // would otherwise show
	id := seedNotif(t, db, "u1", "anime-1", 7)
	if err := db.Exec(`UPDATE user_notifications SET invalidated_at=? WHERE id=?`,
		time.Now().UTC(), id).Error; err != nil {
		t.Fatalf("set invalidated: %v", err)
	}
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (invalidated), got %d rows", n)
	}
}

func Test_UnreadCount_ExcludesStale(t *testing.T) {
	db := relevanceTestDB(t)
	// relevant one
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	// caught-up one (should NOT count)
	seedList(t, db, "u1", "anime-2", "watching")
	seedWatch(t, db, "u1", "anime-2", "kodik", 9)
	seedNotif(t, db, "u1", "anime-2", 9)

	r := NewNotificationRepository(db)
	n, err := r.UnreadCount(context.Background(), "u1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("want unread=1 (stale excluded), got %d", n)
	}
}

// Test_List_History_IncludesDismissed: `history` returns active + dismissed
// rows (total counts both), while `all` keeps excluding dismissed and the
// unread count stays active-only in both modes.
func Test_List_History_IncludesDismissed(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)

	// Active + relevant.
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	// Dismissed — AND caught-up, so the relevance predicate would hide it;
	// history must show it anyway (dismissed rows bypass relevance).
	seedList(t, db, "u1", "anime-2", "watching")
	seedWatch(t, db, "u1", "anime-2", "kodik", 9)
	id2 := seedNotif(t, db, "u1", "anime-2", 9)
	if err := db.Exec(`UPDATE user_notifications SET dismissed_at=? WHERE id=?`,
		time.Now().UTC(), id2).Error; err != nil {
		t.Fatalf("set dismissed: %v", err)
	}

	rows, unread, total, err := r.List(context.Background(), "u1", ListHistory, 20, 0)
	if err != nil {
		t.Fatalf("List history: %v", err)
	}
	if len(rows) != 2 || total != 2 {
		t.Fatalf("history: want 2 rows / total 2, got %d / %d", len(rows), total)
	}
	if unread != 1 {
		t.Fatalf("history: unread must stay active-only (1), got %d", unread)
	}

	rows, unread, total, err = r.List(context.Background(), "u1", ListAll, 20, 0)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(rows) != 1 || total != 1 || unread != 1 {
		t.Fatalf("all: want (1,1,1) — dismissed excluded, got (%d,%d,%d)", len(rows), unread, total)
	}
}

// Test_List_History_ExcludesInvalidated: invalidated rows stay hidden even
// in history mode.
func Test_List_History_ExcludesInvalidated(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)

	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	id := seedNotif(t, db, "u1", "anime-1", 7)
	if err := db.Exec(`UPDATE user_notifications SET invalidated_at=? WHERE id=?`,
		time.Now().UTC(), id).Error; err != nil {
		t.Fatalf("set invalidated: %v", err)
	}

	rows, _, total, err := r.List(context.Background(), "u1", ListHistory, 20, 0)
	if err != nil {
		t.Fatalf("List history: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Fatalf("want invalidated hidden from history, got %d rows / total %d", len(rows), total)
	}
}

// Test_List_Deleted_Hidden: a user-deleted row (bin from the history modal)
// disappears from EVERY surface — unread, all, and history — and drops out of
// the unread count, even though it would otherwise be active + relevant.
func Test_List_Deleted_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)

	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5) // watched 5, latest 7 -> would show
	id := seedNotif(t, db, "u1", "anime-1", 7)

	// User bins it from the All-notifications modal.
	if err := r.Delete(context.Background(), "u1", id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	for _, st := range []ListStatus{ListUnread, ListAll, ListHistory} {
		rows, unread, total, err := r.List(context.Background(), "u1", st, 20, 0)
		if err != nil {
			t.Fatalf("List(%s): %v", st, err)
		}
		if len(rows) != 0 || total != 0 || unread != 0 {
			t.Fatalf("%s: deleted row must be hidden (0,0,0), got (%d,%d,%d)",
				st, len(rows), unread, total)
		}
	}

	n, err := r.UnreadCount(context.Background(), "u1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if n != 0 {
		t.Fatalf("UnreadCount must exclude deleted row, got %d", n)
	}
}

// Test_Delete_FreesDedupeForFreshInsert: like a dismissed row, a deleted row
// sits OUTSIDE the partial uk_user_dedupe index, so a later Upsert for the
// same dedupe_key inserts a FRESH notification (new id) rather than reviving
// the binned one.
func Test_Delete_FreesDedupeForFreshInsert(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)

	userID := "u1"
	dedupe := "new_episode:anime-1:kodik:ru:sub:1"
	p7 := []byte(`{"anime_id":"anime-1","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":7,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`)

	first, err := r.Upsert(context.Background(), userID, "new_episode", dedupe, p7)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := r.Delete(context.Background(), userID, first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Re-fire with ep8 (same dedupe) -> must be a FRESH INSERT, not a revival.
	p8 := []byte(`{"anime_id":"anime-1","anime_title":"X","first_unwatched_episode":8,` +
		`"latest_available_episode":8,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`)
	out, err := r.Upsert(context.Background(), userID, "new_episode", dedupe, p8)
	if err != nil {
		t.Fatalf("re-upsert after delete: %v", err)
	}
	if out.ID == first.ID {
		t.Fatalf("expected a fresh row, but Upsert revived the deleted one (id %s)", out.ID)
	}
	if out.DeletedAt != nil {
		t.Fatalf("fresh row must not be deleted, got deleted_at=%v", out.DeletedAt)
	}
	// Two rows total: the deleted tombstone + the fresh active one.
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupe).Scan(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows (tombstone + fresh) after delete+reinsert, got %d", count)
	}
}

func Test_Upsert_RevivesInvalidatedRow(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)

	userID := "u1"
	dedupe := "new_episode:anime-1:kodik:ru:sub:1"
	p7 := []byte(`{"anime_id":"anime-1","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":7,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`)

	// First upsert (ep7), then tombstone it + mark read.
	first, err := r.Upsert(context.Background(), userID, "new_episode", dedupe, p7)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := db.Exec(
		`UPDATE user_notifications SET invalidated_at=?, read_at=? WHERE user_id=? AND dedupe_key=?`,
		time.Now().UTC(), time.Now().UTC(), userID, dedupe).Error; err != nil {
		t.Fatalf("tombstone: %v", err)
	}

	// Re-fire with ep8 (same dedupe) -> must revive: invalidated_at + read_at cleared.
	p8 := []byte(`{"anime_id":"anime-1","anime_title":"X","first_unwatched_episode":8,` +
		`"latest_available_episode":8,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`)
	out, err := r.Upsert(context.Background(), userID, "new_episode", dedupe, p8)
	if err != nil {
		t.Fatalf("revive upsert: %v", err)
	}
	if out.InvalidatedAt != nil {
		t.Fatalf("expected invalidated_at cleared on revival, got %v", out.InvalidatedAt)
	}
	if out.ReadAt != nil {
		t.Fatalf("expected read_at cleared on revival, got %v", out.ReadAt)
	}
	// Prove this was an UPDATE (revival), not a fresh INSERT:
	if out.ID != first.ID {
		t.Fatalf("expected same row revived, got new id %s (was %s)", out.ID, first.ID)
	}
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupe).Scan(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after revival (UPDATE path), got %d", count)
	}
}
