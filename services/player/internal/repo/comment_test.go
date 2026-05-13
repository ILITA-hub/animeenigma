package repo

// Plan-03 tests for SOCIAL-04d / SOCIAL-04e (`01-VALIDATION.md` rows
// 01-Comment-04 / 01-Comment-05). The Wave-0 SKIP stubs are replaced with
// real assertions.
//
// SQLite caveat: SQLite-in-memory does NOT support `gen_random_uuid()` or
// `now()`. AutoMigrate(&domain.Comment{}) emits both defaults and fails to
// parse on SQLite. Tables are therefore created via raw SQL with shapes
// that mirror the production Postgres schema, using
// `lower(hex(randomblob(16)))` as the id default — the same pattern Plan 02
// adopted for `activity_events` in service/review_test.go.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/pagination"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newUUIDHex returns a 32-char hex string suitable as a SQLite TEXT
// primary key. Mirrors `fakeUUID` in cmd/player-api/main_test.go to avoid
// pulling github.com/google/uuid into the player module.
func newUUIDHex(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(b[:])
}

// setupCommentTestDB returns an in-memory SQLite DB with the `comments`
// table created via raw SQL (AutoMigrate fails for the Comment struct
// because `default:gen_random_uuid()` / `default:now()` are Postgres-only).
func setupCommentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")

	stmts := []string{
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
		`CREATE INDEX idx_comments_anime_created ON comments (anime_id, created_at DESC)`,
		`CREATE INDEX idx_comments_user_created  ON comments (user_id,  created_at DESC)`,
		`CREATE INDEX idx_comments_deleted_at    ON comments (deleted_at)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error, "exec: %s", s)
	}
	return db
}

// seedComment inserts a row with the given anime/user/body/createdAt and
// returns the ID. SQLite has 1-second timestamp resolution on
// CURRENT_TIMESTAMP, so callers pass explicit times to keep the
// newest-first ordering stable.
func seedComment(t *testing.T, db *gorm.DB, userID, animeID, body string, createdAt time.Time) string {
	t.Helper()
	id := newUUIDHex(t)
	c := &domain.Comment{
		ID:        id,
		UserID:    userID,
		AnimeID:   animeID,
		Username:  "tester",
		Body:      body,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	require.NoError(t, db.Create(c).Error)
	return id
}

// TestCommentRepo_SoftDelete validates SOCIAL-04d: SoftDelete sets
// deleted_at, the row remains in the table, and ListByAnime / GetByID
// exclude it.
func TestCommentRepo_SoftDelete(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := NewCommentRepository(db)
	ctx := context.Background()

	animeID := newUUIDHex(t)
	t0 := time.Now().UTC().Truncate(time.Second)
	id1 := seedComment(t, db, newUUIDHex(t), animeID, "first",  t0.Add(-2*time.Second))
	id2 := seedComment(t, db, newUUIDHex(t), animeID, "second", t0.Add(-1*time.Second))

	// Soft-delete comment 1.
	require.NoError(t, repo.SoftDelete(ctx, id1))

	// Raw `Unscoped` fetch — deleted_at should now be non-NULL on the row.
	var raw domain.Comment
	require.NoError(t, db.Unscoped().Where("id = ?", id1).First(&raw).Error)
	assert.True(t, raw.DeletedAt.Valid, "deleted_at should be set after SoftDelete")

	// ListByAnime must omit the soft-deleted row entirely.
	got, nextCursor, err := repo.ListByAnime(ctx, animeID, "", 50)
	require.NoError(t, err)
	require.Len(t, got, 1, "ListByAnime excludes soft-deleted rows")
	assert.Equal(t, id2, got[0].ID, "only the surviving row appears")
	assert.Empty(t, nextCursor, "no next page expected")

	// GetByID on the soft-deleted row must return errors.NotFound.
	_, err = repo.GetByID(ctx, id1)
	require.Error(t, err)
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok, "expected AppError, got %T: %v", err, err)
	assert.Equal(t, apperrors.CodeNotFound, appErr.Code)

	// SoftDelete on the already-deleted row is a no-op (idempotent).
	require.NoError(t, repo.SoftDelete(ctx, id1))

	// SoftDelete on a non-existent id is also a no-op.
	require.NoError(t, repo.SoftDelete(ctx, newUUIDHex(t)))
}

// TestCommentRepo_ListByAnime_Cursor validates SOCIAL-04e: cursor
// pagination returns the next slice in newest-first order and the
// opaque base64 cursor round-trips correctly.
func TestCommentRepo_ListByAnime_Cursor(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := NewCommentRepository(db)
	ctx := context.Background()

	animeID := newUUIDHex(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Seed 5 comments with monotonically increasing created_at (newest =
	// last seeded). Newest-first ordering means the result order is the
	// inverse of seeding.
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		ids[i] = seedComment(
			t, db, newUUIDHex(t), animeID,
			"body-"+string(rune('A'+i)),
			now.Add(time.Duration(i-5)*time.Second), // now-5s, now-4s, ..., now-1s
		)
	}
	// Newest-first expected order is the reverse of `ids`:
	expectedOrder := []string{ids[4], ids[3], ids[2], ids[1], ids[0]}

	// First page: limit 3.
	page1, cursor1, err := repo.ListByAnime(ctx, animeID, "", 3)
	require.NoError(t, err)
	require.Len(t, page1, 3, "first page returns 3 rows")
	assert.Equal(t, expectedOrder[:3], idsOf(page1), "newest-first order")
	require.NotEmpty(t, cursor1, "first page must emit a next-cursor since 5 > 3")

	// Verify creation timestamps are strictly newest-first across the page.
	assert.True(t, page1[0].CreatedAt.After(page1[1].CreatedAt))
	assert.True(t, page1[1].CreatedAt.After(page1[2].CreatedAt))

	// Round-trip: decode cursor1 and assert it points at the 3rd result on
	// page 1 (the last visible row on that page).
	decoded, err := pagination.DecodeCursor(cursor1)
	require.NoError(t, err)
	require.NotNil(t, decoded)
	assert.Equal(t, page1[2].ID, decoded.ID, "cursor.ID points at last row on page 1")
	assert.True(t,
		decoded.Timestamp.Equal(page1[2].CreatedAt),
		"cursor.Timestamp matches last row's created_at: %v vs %v",
		decoded.Timestamp, page1[2].CreatedAt,
	)

	// Second page: pass cursor1, limit 3, expect the remaining 2 rows.
	page2, cursor2, err := repo.ListByAnime(ctx, animeID, cursor1, 3)
	require.NoError(t, err)
	require.Len(t, page2, 2, "second page returns the remaining 2 rows")
	assert.Equal(t, expectedOrder[3:], idsOf(page2))
	assert.Empty(t, cursor2, "no next page expected when results <= limit")

	// Invalid cursor → errors.InvalidInput.
	_, _, err = repo.ListByAnime(ctx, animeID, "!!!not-base64!!!", 3)
	require.Error(t, err)
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperrors.CodeInvalidInput, appErr.Code)
}

func idsOf(cs []*domain.Comment) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
