package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestListService_UpdateListEntry_StatusActivityAggregation covers the same-day
// aggregation of status_change activity events: rapid status toggles on the
// same anime must collapse into ONE feed event carrying the latest status,
// mirroring the per-day review dedup (GetTodayByUserAnimeType + Update).
// Regression for the 2026-07-08 report: plan_to_watch → completed →
// plan_to_watch within a minute produced three feed rows.
func TestListService_UpdateListEntry_StatusActivityAggregation(t *testing.T) {
	seed := func(t *testing.T) (*ListService, *gorm.DB) {
		svc, db := setupListServiceTestDB(t)
		require.NoError(t, db.Exec(
			`INSERT INTO animes (id, name, episodes_count, deleted_at) VALUES (?, ?, ?, NULL)`,
			"anime-1", "Test Anime", 12).Error)
		return svc, db
	}

	setStatus := func(t *testing.T, svc *ListService, status string) {
		t.Helper()
		_, err := svc.UpdateListEntry(context.Background(), "user-1", "tNeymik",
			&domain.UpdateListRequest{AnimeID: "anime-1", Status: status})
		require.NoError(t, err)
	}

	t.Run("same-day toggles collapse into one event with the latest status", func(t *testing.T) {
		svc, db := seed(t)

		setStatus(t, svc, "plan_to_watch")
		setStatus(t, svc, "completed")
		setStatus(t, svc, "plan_to_watch")

		var evs []domain.ActivityEvent
		require.NoError(t, db.
			Where("user_id = ? AND anime_id = ? AND type = ?", "user-1", "anime-1", "status_change").
			Find(&evs).Error)
		require.Len(t, evs, 1, "same-day status toggles must aggregate into a single activity event")
		assert.Equal(t, "plan_to_watch", evs[0].NewValue, "aggregated event must carry the LATEST status")
		assert.Equal(t, "", evs[0].OldValue, "aggregated event must keep the day-start old status")
	})

	t.Run("unchanged status records no event", func(t *testing.T) {
		svc, db := seed(t)

		setStatus(t, svc, "watching")
		setStatus(t, svc, "watching")

		assert.EqualValues(t, 1, activityRowCount(t, db, "user-1", "anime-1", "status_change"))
	})

	t.Run("import path (empty username) records no event", func(t *testing.T) {
		svc, db := seed(t)

		_, err := svc.UpdateListEntry(context.Background(), "user-1", "",
			&domain.UpdateListRequest{AnimeID: "anime-1", Status: "completed"})
		require.NoError(t, err)

		assert.EqualValues(t, 0, activityRowCount(t, db, "user-1", "anime-1", "status_change"))
	})
}
