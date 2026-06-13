package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedBulkEntry inserts a minimal anime_list row for the bulk-update tests,
// reusing the in-memory SQLite harness from list_mark_completed_test.go
// (setupListServiceTestDB). The schema there already has every column
// AnimeListEntry maps, so a hand-written INSERT is sufficient.
func seedBulkEntry(t *testing.T, db *gorm.DB, userID, animeID, status string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes, is_rewatching, rewatch_count, created_at, updated_at)
		 VALUES (?,?,?,?,0,0,0,0,now(),now())`,
		"al-"+animeID, userID, animeID, status).Error)
}

func TestBulkUpdate_SetStatus_UpdatesAllAndSetsCompletedAt(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()

	seedBulkEntry(t, db, "u1", "anime-1", "watching")
	seedBulkEntry(t, db, "u1", "anime-2", "watching")

	updated, failed, err := svc.BulkUpdate(ctx, "u1", "tester", &domain.BulkUpdateRequest{
		AnimeIDs: []string{"anime-1", "anime-2"},
		Action:   "set_status",
		Status:   "completed",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	assert.Equal(t, 0, failed)

	for _, id := range []string{"anime-1", "anime-2"} {
		entry, err := svc.GetUserAnimeEntry(ctx, "u1", id)
		require.NoError(t, err)
		require.NotNil(t, entry, "entry %s must still exist", id)
		assert.Equal(t, "completed", entry.Status, "status of %s must be updated", id)
		assert.NotNil(t, entry.CompletedAt, "completed entry %s must have CompletedAt auto-set", id)
	}
}

func TestBulkUpdate_SetStatus_NonCompletedStatus(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()

	seedBulkEntry(t, db, "u1", "anime-1", "watching")
	seedBulkEntry(t, db, "u1", "anime-2", "watching")

	updated, failed, err := svc.BulkUpdate(ctx, "u1", "tester", &domain.BulkUpdateRequest{
		AnimeIDs: []string{"anime-1", "anime-2"},
		Action:   "set_status",
		Status:   "on_hold",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	assert.Equal(t, 0, failed)

	entry, err := svc.GetUserAnimeEntry(ctx, "u1", "anime-1")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "on_hold", entry.Status)
}

func TestBulkUpdate_Remove_DeletesAllListedEntries(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()

	seedBulkEntry(t, db, "u1", "anime-1", "watching")
	seedBulkEntry(t, db, "u1", "anime-2", "completed")

	updated, failed, err := svc.BulkUpdate(ctx, "u1", "tester", &domain.BulkUpdateRequest{
		AnimeIDs: []string{"anime-1", "anime-2"},
		Action:   "remove",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	assert.Equal(t, 0, failed)

	for _, id := range []string{"anime-1", "anime-2"} {
		entry, err := svc.GetUserAnimeEntry(ctx, "u1", id)
		require.NoError(t, err)
		assert.Nil(t, entry, "removed entry %s must no longer exist", id)
	}
}

func TestBulkUpdate_Validation(t *testing.T) {
	svc, _ := setupListServiceTestDB(t)
	ctx := context.Background()

	t.Run("empty anime_ids", func(t *testing.T) {
		_, _, err := svc.BulkUpdate(ctx, "u1", "tester", &domain.BulkUpdateRequest{
			AnimeIDs: nil,
			Action:   "set_status",
			Status:   "completed",
		})
		require.Error(t, err)
	})

	t.Run("unknown action", func(t *testing.T) {
		_, _, err := svc.BulkUpdate(ctx, "u1", "tester", &domain.BulkUpdateRequest{
			AnimeIDs: []string{"anime-1"},
			Action:   "frobnicate",
		})
		require.Error(t, err)
	})

	t.Run("invalid status for set_status", func(t *testing.T) {
		_, _, err := svc.BulkUpdate(ctx, "u1", "tester", &domain.BulkUpdateRequest{
			AnimeIDs: []string{"anime-1"},
			Action:   "set_status",
			Status:   "not_a_status",
		})
		require.Error(t, err)
	})
}
