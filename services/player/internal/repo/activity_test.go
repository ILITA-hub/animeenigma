package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupActivityTestDB(t *testing.T) *ActivityRepository {
	db := setupTestDB(t)

	// Create animes table (needed for Preload("Anime") in GetFeed)
	err := db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		name_ru TEXT,
		poster_url TEXT,
		episodes_count INTEGER DEFAULT 0,
		episodes_aired INTEGER DEFAULT 0
	)`).Error
	require.NoError(t, err)

	// Create activity_events table for SQLite (no gen_random_uuid())
	err = db.Exec(`CREATE TABLE activity_events (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		username TEXT,
		anime_id TEXT,
		type TEXT,
		old_value TEXT,
		new_value TEXT,
		content TEXT,
		created_at DATETIME,
		deleted_at DATETIME
	)`).Error
	require.NoError(t, err)

	return NewActivityRepository(db)
}

func TestActivityRepository_Create(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	event := &domain.ActivityEvent{
		ID:        "evt-1",
		UserID:    "user-1",
		Username:  "testuser",
		AnimeID:   "anime-1",
		Type:      "status_change",
		OldValue:  "plan_to_watch",
		NewValue:  "watching",
		CreatedAt: time.Now(),
	}

	err := repo.Create(ctx, event)
	require.NoError(t, err)
}

func TestActivityRepository_Create_SetsCreatedAt(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	event := &domain.ActivityEvent{
		ID:       "evt-2",
		UserID:   "user-1",
		Username: "testuser",
		AnimeID:  "anime-1",
		Type:     "score",
		NewValue: "8",
	}

	err := repo.Create(ctx, event)
	require.NoError(t, err)
	assert.False(t, event.CreatedAt.IsZero(), "CreatedAt should be set automatically")
}

func TestActivityRepository_GetFeed_Empty(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	events, hasMore, err := repo.GetFeed(ctx, 10, "")
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.False(t, hasMore)
}

func TestActivityRepository_GetFeed_ReturnsEvents(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	now := time.Now()
	for i := 0; i < 3; i++ {
		event := &domain.ActivityEvent{
			ID:        "evt-" + string(rune('a'+i)),
			UserID:    "user-1",
			Username:  "testuser",
			AnimeID:   "anime-1",
			Type:      "status_change",
			NewValue:  "watching",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, repo.Create(ctx, event))
	}

	events, hasMore, err := repo.GetFeed(ctx, 10, "")
	require.NoError(t, err)
	assert.Len(t, events, 3)
	assert.False(t, hasMore)
	// Should be ordered newest first
	assert.Equal(t, "evt-c", events[0].ID)
	assert.Equal(t, "evt-b", events[1].ID)
	assert.Equal(t, "evt-a", events[2].ID)
}

func TestActivityRepository_GetFeed_HasMore(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	now := time.Now()
	for i := 0; i < 5; i++ {
		event := &domain.ActivityEvent{
			ID:        "evt-" + string(rune('a'+i)),
			UserID:    "user-1",
			Username:  "testuser",
			AnimeID:   "anime-1",
			Type:      "status_change",
			NewValue:  "watching",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, repo.Create(ctx, event))
	}

	events, hasMore, err := repo.GetFeed(ctx, 3, "")
	require.NoError(t, err)
	assert.Len(t, events, 3)
	assert.True(t, hasMore)
}

func TestActivityRepository_GetFeed_CursorPagination(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	now := time.Now()
	for i := 0; i < 5; i++ {
		event := &domain.ActivityEvent{
			ID:        "evt-" + string(rune('a'+i)),
			UserID:    "user-1",
			Username:  "testuser",
			AnimeID:   "anime-1",
			Type:      "status_change",
			NewValue:  "watching",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, repo.Create(ctx, event))
	}

	// First page: get 2 events
	page1, hasMore1, err := repo.GetFeed(ctx, 2, "")
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.True(t, hasMore1)
	assert.Equal(t, "evt-e", page1[0].ID)
	assert.Equal(t, "evt-d", page1[1].ID)

	// Second page: use last event ID as cursor
	page2, hasMore2, err := repo.GetFeed(ctx, 2, page1[1].ID)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	assert.True(t, hasMore2)
	assert.Equal(t, "evt-c", page2[0].ID)
	assert.Equal(t, "evt-b", page2[1].ID)

	// Third page: last event
	page3, hasMore3, err := repo.GetFeed(ctx, 2, page2[1].ID)
	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.False(t, hasMore3)
	assert.Equal(t, "evt-a", page3[0].ID)
}

func TestActivityRepository_GetFeed_InvalidCursor(t *testing.T) {
	repo := setupActivityTestDB(t)
	ctx := context.Background()

	_, _, err := repo.GetFeed(ctx, 10, "nonexistent-id")
	assert.Error(t, err, "should error on invalid cursor ID")
}
