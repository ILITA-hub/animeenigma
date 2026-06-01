package repo

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// registerSQLiteGreatestOnce registers a custom SQLite driver
// "sqlite3_with_greatest" that exposes a GREATEST(a, b) scalar function so
// that production-shape SQL using GREATEST (Postgres-only) is exercisable
// against an in-memory SQLite test DB. Idempotent.
var registerSQLiteGreatestOnce sync.Once

func registerSQLiteWithGreatest() {
	registerSQLiteGreatestOnce.Do(func() {
		sql.Register("sqlite3_with_greatest", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				greatest := func(a, b int64) int64 {
					if a > b {
						return a
					}
					return b
				}
				return conn.RegisterFunc("greatest", greatest, true)
			},
		})
	})
}

// setupProgressTestDB returns an in-memory SQLite ProgressRepository with the
// watch_progress table created in a shape compatible with both UpsertProgress
// and MarkCompleted (composite unique constraint on user_id, anime_id,
// episode_number — same as production). Registers a GREATEST UDF for SQLite
// so UpsertProgress's GREATEST(watch_progress.duration, ?) Postgres expression
// can execute. Returns the underlying *gorm.DB so tests can seed via raw
// inserts when they want to bypass the upsert path.
func setupProgressTestDB(t *testing.T) (*ProgressRepository, *gorm.DB) {
	registerSQLiteWithGreatest()

	rawDB, err := sql.Open("sqlite3_with_greatest", ":memory:")
	require.NoError(t, err)

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite3_with_greatest",
		Conn:       rawDB,
	}, &gorm.Config{})
	require.NoError(t, err)

	// WR-03 (Phase 8) — Dialect divergence: production Postgres declares
	// watch_progress.completed as BOOLEAN, but this SQLite fixture declares it
	// INTEGER DEFAULT 0 because SQLite has no native bool type. The repo's
	// query uses the literal `false` (a Postgres bool literal). SQLite
	// silently coerces this to the integer 0, so existing queries happen to
	// work cross-dialect — but a future query using `IS FALSE`, `IS NOT TRUE`,
	// or partial-index filters with NULL-handling semantics would NOT execute
	// identically across both backends and could pass these unit tests while
	// failing in production.
	//
	// TODO: revisit this when integration tests against a real Postgres
	// (testcontainers, per CLAUDE.md "Testing" section) land for the player
	// service. The preferred fix is a //go:build integration-tagged Postgres
	// test for ListContinueWatching rather than papering over the type gap
	// here.
	err = db.Exec(`CREATE TABLE watch_progress (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		episode_number INTEGER NOT NULL,
		progress INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		completed INTEGER DEFAULT 0, -- production: BOOLEAN; see WR-03 note above
		watch_count INTEGER DEFAULT 1,
		dropped_off_at INTEGER,
		last_watched_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE (user_id, anime_id, episode_number)
	)`).Error
	require.NoError(t, err)

	return NewProgressRepository(db), db
}

// seedProgressRow inserts a watch_progress row directly via raw SQL, bypassing
// UpsertProgress's GREATEST(...) expression which is Postgres-only. watch_count
// defaults to 1 to match the table default; tests that need a non-default value
// should use seedProgressRowWithCount.
func seedProgressRow(t *testing.T, db *gorm.DB, userID, animeID string, episode, progress, duration int, completed bool) {
	t.Helper()
	seedProgressRowWithCount(t, db, userID, animeID, episode, progress, duration, completed, 1)
}

