package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newSQLiteTriggerDB spins up an in-memory SQLite DB with the autocache_trigger_log
// table created via raw SQL (NOT AutoMigrate — the domain default
// gen_random_uuid() is a Postgres function SQLite can't compile). Skips if the
// driver is unavailable. Reuses registerSQLiteNow() from autocache_config_sqlite_test.go.
func newSQLiteTriggerDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteNow()
	db, err := gorm.Open(&sqlite.Dialector{DriverName: "sqlite3_with_now", DSN: "file:trigger_test?mode=memory&cache=shared"}, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS autocache_trigger_log (
			id TEXT PRIMARY KEY,
			mal_id TEXT NOT NULL,
			target_episode INTEGER NOT NULL,
			reason TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			watch_player TEXT NOT NULL DEFAULT '',
			watch_language TEXT NOT NULL DEFAULT '',
			watch_type TEXT NOT NULL DEFAULT '',
			watched_episode INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		)`).Error; err != nil {
		t.Skipf("create autocache_trigger_log: %v", err)
	}
	db.Exec("DELETE FROM autocache_trigger_log")
	return db
}

// TestTriggerLogRepository_Insert_SetsIDAndTimestamp verifies the cause→effect row
// round-trips: the repo fills a non-empty id + a recent created_at (NOT the
// 0001-01-01 zero value the GORM nullable-default footgun would otherwise send),
// and persists the watcher context + target episode.
func TestTriggerLogRepository_Insert_SetsIDAndTimestamp(t *testing.T) {
	db := newSQLiteTriggerDB(t)
	r := NewTriggerLogRepository(db)

	row := &domain.AutocacheTriggerLog{
		MalID:          "59708",
		TargetEpisode:  2,
		Reason:         "next_ep",
		UserID:         "u1",
		Username:       "tNeymik",
		WatchPlayer:    "english",
		WatchLanguage:  "en",
		WatchType:      "sub",
		WatchedEpisode: 1,
	}
	if err := r.Insert(context.Background(), row); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if row.ID == "" {
		t.Fatal("Insert must fill a non-empty id")
	}
	if row.CreatedAt.Year() <= 1 {
		t.Fatalf("created_at = %v (year %d): zero-value timestamp landed", row.CreatedAt, row.CreatedAt.Year())
	}

	var got domain.AutocacheTriggerLog
	if err := db.Where("mal_id = ? AND target_episode = ?", "59708", 2).First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Username != "tNeymik" || got.WatchedEpisode != 1 || got.Reason != "next_ep" || got.WatchType != "sub" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
