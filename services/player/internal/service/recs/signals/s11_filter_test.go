package signals

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupS11TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	// anime_list is needed by CandidatePoolForUser. Tests that only exercise
	// the anonymous CandidatePool path don't insert into it; the LEFT JOIN
	// gracefully handles an empty table.
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status TEXT
	)`).Error)
	return db
}

func TestS11Filter_EmptyTable(t *testing.T) {
	db := setupS11TestDB(t)
	f := NewS11Filter(db)
	got, err := f.CandidatePool(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS11Filter_ExcludesHiddenAndSoftDeleted(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('visible-1', 0, NULL),
		 ('visible-2', 0, NULL),
		 ('hidden-1', 1, NULL),
		 ('deleted-1', 0, '2026-01-01 00:00:00'),
		 ('hidden-and-deleted-1', 1, '2026-01-01 00:00:00')`,
	).Error)

	f := NewS11Filter(db)
	got, err := f.CandidatePool(context.Background())
	require.NoError(t, err)

	want := map[recs.AnimeID]struct{}{"visible-1": {}, "visible-2": {}}
	gotSet := map[recs.AnimeID]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	assert.Equal(t, want, gotSet, "S11 must include hidden=false AND deleted_at IS NULL only")
}

func TestS11Filter_AllVisible(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('a', 0, NULL),
		 ('b', 0, NULL),
		 ('c', 0, NULL)`,
	).Error)

	f := NewS11Filter(db)
	got, err := f.CandidatePool(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

// CandidatePoolForUser tests — Phase 11 REC-UX-04.
//
// The user-specific path layers `anime_list.status NOT IN (completed, dropped)`
// on top of the anonymous filter. status='watching', 'planned', 'on_hold' must
// all be kept; only 'completed' and 'dropped' (and 'NULL' when no row exists)
// determine eligibility.

func seedAnimeListEntry(t *testing.T, db *gorm.DB, rowID, userID, animeID, status string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status) VALUES (?, ?, ?, ?)`,
		rowID, userID, animeID, status,
	).Error)
}

func TestS11Filter_CandidatePoolForUser_EmptyTable(t *testing.T) {
	db := setupS11TestDB(t)
	f := NewS11Filter(db)
	got, err := f.CandidatePoolForUser(context.Background(), "user-A")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS11Filter_CandidatePoolForUser_NoAnimeListRowsReturnsAllVisible(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('a', 0, NULL),
		 ('b', 0, NULL),
		 ('hidden-1', 1, NULL),
		 ('deleted-1', 0, '2026-01-01 00:00:00')`,
	).Error)

	f := NewS11Filter(db)
	got, err := f.CandidatePoolForUser(context.Background(), "user-A")
	require.NoError(t, err)

	gotSet := map[recs.AnimeID]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	want := map[recs.AnimeID]struct{}{"a": {}, "b": {}}
	assert.Equal(t, want, gotSet, "user with no anime_list rows -> same result as anonymous CandidatePool")
}

func TestS11Filter_CandidatePoolForUser_ExcludesCompletedAndDropped(t *testing.T) {
	db := setupS11TestDB(t)
	// Per the plan's fixture: 5 anime — 1 visible kept, 1 visible completed (drop),
	// 1 hidden, 1 soft-deleted, 1 visible dropped (drop).
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('anime-1', 0, NULL),
		 ('anime-2', 0, NULL),
		 ('anime-3', 1, NULL),
		 ('anime-4', 0, '2026-01-01 00:00:00'),
		 ('anime-5', 0, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-1", "user-A", "anime-1", "watching")
	seedAnimeListEntry(t, db, "al-2", "user-A", "anime-2", "completed")
	seedAnimeListEntry(t, db, "al-5", "user-A", "anime-5", "dropped")

	f := NewS11Filter(db)
	got, err := f.CandidatePoolForUser(context.Background(), "user-A")
	require.NoError(t, err)

	gotSet := map[recs.AnimeID]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	want := map[recs.AnimeID]struct{}{"anime-1": {}}
	assert.Equal(t, want, gotSet, "only visible anime not completed/dropped/hidden/deleted survive")
}

func TestS11Filter_CandidatePoolForUser_KeepsActiveStatuses(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('anime-1', 0, NULL),
		 ('anime-2', 0, NULL),
		 ('anime-5', 0, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-1", "user-B", "anime-1", "planned")
	seedAnimeListEntry(t, db, "al-2", "user-B", "anime-2", "watching")
	seedAnimeListEntry(t, db, "al-5", "user-B", "anime-5", "on_hold")

	f := NewS11Filter(db)
	got, err := f.CandidatePoolForUser(context.Background(), "user-B")
	require.NoError(t, err)

	gotSet := map[recs.AnimeID]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	want := map[recs.AnimeID]struct{}{"anime-1": {}, "anime-2": {}, "anime-5": {}}
	assert.Equal(t, want, gotSet, "watching / planned / on_hold are KEPT; only completed and dropped exclude")
}

// ----------------------------------------------------------------------------
// Phase 14 (REC-ADMIN-01) — FilterAudit tests.
//
// FilterAudit returns the items that S11.CandidatePoolForUser would drop, with
// a reason string per category. Reasons:
//   - "status=completed" — anime where user's anime_list.status='completed'
//   - "status=dropped"   — anime where user's anime_list.status='dropped'
//   - "hidden=true"      — anime where animes.hidden=true (not user-specific)
//
// Soft-deleted anime are NOT in the audit (they're never surfaced anywhere
// in admin debug; "we never tell admins about silently-vanished rows").
//
// Audit is sorted by (reason ASC, anime_id ASC) for deterministic snapshots.
// ----------------------------------------------------------------------------

func TestS11Filter_FilterAudit_EmptyState(t *testing.T) {
	db := setupS11TestDB(t)
	f := NewS11Filter(db)
	got, err := f.FilterAudit(context.Background(), "user-A")
	require.NoError(t, err)
	assert.Empty(t, got, "user with no anime_list rows + no hidden anime -> empty audit")
}

func TestS11Filter_FilterAudit_AllThreeReasons(t *testing.T) {
	db := setupS11TestDB(t)
	// 5 anime — anime-1 (visible, no anime_list), anime-2 (visible, completed),
	// anime-3 (hidden=true), anime-4 (soft-deleted via deleted_at — EXCLUDED
	// silently from audit), anime-5 (visible, dropped).
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('anime-1', 0, NULL),
		 ('anime-2', 0, NULL),
		 ('anime-3', 1, NULL),
		 ('anime-4', 0, '2026-01-01 00:00:00'),
		 ('anime-5', 0, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-2", "user-A", "anime-2", "completed")
	seedAnimeListEntry(t, db, "al-5", "user-A", "anime-5", "dropped")

	f := NewS11Filter(db)
	got, err := f.FilterAudit(context.Background(), "user-A")
	require.NoError(t, err)

	// Expected 3 entries; soft-deleted anime-4 NOT included.
	require.Len(t, got, 3, "expected exactly 3 audit rows (completed + dropped + hidden); soft-deleted excluded")
	// Sorted by (reason ASC, anime_id ASC): hidden=true (anime-3), status=completed (anime-2), status=dropped (anime-5).
	assert.Equal(t, "hidden=true", got[0].Reason)
	assert.Equal(t, "anime-3", got[0].AnimeID)
	assert.Equal(t, "status=completed", got[1].Reason)
	assert.Equal(t, "anime-2", got[1].AnimeID)
	assert.Equal(t, "status=dropped", got[2].Reason)
	assert.Equal(t, "anime-5", got[2].AnimeID)
}

func TestS11Filter_FilterAudit_OnlyHiddenAnime(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('visible-1', 0, NULL),
		 ('hidden-1', 1, NULL),
		 ('hidden-2', 1, NULL)`,
	).Error)

	f := NewS11Filter(db)
	got, err := f.FilterAudit(context.Background(), "user-with-no-list")
	require.NoError(t, err)

	require.Len(t, got, 2)
	for _, entry := range got {
		assert.Equal(t, "hidden=true", entry.Reason)
	}
	// Sorted by anime_id ASC within reason.
	assert.Equal(t, "hidden-1", got[0].AnimeID)
	assert.Equal(t, "hidden-2", got[1].AnimeID)
}

