package service

// Tests for Phase 1 (workstream: social) plan 02 — the refactored
// ReviewService that consumes ListRepository. Validates the activity-event
// dedup contract and the DELETE -> "clear without removing" semantics.

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupReviewServiceTestDB builds the SQLite schema needed by ReviewService:
// anime_list (Phase 1 columns), activity_events, and an empty animes table
// so Preload("Anime") doesn't blow up.
func setupReviewServiceTestDB(t *testing.T) (*ReviewService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	stmts := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT,
			name_jp TEXT,
			poster_url TEXT,
			episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0
		)`,
		`CREATE TABLE genres (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT
		)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			status TEXT DEFAULT 'watching',
			score INTEGER DEFAULT 0,
			episodes INTEGER NOT NULL DEFAULT 0,
			notes TEXT,
			tags TEXT,
			review_text TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			is_rewatching INTEGER DEFAULT 0,
			rewatch_count INTEGER DEFAULT 0,
			priority TEXT,
			mal_id INTEGER,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
		// SQLite lacks gen_random_uuid(); production Postgres assigns the id
		// default. For these tests we make `id` default to a hex-randomblob
		// expression so the Update path (which uses db.Model(event).Updates)
		// has a non-empty primary key to filter on.
		`CREATE TABLE activity_events (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT,
			username TEXT,
			anime_id TEXT,
			type TEXT,
			old_value TEXT,
			new_value TEXT,
			content TEXT,
			created_at DATETIME,
			deleted_at DATETIME
		)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}

	log, err := logger.New(logger.Config{Level: "error", Development: false, Encoding: "json"})
	require.NoError(t, err)
	listRepo := repo.NewListRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	return NewReviewService(listRepo, activityRepo, log), db
}

// activityRowCount returns the number of activity_events rows of the given
// type for a (user, anime) pair.
func activityRowCount(t *testing.T, db *gorm.DB, userID, animeID, eventType string) int64 {
	t.Helper()
	var c int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM activity_events
		 WHERE user_id = ? AND anime_id = ? AND type = ?`,
		userID, animeID, eventType,
	).Scan(&c).Error)
	return c
}

// TestReviewService_CreateOrUpdateReview_EmitsActivityOnce — a single
// create+activity insertion path.
func TestReviewService_CreateOrUpdateReview_EmitsActivityOnce(t *testing.T) {
	svc, db := setupReviewServiceTestDB(t)
	ctx := context.Background()

	got, err := svc.CreateOrUpdateReview(ctx, "user-A", "alice", &domain.CreateReviewRequest{
		AnimeID:    "anime-1",
		Score:      8,
		ReviewText: "loved it",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 8, got.Score)
	assert.Equal(t, "loved it", got.ReviewText)
	assert.Equal(t, "alice", got.Username)

	assert.EqualValues(t, 1, activityRowCount(t, db, "user-A", "anime-1", "review"))
}

// TestReviewService_CreateOrUpdateReview_DedupsWithinSameDay — a second
// call same day must UPDATE the existing activity event in place, not
// create a second one.
func TestReviewService_CreateOrUpdateReview_DedupsWithinSameDay(t *testing.T) {
	svc, db := setupReviewServiceTestDB(t)
	ctx := context.Background()

	_, err := svc.CreateOrUpdateReview(ctx, "user-A", "alice", &domain.CreateReviewRequest{
		AnimeID: "anime-1", Score: 7, ReviewText: "ok",
	})
	require.NoError(t, err)

	// Same day, same (user, anime) — should DEDUP.
	_, err = svc.CreateOrUpdateReview(ctx, "user-A", "alice", &domain.CreateReviewRequest{
		AnimeID: "anime-1", Score: 9, ReviewText: "actually amazing",
	})
	require.NoError(t, err)

	assert.EqualValues(t, 1, activityRowCount(t, db, "user-A", "anime-1", "review"),
		"second create same day must dedup to single activity row")

	// New value should reflect the latest review.
	var newValue string
	require.NoError(t, db.Raw(
		`SELECT new_value FROM activity_events WHERE user_id = ? AND anime_id = ? AND type = 'review'`,
		"user-A", "anime-1",
	).Scan(&newValue).Error)
	assert.Equal(t, "9", newValue, "activity event reflects the most recent score")
}

// TestReviewService_DeleteReview_ClearsScoreAndText — DELETE must set
// score=0 + review_text='' on the existing row but the row stays.
func TestReviewService_DeleteReview_ClearsScoreAndText(t *testing.T) {
	svc, db := setupReviewServiceTestDB(t)
	ctx := context.Background()

	_, err := svc.CreateOrUpdateReview(ctx, "user-A", "alice", &domain.CreateReviewRequest{
		AnimeID: "anime-1", Score: 8, ReviewText: "good",
	})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteReview(ctx, "user-A", "anime-1"))

	// Row must still exist.
	var row domain.AnimeListEntry
	require.NoError(t, db.Where("user_id = ? AND anime_id = ?", "user-A", "anime-1").First(&row).Error,
		"row stays in anime_list after DELETE")
	assert.Equal(t, 0, row.Score, "score cleared")
	assert.Equal(t, "", row.ReviewText, "review_text cleared")
}

// TestReviewService_CreateOrUpdateReview_ScoreValidation — out-of-range
// scores return an InvalidInput error, no row written, no activity.
func TestReviewService_CreateOrUpdateReview_ScoreValidation(t *testing.T) {
	svc, db := setupReviewServiceTestDB(t)
	ctx := context.Background()

	for _, bad := range []int{0, -1, 11, 100} {
		_, err := svc.CreateOrUpdateReview(ctx, "user-A", "alice", &domain.CreateReviewRequest{
			AnimeID: "anime-1", Score: bad, ReviewText: "x",
		})
		assert.Error(t, err, "score=%d must be rejected", bad)
	}
	assert.EqualValues(t, 0, activityRowCount(t, db, "user-A", "anime-1", "review"))
}
