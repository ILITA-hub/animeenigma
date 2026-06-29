package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// sqliteDB spins up a throwaway in-memory SQLite database for unit tests.
// GetShowcaseState reads only the player-owned profile_showcases table via raw
// SQL and never touches the auth-owned users table, so we don't migrate it
// here. The profile_showcases table itself is deliberately NOT created —
// individual tests opt into creating it so the defensive "missing table → none"
// path can be exercised.
func sqliteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// createShowcaseTable creates a minimal player-owned profile_showcases table
// (auth never owns/AutoMigrates it — tests fake it with raw SQL, no player
// import).
func createShowcaseTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(
		`CREATE TABLE profile_showcases (user_id text primary key, blocks text, enabled boolean, updated_at datetime)`,
	).Error)
}

func insertShowcase(t *testing.T, db *gorm.DB, userID, blocks string, enabled bool) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO profile_showcases (user_id, blocks, enabled) VALUES (?, ?, ?)`,
		userID, blocks, enabled,
	).Error)
}

func TestGetShowcaseState_NoTable(t *testing.T) {
	db := sqliteDB(t)
	r := repo.NewUserRepository(db)

	// profile_showcases table intentionally absent → defensive "none".
	got := r.GetShowcaseState(context.Background(), uuid.NewString())
	require.Equal(t, domain.ShowcaseStateNone, got)
}

func TestGetShowcaseState_TableButNoRow(t *testing.T) {
	db := sqliteDB(t)
	createShowcaseTable(t, db)
	r := repo.NewUserRepository(db)

	got := r.GetShowcaseState(context.Background(), uuid.NewString())
	require.Equal(t, domain.ShowcaseStateNone, got)
}

func TestGetShowcaseState_Visible(t *testing.T) {
	db := sqliteDB(t)
	createShowcaseTable(t, db)
	r := repo.NewUserRepository(db)

	userID := uuid.NewString()
	insertShowcase(t, db, userID, `[{"type":"card_collection"}]`, true)

	got := r.GetShowcaseState(context.Background(), userID)
	require.Equal(t, domain.ShowcaseStateVisible, got)
}

func TestGetShowcaseState_Hidden(t *testing.T) {
	db := sqliteDB(t)
	createShowcaseTable(t, db)
	r := repo.NewUserRepository(db)

	userID := uuid.NewString()
	insertShowcase(t, db, userID, `[{"type":"card_collection"}]`, false)

	got := r.GetShowcaseState(context.Background(), userID)
	require.Equal(t, domain.ShowcaseStateHidden, got)
}

func TestGetShowcaseState_EmptyBlocksArray(t *testing.T) {
	db := sqliteDB(t)
	createShowcaseTable(t, db)
	r := repo.NewUserRepository(db)

	userID := uuid.NewString()
	// blocks = "[]" is non-null but empty → none, even when enabled is true.
	insertShowcase(t, db, userID, `[]`, true)

	got := r.GetShowcaseState(context.Background(), userID)
	require.Equal(t, domain.ShowcaseStateNone, got)
}

func TestGetShowcaseState_EmptyBlocksString(t *testing.T) {
	db := sqliteDB(t)
	createShowcaseTable(t, db)
	r := repo.NewUserRepository(db)

	userID := uuid.NewString()
	insertShowcase(t, db, userID, ``, false)

	got := r.GetShowcaseState(context.Background(), userID)
	require.Equal(t, domain.ShowcaseStateNone, got)
}
