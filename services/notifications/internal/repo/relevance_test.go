package repo

import (
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
			clicked_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_user_dedupe ON user_notifications (user_id, dedupe_key)
		 WHERE dismissed_at IS NULL`,
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