func seedProgressRowWithCount(t *testing.T, db *gorm.DB, userID, animeID string, episode, progress, duration int, completed bool, watchCount int) {
	t.Helper()
	now := time.Now()
	err := db.Exec(`INSERT INTO watch_progress
		(id, user_id, anime_id, episode_number, progress, duration, completed, watch_count, last_watched_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"seed-"+userID+"-"+animeID+"-"+itoa(episode), userID, animeID, episode, progress, duration, completed, watchCount, now, now, now).Error
	require.NoError(t, err)
}

// insertProgressAt seeds a watch_progress row with an explicit last_watched_at
// (and matching created_at/updated_at) so ordering-sensitive tests can control
// recency deterministically. Bypasses UpsertProgress's GREATEST() (Postgres-only).
func insertProgressAt(t *testing.T, db *gorm.DB, id, userID, animeID string, episode, progress, duration int, completed bool, ts time.Time) {
	t.Helper()
	err := db.Exec(`INSERT INTO watch_progress
		(id, user_id, anime_id, episode_number, progress, duration, completed, watch_count, last_watched_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, animeID, episode, progress, duration, completed, 1, ts, ts, ts).Error
	require.NoError(t, err)
}

// itoa: tiny stdlib-free int-to-string for test seed IDs.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// TestProgressRepository_UpsertProgress_PreservesCompletedTrue is the
// regression test for the heartbeat bug Phase 3 fixes: the old Upsert always
// overwrote completed with the struct's zero value (false), which meant any
// future "completed=true" row would be flipped back on the next heartbeat.
// UpsertProgress must NOT touch the completed column.
func TestProgressRepository_UpsertProgress_PreservesCompletedTrue(t *testing.T) {
	repo, db := setupProgressTestDB(t)
	ctx := context.Background()

	// Pre-seed a row with completed=true (skip MarkCompleted to keep this
	// test independent of MarkCompleted's correctness).
	seedProgressRow(t, db, "user-1", "anime-1", 5, 0, 0, true)

	// Heartbeat save (this is what UpdateProgress sends — Completed is the
	// zero value because the service no longer hardcodes it).
	heartbeat := &domain.WatchProgress{
		UserID:        "user-1",
		AnimeID:       "anime-1",
		EpisodeNumber: 5,
		Progress:      1200,
		Duration:      1440,
	}
	require.NoError(t, repo.UpsertProgress(ctx, heartbeat))

	// Read back — completed must STILL be true.
	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 5)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed, "UpsertProgress must not flip completed=true back to false")
	assert.Equal(t, 1200, p.Progress, "progress should be updated by heartbeat")
}

func TestProgressRepository_MarkCompleted_CreatesRowIfMissing(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 3))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 3)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed)
	assert.Equal(t, 0, p.Progress, "synthetic backfill row has progress=0")
	assert.Equal(t, 0, p.Duration, "synthetic backfill row has duration=0")
	assert.Equal(t, 3, p.EpisodeNumber)
}

func TestProgressRepository_MarkCompleted_Idempotent(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 7))
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 7))
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 7))

	// Should still have exactly one row, completed=true.
	results, err := repo.GetByUserAndAnime(ctx, "user-1", "anime-1")
	require.NoError(t, err)
	require.Len(t, results, 1, "MarkCompleted must be idempotent — no duplicate rows")
	assert.True(t, results[0].Completed)
}

// TestProgressRepository_MarkCompleted_FlipsExistingFalseRow ensures that
// when a heartbeat already created a watch_progress row (completed=false,
// real progress data), MarkCompleted flips completed=true while preserving
// the existing progress and duration values.
func TestProgressRepository_MarkCompleted_FlipsExistingFalseRow(t *testing.T) {
	repo, db := setupProgressTestDB(t)
	ctx := context.Background()

	// Seed a heartbeat-style row (completed=false, real progress data) via
	// raw SQL to avoid GREATEST() in UpsertProgress under SQLite.
	seedProgressRow(t, db, "user-1", "anime-1", 9, 500, 1440, false)

	// Confirm pre-condition.
	pre, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 9)
	require.NoError(t, err)
	require.NotNil(t, pre)
	require.False(t, pre.Completed, "seeded row starts with completed=false")
	require.Equal(t, 500, pre.Progress)

	// Mark completed.
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 9))

	// Read back: completed=true, progress and duration UNCHANGED.
	post, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 9)
	require.NoError(t, err)
	require.NotNil(t, post)
	assert.True(t, post.Completed, "MarkCompleted flips completed to true")
	assert.Equal(t, 500, post.Progress, "MarkCompleted must preserve existing progress")
	assert.Equal(t, 1440, post.Duration, "MarkCompleted must preserve existing duration")
}

// TestProgressRepository_MarkCompleted_PreservesLastWatchedAtMonotonic
// guarantees that calling MarkCompleted bumps last_watched_at forward.
func TestProgressRepository_MarkCompleted_PreservesLastWatchedAtMonotonic(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 4))
	first, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 4)
	require.NoError(t, err)
	firstTS := first.LastWatchedAt

	time.Sleep(10 * time.Millisecond)

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 4))
	second, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 4)
	require.NoError(t, err)
	assert.True(t, !second.LastWatchedAt.Before(firstTS),
		"second MarkCompleted must update last_watched_at to a value >= the first")
}

