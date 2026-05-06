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
