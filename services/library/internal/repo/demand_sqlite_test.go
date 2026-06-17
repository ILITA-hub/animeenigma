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

// newSQLiteDemandDB spins up an in-memory SQLite DB (with now() registered, so
// the ON CONFLICT DO UPDATE SET requested_at = now() upsert is executable) and
// the autocache_demand table created via AutoMigrate. Skips if the driver is
// unavailable in this build. Reuses the registerSQLiteNow() helper from
// autocache_config_sqlite_test.go.
func newSQLiteDemandDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteNow()
	db, err := gorm.Open(&sqlite.Dialector{DriverName: "sqlite3_with_now", DSN: "file:demand_test?mode=memory&cache=shared"}, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	if err := db.AutoMigrate(&domain.AutocacheDemand{}); err != nil {
		t.Skipf("automigrate autocache_demand: %v", err)
	}
	// cache=shared persists across opens in the same process — start clean.
	db.Exec("DELETE FROM autocache_demand")
	return db
}

// TestDemandRepository_Record_FirstInsertRequestedAtIsRecent is the CR-01
// regression: the FIRST insert of a freshly-wanted (mal_id, episode) must land
// a recent requested_at (≈ now), NOT the zero-value 0001-01-01 that GORM would
// send if Record relied on the SQL DEFAULT now() it never triggers. The
// Phase-09 Planner drains by requested_at, so a year-1 timestamp would sort the
// newest demand as the oldest.
func TestDemandRepository_Record_FirstInsertRequestedAtIsRecent(t *testing.T) {
	db := newSQLiteDemandDB(t)
	r := NewDemandRepository(db)

	before := time.Now().Add(-2 * time.Second)
	if err := r.Record(context.Background(), "12345", 7, domain.DemandReasonBackfill, nil); err != nil {
		t.Fatalf("Record (first insert): %v", err)
	}

	var got domain.AutocacheDemand
	if err := db.Where("mal_id = ? AND episode = ?", "12345", 7).First(&got).Error; err != nil {
		t.Fatalf("read back inserted row: %v", err)
	}

	// The bug would store 0001-01-01 (year 1). Guard both the year-1 trap and
	// the positive recency assertion.
	if got.RequestedAt.Year() <= 1 {
		t.Fatalf("requested_at = %v (year %d): first insert landed the zero-value timestamp (CR-01 regression)",
			got.RequestedAt, got.RequestedAt.Year())
	}
	if got.RequestedAt.Before(before) {
		t.Fatalf("requested_at = %v is before test start %v: not a recent now()", got.RequestedAt, before)
	}
	if got.RequestedAt.After(time.Now().Add(2 * time.Second)) {
		t.Fatalf("requested_at = %v is in the future: unexpected", got.RequestedAt)
	}
}

// TestDemandRepository_Record_ReAssertUpdatesReason is the WR-02 regression: a
// (mal,ep) first recorded as 'backfill' that a later producer re-asserts as
// 'ongoing' must update the stored reason (last-writer-wins), so the Planner
// derives the correct OBS-04 downloads_total{trigger} label rather than mis-
// attributing an A/B-driven download to 'backfill'.
func TestDemandRepository_Record_ReAssertUpdatesReason(t *testing.T) {
	db := newSQLiteDemandDB(t)
	r := NewDemandRepository(db)
	ctx := context.Background()

	if err := r.Record(ctx, "555", 3, domain.DemandReasonBackfill, nil); err != nil {
		t.Fatalf("Record backfill: %v", err)
	}
	if err := r.Record(ctx, "555", 3, domain.DemandReasonOngoing, nil); err != nil {
		t.Fatalf("Record ongoing (re-assert): %v", err)
	}

	var got domain.AutocacheDemand
	if err := db.Where("mal_id = ? AND episode = ?", "555", 3).First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Reason != domain.DemandReasonOngoing {
		t.Fatalf("re-assert reason = %q, want %q (WR-02: ON CONFLICT must refresh reason)", got.Reason, domain.DemandReasonOngoing)
	}

	// And it must still be a single deduped row.
	var count int64
	db.Model(&domain.AutocacheDemand{}).Where("mal_id = ? AND episode = ?", "555", 3).Count(&count)
	if count != 1 {
		t.Fatalf("re-assert produced %d rows, want 1 (composite PK dedup)", count)
	}
}

