package recs

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCoOccTestDB creates the two tables RunOnce reads from / writes to.
// Schema mirrors production (services/player/internal/domain/recs.go +
// services/player/internal/domain/watch.go) for the columns we hit.
func setupCoOccTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status TEXT,
		score INTEGER DEFAULT 0
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE rec_completion_co_occurrence (
		seed_anime_id TEXT NOT NULL,
		candidate_anime_id TEXT NOT NULL,
		co_count INTEGER NOT NULL DEFAULT 0,
		last_computed DATETIME NOT NULL,
		PRIMARY KEY (seed_anime_id, candidate_anime_id)
	)`).Error)

	return db
}

func seedAnimeListRow(t *testing.T, db *gorm.DB, idSuffix int, userID, animeID, status string, score int) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, ?, ?)`,
		idSuffix, userID, animeID, status, score).Error)
}

func countCoOccRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM rec_completion_co_occurrence`).Scan(&n).Error)
	return n
}

func TestCoOccurrenceOrchestrator_RunOnce_PopulatesMatrix(t *testing.T) {
	db := setupCoOccTestDB(t)
	log := logger.Default()
	orch := NewCoOccurrenceOrchestrator(db, log)

	// Three users each completed (anime-A, anime-B) at score 8.
	id := 0
	for _, uid := range []string{"u1", "u2", "u3"} {
		for _, aid := range []string{"anime-A", "anime-B"} {
			seedAnimeListRow(t, db, id, uid, aid, "completed", 8)
			id++
		}
	}

	require.NoError(t, orch.RunOnce(context.Background()))

	// (A,B) and (B,A) should each have co_count=3 (all three users).
	var coAB, coBA int
	require.NoError(t, db.Raw(`SELECT co_count FROM rec_completion_co_occurrence WHERE seed_anime_id=? AND candidate_anime_id=?`,
		"anime-A", "anime-B").Scan(&coAB).Error)
	require.NoError(t, db.Raw(`SELECT co_count FROM rec_completion_co_occurrence WHERE seed_anime_id=? AND candidate_anime_id=?`,
		"anime-B", "anime-A").Scan(&coBA).Error)
	assert.Equal(t, 3, coAB, "all three users completed both (A,B) at score>=7")
	assert.Equal(t, 3, coBA, "co-occurrence is bidirectional — (B,A) also has count 3")
}

func TestCoOccurrenceOrchestrator_RunOnce_IsIdempotent(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	id := 0
	for _, uid := range []string{"u1", "u2"} {
		for _, aid := range []string{"a", "b"} {
			seedAnimeListRow(t, db, id, uid, aid, "completed", 7)
			id++
		}
	}

	require.NoError(t, orch.RunOnce(context.Background()))
	first := countCoOccRows(t, db)
	assert.Greater(t, first, int64(0), "first RunOnce must produce rows")

	require.NoError(t, orch.RunOnce(context.Background()))
	second := countCoOccRows(t, db)
	assert.Equal(t, first, second, "ON CONFLICT DO UPDATE keeps row count stable on re-run")
}

func TestCoOccurrenceOrchestrator_RunOnce_ExcludesScore6(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	// u1 completed (A=6, B=8) — A excluded by score < 7
	seedAnimeListRow(t, db, 1, "u1", "anime-A", "completed", 6)
	seedAnimeListRow(t, db, 2, "u1", "anime-B", "completed", 8)
	// u2 completed (A=7, B=7) — both qualify
	seedAnimeListRow(t, db, 3, "u2", "anime-A", "completed", 7)
	seedAnimeListRow(t, db, 4, "u2", "anime-B", "completed", 7)

	require.NoError(t, orch.RunOnce(context.Background()))

	var coAB int
	require.NoError(t, db.Raw(`SELECT co_count FROM rec_completion_co_occurrence WHERE seed_anime_id=? AND candidate_anime_id=?`,
		"anime-A", "anime-B").Scan(&coAB).Error)
	assert.Equal(t, 1, coAB, "only u2 qualifies on both at score>=7; u1's A=6 disqualifies the pair for u1")
}

func TestCoOccurrenceOrchestrator_RunOnce_ExcludesNonCompleted(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	seedAnimeListRow(t, db, 1, "u1", "anime-A", "watching", 8)
	seedAnimeListRow(t, db, 2, "u1", "anime-B", "watching", 8)

	require.NoError(t, orch.RunOnce(context.Background()))
	assert.Equal(t, int64(0), countCoOccRows(t, db), "non-completed rows must not produce co-occurrence edges")
}

func TestCoOccurrenceOrchestrator_RunOnce_HandlesEmptyAnimeList(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	require.NoError(t, orch.RunOnce(context.Background()))
	assert.Equal(t, int64(0), countCoOccRows(t, db), "empty anime_list must produce zero rows without error")
}

func TestCoOccurrenceOrchestrator_Start_BootTickAndShutdown(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	// One pair of users so the boot tick has data to materialize.
	id := 0
	for _, uid := range []string{"u1", "u2"} {
		for _, aid := range []string{"x", "y"} {
			seedAnimeListRow(t, db, id, uid, aid, "completed", 9)
			id++
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	orch.Start(ctx, 50*time.Millisecond)

	// Allow the boot tick goroutine to run.
	time.Sleep(150 * time.Millisecond)
	cancel()
	// Drain the goroutine.
	time.Sleep(50 * time.Millisecond)

	assert.Greater(t, countCoOccRows(t, db), int64(0), "boot tick must have populated rec_completion_co_occurrence")
}
