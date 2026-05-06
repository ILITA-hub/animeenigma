package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupRecsRepoTestDB builds an in-memory SQLite DB with the four tables
// the new Phase 13 RecsRepository methods touch:
//
//   - rec_user_signals          (UpdateS6Seed writes here)
//   - rec_completion_co_occurrence (GetTopCoOccurrences score=7 reads here)
//   - anime_list                (GetTopCoOccurrences score=5 live query reads here)
//
// Schema mirrors production (see services/player/internal/domain/recs.go +
// services/player/internal/domain/watch.go). The columns reflect what the
// methods actually touch — extra production columns are omitted.
func setupRecsRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE rec_user_signals (
		user_id TEXT PRIMARY KEY,
		s1_vector TEXT NOT NULL DEFAULT '{}',
		s5_affinity TEXT NOT NULL DEFAULT '{}',
		s6_seed_anime_id TEXT,
		s6_seed_completed_at DATETIME,
		s6_seed_score INTEGER,
		last_computed DATETIME NOT NULL
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE rec_completion_co_occurrence (
		seed_anime_id TEXT NOT NULL,
		candidate_anime_id TEXT NOT NULL,
		co_count INTEGER NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL,
		PRIMARY KEY (seed_anime_id, candidate_anime_id)
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status TEXT,
		score INTEGER DEFAULT 0
	)`).Error)

	return db
}

// --- UpdateS6Seed tests ---

func TestRecsRepository_UpdateS6Seed_InsertsWhenNoRow(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()
	completedAt := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, r.UpdateS6Seed(ctx, "user-1", "anime-X", completedAt, 8))

	var (
		seedID, s1, s5 string
		score          int
		ts             time.Time
	)
	row := db.Raw(`SELECT s6_seed_anime_id, s1_vector, s5_affinity, s6_seed_score, s6_seed_completed_at FROM rec_user_signals WHERE user_id = ?`, "user-1").Row()
	require.NoError(t, row.Scan(&seedID, &s1, &s5, &score, &ts))
	assert.Equal(t, "anime-X", seedID, "s6_seed_anime_id must be persisted")
	assert.Equal(t, 8, score, "s6_seed_score must be persisted")
	assert.Equal(t, "{}", s1, "s1_vector default '{}' must be set on fresh insert")
	assert.Equal(t, "{}", s5, "s5_affinity default '{}' must be set on fresh insert")
}

func TestRecsRepository_UpdateS6Seed_UpdatesWithoutClobberingS1S5(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()

	// Seed an existing row with non-default S1 + S5 payloads.
	require.NoError(t, db.Exec(`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		VALUES (?, ?, ?, ?)`,
		"user-2", `{"a":0.5}`, `{"tag:x":0.3}`, time.Now().UTC()).Error)

	require.NoError(t, r.UpdateS6Seed(ctx, "user-2", "anime-Y", time.Now().UTC(), 9))

	var s1, s5, seedID string
	var score int
	row := db.Raw(`SELECT s1_vector, s5_affinity, s6_seed_anime_id, s6_seed_score FROM rec_user_signals WHERE user_id = ?`, "user-2").Row()
	require.NoError(t, row.Scan(&s1, &s5, &seedID, &score))
	assert.Equal(t, `{"a":0.5}`, s1, "s1_vector must be PRESERVED — narrow UpdateS6Seed must not clobber S1")
	assert.Equal(t, `{"tag:x":0.3}`, s5, "s5_affinity must be PRESERVED — narrow UpdateS6Seed must not clobber S5")
	assert.Equal(t, "anime-Y", seedID, "s6_seed_anime_id must be updated")
	assert.Equal(t, 9, score, "s6_seed_score must be updated")
}

func TestRecsRepository_UpdateS6Seed_RefreshesLastComputed(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()

	stale := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	require.NoError(t, db.Exec(`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		VALUES (?, '{}', '{}', ?)`,
		"user-3", stale).Error)

	require.NoError(t, r.UpdateS6Seed(ctx, "user-3", "anime-Z", time.Now().UTC(), 7))

	var fresh time.Time
	row := db.Raw(`SELECT last_computed FROM rec_user_signals WHERE user_id = ?`, "user-3").Row()
	require.NoError(t, row.Scan(&fresh))
	// Use a tolerant 5-minute window — sqlite + Go time round-tripping has
	// precision quirks; the contract is "must be roughly now()", not "must
	// equal an exact instant".
	assert.True(t, fresh.After(stale.Add(time.Minute)), "last_computed must move forward; was %v, want > %v", fresh, stale)
}

// --- GetTopCoOccurrences tests ---

func TestRecsRepository_GetTopCoOccurrences_ReadsMaterializedTableForScore7(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	rows := []struct {
		seed, cand string
		count      int
	}{
		{"seed-A", "c1", 3},
		{"seed-A", "c2", 5},
		{"seed-A", "c3", 1},
		{"seed-A", "c4", 8},
		{"seed-A", "c5", 2},
	}
	for _, x := range rows {
		require.NoError(t, db.Exec(`INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed)
			VALUES (?, ?, ?, ?)`,
			x.seed, x.cand, x.count, now).Error)
	}

	got, err := r.GetTopCoOccurrences(ctx, "seed-A", 7, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"c4", "c2", "c1"}, got, "must sort by co_count DESC and respect limit")
}

func TestRecsRepository_GetTopCoOccurrences_RunsLiveQueryForScore5(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()

	// rec_completion_co_occurrence is intentionally empty — score=5 must
	// fall through to the on-demand anime_list join.
	// Two users (u1, u2) BOTH completed (seed-A, score 5) and (cand-X, score 5)
	// → cand-X co-count = 2.
	// One user (u3) completed (seed-A, score 6) and (cand-Y, score 6) → co-count = 1.
	// User (u4) completed (seed-A, score 4) — below threshold, ignored.
	rows := []struct {
		userID, animeID, status string
		score                   int
	}{
		{"u1", "seed-A", "completed", 5},
		{"u1", "cand-X", "completed", 5},
		{"u2", "seed-A", "completed", 5},
		{"u2", "cand-X", "completed", 5},
		{"u3", "seed-A", "completed", 6},
		{"u3", "cand-Y", "completed", 6},
		{"u4", "seed-A", "completed", 4},
		{"u4", "cand-Z", "completed", 4},
	}
	for i, x := range rows {
		require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, ?, ?)`,
			i, x.userID, x.animeID, x.status, x.score).Error)
	}

	got, err := r.GetTopCoOccurrences(ctx, "seed-A", 5, 5)
	require.NoError(t, err)
	// cand-X has co-count 2, cand-Y has 1, cand-Z is filtered (score 4 < 5).
	require.Len(t, got, 2, "live query must return only score >= 5 candidates")
	assert.Equal(t, "cand-X", got[0], "highest co_count first")
	assert.Equal(t, "cand-Y", got[1])
}

func TestRecsRepository_GetTopCoOccurrences_RespectsLimit(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < 10; i++ {
		require.NoError(t, db.Exec(`INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed)
			VALUES (?, ?, ?, ?)`,
			"seed-A", "cand-"+string(rune('a'+i)), 10-i, now).Error)
	}

	got, err := r.GetTopCoOccurrences(ctx, "seed-A", 7, 3)
	require.NoError(t, err)
	assert.Len(t, got, 3, "limit must be honored")
}

func TestRecsRepository_GetTopCoOccurrences_EmptyOnNoMatches(t *testing.T) {
	db := setupRecsRepoTestDB(t)
	r := NewRecsRepository(db)
	ctx := context.Background()

	got, err := r.GetTopCoOccurrences(ctx, "nonexistent-seed", 7, 5)
	require.NoError(t, err)
	assert.Empty(t, got, "nonexistent seed must return empty slice without error")

	got5, err := r.GetTopCoOccurrences(ctx, "nonexistent-seed", 5, 5)
	require.NoError(t, err)
	assert.Empty(t, got5, "live-query path must return empty slice without error")
}
