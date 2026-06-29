package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupShowcaseRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE profile_showcases (
		user_id TEXT PRIMARY KEY,
		blocks TEXT NOT NULL DEFAULT '[]',
		enabled INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	return db
}

func TestShowcaseRepo_GetEmpty(t *testing.T) {
	repo := NewShowcaseRepository(setupShowcaseRepoDB(t))
	got, err := repo.Get(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, "u1", got.UserID)
	require.Equal(t, "[]", got.Blocks)
	require.False(t, got.Enabled)
}

func TestShowcaseRepo_UpsertThenGet(t *testing.T) {
	repo := NewShowcaseRepository(setupShowcaseRepoDB(t))
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, "u1", `[{"type":"about","order":0,"config":{}}]`, true))
	got, err := repo.Get(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got.Blocks, `"about"`)
	require.True(t, got.Enabled)

	// second upsert replaces blocks AND enabled
	require.NoError(t, repo.Upsert(ctx, "u1", `[{"type":"stats","order":0,"config":{}}]`, false))
	got2, err := repo.Get(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got2.Blocks, `"stats"`)
	require.NotContains(t, got2.Blocks, `"about"`)
	require.False(t, got2.Enabled)
}
