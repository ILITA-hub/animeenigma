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

func TestSessionRepo_Touch_BumpsLastSeenAndExpiry(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	old := time.Now().Add(-48 * time.Hour)
	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: padHash("touch"),
		UserAgent:        "go-test",
		IP:               "1.1.1.1",
		LastSeenAt:       old,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, r.Create(context.Background(), s))

	far := time.Now().Add(1000 * time.Hour)
	require.NoError(t, r.Touch(context.Background(), s.ID, "2.2.2.2", time.Now(), far))

	got, err := r.FindAliveByHash(context.Background(), s.RefreshTokenHash)
	require.NoError(t, err)
	require.Equal(t, "2.2.2.2", got.IP)
	require.True(t, got.LastSeenAt.After(old), "last_seen should advance")
	require.True(t, got.ExpiresAt.After(time.Now().Add(900*time.Hour)), "expiry should be pushed far out")
}

func TestSessionRepo_Touch_NoOpOnRevoked(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: padHash("revoked"),
		UserAgent:        "go-test",
		LastSeenAt:       time.Now(),
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, r.Create(context.Background(), s))
	require.NoError(t, r.Revoke(context.Background(), s.ID, userID))

	_ = r.Touch(context.Background(), s.ID, "3.3.3.3", time.Now(), time.Now().Add(1000*time.Hour))
	_, err := r.FindAliveByHash(context.Background(), s.RefreshTokenHash)
	require.Error(t, err, "revoked session must stay un-findable")
}

func TestSessionRepo_Cleanup_DeletesRevokedOlderThan7d(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	staleRevoked := &domain.UserSession{
		UserID: userID, RefreshTokenHash: padHash("stale"),
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(1000 * time.Hour),
		RevokedAt: ptrTime(time.Now().Add(-8 * 24 * time.Hour)),
	}
	require.NoError(t, r.Create(context.Background(), staleRevoked))

	alive := &domain.UserSession{
		UserID: userID, RefreshTokenHash: padHash("alive"),
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(1000 * time.Hour),
	}
	require.NoError(t, r.Create(context.Background(), alive))

	n, err := r.Cleanup(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, int64(1))

	// The specific stale-revoked row must be gone.
	var staleCount int64
	require.NoError(t, db.Model(&domain.UserSession{}).
		Where("id = ?", staleRevoked.ID).Count(&staleCount).Error)
	require.Equal(t, int64(0), staleCount, "stale revoked session must be deleted")

	// The alive session must survive.
	_, err = r.FindAliveByHash(context.Background(), alive.RefreshTokenHash)
	require.NoError(t, err, "alive session must survive cleanup")
}

// padHash builds a deterministic 64-char hash from a prefix.
func padHash(prefix string) string {
	raw := prefix + uuid.NewString()
	for len(raw) < 64 {
		raw += "0"
	}
	return raw[:64]
}

func ptrTime(t time.Time) *time.Time { return &t }
