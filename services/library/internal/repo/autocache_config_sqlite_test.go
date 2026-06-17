package repo

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqliteNowOnce registers a SQLite driver ("sqlite3_with_now") that exposes a
// now() SQL function, so the repo's gorm.Expr("now()") updated_at bump (a
// Postgres builtin) executes under the in-memory test DB. This lets the few
// portable repo behaviors (RowsAffected accounting for WR-04) be unit-tested
// without a Postgres container.
var sqliteNowOnce sync.Once

func registerSQLiteNow() {
	sqliteNowOnce.Do(func() {
		sql.Register("sqlite3_with_now", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true)
			},
		})
	})
}

// newSQLiteConfigDB spins up an in-memory SQLite DB (with now() registered) and
// the autocache_config table created via AutoMigrate. Skips if the driver is
// unavailable in this build.
func newSQLiteConfigDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteNow()
	db, err := gorm.Open(&sqlite.Dialector{DriverName: "sqlite3_with_now", DSN: "file::memory:?cache=shared"}, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	if err := db.AutoMigrate(&domain.AutocacheConfig{}); err != nil {
		t.Skipf("automigrate autocache_config: %v", err)
	}
	// cache=shared persists across opens in the same process — start clean.
	db.Exec("DELETE FROM autocache_config")
	return db
}

// TestAutocacheConfig_Patch_MissingSingletonRow is the WR-04 regression: when the
// id=1 seed row is absent (truncated table / failed migration-006 seed), Patch
// must surface an Internal error rather than silently writing to nowhere and
// returning success.
func TestAutocacheConfig_Patch_MissingSingletonRow(t *testing.T) {
	db := newSQLiteConfigDB(t)
	r := NewAutocacheConfigRepository(db)

	_, err := r.Patch(context.Background(), map[string]any{"min_seeders": 5})
	if err == nil {
		t.Fatalf("Patch with no seed row = nil error, want Internal (broken migration)")
	}
	var appErr *liberrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != liberrors.CodeInternal {
		t.Fatalf("Patch error = %v, want CodeInternal", err)
	}
}

// TestAutocacheConfig_Patch_PresentSingletonRow proves the happy path still
// updates the existing id=1 row (RowsAffected==1) and returns the fresh config.
func TestAutocacheConfig_Patch_PresentSingletonRow(t *testing.T) {
	db := newSQLiteConfigDB(t)
	if err := db.Create(&domain.AutocacheConfig{ID: 1, MinSeeders: 3, QualityCap: 1080, BudgetBytes: 107374182400}).Error; err != nil {
		t.Fatalf("seed singleton: %v", err)
	}
	r := NewAutocacheConfigRepository(db)

	cfg, err := r.Patch(context.Background(), map[string]any{"min_seeders": 5})
	if err != nil {
		t.Fatalf("Patch with seed row present = %v, want nil", err)
	}
	if cfg.MinSeeders != 5 {
		t.Fatalf("MinSeeders after patch = %d, want 5", cfg.MinSeeders)
	}
}
