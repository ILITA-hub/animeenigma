package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupShowcaseService(t *testing.T) *ShowcaseService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE profile_showcases (
		user_id TEXT PRIMARY KEY,
		blocks TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	log, err := logger.New(logger.Config{Level: "error", Development: false})
	require.NoError(t, err)
	return NewShowcaseService(repo.NewShowcaseRepository(db), log)
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestShowcaseService_SaveAndGet_SortsByOrder(t *testing.T) {
	svc := setupShowcaseService(t)
	ctx := context.Background()
	blocks := []domain.Block{
		{Type: domain.BlockStats, Order: 5, Config: raw(t, map[string]any{})},
		{Type: domain.BlockAbout, Order: 1, Config: raw(t, map[string]string{"text": "hi"})},
	}
	require.NoError(t, svc.SaveShowcase(ctx, "u1", blocks))

	got, err := svc.GetShowcase(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	// re-numbered + sorted: about(0) before stats(1)
	require.Equal(t, domain.BlockAbout, got[0].Type)
	require.Equal(t, 0, got[0].Order)
	require.Equal(t, domain.BlockStats, got[1].Type)
	require.Equal(t, 1, got[1].Order)
}

func TestShowcaseService_GetEmpty(t *testing.T) {
	svc := setupShowcaseService(t)
	got, err := svc.GetShowcase(context.Background(), "nobody")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestShowcaseService_SaveRejectsInvalid(t *testing.T) {
	svc := setupShowcaseService(t)
	err := svc.SaveShowcase(context.Background(), "u1", []domain.Block{
		{Type: "bogus", Order: 0, Config: raw(t, map[string]any{})},
	})
	require.Error(t, err)
}
