package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSecretFeatureTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`CREATE TABLE secret_feature_flags (
			key TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at DATETIME
		)`).Error)
	return db
}

func TestSecretFeatureRepository_GetAll_Empty(t *testing.T) {
	r := NewSecretFeatureRepository(setupSecretFeatureTestDB(t))
	got, err := r.GetAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSecretFeatureRepository_SetAndGetAll(t *testing.T) {
	r := NewSecretFeatureRepository(setupSecretFeatureTestDB(t))
	ctx := context.Background()

	require.NoError(t, r.Set(ctx, "themes", false))
	require.NoError(t, r.Set(ctx, "__roulette__", true))

	got, err := r.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.False(t, got["themes"])
	assert.True(t, got["__roulette__"])
}

func TestSecretFeatureRepository_Set_UpsertsExisting(t *testing.T) {
	r := NewSecretFeatureRepository(setupSecretFeatureTestDB(t))
	ctx := context.Background()

	require.NoError(t, r.Set(ctx, "game", false))
	require.NoError(t, r.Set(ctx, "game", true)) // flip back — must UPDATE, not duplicate

	got, err := r.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 1, "upsert must not create a second row for the same key")
	assert.True(t, got["game"])
}
