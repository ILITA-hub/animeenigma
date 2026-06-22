package recs

import (
	"context"
	"fmt"
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
	// A bare :memory: SQLite DB is per-connection. Once RunOnce wraps its work
	// in a Transaction, GORM checks out a dedicated connection; with the
	// boot-tick goroutine reading concurrently the pool opens a SECOND
	// connection that — for a bare :memory: DSN — sees a fresh, schema-less
	// in-memory DB and fails with "no such table". A uniquely-named shared-cache
	// in-memory DB makes every pooled connection share ONE backing database;
	// holding a sentinel connection open for the whole test keeps that shared
	// DB alive (shared-cache memory DBs are destroyed when the LAST connection
	// closes) and retaining idle connections avoids churn. The unique name
	// isolates this test DB from every other test's DB in the process.
	// Production uses Postgres, where none of this applies.
	dsn := fmt.Sprintf("file:coocc_%d_%p?mode=memory&cache=shared", time.Now().UnixNano(), t)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetConnMaxIdleTime(0)

	// Sentinel connection held open until the test ends so the shared-cache
	// in-memory DB is never destroyed mid-test by a transient zero-connection
	// window.
	sentinel, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sentinel.Close() })

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

// L634: the cron must be authoritative — pairs that stop co-occurring (and any
// pre-existing stale rows) must be reaped, not persist forever. The
// delete-stale-after-upsert pattern: rows whose last_computed predates this
// run's start are deleted.
func TestCoOccurrenceOrchestrator_RunOnce_ReapsStaleRows(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	// Two users completed (A,B) so the pair currently co-occurs.
	id := 0
	for _, uid := range []string{"u1", "u2"} {
		for _, aid := range []string{"anime-A", "anime-B"} {
			seedAnimeListRow(t, db, id, uid, aid, "completed", 8)
			id++
		}
	}

	// Inject a pre-existing stale row (C,D) that does NOT co-occur in the
	// current anime_list, stamped far in the past. After RunOnce it must be
	// gone (reaped by the post-upsert delete-stale sweep).
	require.NoError(t, db.Exec(
		`INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed) VALUES (?, ?, ?, ?)`,
		"anime-C", "anime-D", 99, "2000-01-01 00:00:00",
	).Error)

	require.NoError(t, orch.RunOnce(context.Background()))

	// Stale (C,D) must be reaped.
	var staleCount int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM rec_completion_co_occurrence WHERE seed_anime_id=? AND candidate_anime_id=?`,
		"anime-C", "anime-D").Scan(&staleCount).Error)
	assert.Equal(t, int64(0), staleCount, "pre-existing stale row that no longer co-occurs must be reaped")

	// Current (A,B)/(B,A) pairs must survive with refreshed last_computed.
	var abCount int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM rec_completion_co_occurrence WHERE seed_anime_id=? AND candidate_anime_id=?`,
		"anime-A", "anime-B").Scan(&abCount).Error)
	assert.Equal(t, int64(1), abCount, "currently co-occurring pair must survive the reap")
}

// L634: a pair that DROPS OUT (stops co-occurring) between runs must be reaped
// on the next run — proves the table is rebuilt by generation, not just
// appended to.
func TestCoOccurrenceOrchestrator_RunOnce_ReapsDroppedPair(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())

	// u1 completed (A,B); u2 completed (X,Y). Two distinct co-occurring pairs.
	seedAnimeListRow(t, db, 1, "u1", "anime-A", "completed", 8)
	seedAnimeListRow(t, db, 2, "u1", "anime-B", "completed", 8)
	seedAnimeListRow(t, db, 3, "u2", "anime-X", "completed", 8)
	seedAnimeListRow(t, db, 4, "u2", "anime-Y", "completed", 8)

	require.NoError(t, orch.RunOnce(context.Background()))
	// (A,B),(B,A),(X,Y),(Y,X) = 4 rows.
	assert.Equal(t, int64(4), countCoOccRows(t, db), "two pairs materialize to 4 directed rows")

	// Force a measurable last_computed boundary: sleep so the next run's
	// start time is strictly after the first run's stamps.
	time.Sleep(1100 * time.Millisecond)

	// Now u1 un-completes B -> (A,B) no longer co-occurs.
	require.NoError(t, db.Exec(`DELETE FROM anime_list WHERE user_id=? AND anime_id=?`, "u1", "anime-B").Error)

	require.NoError(t, orch.RunOnce(context.Background()))

	// (A,B)/(B,A) must be reaped; (X,Y)/(Y,X) survive.
	assert.Equal(t, int64(2), countCoOccRows(t, db), "dropped (A,B) pair must be reaped, leaving only (X,Y)/(Y,X)")
	var abCount int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM rec_completion_co_occurrence WHERE seed_anime_id=? AND candidate_anime_id=?`,
		"anime-A", "anime-B").Scan(&abCount).Error)
	assert.Equal(t, int64(0), abCount, "the pair that stopped co-occurring must be gone")
}

// L641: each Start-driven RunOnce must run under a deadline so a hung
// materialize aborts instead of stalling the 24h ticker forever.
func TestCoOccurrenceOrchestrator_RunTickCarriesDeadline(t *testing.T) {
	db := setupCoOccTestDB(t)
	orch := NewCoOccurrenceOrchestrator(db, logger.Default())
	orch.tickTimeout = 5 * time.Second

	// runTick must pass a deadlined ctx into RunOnce.
	orch.runTick(context.Background(), "test")
	assert.True(t, orch.lastTickHadDeadline, "per-tick RunOnce must carry a deadline (audit L641)")
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