func TestS11Filter_FilterAudit_OnlyUserSpecific(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('a-1', 0, NULL),
		 ('a-2', 0, NULL),
		 ('a-3', 0, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-1", "user-A", "a-1", "completed")
	seedAnimeListEntry(t, db, "al-2", "user-A", "a-2", "dropped")
	// Active status — must NOT appear in audit.
	seedAnimeListEntry(t, db, "al-3", "user-A", "a-3", "watching")

	f := NewS11Filter(db)
	got, err := f.FilterAudit(context.Background(), "user-A")
	require.NoError(t, err)

	require.Len(t, got, 2)
	// Sorted by reason ASC: completed first, dropped second.
	assert.Equal(t, "status=completed", got[0].Reason)
	assert.Equal(t, "a-1", got[0].AnimeID)
	assert.Equal(t, "status=dropped", got[1].Reason)
	assert.Equal(t, "a-2", got[1].AnimeID)
}

func TestS11Filter_FilterAudit_OtherUsersListIgnored(t *testing.T) {
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('a-1', 0, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-A", "user-A", "a-1", "completed")

	f := NewS11Filter(db)
	got, err := f.FilterAudit(context.Background(), "user-B")
	require.NoError(t, err)

	assert.Empty(t, got, "user-B's audit must NOT include user-A's completed anime")
}

func TestS11Filter_FilterAudit_HiddenAndCompletedAnimeShownTwice(t *testing.T) {
	// An anime that is BOTH hidden AND in the user's completed list emits
	// two audit rows — one per applicable reason. The panel surfaces both
	// so admins see every reason that triggered.
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('dual-1', 1, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-1", "user-A", "dual-1", "completed")

	f := NewS11Filter(db)
	got, err := f.FilterAudit(context.Background(), "user-A")
	require.NoError(t, err)

	// Two rows: hidden=true + status=completed
	require.Len(t, got, 2)
	reasons := map[string]bool{got[0].Reason: true, got[1].Reason: true}
	assert.True(t, reasons["hidden=true"], "expected hidden=true row")
	assert.True(t, reasons["status=completed"], "expected status=completed row")
}

func TestS11Filter_CandidatePoolForUser_OtherUsersStatusIgnored(t *testing.T) {
	// User-A has anime-1 completed; user-B has nothing. user-B's pool must
	// still include anime-1 because the filter is per-user.
	db := setupS11TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, hidden, deleted_at) VALUES
		 ('anime-1', 0, NULL),
		 ('anime-2', 0, NULL)`,
	).Error)
	seedAnimeListEntry(t, db, "al-A1", "user-A", "anime-1", "completed")

	f := NewS11Filter(db)
	got, err := f.CandidatePoolForUser(context.Background(), "user-B")
	require.NoError(t, err)
	gotSet := map[recs.AnimeID]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	want := map[recs.AnimeID]struct{}{"anime-1": {}, "anime-2": {}}
	assert.Equal(t, want, gotSet, "user-B's pool must NOT be affected by user-A's status")
}