// TestDemandRepository_Record_ReAssertKeepsFirstSeenRequestedAt is the WR-01
// regression: a re-assert must NOT bump requested_at. Keeping the original
// first-seen time makes the Drain ASC ordering stable so a frequently-re-
// asserted ongoing demand cannot starve behind static backfill rows.
func TestDemandRepository_Record_ReAssertKeepsFirstSeenRequestedAt(t *testing.T) {
	db := newSQLiteDemandDB(t)
	r := NewDemandRepository(db)
	ctx := context.Background()

	if err := r.Record(ctx, "777", 5, domain.DemandReasonOngoing, nil); err != nil {
		t.Fatalf("Record first: %v", err)
	}
	var first domain.AutocacheDemand
	if err := db.Where("mal_id = ? AND episode = ?", "777", 5).First(&first).Error; err != nil {
		t.Fatalf("read first: %v", err)
	}

	// Sleep past the sqlite now() second-resolution so a (wrongly) bumped
	// timestamp would be strictly newer and detectable.
	time.Sleep(1100 * time.Millisecond)

	if err := r.Record(ctx, "777", 5, domain.DemandReasonOngoing, nil); err != nil {
		t.Fatalf("Record re-assert: %v", err)
	}
	var second domain.AutocacheDemand
	if err := db.Where("mal_id = ? AND episode = ?", "777", 5).First(&second).Error; err != nil {
		t.Fatalf("read second: %v", err)
	}

	if second.RequestedAt.After(first.RequestedAt) {
		t.Fatalf("re-assert bumped requested_at from %v to %v (WR-01: must keep first-seen time for stable FIFO ordering)",
			first.RequestedAt, second.RequestedAt)
	}
}

// TestDemandRepository_Drain_FifoNoStarvation interleaves a static backfill demand
// (first-seen oldest) with an ongoing demand that gets re-asserted repeatedly, and
// asserts the re-asserted ongoing demand is NOT pushed behind the backfill one in
// the Drain ASC order (the WR-01 FIFO-stability contract). Pre-fix, each re-assert
// re-stamped requested_at=now(), sinking the ongoing row to the back forever.
func TestDemandRepository_Drain_FifoNoStarvation(t *testing.T) {
	db := newSQLiteDemandDB(t)
	r := NewDemandRepository(db)
	ctx := context.Background()

	// Ongoing demand seen FIRST (oldest first-seen time).
	if err := r.Record(ctx, "100", 1, domain.DemandReasonOngoing, nil); err != nil {
		t.Fatalf("record ongoing: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	// Backfill demand seen later.
	if err := r.Record(ctx, "200", 1, domain.DemandReasonBackfill, nil); err != nil {
		t.Fatalf("record backfill: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	// Re-assert the ongoing demand a few times (Logic A every sweep). Pre-fix this
	// would re-stamp it newer than the backfill row and reorder the FIFO.
	for i := 0; i < 3; i++ {
		if err := r.Record(ctx, "100", 1, domain.DemandReasonOngoing, nil); err != nil {
			t.Fatalf("re-assert ongoing #%d: %v", i, err)
		}
	}

	rows, err := r.Drain(ctx, 50)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("drained %d rows, want 2", len(rows))
	}
	// The ongoing demand (first-seen oldest) must still drain FIRST despite the
	// re-asserts — it holds its queue position.
	if rows[0].MALID != "100" {
		t.Fatalf("FIFO starvation (WR-01): re-asserted ongoing demand drained at position %d, expected first; order=[%s, %s]",
			1, rows[0].MALID, rows[1].MALID)
	}
}

// TestDemandRepository_Record_PersistsTitles is the v4.1 fix regression: the
// ordered title list a producer supplies must round-trip through the column so
// the Planner can search trackers by title (name_jp → romaji → name_en) instead
// of the useless "<mal_id> <episode>" query.
func TestDemandRepository_Record_PersistsTitles(t *testing.T) {
	db := newSQLiteDemandDB(t)
	r := NewDemandRepository(db)
	ctx := context.Background()

	titles := []string{"よ", "Youkoso Jitsuryoku", "Classroom of the Elite"}
	if err := r.Record(ctx, "59708", 12, domain.DemandReasonOngoing, titles); err != nil {
		t.Fatalf("Record with titles: %v", err)
	}

	var got domain.AutocacheDemand
	if err := db.Where("mal_id = ? AND episode = ?", "59708", 12).First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	st := got.SearchTitles()
	if len(st) != 3 || st[0] != "よ" || st[1] != "Youkoso Jitsuryoku" || st[2] != "Classroom of the Elite" {
		t.Fatalf("SearchTitles() = %#v, want ordered [jp, romaji, en]", st)
	}
}

// TestDemandRepository_Record_ReAssertEmptyTitlesKeepsExisting verifies a later
// title-less re-assert does NOT blank a populated titles column (a legacy/empty
// caller must not erase a good title set the Planner needs).
func TestDemandRepository_Record_ReAssertEmptyTitlesKeepsExisting(t *testing.T) {
	db := newSQLiteDemandDB(t)
	r := NewDemandRepository(db)
	ctx := context.Background()

	if err := r.Record(ctx, "59708", 12, domain.DemandReasonNextEp, []string{"Youkoso"}); err != nil {
		t.Fatalf("Record with titles: %v", err)
	}
	if err := r.Record(ctx, "59708", 12, domain.DemandReasonOngoing, nil); err != nil {
		t.Fatalf("Record re-assert no titles: %v", err)
	}

	var got domain.AutocacheDemand
	if err := db.Where("mal_id = ? AND episode = ?", "59708", 12).First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Titles != "Youkoso" {
		t.Fatalf("titles after empty re-assert = %q, want preserved %q", got.Titles, "Youkoso")
	}
	if got.Reason != domain.DemandReasonOngoing {
		t.Fatalf("reason should still refresh = %q, want ongoing", got.Reason)
	}
}
