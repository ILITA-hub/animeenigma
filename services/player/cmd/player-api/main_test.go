package main

// Plan-01 test for SOCIAL-NF-02 (migration idempotency). Verification map row
// `01-Migrate-01` in `.planning/workstreams/social/phases/01-social-reviews-comments/01-VALIDATION.md`.
//
// What this test proves:
//   - First invocation of runSocialMigration against a fresh DB seeded with
//     reviews + anime_list copies every reviews row into anime_list with the
//     expected score/review_text/username semantics, and drops the reviews
//     table.
//   - Second invocation against the post-migration DB is a complete no-op:
//     the reviews table is still gone, anime_list count is unchanged, no
//     panic, no error.
//
// SQLite caveat: SQLite-in-memory does NOT support gen_random_uuid(). The
// helper detects the dialect via db.Dialector.Name() and substitutes a
// hex(randomblob(...)) expression for the new-row UUID column.
//
// This file lives in package `main` so it can reach the unexported helper
// runSocialMigration directly. The plan-00 SUMMARY notes this file is force-
// added past the `**/player-api` .gitignore glob.

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeUUID returns a 32-char hex string. SQLite does not validate UUID
// shape, so this is sufficient for primary-key uniqueness in the test
// fixtures without pulling github.com/google/uuid into the player module.
func fakeUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(b[:])
}

// setupSocialMigrationTestDB returns an in-memory SQLite DB with the schema
// shapes runSocialMigration expects: anime_list (post-Task-1.1 columns),
// reviews (legacy), and a minimal users table (not owned by the player
// service — fabricated in raw SQL so step B has something to JOIN against).
//
// Tables are created via raw SQL rather than AutoMigrate because the
// production GORM tags contain Postgres-specific defaults (gen_random_uuid(),
// now()) that SQLite refuses to parse. The shapes here mirror what
// `&domain.AnimeListEntry{}` and `&domain.Review{}` produce on Postgres, with
// the unique index `(user_id, anime_id)` that ON CONFLICT relies on.
func setupSocialMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")

	stmts := []string{
		// anime_list — post-Task-1.1 shape (review_text + username present).
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			status TEXT DEFAULT 'watching',
			score INTEGER DEFAULT 0,
			episodes INTEGER DEFAULT 0,
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
		// reviews — legacy shape (matches domain.Review).
		`CREATE TABLE reviews (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			username TEXT,
			score INTEGER DEFAULT 0,
			review_text TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
		// users stand-in — owned by the auth service in production. Only
		// the (id, username) columns matter for the step-B backfill JOIN.
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL
		)`,
	}
	for _, sql := range stmts {
		require.NoError(t, db.Exec(sql).Error, "create test schema")
	}

	return db
}

// testLogger returns a logger discarded into the test stream. Production code
// uses log.Infow / log.Fatalw — only the Info path is exercised here, so a
// regular WARN-or-above logger keeps test output quiet.
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	l, err := logger.New(logger.Config{
		Level:       "warn",
		Development: false,
		Encoding:    "json",
	})
	require.NoError(t, err, "construct test logger")
	return l
}

