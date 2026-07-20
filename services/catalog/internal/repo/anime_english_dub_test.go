package repo

import (
	"context"
	"testing"
	"time"

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

// The candidate query must never return a title without an EN source: no EN
// source means no EN dub, and probing them would put ~4800 pointless calls on
// the wire.
func TestListEnglishDubCandidates_SkipsNonEnglishTitles(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "en", "Has EN source")
	seedBrowseAnime(t, db, "noen", "No EN source")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1 WHERE id='en'`).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "en", got[0].ID)
}

// Never-probed titles outrank stale ones.
func TestListEnglishDubCandidates_NeverProbedFirst(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "stale", "Probed long ago")
	seedBrowseAnime(t, db, "fresh", "Never probed")
	require.NoError(t, db.Exec(
		`UPDATE animes SET has_english=1, english_dub_checked_at=? WHERE id='stale'`,
		time.Now().UTC().Add(-90*24*time.Hour),
	).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1 WHERE id='fresh'`).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "fresh", got[0].ID, "never-probed must sort first")
}

// A recently probed released title is not a candidate.
func TestListEnglishDubCandidates_ExcludesFreshlyProbed(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "done", "Probed yesterday")
	require.NoError(t, db.Exec(
		`UPDATE animes SET has_english=1, status='released', english_dub_checked_at=? WHERE id='done'`,
		time.Now().UTC().Add(-24*time.Hour),
	).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Ongoing titles come back sooner than released ones — dubs ship after subs.
func TestListEnglishDubCandidates_OngoingRecheckedSooner(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "ong", "Ongoing probed 10 days ago")
	require.NoError(t, db.Exec(
		`UPDATE animes SET has_english=1, status='ongoing', english_dub_checked_at=? WHERE id='ong'`,
		time.Now().UTC().Add(-10*24*time.Hour),
	).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "ong", got[0].ID)
}

// An unreachable probe must still stamp, or the same title is retried on
// every tick and the loop never rotates.
func TestTouchEnglishDubChecked_StampsWithoutChangingVerdict(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a1", "Unreachable")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1, has_english_dub=1 WHERE id='a1'`).Error)

	require.NoError(t, r.TouchEnglishDubChecked(context.Background(), "a1"))

	var got struct {
		HasEnglishDub       bool
		EnglishDubCheckedAt *string
	}
	require.NoError(t, db.Raw(
		`SELECT has_english_dub, english_dub_checked_at FROM animes WHERE id = 'a1'`,
	).Scan(&got).Error)
	assert.True(t, got.HasEnglishDub, "verdict must be preserved")
	assert.NotNil(t, got.EnglishDubCheckedAt)
}

func TestCountEnglishDubUnchecked(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a", "unchecked")
	seedBrowseAnime(t, db, "b", "checked")
	seedBrowseAnime(t, db, "c", "no en source")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1 WHERE id IN ('a','b')`).Error)
	require.NoError(t, db.Exec(
		`UPDATE animes SET english_dub_checked_at=? WHERE id='b'`, time.Now().UTC(),
	).Error)

	n, err := r.CountEnglishDubUnchecked(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}
