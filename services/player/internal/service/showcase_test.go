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
		enabled INTEGER NOT NULL DEFAULT 0,
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
	enabled, err := svc.SaveShowcase(ctx, "u1", blocks, true)
	require.NoError(t, err)
	require.True(t, enabled)

	got, gotEnabled, err := svc.GetShowcase(ctx, "u1")
	require.NoError(t, err)
	require.True(t, gotEnabled)
	require.Len(t, got, 2)
	// re-numbered + sorted: about(0) before stats(1)
	require.Equal(t, domain.BlockAbout, got[0].Type)
	require.Equal(t, 0, got[0].Order)
	require.Equal(t, domain.BlockStats, got[1].Type)
	require.Equal(t, 1, got[1].Order)
}

func TestShowcaseService_GetEmpty(t *testing.T) {
	svc := setupShowcaseService(t)
	got, enabled, err := svc.GetShowcase(context.Background(), "nobody")
	require.NoError(t, err)
	require.Empty(t, got)
	require.False(t, enabled)
}

func TestShowcaseService_SaveRejectsInvalid(t *testing.T) {
	svc := setupShowcaseService(t)
	_, err := svc.SaveShowcase(context.Background(), "u1", []domain.Block{
		{Type: "bogus", Order: 0, Config: raw(t, map[string]any{})},
	}, true)
	require.Error(t, err)
}

// Coerce rule: an empty showcase can never be enabled, even if the caller
// requests enabled=true.
func TestShowcaseService_SaveCoercesEmptyToDisabled(t *testing.T) {
	svc := setupShowcaseService(t)
	ctx := context.Background()

	enabled, err := svc.SaveShowcase(ctx, "u1", []domain.Block{}, true)
	require.NoError(t, err)
	require.False(t, enabled, "empty showcase must coerce enabled to false")

	got, gotEnabled, err := svc.GetShowcase(ctx, "u1")
	require.NoError(t, err)
	require.Empty(t, got)
	require.False(t, gotEnabled)
}

// Saving non-empty content with enabled=false stays hidden (no coercion up).
func TestShowcaseService_SaveKeepsDisabledWhenRequested(t *testing.T) {
	svc := setupShowcaseService(t)
	ctx := context.Background()
	blocks := []domain.Block{
		{Type: domain.BlockAbout, Order: 0, Config: raw(t, map[string]string{"text": "hi"})},
	}
	enabled, err := svc.SaveShowcase(ctx, "u1", blocks, false)
	require.NoError(t, err)
	require.False(t, enabled)

	got, gotEnabled, err := svc.GetShowcase(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.False(t, gotEnabled)
}
