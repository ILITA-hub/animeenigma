package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Group 7 — is_rewatching PATCH semantics + manual-completion bump.
// A PATCH that omits is_rewatching must preserve it (an edit mid-rewatch must
// not silently end the cycle and skip the finale bump), and manually setting
// status=completed while rewatching mirrors the IncrementEpisodes finale
// branch: rewatch_count++ and the flag clears.

func setRewatching(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`UPDATE anime_list SET is_rewatching = true WHERE user_id = ?`, userID).Error)
}

func fetchRewatchState(t *testing.T, db *gorm.DB, userID string) (bool, int, string) {
	t.Helper()
	var row struct {
		IsRewatching bool
		RewatchCount int
		Status       string
	}
	require.NoError(t, db.Raw(
		`SELECT is_rewatching, rewatch_count, status FROM anime_list WHERE user_id = ?`,
		userID).Scan(&row).Error)
	return row.IsRewatching, row.RewatchCount, row.Status
}

func TestUpdateListEntry_NilIsRewatching_PreservesFlag(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "watching", 1)
	setRewatching(t, db, "u1")

	_, err := svc.UpdateListEntry(context.Background(), "u1", "u1", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "watching",
	})
	require.NoError(t, err)

	flag, count, _ := fetchRewatchState(t, db, "u1")
	assert.True(t, flag, "PATCH omitting is_rewatching must preserve the flag")
	assert.Equal(t, 1, count, "count untouched by a plain edit")
}

func TestUpdateListEntry_ManualCompletionWhileRewatching_BumpsAndClears(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "watching", 1)
	setRewatching(t, db, "u1")

	_, err := svc.UpdateListEntry(context.Background(), "u1", "u1", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed",
	})
	require.NoError(t, err)

	flag, count, status := fetchRewatchState(t, db, "u1")
	assert.False(t, flag, "manual completion ends the rewatch cycle")
	assert.Equal(t, 2, count, "manual completion bumps the tally once")
	assert.Equal(t, "completed", status)
}

func TestUpdateListEntry_ManualCompletionNotRewatching_DoesNotBump(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "watching", 1)

	_, err := svc.UpdateListEntry(context.Background(), "u1", "u1", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed",
	})
	require.NoError(t, err)

	flag, count, _ := fetchRewatchState(t, db, "u1")
	assert.False(t, flag)
	assert.Equal(t, 1, count, "first completion must not touch rewatch_count")
}

func TestUpdateListEntry_ExplicitCountSuppressesAutoBump(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "watching", 1)
	setRewatching(t, db, "u1")

	// An authoritative import-style write (status=completed + explicit count)
	// must land exactly the supplied count — no +1 on top.
	_, err := svc.UpdateListEntry(context.Background(), "u1", "u1", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed", RewatchCount: ptrInt(5),
	})
	require.NoError(t, err)

	flag, count, _ := fetchRewatchState(t, db, "u1")
	assert.False(t, flag, "completion still clears the flag")
	assert.Equal(t, 5, count, "explicit count is authoritative — no auto-bump on top")
}

func TestUpdateListEntry_AlreadyCompletedEdit_DoesNotRebump(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedListEntryWithRewatch(t, db, "u1", "anime-1", "completed", 2)
	setRewatching(t, db, "u1") // stale stuck flag on a completed row

	// Re-saving an already-completed entry (e.g. editing the score) is not a
	// completion transition — the tally must not climb.
	_, err := svc.UpdateListEntry(context.Background(), "u1", "u1", &domain.UpdateListRequest{
		AnimeID: "anime-1", Status: "completed",
	})
	require.NoError(t, err)

	_, count, _ := fetchRewatchState(t, db, "u1")
	assert.Equal(t, 2, count, "no transition → no bump")
}
