//go:build integration

package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// dbForTest connects to the dev postgres. Run via `make dev` first, then:
//
//	INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/...
func dbForTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost port=5432 user=postgres password=postgres dbname=animeenigma sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.UserSession{}))
	return db
}

// seedUser inserts a minimal users row and returns its ID. user_sessions has
// an FK on users(id), so we need a real user.
func seedUser(t *testing.T, db *gorm.DB) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, password_hash, public_id, role, public_statuses) VALUES (?, ?, '', ?, 'user', '{}')",
		id, "test_"+id[:8], "pub_"+id[:8],
	).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", id) })
	return id
}

func TestSessionRepo_CreateAndFindByHash(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	hashLen64 := func(prefix string) string {
		// Build a 64-char hex-like hash deterministically.
		raw := prefix + uuid.NewString()
		// pad/truncate to 64 chars
		for len(raw) < 64 {
			raw += "0"
		}
		return raw[:64]
	}

	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: hashLen64("a"),
		UserAgent:        "go-test",
		ExpiresAt:        time.Now().Add(time.Hour),
		LastSeenAt:       time.Now(),
	}
	require.NoError(t, r.Create(context.Background(), s))
	require.NotEmpty(t, s.ID)

	found, err := r.FindAliveByHash(context.Background(), s.RefreshTokenHash)
	require.NoError(t, err)
	require.Equal(t, s.ID, found.ID)
}

func TestSessionRepo_RotateCASWinAndGracePath(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	hashLen64 := func(prefix string) string {
		raw := prefix + uuid.NewString()
		for len(raw) < 64 {
			raw += "0"
		}
		return raw[:64]
	}

	oldHash := hashLen64("old")
	newHash := hashLen64("new")
	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: oldHash,
		ExpiresAt:        time.Now().Add(time.Hour),
		LastSeenAt:       time.Now(),
	}
	require.NoError(t, r.Create(context.Background(), s))

	// Winner CAS — rotates.
	res1, err := r.Rotate(context.Background(), s.ID, oldHash, newHash, "1.2.3.4", time.Now().Add(30*24*time.Hour))
	require.NoError(t, err)
	require.True(t, res1.Rotated)
	require.Equal(t, newHash, res1.Session.RefreshTokenHash)

	// Loser CAS with the same oldHash — should hit the grace path, not rotate.
	res2, err := r.Rotate(context.Background(), s.ID, oldHash, hashLen64("loser"), "5.6.7.8", time.Now().Add(30*24*time.Hour))
	require.NoError(t, err)
	require.False(t, res2.Rotated, "expected grace path (no rotation) on second CAS with same oldHash")
	require.Equal(t, newHash, res2.Session.RefreshTokenHash, "row hash should still be the winner's")
}

func TestSessionRepo_RevokeAndRevokeOthers(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	hashLen64 := func(prefix string) string {
		raw := prefix + uuid.NewString()
		for len(raw) < 64 {
			raw += "0"
		}
		return raw[:64]
	}

	mk := func(tag string) *domain.UserSession {
		s := &domain.UserSession{
			UserID:           userID,
			RefreshTokenHash: hashLen64(tag),
			ExpiresAt:        time.Now().Add(time.Hour),
			LastSeenAt:       time.Now(),
		}
		require.NoError(t, r.Create(context.Background(), s))
		return s
	}

	a, b, c := mk("a"), mk("b"), mk("c")

	// Revoke single
	require.NoError(t, r.Revoke(context.Background(), a.ID, userID))
	alive, err := r.ListAlive(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, alive, 2)

	// Revoke single by wrong user → NotFound
	otherUser := seedUser(t, db)
	err = r.Revoke(context.Background(), b.ID, otherUser)
	require.Error(t, err)

	// Revoke others (keep c)
	count, err := r.RevokeOthers(context.Background(), userID, c.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count) // b only; a was already revoked

	alive, err = r.ListAlive(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, alive, 1)
	require.Equal(t, c.ID, alive[0].ID)
}
