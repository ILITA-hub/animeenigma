package signals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newS8TestDB creates an in-memory SQLite DB with the minimal schema S8
// needs: animes (id + franchise) and anime_list (user scores). Distinct
// name from s7's newTestDB — same package.
func newS8TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id        TEXT PRIMARY KEY,
		franchise TEXT NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id       TEXT PRIMARY KEY,
		user_id  TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status   TEXT,
		score    INTEGER -- nullable: unscored rows stored as NULL
	)`).Error)
	return db
}

func s8Seed(t *testing.T, db *gorm.DB, sql string, args ...interface{}) {
	t.Helper()
	require.NoError(t, db.Exec(sql, args...).Error)
}

func TestS8_ID(t *testing.T) {
	s := NewS8Franchise(newS8TestDB(t))
	assert.Equal(t, "s8", string(s.ID()))
}

func TestS8_FranchiseMatchScaledByUserScore(t *testing.T) {
	db := newS8TestDB(t)
	// Seed: user rated "frieren" franchise entry 9 → candidate in same
	// franchise scores (9-5)/5 = 0.8.
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'frieren'), ('cand-1', 'frieren'), ('cand-2', 'other')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 9)`)

	got, err := NewS8Franchise(db).Score(context.Background(), "u1",
		[]string{"cand-1", "cand-2"})
	require.NoError(t, err)
	assert.InDelta(t, 0.8, float64(got["cand-1"]), 0.0001)
	_, hasOther := got["cand-2"]
	assert.False(t, hasOther, "unrelated franchise must be omitted")
}

func TestS8_Score10ClampsToOne(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'f'), ('cand-1', 'f')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 10)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, float64(got["cand-1"]), 0.0001)
}

func TestS8_BestScoreWinsAcrossFranchiseEntries(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-lo', 'f'), ('seed-hi', 'f'), ('cand-1', 'f')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES
		('l1', 'u1', 'seed-lo', 'dropped', 6),
		('l2', 'u1', 'seed-hi', 'completed', 8)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	// MAX(6, 8) = 8 → (8-5)/5 = 0.6
	assert.InDelta(t, 0.6, float64(got["cand-1"]), 0.0001)
}

func TestS8_NeutralAndLowScoresSilent(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'f'), ('cand-1', 'f')`)
	// score 5 → (5-5)/5 = 0 → omitted. Also NULL score → excluded by SQL.
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES
		('l1', 'u1', 'seed-1', 'completed', 5),
		('l2', 'u1', 'seed-1', 'watching', NULL)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS8_CandidateWithoutFranchiseOmitted(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'f'), ('cand-nofr', '')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 9)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-nofr"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS8_AnonymousAndEmptyPoolSilent(t *testing.T) {
	db := newS8TestDB(t)
	got, err := NewS8Franchise(db).Score(context.Background(), "", []string{"x"})
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = NewS8Franchise(db).Score(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
