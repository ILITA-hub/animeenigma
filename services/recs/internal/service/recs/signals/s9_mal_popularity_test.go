package signals

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newS9TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id          TEXT PRIMARY KEY,
		mal_members INTEGER NOT NULL DEFAULT 0
	)`).Error)
	return db
}

func TestS9_ID(t *testing.T) {
	assert.Equal(t, "s9", string(NewS9MalPopularity(newS9TestDB(t)).ID()))
}

func TestS9_PrecomputeNoop(t *testing.T) {
	require.NoError(t, NewS9MalPopularity(newS9TestDB(t)).Precompute(context.Background(), "u1"))
}

func TestS9_LogScaledAndZeroOmitted(t *testing.T) {
	db := newS9TestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO animes (id, mal_members) VALUES
		('zero', 0), ('small', 100), ('huge', 1000000)`).Error)

	got, err := NewS9MalPopularity(db).Score(context.Background(), "u1",
		[]string{"zero", "small", "huge"})
	require.NoError(t, err)

	// Zero members omitted (normalizer treats absent as 0).
	_, hasZero := got["zero"]
	assert.False(t, hasZero, "mal_members==0 must be omitted")

	assert.InDelta(t, math.Log1p(100), float64(got["small"]), 1e-9)
	assert.InDelta(t, math.Log1p(1000000), float64(got["huge"]), 1e-9)
	// Heavy-tail sanity: huge strictly greater, but log keeps it within one
	// order of magnitude of small (not 10000x).
	assert.Greater(t, float64(got["huge"]), float64(got["small"]))
	assert.Less(t, float64(got["huge"]), 3*float64(got["small"]))
}

func TestS9_EmptyPoolSilent(t *testing.T) {
	got, err := NewS9MalPopularity(newS9TestDB(t)).Score(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
