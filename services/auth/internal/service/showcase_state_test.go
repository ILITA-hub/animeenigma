package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// newTestUserService builds a UserService backed by an in-memory SQLite DB.
// We hand-craft a SQLite-friendly users table (the real domain.User has a
// Postgres-only text[] column that AutoMigrate can't reproduce on SQLite) — the
// repo only SELECTs the columns we provide. The player-owned profile_showcases
// table is created only if withShowcaseTable is true (so the defensive
// missing-table path is testable).
func newTestUserService(t *testing.T, withShowcaseTable bool) (*UserService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`CREATE TABLE users (
			id text primary key, username text, password_hash text, telegram_id integer,
			public_id text, public_statuses text, activity_visibility text default 'all',
			avatar text, timezone text, api_key_hash text, role text default 'user',
			created_at datetime, updated_at datetime, deleted_at datetime
		)`,
	).Error)
	if withShowcaseTable {
		require.NoError(t, db.Exec(
			`CREATE TABLE profile_showcases (user_id text primary key, blocks text, enabled boolean, updated_at datetime)`,
		).Error)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	r := repo.NewUserRepository(db)
	sr := repo.NewSessionRepository(db)
	return NewUserService(r, sr, logger.Default()), db
}

func seedTestUser(t *testing.T, db *gorm.DB) (id, publicID string) {
	t.Helper()
	id = uuid.NewString()
	publicID = "pub_" + id[:8]
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, public_id, activity_visibility, role) VALUES (?, ?, ?, 'all', 'user')`,
		id, "user_"+id[:8], publicID,
	).Error)
	return id, publicID
}

func TestGetPublicProfile_SetsShowcaseState(t *testing.T) {
	svc, db := newTestUserService(t, true)
	id, _ := seedTestUser(t, db)
	require.NoError(t, db.Exec(
		`INSERT INTO profile_showcases (user_id, blocks, enabled) VALUES (?, ?, ?)`,
		id, `[{"type":"card_collection"}]`, true,
	).Error)

	pub, err := svc.GetPublicProfile(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, domain.ShowcaseStateVisible, pub.ShowcaseState)
}

func TestGetPublicProfileByPublicID_SetsShowcaseState(t *testing.T) {
	svc, db := newTestUserService(t, true)
	id, publicID := seedTestUser(t, db)
	require.NoError(t, db.Exec(
		`INSERT INTO profile_showcases (user_id, blocks, enabled) VALUES (?, ?, ?)`,
		id, `[{"type":"card_collection"}]`, false,
	).Error)

	pub, err := svc.GetPublicProfileByPublicID(context.Background(), publicID)
	require.NoError(t, err)
	require.Equal(t, domain.ShowcaseStateHidden, pub.ShowcaseState)
}

func TestGetPublicProfile_NoShowcaseRow_None(t *testing.T) {
	svc, db := newTestUserService(t, true)
	id, _ := seedTestUser(t, db)

	pub, err := svc.GetPublicProfile(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, domain.ShowcaseStateNone, pub.ShowcaseState)
}

func TestGetPublicProfile_NoShowcaseTable_None(t *testing.T) {
	// player hasn't migrated the table yet → defensive "none", profile still loads.
	svc, db := newTestUserService(t, false)
	id, _ := seedTestUser(t, db)

	pub, err := svc.GetPublicProfile(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, domain.ShowcaseStateNone, pub.ShowcaseState)
}