// Phase 5 (G-02) — rewatch detection: watch_count must remain at 1 on first
// completion, increment by 1 on each subsequent completion of the same row.
func TestProgressRepository_MarkCompleted_FirstCompletion_WatchCountIs1(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 1))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, 1, p.WatchCount, "first completion must set watch_count=1")
}

func TestProgressRepository_MarkCompleted_FlippingHeartbeatRow_WatchCountIs1(t *testing.T) {
	repo, db := setupProgressTestDB(t)
	ctx := context.Background()

	// Heartbeat row exists (completed=false). Flipping it to completed must
	// set watch_count=1 — not 2 — because the user only finished it once.
	seedProgressRow(t, db, "user-1", "anime-1", 2, 600, 1440, false)

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 2))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 2)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed)
	assert.Equal(t, 1, p.WatchCount, "first completion of a heartbeat row must set watch_count=1")
}

func TestProgressRepository_MarkCompleted_RewatchIncrementsWatchCount(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 5))
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 5))
	require.NoError(t, repo.MarkCompleted(ctx, "user-1", "anime-1", 5))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 5)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed)
	assert.Equal(t, 3, p.WatchCount, "three completions = first + two rewatches")
}

// Phase 5 (G-01) — drop-off beacon: MarkDropOff records the abandon position
// without touching the completed flag, regardless of starting state.
func TestProgressRepository_MarkDropOff_CreatesRowIfMissing(t *testing.T) {
	repo, _ := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, repo.MarkDropOff(ctx, "user-1", "anime-1", 1, 360))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 1)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.NotNil(t, p.DroppedOffAt, "dropped_off_at must be set")
	assert.Equal(t, 360, *p.DroppedOffAt)
	assert.Equal(t, 360, p.Progress, "progress reflects abandon position on synthesized row")
	assert.False(t, p.Completed, "drop-off must not flip completed=true")
}

func TestProgressRepository_MarkDropOff_PreservesCompletedTrue(t *testing.T) {
	repo, db := setupProgressTestDB(t)
	ctx := context.Background()

	// User finished episode, then later opened the page mid-rewatch and closed
	// the tab partway. Drop-off must NOT clobber the completed=true flag.
	seedProgressRow(t, db, "user-1", "anime-1", 7, 1440, 1440, true)

	require.NoError(t, repo.MarkDropOff(ctx, "user-1", "anime-1", 7, 200))

	p, err := repo.GetByUserAnimeEpisode(ctx, "user-1", "anime-1", 7)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.Completed, "drop-off must not reset completed=true")
	require.NotNil(t, p.DroppedOffAt)
	assert.Equal(t, 200, *p.DroppedOffAt)
	// progress preserves max via GREATEST — original 1440 wins over 200.
	assert.Equal(t, 1440, p.Progress, "progress must not regress on drop-off")
}

