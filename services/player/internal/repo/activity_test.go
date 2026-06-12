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

	// Create animes table (needed for Preload("Anime") in GetFeed).
	// `rating` feeds the activity-visibility hentai predicate.
	err := db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		name_ru TEXT,
		poster_url TEXT,
		rating TEXT,
		episodes_count INTEGER DEFAULT 0,
		episodes_aired INTEGER DEFAULT 0
	)`).Error
	require.NoError(t, err)

	// users + genre tables back GetFeed's activity_visibility filtering
	// (LEFT JOIN users; hentai predicate joins anime_genres/genres).
	err = db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		avatar TEXT,
		activity_visibility TEXT
	)`).Error
	require.NoError(t, err)
	err = db.Exec(`CREATE TABLE genres (id TEXT PRIMARY KEY, name TEXT)`).Error
	require.NoError(t, err)
	err = db.Exec(`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`).Error
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

// --- activity_visibility enforcement (design 2026-06-12) -------------------

func seedVisibilityFixtures(t *testing.T, r *ActivityRepository) {
	t.Helper()
	// anime-sfw is ordinary; anime-rx is rated rx; anime-genre carries the
	// Hentai genre without the rx rating (covers both predicate branches).
	require.NoError(t, r.db.Exec(`INSERT INTO animes (id, name, rating) VALUES
		('anime-sfw', 'Normal Show', 'pg_13'),
		('anime-rx', 'Rated Show', 'rx'),
		('anime-genre', 'Genre Show', 'r')`).Error)
	require.NoError(t, r.db.Exec(`INSERT INTO genres (id, name) VALUES ('g-hentai', 'Hentai')`).Error)
	require.NoError(t, r.db.Exec(`INSERT INTO anime_genres (anime_id, genre_id) VALUES ('anime-genre', 'g-hentai')`).Error)
}

func seedVisibilityEvent(t *testing.T, r *ActivityRepository, id, userID, animeID string, at time.Time) {
	t.Helper()
	require.NoError(t, r.Create(context.Background(), &domain.ActivityEvent{
		ID: id, UserID: userID, Username: "u-" + userID, AnimeID: animeID,
		Type: "status_change", NewValue: "watching", CreatedAt: at,
	}))
}

func feedIDs(t *testing.T, r *ActivityRepository) []string {
	t.Helper()
	events, _, err := r.GetFeed(context.Background(), 50, "")
	require.NoError(t, err)
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestActivityRepository_GetFeed_VisibilityNone_HidesAllUserEvents(t *testing.T) {
	r := setupActivityTestDB(t)
	seedVisibilityFixtures(t, r)
	require.NoError(t, r.db.Exec(`INSERT INTO users (id, activity_visibility) VALUES
		('hidden-user', 'none'), ('open-user', 'all')`).Error)

	now := time.Now()
	seedVisibilityEvent(t, r, "evt-hidden", "hidden-user", "anime-sfw", now)
	seedVisibilityEvent(t, r, "evt-open", "open-user", "anime-sfw", now.Add(time.Second))

	assert.Equal(t, []string{"evt-open"}, feedIDs(t, r))
}

func TestActivityRepository_GetFeed_VisibilityNonHentai_HidesOnly18Plus(t *testing.T) {
	r := setupActivityTestDB(t)
	seedVisibilityFixtures(t, r)
	require.NoError(t, r.db.Exec(`INSERT INTO users (id, activity_visibility) VALUES ('shy-user', 'non_hentai')`).Error)

	now := time.Now()
	seedVisibilityEvent(t, r, "evt-sfw", "shy-user", "anime-sfw", now)
	seedVisibilityEvent(t, r, "evt-rx", "shy-user", "anime-rx", now.Add(time.Second))
	seedVisibilityEvent(t, r, "evt-genre", "shy-user", "anime-genre", now.Add(2*time.Second))

	assert.Equal(t, []string{"evt-sfw"}, feedIDs(t, r),
		"rx-rated and Hentai-genre events must be hidden; the SFW event stays")
}

func TestActivityRepository_GetFeed_VisibilityAll_ShowsHentai(t *testing.T) {
	r := setupActivityTestDB(t)
	seedVisibilityFixtures(t, r)
	require.NoError(t, r.db.Exec(`INSERT INTO users (id, activity_visibility) VALUES ('open-user', 'all')`).Error)

	seedVisibilityEvent(t, r, "evt-rx", "open-user", "anime-rx", time.Now())

	assert.Equal(t, []string{"evt-rx"}, feedIDs(t, r))
}

func TestActivityRepository_GetFeed_MissingUsersRow_DefaultsToVisible(t *testing.T) {
	r := setupActivityTestDB(t)
	seedVisibilityFixtures(t, r)
	// No users row at all — LEFT JOIN + COALESCE must keep the event visible
	// (rows predating the column / orphaned events keep pre-feature behaviour).
	seedVisibilityEvent(t, r, "evt-orphan", "ghost-user", "anime-rx", time.Now())

	assert.Equal(t, []string{"evt-orphan"}, feedIDs(t, r))
}