// TestSocialMigration_Idempotent validates SOCIAL-NF-02 end-to-end against an
// in-memory SQLite database: first invocation merges reviews into anime_list
// and drops `reviews`; second invocation is a complete no-op.
func TestSocialMigration_Idempotent(t *testing.T) {
	db := setupSocialMigrationTestDB(t)
	log := testLogger(t)

	// ---------- Seed ----------
	userA := fakeUUID(t)
	userB := fakeUUID(t)
	animeA := fakeUUID(t)
	animeB := fakeUUID(t)
	now := time.Now().UTC()

	// users — only userA has a row. userB intentionally absent so step B
	// can NOT backfill their username (the username they got from reviews
	// must win).
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username) VALUES (?, ?)`,
		userA, "userA_db",
	).Error, "seed users.userA")

	// Overlap row in anime_list: same (userA, animeA) the first review
	// will conflict against. Score=7 must be preserved (existing
	// watchlist score wins). review_text is empty pre-migration.
	require.NoError(t, db.Create(&domain.AnimeListEntry{
		ID:        fakeUUID(t),
		UserID:    userA,
		AnimeID:   animeA,
		Status:    "watching",
		Score:     7,
		Episodes:  0,
		Notes:     "",
		Tags:      "",
		CreatedAt: now,
		UpdatedAt: now,
	}).Error, "seed overlap anime_list row")

	// Two reviews: one overlap (userA / animeA) and one fresh
	// (userB / animeB) that has no matching anime_list row.
	require.NoError(t, db.Create(&domain.Review{
		ID:         fakeUUID(t),
		UserID:     userA,
		AnimeID:    animeA,
		Username:   "userA_review",
		Score:      9,
		ReviewText: "great show",
		CreatedAt:  now,
		UpdatedAt:  now,
	}).Error, "seed reviews.userA")
	require.NoError(t, db.Create(&domain.Review{
		ID:         fakeUUID(t),
		UserID:     userB,
		AnimeID:    animeB,
		Username:   "userB_review",
		Score:      5,
		ReviewText: "meh",
		CreatedAt:  now,
		UpdatedAt:  now,
	}).Error, "seed reviews.userB")

	// ---------- First invocation ----------
	require.NoError(t, runSocialMigration(db, log), "first runSocialMigration")

	// reviews table is gone.
	assert.False(t,
		db.Migrator().HasTable("reviews"),
		"reviews table should be dropped after first run",
	)

	// anime_list now has exactly 2 rows.
	var listCount int64
	require.NoError(t, db.Model(&domain.AnimeListEntry{}).Count(&listCount).Error)
	assert.EqualValues(t, 2, listCount, "anime_list row count after first run")

	// Overlap row: score=7 preserved (existing watchlist score wins);
	// review_text + username copied from reviews.
	var overlap domain.AnimeListEntry
	require.NoError(t,
		db.Where("user_id = ? AND anime_id = ?", userA, animeA).First(&overlap).Error,
		"fetch overlap row",
	)
	assert.Equal(t, 7, overlap.Score,
		"existing anime_list.score=7 must NOT be overwritten by reviews.score=9")
	assert.Equal(t, "great show", overlap.ReviewText, "overlap review_text")
	// userA had an existing users.username — step B may overwrite the
	// review-supplied username with the canonical users.username. Either
	// value is acceptable as long as it is non-empty.
	assert.NotEmpty(t, overlap.Username, "overlap username non-empty")

	// Fresh row (userB / animeB): created from scratch with status=completed.
	var fresh domain.AnimeListEntry
	require.NoError(t,
		db.Where("user_id = ? AND anime_id = ?", userB, animeB).First(&fresh).Error,
		"fetch fresh row",
	)
	assert.Equal(t, "completed", fresh.Status, "fresh row default status")
	assert.Equal(t, 5, fresh.Score, "fresh row inherits reviews.score=5")
	assert.Equal(t, "meh", fresh.ReviewText, "fresh row review_text")
	// userB has no users row — step-B backfill skipped them, so the
	// review-supplied username wins.
	assert.Equal(t, "userB_review", fresh.Username, "fresh row username from review")

	// ---------- Second invocation (idempotency) ----------
	require.NoError(t, runSocialMigration(db, log), "second runSocialMigration")

	// Still no reviews table.
	assert.False(t,
		db.Migrator().HasTable("reviews"),
		"reviews table stays dropped after second run",
	)

	// anime_list count unchanged (no double-copy).
	var listCountAfter int64
	require.NoError(t, db.Model(&domain.AnimeListEntry{}).Count(&listCountAfter).Error)
	assert.EqualValues(t, listCount, listCountAfter,
		"second invocation must NOT mutate anime_list row count")
}
