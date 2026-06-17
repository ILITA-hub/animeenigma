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
}

func TestShowcaseRepo_UpsertThenGet(t *testing.T) {
	repo := NewShowcaseRepository(setupShowcaseRepoDB(t))
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, "u1", `[{"type":"about","order":0,"config":{}}]`))
	got, err := repo.Get(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got.Blocks, `"about"`)

	// second upsert replaces
	require.NoError(t, repo.Upsert(ctx, "u1", `[{"type":"stats","order":0,"config":{}}]`))
	got2, err := repo.Get(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got2.Blocks, `"stats"`)
	require.NotContains(t, got2.Blocks, `"about"`)
}
