package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// episodeSQLiteDDL is a sqlite-friendly library_episodes schema: GORM's
// AutoMigrate can't materialize the Postgres-only `gen_random_uuid()` default
// or the episode_source/episode_track enum types, so the table is created by
// hand with the same column NAMES the repo queries + GORM Create bind to. The
// storyboard queries under test are portable (Go-computed cutoff + bound
// params); only the fixture DDL is dialect-specific.
const episodeSQLiteDDL = `
CREATE TABLE IF NOT EXISTS library_episodes (
	id TEXT PRIMARY KEY,
	shikimori_id TEXT NOT NULL,
	episode_number INTEGER NOT NULL,
	job_id TEXT,
	minio_path TEXT NOT NULL,
	duration_sec INTEGER,
	size_bytes INTEGER,
	source TEXT NOT NULL DEFAULT 'admin',
	track TEXT NOT NULL DEFAULT 'raw',
	downloaded_at DATETIME,
	last_fetch_at DATETIME,
	fetch_count INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME,
	has_storyboard INTEGER NOT NULL DEFAULT 0
);`

// newSQLiteEpisodeDB spins up an in-memory SQLite DB and creates
// library_episodes via the hand-written DDL. Skips if the driver is unavailable
// in this build. Reuses registerSQLiteNow() from autocache_config_sqlite_test.go.
func newSQLiteEpisodeDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteNow()
	db, err := gorm.Open(&sqlite.Dialector{DriverName: "sqlite3_with_now", DSN: "file:episode_sb_test?mode=memory&cache=shared"}, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	if err := db.Exec(episodeSQLiteDDL).Error; err != nil {
		t.Skipf("create library_episodes (sqlite): %v", err)
	}
	db.Exec("DELETE FROM library_episodes")
	return db
}

func mkEpisode(t *testing.T, db *gorm.DB, id string, createdAt time.Time, hasStoryboard bool) {
	t.Helper()
	ep := &domain.Episode{
		ID:            id,
		ShikimoriID:   "s-" + id,
		EpisodeNumber: 1,
		MinioPath:     "aeProvider/" + id + "/RAW/1/",
		Source:        domain.EpisodeSourceAdmin,
		Track:         domain.EpisodeTrackRaw,
		HasStoryboard: hasStoryboard,
		CreatedAt:     createdAt,
	}
	if err := db.Create(ep).Error; err != nil {
		t.Fatalf("seed episode %s: %v", id, err)
	}
}

// TestListWithoutStoryboard_OldestFirstSkipsFreshAndFlagged asserts the three
// filters of the backfill list query: has_storyboard=false only, older-than-10-
// minutes only, oldest-first, capped at limit.
func TestListWithoutStoryboard_OldestFirstSkipsFreshAndFlagged(t *testing.T) {
	db := newSQLiteEpisodeDB(t)
	r := NewEpisodeRepository(db)
	now := time.Now()

	// old + no storyboard → eligible (oldest)
	mkEpisode(t, db, "old1", now.Add(-3*time.Hour), false)
	// old + no storyboard → eligible (newer than old1)
	mkEpisode(t, db, "old2", now.Add(-1*time.Hour), false)
	// old + HAS storyboard → excluded
	mkEpisode(t, db, "done", now.Add(-2*time.Hour), true)
	// fresh (<10min) + no storyboard → excluded (ingest-time pass covers it)
	mkEpisode(t, db, "fresh", now.Add(-2*time.Minute), false)

	got, err := r.ListWithoutStoryboard(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListWithoutStoryboard: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d episodes, want 2 (old1, old2); ids=%v", len(got), ids(got))
	}
	if got[0].ID != "old1" || got[1].ID != "old2" {
		t.Fatalf("order = %v, want [old1 old2] (created_at ASC)", ids(got))
	}
}

// TestListWithoutStoryboard_RespectsLimit — the worker asks for one at a time.
func TestListWithoutStoryboard_RespectsLimit(t *testing.T) {
	db := newSQLiteEpisodeDB(t)
	r := NewEpisodeRepository(db)
	now := time.Now()
	mkEpisode(t, db, "a", now.Add(-3*time.Hour), false)
	mkEpisode(t, db, "b", now.Add(-2*time.Hour), false)

	got, err := r.ListWithoutStoryboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListWithoutStoryboard: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("limit=1 → %v, want [a] (oldest)", ids(got))
	}
}

// TestSetHasStoryboard_FlipsFlag verifies the flag flip and the tolerant no-op
// on an unknown id.
func TestSetHasStoryboard_FlipsFlag(t *testing.T) {
	db := newSQLiteEpisodeDB(t)
	r := NewEpisodeRepository(db)
	ctx := context.Background()
	mkEpisode(t, db, "e1", time.Now().Add(-1*time.Hour), false)

	if err := r.SetHasStoryboard(ctx, "e1"); err != nil {
		t.Fatalf("SetHasStoryboard: %v", err)
	}
	var got domain.Episode
	if err := db.Where("id = ?", "e1").First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !got.HasStoryboard {
		t.Fatal("has_storyboard = false after SetHasStoryboard, want true")
	}
	// e1 must now drop out of the backfill list.
	remaining, err := r.ListWithoutStoryboard(ctx, 10)
	if err != nil {
		t.Fatalf("ListWithoutStoryboard: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("after flip, list = %v, want empty", ids(remaining))
	}
	// Unknown id is a tolerated no-op (row may have been evicted).
	if err := r.SetHasStoryboard(ctx, "nope"); err != nil {
		t.Fatalf("SetHasStoryboard(unknown) = %v, want nil no-op", err)
	}
}

func ids(eps []domain.Episode) []string {
	out := make([]string, len(eps))
	for i, e := range eps {
		out[i] = e.ID
	}
	return out
}