// TestProgressRepository_ListContinueWatching covers the rewritten
// list-driven semantics (2026-06-01): the rail is driven by anime_list
// status='watching' intersected with REAL available episodes.
//
//   - in-progress latest row   -> resume that episode (saved progress)
//   - completed latest row      -> advance to next episode IF it exists
//   - caught up to aired/total   -> excluded (nothing left to continue)
//   - status != 'watching'       -> excluded (dropped / completed / etc.)
//   - not in any list            -> excluded (the reported "random anime" bug)
//   - cross-user isolation, empty case, limit clamp.
func TestProgressRepository_ListContinueWatching(t *testing.T) {
	r, db := setupProgressTestDB(t)
	ctx := context.Background()

	// animes table now needs episodes_aired (the real-episode cap for ongoing).
	require.NoError(t, db.Exec(`CREATE TABLE animes (
        id TEXT PRIMARY KEY,
        name TEXT, name_ru TEXT, name_jp TEXT,
        poster_url TEXT,
        episodes_count INTEGER DEFAULT 0,
        episodes_aired INTEGER DEFAULT 0,
        deleted_at DATETIME
    )`).Error)

	// anime_list now drives the rail.
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
        id TEXT PRIMARY KEY,
        user_id TEXT,
        anime_id TEXT,
        status TEXT,
        deleted_at DATETIME
    )`).Error)

	seedAnime := func(id, name string, count, aired int) {
		require.NoError(t, db.Exec(
			`INSERT INTO animes (id, name, poster_url, episodes_count, episodes_aired) VALUES (?, ?, ?, ?, ?)`,
			id, name, "/"+id+".jpg", count, aired).Error)
	}
	seedList := func(user, animeID, status string) {
		require.NoError(t, db.Exec(
			`INSERT INTO anime_list (id, user_id, anime_id, status) VALUES (?, ?, ?, ?)`,
			"al-"+user+"-"+animeID, user, animeID, status).Error)
	}

	now := time.Now()

	// A: watching, E1 completed (older) + E2 in-progress (newest) -> resume E2.
	seedAnime("anime-A", "Anime A", 12, 0)
	seedList("user-1", "anime-A", "watching")
	insertProgressAt(t, db, "seed-A1", "user-1", "anime-A", 1, 1200, 1400, true, now.Add(-10*time.Minute))
	insertProgressAt(t, db, "seed-A2", "user-1", "anime-A", 2, 600, 1400, false, now)

	// C: watching, latest E3 completed, 3<12 -> advance to E4 (progress 0).
	seedAnime("anime-C", "Anime C", 12, 0)
	seedList("user-1", "anime-C", "watching")
	insertProgressAt(t, db, "seed-C3", "user-1", "anime-C", 3, 1400, 1400, true, now.Add(-30*time.Minute))

	// B: watching, E5 in-progress (oldest) -> resume E5, sorts last.
	seedAnime("anime-B", "Anime B", 24, 0)
	seedList("user-1", "anime-B", "watching")
	insertProgressAt(t, db, "seed-B5", "user-1", "anime-B", 5, 300, 1500, false, now.Add(-1*time.Hour))

	// D: watching, finished all 12 (latest E12 completed). next 13>12 -> EXCLUDE.
	seedAnime("anime-D", "Anime D", 12, 0)
	seedList("user-1", "anime-D", "watching")
	insertProgressAt(t, db, "seed-D12", "user-1", "anime-D", 12, 1400, 1400, true, now)

	// G: ongoing, aired=5, latest E5 completed. next 6>aired 5 -> EXCLUDE.
	seedAnime("anime-G", "Anime G", 24, 5)
	seedList("user-1", "anime-G", "watching")
	insertProgressAt(t, db, "seed-G5", "user-1", "anime-G", 5, 1400, 1400, true, now)

	// E: NOT in any list, has in-progress E1 -> EXCLUDE (the reported bug).
	seedAnime("anime-E", "Anime E", 12, 0)
	insertProgressAt(t, db, "seed-E1", "user-1", "anime-E", 1, 600, 1400, false, now)

	// F: list status='dropped', in-progress E2 -> EXCLUDE (the dropped case).
	seedAnime("anime-F", "Anime F", 12, 0)
	seedList("user-1", "anime-F", "dropped")
	insertProgressAt(t, db, "seed-F2", "user-1", "anime-F", 2, 553, 553, false, now)

	// Different user — must NOT leak into user-1's rows.
	seedAnime("anime-Z", "Anime Z", 12, 0)
	seedList("user-2", "anime-Z", "watching")
	insertProgressAt(t, db, "seed-Z1", "user-2", "anime-Z", 1, 100, 1400, false, now)

	// --- Happy path: only A, C, B in last_watched DESC order ---
	items, err := r.ListContinueWatching(ctx, "user-1", 10)
	require.NoError(t, err)
	require.Len(t, items, 3, "only watching-list anime with a real next episode")

	assert.Equal(t, "anime-A", items[0].Anime.ID)
	assert.Equal(t, 2, items[0].EpisodeNumber, "resume in-progress E2")
	assert.Equal(t, 600, items[0].Progress)
	assert.Equal(t, 1400, items[0].Duration)
	assert.Equal(t, "Anime A", items[0].Anime.Name)
	assert.Equal(t, 12, items[0].Anime.EpisodesCount)

	assert.Equal(t, "anime-C", items[1].Anime.ID)
	assert.Equal(t, 4, items[1].EpisodeNumber, "advance past completed E3 -> E4")
	assert.Equal(t, 0, items[1].Progress, "advanced episode starts at progress 0")

	assert.Equal(t, "anime-B", items[2].Anime.ID)
	assert.Equal(t, 5, items[2].EpisodeNumber)

	// Explicitly assert the excluded anime never appear.
	for _, it := range items {
		switch it.Anime.ID {
		case "anime-D", "anime-G", "anime-E", "anime-F", "anime-Z":
			t.Fatalf("anime %s must be excluded from continue-watching", it.Anime.ID)
		}
	}

	// --- Empty path ---
	empty, err := r.ListContinueWatching(ctx, "user-no-rows", 10)
	require.NoError(t, err)
	assert.Empty(t, empty)

	// --- Limit clamp (smoke): 0 -> default 10, 999 -> clamp 20. ---
	itemsZero, err := r.ListContinueWatching(ctx, "user-1", 0)
	require.NoError(t, err)
	assert.Len(t, itemsZero, 3)
	itemsHuge, err := r.ListContinueWatching(ctx, "user-1", 999)
	require.NoError(t, err)
	assert.Len(t, itemsHuge, 3)
}

// TestProgressRepository_GetBulkProgress covers the happy path (mixed in-progress
// + completed across multiple anime, one entry per anime keyed by the highest
// episode_number), empty IDs slice, missing anime omission, cross-user
// isolation, and the strict Completed gating (latest_episode row.completed=true
// AND >= episodes_count). Phase 9 (UX-16).
func TestProgressRepository_GetBulkProgress(t *testing.T) {
	r, db := setupProgressTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.Exec(`CREATE TABLE animes (
        id TEXT PRIMARY KEY,
        name TEXT,
        episodes_count INTEGER DEFAULT 0,
        episodes_aired INTEGER DEFAULT 0,
        deleted_at DATETIME
    )`).Error)

	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, ?, ?)`,
		"anime-A", "Anime A", 12, 12).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, ?, ?)`,
		"anime-B", "Anime B", 24, 12).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, ?, ?)`,
		"anime-C", "Anime C", 12, 12).Error)

	// Anime A: E1 completed, E3 completed. Latest = E3 (highest episode_number)
	// — completed=true on the latest row but 3 < 12, so badge "Completed" is
	// false (still in progress).
	seedProgressRow(t, db, "user-1", "anime-A", 1, 600, 1400, true)
	seedProgressRow(t, db, "user-1", "anime-A", 3, 1400, 1400, true)

	// Anime B: E5 in-progress only. Latest = E5, completed=false.
	seedProgressRow(t, db, "user-1", "anime-B", 5, 800, 1500, false)

	// Anime C: every episode completed (E1..E12). Latest = E12, completed=true
	// AND reached EpisodesCount — Completed flips true.
	for ep := 1; ep <= 12; ep++ {
		seedProgressRow(t, db, "user-1", "anime-C", ep, 1400, 1400, true)
	}

	// Different user — must NOT leak into user-1's map.
	seedProgressRow(t, db, "user-2", "anime-A", 9, 100, 1400, false)

	// --- Happy path ---
	m, err := r.GetBulkProgress(ctx, "user-1",
		[]string{"anime-A", "anime-B", "anime-C", "anime-missing"})
	require.NoError(t, err)
	require.Len(t, m, 3, "missing anime omitted from map")

	entryA, ok := m["anime-A"]
	require.True(t, ok)
	assert.Equal(t, 3, entryA.LatestEpisode, "highest episode_number wins (E3, not E1)")
	assert.False(t, entryA.Completed,
		"row.completed=true but 3 < 12 — badge Completed must be false (still in progress)")

	entryB, ok := m["anime-B"]
	require.True(t, ok)
	assert.Equal(t, 5, entryB.LatestEpisode)
	assert.False(t, entryB.Completed)

	entryC, ok := m["anime-C"]
	require.True(t, ok)
	assert.Equal(t, 12, entryC.LatestEpisode)
	assert.True(t, entryC.Completed,
		"row.completed=true AND 12 >= episodes_count(12) — Completed flips true")

	// --- Empty IDs slice ---
	empty, err := r.GetBulkProgress(ctx, "user-1", []string{})
	require.NoError(t, err)
	assert.Empty(t, empty)

	// --- Cross-user isolation ---
	otherUser, err := r.GetBulkProgress(ctx, "user-no-rows",
		[]string{"anime-A", "anime-B", "anime-C"})
	require.NoError(t, err)
	assert.Empty(t, otherUser)
}
