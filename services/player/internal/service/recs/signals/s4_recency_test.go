package signals

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupS4TestDB creates an in-memory SQLite DB with a minimal animes table
// shaped to match the production columns S4 reads (id, status, aired_on,
// hidden, deleted_at). Returns the gorm.DB so tests can seed via raw inserts.
func setupS4TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		status TEXT,
		aired_on DATETIME,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	return db
}

// seedAnime inserts a row directly into the animes table.
func seedAnime(t *testing.T, db *gorm.DB, id, status string, airedOn *time.Time) {
	t.Helper()
	if airedOn != nil {
		require.NoError(t, db.Exec(
			`INSERT INTO animes (id, status, aired_on, hidden) VALUES (?, ?, ?, 0)`,
			id, status, *airedOn,
		).Error)
		return
	}
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, status, aired_on, hidden) VALUES (?, ?, NULL, 0)`,
		id, status,
	).Error)
}

func TestS4Recency_ID(t *testing.T) {
	db := setupS4TestDB(t)
	s4 := NewS4Recency(db)
	assert.Equal(t, recs.SignalID("s4"), s4.ID())
}

func TestS4Recency_PrecomputeIsNoop(t *testing.T) {
	db := setupS4TestDB(t)
	s4 := NewS4Recency(db)
	err := s4.Precompute(context.Background(), "")
	assert.NoError(t, err, "S4.Precompute is a no-op (orchestrator owns persistence); must not error")
}

func TestS4Recency_ScoreEmptyCandidates(t *testing.T) {
	db := setupS4TestDB(t)
	s4 := NewS4Recency(db)
	got, err := s4.Score(context.Background(), "", []recs.AnimeID{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS4Recency_ScoreOngoingReturns1(t *testing.T) {
	db := setupS4TestDB(t)
	seedAnime(t, db, "a-ongoing", "ongoing", nil)
	s4 := NewS4Recency(db)
	got, err := s4.Score(context.Background(), "", []recs.AnimeID{"a-ongoing"})
	require.NoError(t, err)
	assert.Equal(t, recs.RawScore(1.0), got["a-ongoing"])
}

func TestS4Recency_ScoreReleasedAired30DaysAgoReturns07(t *testing.T) {
	db := setupS4TestDB(t)
	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour)
	seedAnime(t, db, "a-recent", "released", &thirtyDaysAgo)
	s4 := NewS4Recency(db)
	got, err := s4.Score(context.Background(), "", []recs.AnimeID{"a-recent"})
	require.NoError(t, err)
	assert.InDelta(t, 0.7, float64(got["a-recent"]), 1e-9)
}

func TestS4Recency_ScoreReleasedAiredOver90DaysReturns0(t *testing.T) {
	db := setupS4TestDB(t)
	oldDate := time.Now().UTC().Add(-200 * 24 * time.Hour)
	seedAnime(t, db, "a-old", "released", &oldDate)
	s4 := NewS4Recency(db)
	got, err := s4.Score(context.Background(), "", []recs.AnimeID{"a-old"})
	require.NoError(t, err)
	// Older than 90d: omitted from map (treated as zero by normalizer) OR explicit 0.
	if v, present := got["a-old"]; present {
		assert.Equal(t, recs.RawScore(0), v, "released anime older than 90d must score 0")
	}
}

func TestS4Recency_ScoreReleasedAiredOnNilReturns0(t *testing.T) {
	db := setupS4TestDB(t)
	seedAnime(t, db, "a-noaired", "released", nil)
	s4 := NewS4Recency(db)
	got, err := s4.Score(context.Background(), "", []recs.AnimeID{"a-noaired"})
	require.NoError(t, err)
	if v, present := got["a-noaired"]; present {
		assert.Equal(t, recs.RawScore(0), v, "released anime with nil aired_on must score 0")
	}
}

func TestS4Recency_ScoreMixedFixture(t *testing.T) {
	db := setupS4TestDB(t)
	now := time.Now().UTC()
	recent := now.Add(-30 * 24 * time.Hour)
	old := now.Add(-200 * 24 * time.Hour)
	seedAnime(t, db, "ongoing-1", "ongoing", nil)
	seedAnime(t, db, "recent-1", "released", &recent)
	seedAnime(t, db, "old-1", "released", &old)
	seedAnime(t, db, "announced-1", "announced", nil)

	s4 := NewS4Recency(db)
	got, err := s4.Score(context.Background(), "", []recs.AnimeID{
		"ongoing-1", "recent-1", "old-1", "announced-1",
	})
	require.NoError(t, err)

	assert.Equal(t, recs.RawScore(1.0), got["ongoing-1"])
	assert.InDelta(t, 0.7, float64(got["recent-1"]), 1e-9)
	if v, present := got["old-1"]; present {
		assert.Equal(t, recs.RawScore(0), v)
	}
	if v, present := got["announced-1"]; present {
		assert.Equal(t, recs.RawScore(0), v)
	}
}
