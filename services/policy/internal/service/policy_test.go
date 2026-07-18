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
	require.True(t, rs.Roulette["fanfic"]) // D2 parity: admins rolled fanfic in the old roster
	require.Equal(t, "admin", rs.FailSafe["showcase-editor"])
	require.True(t, rs.Roulette["showcase-editor"])
	require.True(t, rs.Roulette["my-feedback"])
	require.True(t, rs.Roulette["following"])
}

func TestResolveForUser_visibleAndRoulette(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))

	// normal user: sees everyone-flags, not admin-only fanfic/gacha/showcase-editor.
	mine, err := s.ResolveForUser(ctx, "u1", domain.RoleUser)
	require.NoError(t, err)
	require.Contains(t, mine.Visible, "anidle")
	require.NotContains(t, mine.Visible, "fanfic")
	require.NotContains(t, mine.Visible, "showcase-editor")
	require.Contains(t, mine.Visible, "my-feedback") // any-authenticated, not admin-only
	require.Contains(t, mine.Visible, "following")
	require.Contains(t, mine.Roulette, "following")
	require.Contains(t, mine.Roulette, "anidle")
	require.NotContains(t, mine.Roulette, "gacha") // roulette-OFF
	require.True(t, mine.RouletteEnabled)

	// admin: also sees fanfic + gacha + showcase-editor, and rolls fanfic/showcase-editor.
	adminMine, err := s.ResolveForUser(ctx, "a1", domain.RoleAdmin)
	require.NoError(t, err)
	require.Contains(t, adminMine.Visible, "fanfic")
	require.Contains(t, adminMine.Visible, "gacha")
	require.Contains(t, adminMine.Visible, "showcase-editor")
	require.Contains(t, adminMine.Roulette, "fanfic")
	require.Contains(t, adminMine.Roulette, "showcase-editor")

	// anonymous: never sees my-feedback (regression guard for the D2 parity bug —
	// my-feedback was previously seeded `everyone`, leaking it to anon visitors).
	anonMine, err := s.ResolveForUser(ctx, "", "")
	require.NoError(t, err)
	require.NotContains(t, anonMine.Visible, "my-feedback")
	require.NotContains(t, anonMine.Visible, "following")
	require.NotContains(t, anonMine.Visible, "showcase-editor")
	require.NotContains(t, anonMine.Visible, "fanfic")
	require.Contains(t, anonMine.Visible, "anidle")
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
