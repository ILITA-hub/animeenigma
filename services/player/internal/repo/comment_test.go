package repo

// Wave-0 scaffold for SOCIAL-04d / SOCIAL-04e (`01-VALIDATION.md` rows
// 01-Comment-04 / 01-Comment-05). Tests are skipped today; plan 03 fills the
// assertions.
//
// SQLite caveat: SQLite-in-memory does NOT support `gen_random_uuid()`. When
// plan 03 wires the real assertions, every domain.Comment fixture must set
// `ID = uuid.NewString()` explicitly before the Create call. The
// `setupCommentTestDB` helper below uses AutoMigrate, which does NOT execute
// the `gen_random_uuid()` default — the column simply has no default on SQLite.

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCommentTestDB mirrors the in-memory SQLite pattern from sync_test.go and
// repo/sync_test.go. Plan 03 may extend it with seed data.
func setupCommentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&domain.Comment{}, &domain.AnimeInfo{}); err != nil {
		t.Fatalf("failed to auto-migrate test schema: %v", err)
	}
	return db
}

// TestCommentRepo_SoftDelete validates SOCIAL-04d: SoftDelete sets deleted_at,
// the row remains in the table, and ListByAnime / GetByID exclude it.
func TestCommentRepo_SoftDelete(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
	_ = setupCommentTestDB
}

// TestCommentRepo_ListByAnime_Cursor validates SOCIAL-04e: cursor pagination
// returns the next 50 rows in newest-first order and the opaque base64 cursor
// round-trips correctly.
func TestCommentRepo_ListByAnime_Cursor(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
	_ = setupCommentTestDB
}
