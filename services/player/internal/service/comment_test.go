package service

// Plan-03 tests for SOCIAL-04f (rate limit) and SOCIAL-05 (activity event
// emission). Maps to `01-VALIDATION.md` rows 01-Comment-06 and
// 01-Activity-01.
//
// SQLite caveat: AutoMigrate(&domain.Comment{}) fails on SQLite because
// the production GORM tags carry Postgres-only defaults
// (`default:gen_random_uuid()` / `default:now()`). The schema is therefore
// created via raw SQL, mirroring the production shape but with
// `lower(hex(randomblob(16)))` as the id default — same pattern Plan 02
// adopted for `activity_events` in service/review_test.go.

import (
	"context"
	"strings"
	"testing"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCommentServiceTestDB builds the SQLite schema needed by
// CommentService: `comments` and `activity_events`. Both tables get a
// `randomblob(16)` id default so any flow that doesn't pre-assign IDs
// (Create followed by an Update on the cached row) still works.
func setupCommentServiceTestDB(t *testing.T) (*CommentService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")

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
		`CREATE TABLE comments (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			username TEXT,
			body TEXT NOT NULL,
			parent_id TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME
		)`,
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
	commentRepo := repo.NewCommentRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	return NewCommentService(commentRepo, activityRepo, log), db
}

// activityCommentRowCount returns the number of activity_events rows of
// type='comment' for a (user, anime) pair.
func activityCommentRowCount(t *testing.T, db *gorm.DB, userID, animeID string) int64 {
	t.Helper()
	var c int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM activity_events
		 WHERE user_id = ? AND anime_id = ? AND type = 'comment'`,
		userID, animeID,
	).Scan(&c).Error)
	return c
}

// TestCommentService_RateLimit validates SOCIAL-04f: the 11th
// CreateComment inside an hour for the same (user, anime) pair returns
// errors.RateLimited(). Isolation by user and by anime is also asserted.
func TestCommentService_RateLimit(t *testing.T) {
	svc, _ := setupCommentServiceTestDB(t)
	ctx := context.Background()

	const animeA = "anime-A"
	const animeB = "anime-B"
	const userU1 = "user-1"
	const userU2 = "user-2"

	// 10 successful creates for (userU1, animeA).
	for i := 0; i < 10; i++ {
		_, err := svc.CreateComment(ctx, userU1, "alice", animeA, &domain.CreateCommentRequest{
			Body: "hi",
		})
		require.NoError(t, err, "call #%d should succeed", i+1)
	}

	// 11th call → errors.RateLimited.
	_, err := svc.CreateComment(ctx, userU1, "alice", animeA, &domain.CreateCommentRequest{
		Body: "hi",
	})
	require.Error(t, err, "11th call must be rate-limited")
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok, "expected AppError, got %T: %v", err, err)
	assert.Equal(t, apperrors.CodeRateLimited, appErr.Code)

	// Isolation: a different user on the same anime is not rate-limited.
	_, err = svc.CreateComment(ctx, userU2, "bob", animeA, &domain.CreateCommentRequest{
		Body: "hi",
	})
	require.NoError(t, err, "different user must not be rate-limited by user-1's bucket")

	// Isolation: same user on a different anime is not rate-limited.
	_, err = svc.CreateComment(ctx, userU1, "alice", animeB, &domain.CreateCommentRequest{
		Body: "hi",
	})
	require.NoError(t, err, "different anime must not be rate-limited by user-1+anime-A bucket")
}

// TestCommentService_EmitsActivity validates SOCIAL-05: every successful
// CreateComment writes exactly one activity_events row with type='comment'
// and the truncated content preview (≤ 300 runes + "…" suffix). NO per-day
// dedup — every create emits a distinct row.
func TestCommentService_EmitsActivity(t *testing.T) {
	svc, db := setupCommentServiceTestDB(t)
	ctx := context.Background()

	const userID = "user-1"
	const animeID = "anime-1"

	// First create — body fits comfortably under 300 runes.
	_, err := svc.CreateComment(ctx, userID, "alice", animeID, &domain.CreateCommentRequest{
		Body: "hello world",
	})
	require.NoError(t, err)

	assert.EqualValues(t, 1, activityCommentRowCount(t, db, userID, animeID),
		"first create emits one activity row")

	// Inspect the row directly.
	var first struct {
		Type     string
		Content  string
		Username string
	}
	require.NoError(t, db.Raw(
		`SELECT type, content, username FROM activity_events
		 WHERE user_id = ? AND anime_id = ?`,
		userID, animeID,
	).Scan(&first).Error)
	assert.Equal(t, "comment", first.Type)
	assert.Equal(t, "hello world", first.Content,
		"short body is stored verbatim (no truncation)")
	assert.Equal(t, "alice", first.Username)

	// Second create — same user, same anime, same day → MUST emit a SECOND
	// row. Comments do not dedup (this is the divergence from reviews).
	_, err = svc.CreateComment(ctx, userID, "alice", animeID, &domain.CreateCommentRequest{
		Body: "second comment",
	})
	require.NoError(t, err)

	assert.EqualValues(t, 2, activityCommentRowCount(t, db, userID, animeID),
		"comment activity events do NOT dedup; second create emits second row")

	// Third create — body of 350 'a's. Content preview must be exactly 300
	// 'a's + the "…" rune (rune count = 301).
	long := strings.Repeat("a", 350)
	_, err = svc.CreateComment(ctx, userID, "alice", animeID, &domain.CreateCommentRequest{
		Body: long,
	})
	require.NoError(t, err)

	assert.EqualValues(t, 3, activityCommentRowCount(t, db, userID, animeID))

	// Fetch the most recent row (created_at DESC) and assert its content
	// preview shape.
	var lastContent string
	require.NoError(t, db.Raw(
		`SELECT content FROM activity_events
		 WHERE user_id = ? AND anime_id = ?
		 ORDER BY created_at DESC, id DESC LIMIT 1`,
		userID, animeID,
	).Scan(&lastContent).Error)

	runes := []rune(lastContent)
	assert.Equal(t, 301, len(runes),
		"truncated preview has exactly 300 body runes + 1 '…' = 301 runes")
	assert.True(t, strings.HasSuffix(lastContent, "…"),
		"truncated preview ends with the '…' rune")
	// Body portion is exactly 300 'a's.
	assert.Equal(t, strings.Repeat("a", 300), string(runes[:300]),
		"body portion is exactly 300 'a's")
}
