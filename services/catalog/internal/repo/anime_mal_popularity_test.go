package repo

// Test for AnimeRepository.SetMALPopularity — the targeted popularity update
// that feeds the recs relative-MAL-popularity signal for announced titles. It
// must write mal_members/mal_favorites and leave every other column intact
// (same no-clobber contract as SetFranchise). In-memory SQLite + raw DDL,
// matching the anime_update_test.go pattern.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMALPopularityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		name TEXT,
		franchise TEXT,
		franchise_checked INTEGER DEFAULT 0,
		mal_members INTEGER DEFAULT 0,
		mal_favorites INTEGER DEFAULT 0,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error)
	return db
}

func TestSetMALPopularity_WritesAndPreservesOtherColumns(t *testing.T) {
	db := setupMALPopularityTestDB(t)
	repo := NewAnimeRepository(db)
	ctx := context.Background()

	// Seed a row with a franchise already set — must survive the update.
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, franchise, franchise_checked) VALUES (?,?,?,1)`,
		"a1", "Witch Watch 2nd Season", "witch-watch",
	).Error)

	require.NoError(t, repo.SetMALPopularity(ctx, "a1", 424242, 1717))

	var row struct {
		MalMembers       int
		MalFavorites     int
		Franchise        string
		FranchiseChecked bool
		Name             string
	}
	require.NoError(t, db.Raw(
		`SELECT mal_members, mal_favorites, franchise, franchise_checked, name FROM animes WHERE id = ?`,
		"a1",
	).Scan(&row).Error)

	require.Equal(t, 424242, row.MalMembers)
	require.Equal(t, 1717, row.MalFavorites)
	// No-clobber: unrelated columns untouched.
	require.Equal(t, "witch-watch", row.Franchise)
	require.True(t, row.FranchiseChecked)
	require.Equal(t, "Witch Watch 2nd Season", row.Name)
}
