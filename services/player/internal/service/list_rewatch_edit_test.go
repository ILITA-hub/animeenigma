package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Group 4 — manual rewatch_count edit via UpdateListEntry (design 2026-06-05).
// Free-form >=0 integer, clamped to [0, MaxRewatchCount]. nil pointer = leave
// the existing value untouched (PATCH semantics).

func seedListEntryWithRewatch(t *testing.T, db *gorm.DB, userID, animeID, status string, rewatchCount int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count) VALUES (?,?,12)`, animeID, "A").Error)
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, episodes, is_rewatching, rewatch_count, created_at, updated_at)
		 VALUES (?,?,?,?,12,0,?,now(),now())`,
		"al-"+animeID, userID, animeID, status, rewatchCount).Error)
}

func ptrInt(n int) *int { return &n }

func TestUpdateListEntry_SetsRewatchCount(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "completed", 0)

	_, err := svc.UpdateListEntry(ctx, "u1", "", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed", RewatchCount: ptrInt(3),
	})
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, 3, row.RewatchCount)
	assert.Equal(t, "completed", row.Status, "editing rewatch_count must not change status")
}

func TestUpdateListEntry_NilRewatchCount_LeavesValueUntouched(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "completed", 2)

	_, err := svc.UpdateListEntry(ctx, "u1", "", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed", RewatchCount: nil,
	})
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, 2, row.RewatchCount, "nil pointer must not clobber the existing count")
}

func TestUpdateListEntry_NegativeRewatchCount_ClampsToZero(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "completed", 5)

	_, err := svc.UpdateListEntry(ctx, "u1", "", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed", RewatchCount: ptrInt(-5),
	})
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, 0, row.RewatchCount, "negative input clamps to 0")
}

func TestUpdateListEntry_HugeRewatchCount_ClampsToMax(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "completed", 0)

	_, err := svc.UpdateListEntry(ctx, "u1", "", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed", RewatchCount: ptrInt(1_000_000),
	})
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, domain.MaxRewatchCount, row.RewatchCount, "absurd input clamps to MaxRewatchCount")
}
