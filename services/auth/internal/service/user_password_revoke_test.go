package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// newUserServiceWithSessions builds a UserService backed by an in-memory SQLite
// DB with hand-crafted `users` and `user_sessions` tables (the real Postgres
// domain models use pg-only column types AutoMigrate can't reproduce on SQLite,
// but the repo methods exercised here only touch the columns created below).
func newUserServiceWithSessions(t *testing.T) (*UserService, *gorm.DB) {
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
	require.NoError(t, db.Exec(
		`CREATE TABLE user_sessions (
			id text primary key, user_id text, refresh_token_hash text,
			user_agent text default '', ip text default '',
			created_at datetime, last_seen_at datetime, expires_at datetime, revoked_at datetime
		)`,
	).Error)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewUserService(repo.NewUserRepository(db), repo.NewSessionRepository(db), logger.Default()), db
}

// seedUserWithPassword inserts a user whose password_hash matches `password`.
func seedUserWithPassword(t *testing.T, db *gorm.DB, password string) string {
	t.Helper()
	id := uuid.NewString()
	hash, err := HashPassword(password)
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, password_hash, public_id, activity_visibility, role)
		 VALUES (?, ?, ?, ?, 'all', 'user')`,
		id, "user_"+id[:8], hash, "pub_"+id[:8],
	).Error)
	return id
}

// seedSession inserts an alive session with an explicit id for the user.
func seedSession(t *testing.T, db *gorm.DB, sessionID, userID string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO user_sessions (id, user_id, refresh_token_hash, created_at, last_seen_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, userID, "hash_"+sessionID, now, now, now.Add(24*time.Hour),
	).Error)
}

// aliveSessionIDs returns the ids of the user's not-yet-revoked sessions.
func aliveSessionIDs(t *testing.T, db *gorm.DB, userID string) []string {
	t.Helper()
	var ids []string
	require.NoError(t, db.Raw(
		`SELECT id FROM user_sessions WHERE user_id = ? AND revoked_at IS NULL ORDER BY id`, userID,
	).Scan(&ids).Error)
	return ids
}

func strptr(s string) *string { return &s }

// A successful password change must revoke every OTHER session (killing a
// stolen refresh token) while sparing the session the user is changing the
// password from, so they stay logged in on that device.
func TestUpdate_PasswordChange_RevokesOtherSessions_SparesCurrent(t *testing.T) {
	svc, db := newUserServiceWithSessions(t)
	ctx := context.Background()

	userID := seedUserWithPassword(t, db, "oldpass123")
	seedSession(t, db, "sess-current", userID)
	seedSession(t, db, "sess-attacker", userID)
	seedSession(t, db, "sess-other", userID)

	user, err := svc.Update(ctx, userID, "sess-current", &domain.UpdateUserRequest{
		CurrentPassword: strptr("oldpass123"),
		NewPassword:     strptr("newpass456"),
	})
	require.NoError(t, err)

	// New password is persisted.
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("newpass456")))

	// Only the current session survives; the attacker's (and every other) died.
	require.Equal(t, []string{"sess-current"}, aliveSessionIDs(t, db, userID))
}

// A username-only update (no password fields) must not revoke any session.
func TestUpdate_UsernameOnly_DoesNotRevokeSessions(t *testing.T) {
	svc, db := newUserServiceWithSessions(t)
	ctx := context.Background()

	userID := seedUserWithPassword(t, db, "oldpass123")
	seedSession(t, db, "sess-a", userID)
	seedSession(t, db, "sess-b", userID)

	_, err := svc.Update(ctx, userID, "sess-a", &domain.UpdateUserRequest{
		Username: strptr("renamed_user"),
	})
	require.NoError(t, err)

	require.Equal(t, []string{"sess-a", "sess-b"}, aliveSessionIDs(t, db, userID))
}

// When the caller's own session id is unknown (empty), fail safe: revoke ALL
// of the user's sessions rather than leaving a possibly-stolen token alive.
func TestUpdate_PasswordChange_UnknownCurrentSession_RevokesAll(t *testing.T) {
	svc, db := newUserServiceWithSessions(t)
	ctx := context.Background()

	userID := seedUserWithPassword(t, db, "oldpass123")
	seedSession(t, db, "sess-1", userID)
	seedSession(t, db, "sess-2", userID)

	_, err := svc.Update(ctx, userID, "", &domain.UpdateUserRequest{
		CurrentPassword: strptr("oldpass123"),
		NewPassword:     strptr("newpass456"),
	})
	require.NoError(t, err)

	require.Empty(t, aliveSessionIDs(t, db, userID))
}
