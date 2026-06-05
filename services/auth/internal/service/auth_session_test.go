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

func TestRefreshToken_NonRotating_SameTokenFreshAccess(t *testing.T) {
	svc, _, sRepo := newTestService(t)
	ctx := context.Background()
	sc := service.SessionContext{UserAgent: "go-test", IP: "1.2.3.4"}

	username := "u_" + time.Now().Format("150405") + "_a"
	resp, err := svc.Register(ctx, &domain.RegisterRequest{Username: username, Password: "password123"}, sc)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(resp.RefreshToken, "rt_"))
	first := resp.RefreshToken

	// JWT iat/exp are second-granularity, so a refresh in the same wall-clock
	// second as Register mints a byte-identical access token. Cross a second
	// boundary so the freshly-minted token is observably distinct.
	time.Sleep(1100 * time.Millisecond)

	// Refresh: same refresh token returned (resp has no new RT), brand-new access token.
	resp2, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, service.SessionContext{UserAgent: "go-test", IP: "5.6.7.8"})
	require.NoError(t, err)
	require.NotEmpty(t, resp2.AccessToken)
	require.NotEqual(t, resp.AccessToken, resp2.AccessToken, "a fresh access token should be minted")

	// Refresh again with the SAME original token from a DIFFERENT IP — still
	// works (no rotation, no race), and Touch must stamp the new IP.
	resp3, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, service.SessionContext{UserAgent: "go-test", IP: "9.9.9.9"})
	require.NoError(t, err)
	require.NotEmpty(t, resp3.AccessToken)

	// Exactly one alive session; IP reflects the LAST refresh's sc.IP, proving
	// Touch wrote it (create IP was 1.2.3.4, so 9.9.9.9 can only come from Touch).
	sessions, err := sRepo.ListAlive(ctx, resp.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "9.9.9.9", sessions[0].IP, "Touch stamps the latest refresh IP")
}

func TestRefreshToken_RevokedSession_Fails(t *testing.T) {
	svc, _, sRepo := newTestService(t)
	ctx := context.Background()
	sc := service.SessionContext{UserAgent: "go-test", IP: "1.2.3.4"}

	username := "u_" + time.Now().Format("150405") + "_b"
	resp, err := svc.Register(ctx, &domain.RegisterRequest{Username: username, Password: "password123"}, sc)
	require.NoError(t, err)

	sessions, err := sRepo.ListAlive(ctx, resp.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, sRepo.Revoke(ctx, sessions[0].ID, resp.User.ID))

	_, err = svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: resp.RefreshToken}, sc)
	require.Error(t, err, "refresh on a revoked session must fail")
}
