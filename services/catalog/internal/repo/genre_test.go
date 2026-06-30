package repo

// Tests for GenreRepository.ReplaceAnimeGenresBatch — the bulk join-table
// rewrite used by BatchRefreshAnime. The old per-anime SetAnimeGenres issued
// four statements per anime (SELECT anime, SELECT genres, DELETE join, INSERT
// join); the batch variant collapses a whole chunk of anime into two
// statements (one DELETE over all anime IDs, one batched INSERT of every link)
// and assumes the referenced genres already exist (callers upsert first).
//
// Uses in-memory SQLite with raw DDL, matching anime_update_test.go.

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGenreJoinTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, genre_id)
	)`).Error)

	return db
}

// linksFor reads the current genre IDs linked to an anime, sorted for stable
// comparison.
func linksFor(t *testing.T, db *gorm.DB, animeID string) []string {
	t.Helper()
	var ids []string
	require.NoError(t, db.Raw(
		"SELECT genre_id FROM anime_genres WHERE anime_id = ?", animeID,
	).Scan(&ids).Error)
	sort.Strings(ids)
	return ids
}

func seedLink(t *testing.T, db *gorm.DB, animeID, genreID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?)", animeID, genreID,
	).Error)
}

func TestReplaceAnimeGenresBatch_ReplacesAndIsolates(t *testing.T) {
	db := setupGenreJoinTestDB(t)
	r := NewGenreRepository(db)
	ctx := context.Background()

	// Pre-existing links: A→{1,2}, B→{3}, C→{5}.
	seedLink(t, db, "A", "1")
	seedLink(t, db, "A", "2")
	seedLink(t, db, "B", "3")
	seedLink(t, db, "C", "5")

	// Replace links for A and B only. C is NOT in the map and must be untouched.
	// A's set changes (drop 1, keep 2, add 4); B is cleared (empty slice).
	err := r.ReplaceAnimeGenresBatch(ctx, map[string][]string{
		"A": {"2", "4"},
		"B": {},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"2", "4"}, linksFor(t, db, "A"), "A links not replaced")
	assert.Empty(t, linksFor(t, db, "B"), "B links not cleared by empty slice")
	assert.Equal(t, []string{"5"}, linksFor(t, db, "C"), "C (not in map) was clobbered")
}

func TestReplaceAnimeGenresBatch_DeduplicatesPairs(t *testing.T) {
	db := setupGenreJoinTestDB(t)
	r := NewGenreRepository(db)
	ctx := context.Background()

	// Same (anime, genre) pair supplied twice must not violate the composite PK.
	err := r.ReplaceAnimeGenresBatch(ctx, map[string][]string{
		"A": {"1", "1", "2"},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"1", "2"}, linksFor(t, db, "A"))
}

func TestReplaceAnimeGenresBatch_Empty(t *testing.T) {
	db := setupGenreJoinTestDB(t)
	r := NewGenreRepository(db)
	ctx := context.Background()

	require.NoError(t, r.ReplaceAnimeGenresBatch(ctx, nil))
	require.NoError(t, r.ReplaceAnimeGenresBatch(ctx, map[string][]string{}))
}
