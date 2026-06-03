//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// dbForTest connects to the dev postgres. Run via `make dev` first, then:
//
//	INTEGRATION=1 go test -tags=integration ./services/auth/internal/service/... -v
func dbForTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost port=5432 user=postgres password=postgres dbname=animeenigma sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	// Migrate only UserSession — the users table already exists in the dev DB
	// and AutoMigrating User{} triggers constraint rename errors on the live schema.
	require.NoError(t, db.AutoMigrate(&domain.UserSession{}))
	return db
}

func newTestService(t *testing.T) (*service.AuthService, *repo.UserRepository, *repo.SessionRepository) {
	t.Helper()
	db := dbForTest(t)

	c, err := cache.New(cache.Config{Host: "localhost", Port: 6379})
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	uRepo := repo.NewUserRepository(db)
	sRepo := repo.NewSessionRepository(db)
	jwtCfg := authz.JWTConfig{
		Secret:          "test-secret",
		Issuer:          "test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	logr := logger.Default()
	return service.NewAuthService(uRepo, sRepo, c, jwtCfg, "", 6*time.Hour, logr), uRepo, sRepo
}

func TestRefreshToken_PersistentPath_RotatesAndReturnsNewRT(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	sc := service.SessionContext{UserAgent: "go-test", IP: "1.2.3.4"}

	username := "u_" + time.Now().Format("150405") + "_a"
	resp, err := svc.Register(ctx, &domain.RegisterRequest{Username: username, Password: "password123"}, sc)
	require.NoError(t, err)
	require.NotEmpty(t, resp.RefreshToken, "register should return an opaque RT")
	require.True(t, strings.HasPrefix(resp.RefreshToken, "rt_"), "RT should be opaque rt_*")
	first := resp.RefreshToken

	// Refresh once → new RT issued, rotated=true.
	resp2, rotated, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, sc)
	require.NoError(t, err)
	require.True(t, rotated)
	require.NotEqual(t, first, resp2.RefreshToken)
	require.True(t, strings.HasPrefix(resp2.RefreshToken, "rt_"))

	// Reuse old RT within grace → grace path, rotated=false, no new RT in resp.
	resp3, rotated2, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, sc)
	require.NoError(t, err)
	require.False(t, rotated2)
	require.Empty(t, resp3.RefreshToken, "grace path should not return a new RT")
	require.NotEmpty(t, resp3.AccessToken, "grace path should still mint a fresh access token")

	// Reusing the original RT AGAIN must still succeed on the grace path (the
	// window slides on each hit), proving a desynced browser stuck on the
	// previous token keeps working across repeated refreshes instead of being
	// logged out — the core of the random-logout fix.
	resp4, rotated3, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, sc)
	require.NoError(t, err)
	require.False(t, rotated3, "repeated previous-token reuse must stay on the grace path")
	require.Empty(t, resp4.RefreshToken)
	require.NotEmpty(t, resp4.AccessToken, "repeated grace hit should still mint a fresh access token")
}

func TestRefreshToken_LegacyJWT_UpgradesToSession(t *testing.T) {
	svc, uRepo, sRepo := newTestService(t)
	ctx := context.Background()

	// Make a user
	user := &domain.User{
		Username:     "legacy_" + time.Now().Format("150405"),
		PasswordHash: "x",
		Role:         authz.RoleUser,
	}
	require.NoError(t, uRepo.Create(ctx, user))

	// Mint a legacy 7-day refresh JWT directly via JWTManager (using same secret as svc).
	jm := authz.NewJWTManager(authz.JWTConfig{
		Secret:          "test-secret",
		Issuer:          "test",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	pair, err := jm.GenerateTokenPair(user.ID, user.Username, user.Role, "")
	require.NoError(t, err)

	// Refresh using the legacy JWT — should upgrade to a real session.
	resp, rotated, err := svc.RefreshToken(
		ctx,
		&domain.RefreshRequest{RefreshToken: pair.RefreshToken},
		service.SessionContext{UserAgent: "legacy", IP: "9.9.9.9"},
	)
	require.NoError(t, err)
	require.True(t, rotated, "legacy upgrade should return rotated=true so handler sets cookie")
	require.NotEmpty(t, resp.RefreshToken)
	require.True(t, strings.HasPrefix(resp.RefreshToken, "rt_"), "upgraded token should be opaque rt_*")

	// Confirm a session row now exists for the user.
	sessions, err := sRepo.ListAlive(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}
