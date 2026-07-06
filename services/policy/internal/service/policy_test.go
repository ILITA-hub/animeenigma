package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSvc(t *testing.T) *PolicyService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.FeatureFlag{}))
	return NewPolicyService(repo.NewFeatureFlagRepository(db), logger.Default())
}

func TestSeedDefaults_isIdempotent_andMasterOn(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))
	require.NoError(t, s.SeedDefaults(ctx)) // second call must not duplicate/clobber
	rs, err := s.Ruleset(ctx)
	require.NoError(t, err)
	require.True(t, rs.RouletteEnabled)
	require.Contains(t, rs.Flags, "fanfic")
	require.NotContains(t, rs.Flags, domain.RouletteMasterKey) // master collapsed out
	require.Equal(t, "admin", rs.FailSafe["fanfic"])
	require.False(t, rs.Roulette["gacha"]) // seeded roulette-OFF
	require.True(t, rs.Roulette["anidle"])
}

func TestResolveForUser_visibleAndRoulette(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))

	// normal user: sees everyone-flags, not admin-only fanfic/gacha.
	mine, err := s.ResolveForUser(ctx, "u1", domain.RoleUser)
	require.NoError(t, err)
	require.Contains(t, mine.Visible, "anidle")
	require.NotContains(t, mine.Visible, "fanfic")
	require.Contains(t, mine.Roulette, "anidle")
	require.NotContains(t, mine.Roulette, "gacha") // roulette-OFF
	require.True(t, mine.RouletteEnabled)

	// admin: also sees fanfic + gacha.
	adminMine, err := s.ResolveForUser(ctx, "a1", domain.RoleAdmin)
	require.NoError(t, err)
	require.Contains(t, adminMine.Visible, "fanfic")
	require.Contains(t, adminMine.Visible, "gacha")
}

func TestSetFlag_thenAllowUserGetsAccess(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))
	// grant fanfic to a single non-admin user (the "Oronemu first" case).
	require.NoError(t, s.SetFlag(ctx, "fanfic",
		domain.Audience{Roles: []string{"admin"}, AllowUsers: []string{"oronemu"}}, false, "admin", "Fanfic engine"))
	mine, err := s.ResolveForUser(ctx, "oronemu", domain.RoleUser)
	require.NoError(t, err)
	require.Contains(t, mine.Visible, "fanfic")
}

func TestSetFlag_rejectsMasterKeyAndBadFailSafe(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.Error(t, s.SetFlag(ctx, domain.RouletteMasterKey, domain.Audience{}, false, "admin", ""))
	require.Error(t, s.SetFlag(ctx, "x", domain.Audience{}, false, "bogus", ""))
}

func TestSetRoulette_masterOff(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))
	require.NoError(t, s.SetRoulette(ctx, false))
	rs, err := s.Ruleset(ctx)
	require.NoError(t, err)
	require.False(t, rs.RouletteEnabled)
}
