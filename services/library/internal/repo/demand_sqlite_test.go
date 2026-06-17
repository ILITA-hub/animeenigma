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
	if err := r.Record(context.Background(), "12345", 7, domain.DemandReasonBackfill); err != nil {
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
