package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCompatTestDB returns an in-memory SQLite CompatibilityRepository with the
// table shapes ListEntries' Preload("Anime").Preload("Anime.Genres") chain
// needs: anime_list (base rows), animes (the Anime belongs-to target) and
// anime_genres + genres (the many2many). Tables are hand-rolled in raw SQL
// because the production GORM tags carry Postgres-only defaults
// (gen_random_uuid()) SQLite refuses to parse — same convention as
// progress_test.go / main_test.go.
func setupCompatTestDB(t *testing.T) (*CompatibilityRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")

	stmts := []string{
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			score INTEGER DEFAULT 0
		)`,
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			deleted_at DATETIME
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
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}
	return NewCompatibilityRepository(db), db
}

// TestCompatibilityRepository_ListEntries_CapsAtMax (audit L606) — a
// pathologically large watchlist must be capped at maxCompatEntries so the
// genre preload can't run unbounded. RED (no .Limit): returns all N rows;
// GREEN (.Limit(maxCompatEntries)): returns exactly maxCompatEntries.
func TestCompatibilityRepository_ListEntries_CapsAtMax(t *testing.T) {
	r, db := setupCompatTestDB(t)
	ctx := context.Background()

	// Seed maxCompatEntries + 50 rows for one user, each pointing at a distinct
	// anime so the Preload has rows to chase.
	total := maxCompatEntries + 50
	for i := 0; i < total; i++ {
		id := "al-" + itoa(i)
		animeID := "anime-" + itoa(i)
		require.NoError(t, db.Exec(
			`INSERT INTO anime_list (id, user_id, anime_id, score) VALUES (?, ?, ?, ?)`,
			id, "user-1", animeID, 7).Error)
		require.NoError(t, db.Exec(
			`INSERT INTO animes (id, name) VALUES (?, ?)`,
			animeID, "Anime "+itoa(i)).Error)
	}

	entries, err := r.ListEntries(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, entries, maxCompatEntries,
		"ListEntries must cap the unbounded watchlist at maxCompatEntries")
}

// TestCompatibilityRepository_ListEntries_UnderCapReturnsAll confirms the cap is
// a ceiling, not a floor — a small list returns every row with its genre IDs.
func TestCompatibilityRepository_ListEntries_UnderCapReturnsAll(t *testing.T) {
	r, db := setupCompatTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, score) VALUES (?, ?, ?, ?)`,
		"al-1", "user-1", "anime-1", 8).Error)
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name) VALUES (?, ?)`, "anime-1", "Anime One").Error)
	require.NoError(t, db.Exec(`INSERT INTO genres (id, name) VALUES (?, ?)`, "g1", "Action").Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?)`, "anime-1", "g1").Error)

	entries, err := r.ListEntries(ctx, "user-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "anime-1", entries[0].AnimeID)
	assert.Equal(t, 8, entries[0].Score)
	assert.Equal(t, []string{"g1"}, entries[0].GenreIDs, "genre IDs eagerly loaded via preload")
}
