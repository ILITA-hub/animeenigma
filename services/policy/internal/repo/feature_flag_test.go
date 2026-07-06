package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *FeatureFlagRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.FeatureFlag{}))
	return NewFeatureFlagRepository(db)
}

func TestUpsertAndGetAll_roundTripsSlicesAndBool(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	in := domain.FeatureFlag{
		Key: "fanfic", Roles: domain.StringList{"admin"},
		AllowUsers: domain.StringList{"u1", "u2"}, Roulette: false, FailSafe: "admin", Label: "Fanfic",
	}
	require.NoError(t, r.Upsert(ctx, in))

	all, err := r.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "fanfic", all[0].Key)
	require.Equal(t, domain.StringList{"admin"}, all[0].Roles)
	require.Equal(t, domain.StringList{"u1", "u2"}, all[0].AllowUsers)
	require.False(t, all[0].Roulette) // GORM zero-value bool must persist as false
}

func TestUpsert_replacesExisting(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, r.Upsert(ctx, domain.FeatureFlag{Key: "gacha", Roles: domain.StringList{"admin"}, FailSafe: "admin"}))
	require.NoError(t, r.Upsert(ctx, domain.FeatureFlag{Key: "gacha", Roles: domain.StringList{"user"}, Roulette: true, FailSafe: "everyone"}))
	all, err := r.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, domain.StringList{"user"}, all[0].Roles)
	require.True(t, all[0].Roulette)
}

func TestSeedIfAbsent_doesNotClobber(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, r.Upsert(ctx, domain.FeatureFlag{Key: "gacha", Roulette: true, FailSafe: "admin"}))
	require.NoError(t, r.SeedIfAbsent(ctx, domain.FeatureFlag{Key: "gacha", Roulette: false, FailSafe: "admin"}))
	all, err := r.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.True(t, all[0].Roulette) // seed must NOT overwrite the admin's toggle
}
