package repo

// Tests for Phase 1 (workstream: social) plan 02 — the review-shaped queries
// that power the six review endpoints (now backed by the unified
// anime_list table). Each test pre-creates an `anime_list` row via raw SQL
// (matching the production SQLite-portable schema used in the migration
// test) and exercises the new methods against an in-memory database.

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupListReviewTestDB returns an in-memory SQLite DB populated with the
// `anime_list` schema (including the Phase 1 social columns review_text +
// username) and an empty `animes` table so Preload("Anime") doesn't blow up.
func setupListReviewTestDB(t *testing.T) *gorm.DB {
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
		`CREATE TABLE anime_genres (
			anime_id TEXT,
			genre_id TEXT
		)`,
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
			priority TEXT,
			mal_id INTEGER,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
	}
	for _, ddl := range stmts {
		require.NoError(t, db.Exec(ddl).Error)
	}
	return db
}

// seedListEntry inserts a row directly via raw SQL.
func seedListEntry(t *testing.T, db *gorm.DB, e domain.AnimeListEntry) {
	t.Helper()
	if e.ID == "" {
		e.ID = e.UserID + "-" + e.AnimeID // deterministic, fine for tests
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = e.CreatedAt
	}
	require.NoError(t, db.Exec(`
		INSERT INTO anime_list (
			id, user_id, anime_id, status, score, episodes,
			notes, tags, review_text, username, is_rewatching,
			priority, mal_id, started_at, completed_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.AnimeID, e.Status, e.Score, e.Episodes,
		e.Notes, e.Tags, e.ReviewText, e.Username, e.IsRewatching,
		e.Priority, e.MalID, e.StartedAt, e.CompletedAt,
		e.CreatedAt, e.UpdatedAt,
	).Error)
}

// TestListRepo_GetReviewsByAnime_IncludesScoreOnlyRows — proves the filter
// `(score > 0 OR review_text != '')` so MAL-imported `score=8` rows surface
// alongside written-review rows.
func TestListRepo_GetReviewsByAnime_IncludesScoreOnlyRows(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	animeID := "anime-1"
	// row 1: score-only (MAL-imported style — no written review)
	seedListEntry(t, db, domain.AnimeListEntry{
		UserID: "user-A", AnimeID: animeID,
		Status: "completed", Score: 8, ReviewText: "",
	})
	// row 2: review-only (score=0, but has text — the legacy "wrote a review without scoring" case)
	seedListEntry(t, db, domain.AnimeListEntry{
		UserID: "user-B", AnimeID: animeID,
		Status: "completed", Score: 0, ReviewText: "great",
	})
	// row 3: score=0 AND empty review — should be FILTERED OUT
	seedListEntry(t, db, domain.AnimeListEntry{
		UserID: "user-C", AnimeID: animeID,
		Status: "plan_to_watch", Score: 0, ReviewText: "",
	})

	entries, err := r.GetReviewsByAnime(ctx, animeID)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "score-only AND review-only rows both included; empty-on-both excluded")

	// Confirm both user-A (score-only) and user-B (review-only) appear.
	users := map[string]bool{}
	for _, e := range entries {
		users[e.UserID] = true
	}
	assert.True(t, users["user-A"], "score-only row appears")
	assert.True(t, users["user-B"], "review-only row appears")
	assert.False(t, users["user-C"], "empty-on-both row is excluded")
}

// TestListRepo_UpsertReview_PreservesExistingWatchlistFields — UpsertReview
// must NOT clobber status/episodes/notes/tags on a row that already exists in
// the watchlist (e.g. status='watching', episodes=5).
func TestListRepo_UpsertReview_PreservesExistingWatchlistFields(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	userID := "user-A"
	animeID := "anime-1"
	seedListEntry(t, db, domain.AnimeListEntry{
		UserID: userID, AnimeID: animeID,
		Status: "watching", Episodes: 5, Notes: "foo", Tags: "fav",
		Score: 0, ReviewText: "", Username: "",
	})

	got, err := r.UpsertReview(ctx, userID, animeID, "bob", 9, "cool")
	require.NoError(t, err)
	require.NotNil(t, got)

	// Reload from DB.
	var row domain.AnimeListEntry
	require.NoError(t, db.Where("user_id = ? AND anime_id = ?", userID, animeID).First(&row).Error)

	assert.Equal(t, "watching", row.Status, "status preserved")
	assert.Equal(t, 5, row.Episodes, "episodes preserved")
	assert.Equal(t, "foo", row.Notes, "notes preserved")
	assert.Equal(t, "fav", row.Tags, "tags preserved")
	assert.Equal(t, 9, row.Score, "score updated to new value")
	assert.Equal(t, "cool", row.ReviewText, "review_text updated")
	assert.Equal(t, "bob", row.Username, "username updated")
}

// TestListRepo_UpsertReview_CreatesRowWhenAbsent — when no row exists yet,
// UpsertReview must create one with status='completed' and the supplied
// score/review_text/username.
func TestListRepo_UpsertReview_CreatesRowWhenAbsent(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	got, err := r.UpsertReview(ctx, "user-X", "anime-X", "carol", 8, "nice")
	require.NoError(t, err)
	require.NotNil(t, got)

	var row domain.AnimeListEntry
	require.NoError(t, db.Where("user_id = ? AND anime_id = ?", "user-X", "anime-X").First(&row).Error)
	assert.Equal(t, "completed", row.Status, "new row defaults to status=completed")
	assert.Equal(t, 8, row.Score)
	assert.Equal(t, "nice", row.ReviewText)
	assert.Equal(t, "carol", row.Username)
}

// TestListRepo_ClearReview_PreservesRow — DELETE /api/anime/:id/reviews
// must set score=0 + review_text='' but the anime_list row STAYS (it just
// drops out of the public reviews filter).
func TestListRepo_ClearReview_PreservesRow(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	userID := "user-A"
	animeID := "anime-1"
	_, err := r.UpsertReview(ctx, userID, animeID, "bob", 9, "cool")
	require.NoError(t, err)

	// Now clear.
	require.NoError(t, r.ClearReview(ctx, userID, animeID))

	// Row still exists.
	var row domain.AnimeListEntry
	err = db.Where("user_id = ? AND anime_id = ?", userID, animeID).First(&row).Error
	require.NoError(t, err, "row must still exist after ClearReview")
	assert.Equal(t, 0, row.Score, "score cleared to 0")
	assert.Equal(t, "", row.ReviewText, "review_text cleared to ''")
}

// TestListRepo_ClearReview_NoMatchIsNoOp — calling ClearReview on a missing
// row must not error.
func TestListRepo_ClearReview_NoMatchIsNoOp(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	err := r.ClearReview(ctx, "nope", "nope")
	assert.NoError(t, err, "ClearReview on missing row is idempotent no-op")
}

// TestListRepo_GetAnimeRating_ExcludesZeroScores — avg/count must only
// consider rows with score>0.
func TestListRepo_GetAnimeRating_ExcludesZeroScores(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	animeID := "anime-1"
	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u1", AnimeID: animeID, Score: 7})
	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u2", AnimeID: animeID, Score: 0, ReviewText: "no score but text"})
	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u3", AnimeID: animeID, Score: 9})

	rating, err := r.GetAnimeRating(ctx, animeID)
	require.NoError(t, err)
	require.NotNil(t, rating)
	assert.Equal(t, animeID, rating.AnimeID)
	assert.InDelta(t, 8.0, rating.AverageScore, 0.0001, "avg of {7,9} = 8")
	assert.Equal(t, 2, rating.TotalReviews, "count excludes score=0 rows")
}

// TestListRepo_GetUserReview_ReturnsRow — happy path: existing row with
// either score>0 or review_text!='' returns the entry.
func TestListRepo_GetUserReview_ReturnsRow(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	userID := "user-A"
	animeID := "anime-1"
	seedListEntry(t, db, domain.AnimeListEntry{
		UserID: userID, AnimeID: animeID, Score: 8, ReviewText: "good",
	})

	got, err := r.GetUserReview(ctx, userID, animeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 8, got.Score)
	assert.Equal(t, "good", got.ReviewText)
}

// TestListRepo_GetUserReview_NotFoundWhenEmpty — score=0 AND review_text=''
// must surface as a NotFound (an existing row with no review content is
// treated the same as "no review yet" from the API perspective).
func TestListRepo_GetUserReview_NotFoundWhenEmpty(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	seedListEntry(t, db, domain.AnimeListEntry{
		UserID: "user-A", AnimeID: "anime-1", Score: 0, ReviewText: "",
	})

	got, err := r.GetUserReview(ctx, "user-A", "anime-1")
	assert.Nil(t, got, "empty-on-both row returns nil")
	assert.Error(t, err, "and an error (NotFound)")
}

// TestListRepo_GetBatchAnimeRatings — aggregates per anime_id; excludes
// score=0 rows.
func TestListRepo_GetBatchAnimeRatings(t *testing.T) {
	db := setupListReviewTestDB(t)
	r := NewListRepository(db)
	ctx := context.Background()

	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u1", AnimeID: "a1", Score: 6})
	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u2", AnimeID: "a1", Score: 8})
	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u3", AnimeID: "a2", Score: 10})
	seedListEntry(t, db, domain.AnimeListEntry{UserID: "u4", AnimeID: "a2", Score: 0, ReviewText: "no score"})

	got, err := r.GetBatchAnimeRatings(ctx, []string{"a1", "a2", "a3"})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Contains(t, got, "a1")
	require.Contains(t, got, "a2")
	assert.NotContains(t, got, "a3", "anime with no scoring rows not in map")

	assert.InDelta(t, 7.0, got["a1"].AverageScore, 0.0001)
	assert.Equal(t, 2, got["a1"].TotalReviews)
	assert.InDelta(t, 10.0, got["a2"].AverageScore, 0.0001)
	assert.Equal(t, 1, got["a2"].TotalReviews)
}
