package service

import (
	"context"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSecretFeatureService(t *testing.T) *SecretFeatureService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`CREATE TABLE secret_feature_flags (
			key TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1, updated_at DATETIME
		)`).Error)
	return NewSecretFeatureService(repo.NewSecretFeatureRepository(db), nil)
}

func TestSecretFeatureService_GetConfig_DefaultsEnabled(t *testing.T) {
	s := newSecretFeatureService(t)
	cfg, err := s.GetConfig(context.Background())
	require.NoError(t, err)
	assert.True(t, cfg.RouletteEnabled, "empty store ⇒ roulette enabled (fail-open)")
	assert.Empty(t, cfg.Features)
}

func TestSecretFeatureService_GetConfig_StripsMasterKey(t *testing.T) {
	s := newSecretFeatureService(t)
	ctx := context.Background()
	require.NoError(t, s.SetRoulette(ctx, false))
	require.NoError(t, s.SetFeature(ctx, "themes", false))

	cfg, err := s.GetConfig(ctx)
	require.NoError(t, err)
	assert.False(t, cfg.RouletteEnabled)
	assert.Len(t, cfg.Features, 1, "master key must not appear in the features map")
	assert.False(t, cfg.Features["themes"])
	_, hasMaster := cfg.Features[domain.RouletteMasterKey]
	assert.False(t, hasMaster)
}

func TestSecretFeatureService_PublicState_DisabledKeysSortedNoMaster(t *testing.T) {
	s := newSecretFeatureService(t)
	ctx := context.Background()
	require.NoError(t, s.SetRoulette(ctx, false))
	require.NoError(t, s.SetFeature(ctx, "themes", false))
	require.NoError(t, s.SetFeature(ctx, "game", false))
	require.NoError(t, s.SetFeature(ctx, "anidle", true)) // enabled ⇒ not in disabled list

	state, err := s.PublicState(ctx)
	require.NoError(t, err)
	assert.False(t, state.RouletteEnabled)
	assert.Equal(t, []string{"game", "themes"}, state.DisabledKeys, "sorted, enabled + master excluded")
}

func TestSecretFeatureService_SetFeature_RejectsReservedAndEmpty(t *testing.T) {
	s := newSecretFeatureService(t)
	ctx := context.Background()

	for _, key := range []string{"", domain.RouletteMasterKey} {
		err := s.SetFeature(ctx, key, false)
		require.Error(t, err)
		appErr, ok := liberrors.IsAppError(err)
		require.True(t, ok)
		assert.Equal(t, liberrors.CodeInvalidInput, appErr.Code)
	}
}
