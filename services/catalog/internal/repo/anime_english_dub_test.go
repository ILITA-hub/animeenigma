package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SetEnglishDub must move BOTH columns together. A verdict without a
// timestamp would be re-probed by the backfiller forever.
func TestSetEnglishDub_WritesVerdictAndStamp(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a1", "Cowboy Bebop")

	require.NoError(t, r.SetEnglishDub(context.Background(), "a1", true))

	var got struct {
		HasEnglishDub       bool
		EnglishDubCheckedAt *string
	}
	require.NoError(t, db.Raw(
		`SELECT has_english_dub, english_dub_checked_at FROM animes WHERE id = 'a1'`,
	).Scan(&got).Error)

	assert.True(t, got.HasEnglishDub, "verdict not written")
	assert.NotNil(t, got.EnglishDubCheckedAt, "checked_at not stamped")
}

// A false verdict is still a verdict: it must stamp too, or the title is
// re-probed on every tick.
func TestSetEnglishDub_FalseVerdictAlsoStamps(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a1", "Cowboy Bebop")

	require.NoError(t, r.SetEnglishDub(context.Background(), "a1", false))

	var got struct {
		HasEnglishDub       bool
		EnglishDubCheckedAt *string
	}
	require.NoError(t, db.Raw(
		`SELECT has_english_dub, english_dub_checked_at FROM animes WHERE id = 'a1'`,
	).Scan(&got).Error)

	assert.False(t, got.HasEnglishDub)
	assert.NotNil(t, got.EnglishDubCheckedAt, "checked_at not stamped on a false verdict")
}
