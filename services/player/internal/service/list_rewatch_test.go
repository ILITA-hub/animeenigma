package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func markReq(ep int) *domain.MarkEpisodeWatchedRequest {
	return &domain.MarkEpisodeWatchedRequest{Episode: ep, Player: "kodik", Language: "ru", WatchType: "sub"}
}

// Group 2 — Rewatch lifecycle (design 2026-06-05).
//
// Terminal state (completed + fully watched) → click "Пересмотреть":
//   status='watching', episodes=0, is_rewatching=true,
//   watch_progress.completed=false  (watch_history preserved),
//   rewatch_count UNCHANGED.
// Then watching→completed (finale) WHILE is_rewatching:
//   rewatch_count++, is_rewatching=false.

type listRow struct {
	Status       string
	Episodes     int
	IsRewatching bool
	RewatchCount int
}

func readListRow(t *testing.T, db *gorm.DB, userID, animeID string) listRow {
	t.Helper()
	var r listRow
	err := db.Raw(
		`SELECT status, episodes, is_rewatching, rewatch_count
		   FROM anime_list WHERE user_id=? AND anime_id=?`,
		userID, animeID,
	).Scan(&r).Error
	require.NoError(t, err)
	return r
}

func countWatchHistory(t *testing.T, db *gorm.DB, userID, animeID string) int {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM watch_history WHERE user_id=? AND anime_id=?`,
		userID, animeID,
	).Scan(&n).Error)
	return int(n)
}

// seedCompletedFullyWatched inserts a 12-ep anime the user finished and marked
// completed: anime_list(completed, episodes=12, rewatch_count=N), all 12
// watch_progress rows completed=true, and one watch_history audit row.
func seedCompletedFullyWatched(t *testing.T, db *gorm.DB, userID, animeID string, rewatchCount int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count) VALUES (?,?,12)`,
		animeID, "Test Anime").Error)
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, episodes, is_rewatching, rewatch_count, created_at, updated_at)
		 VALUES (?,?,?,'completed',12,0,?,now(),now())`,
		"al-"+animeID, userID, animeID, rewatchCount).Error)
	for ep := 1; ep <= 12; ep++ {
		require.NoError(t, db.Exec(
			`INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, watch_count, last_watched_at, created_at, updated_at)
			 VALUES (?,?,?,?,1400,1400,1,1,now(),now(),now())`,
			"wp-"+animeID+"-"+itoa(ep), userID, animeID, ep).Error)
	}
	require.NoError(t, db.Exec(
		`INSERT INTO watch_history (id, user_id, anime_id, episode_number, player, language, watch_type, watched_at)
		 VALUES (?,?,?,12,'kodik','ru','sub',now())`,
		"wh-"+animeID, userID, animeID).Error)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestRewatch_ResetsToFreshWatchingCycle(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedCompletedFullyWatched(t, db, "u1", "anime-1", 0)

	_, err := svc.Rewatch(ctx, "u1", "anime-1")
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, "watching", row.Status, "rewatch moves the entry back to 'watching'")
	assert.Equal(t, 0, row.Episodes, "episodes reset to 0 so My List shows the new cycle")
	assert.True(t, row.IsRewatching, "is_rewatching flag set for the active rewatch")
	assert.Equal(t, 0, row.RewatchCount, "rewatch_count NOT bumped on start (only on completion)")
}

func TestRewatch_ResetsWatchProgressCompletedFlags(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedCompletedFullyWatched(t, db, "u1", "anime-1", 0)

	_, err := svc.Rewatch(ctx, "u1", "anime-1")
	require.NoError(t, err)

	// Episode 6 must no longer count as completed → resume machine sees a fresh
	// cycle and the button can walk 0 → partial → full again.
	exists, completed := readWatchProgressCompleted(t, db, "u1", "anime-1", 6)
	assert.True(t, exists, "rows are kept (reset, not deleted)")
	assert.False(t, completed, "watch_progress.completed reset to false on rewatch")
}

func TestRewatch_PreservesWatchHistoryAuditTrail(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedCompletedFullyWatched(t, db, "u1", "anime-1", 0)
	before := countWatchHistory(t, db, "u1", "anime-1")

	_, err := svc.Rewatch(ctx, "u1", "anime-1")
	require.NoError(t, err)

	assert.Equal(t, before, countWatchHistory(t, db, "u1", "anime-1"),
		"watch_history is append-only — rewatch must not delete the audit trail")
}

func TestRewatch_FinaleWhileRewatching_IncrementsCountAndClearsFlag(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedCompletedFullyWatched(t, db, "u1", "anime-1", 0)
	_, err := svc.Rewatch(ctx, "u1", "anime-1")
	require.NoError(t, err)

	// Re-watch all the way to the finale (ep 12 == episodes_count → auto-complete).
	for ep := 1; ep <= 12; ep++ {
		_, err := svc.MarkEpisodeWatched(ctx, "u1", "anime-1", markReq(ep))
		require.NoError(t, err)
	}

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, "completed", row.Status, "finishing the rewatch auto-completes again")
	assert.Equal(t, 1, row.RewatchCount, "completing a rewatch increments rewatch_count once")
	assert.False(t, row.IsRewatching, "flag cleared once the rewatch completes")
}

func TestMarkFinale_NotRewatching_DoesNotIncrement(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	// Fresh first watch: in list as 'watching', is_rewatching=false, count=0.
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count) VALUES ('anime-1','A',12)`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, episodes, is_rewatching, rewatch_count, created_at, updated_at)
		 VALUES ('al-1','u1','anime-1','watching',11,0,0,now(),now())`).Error)

	_, err := svc.MarkEpisodeWatched(ctx, "u1", "anime-1", markReq(12))
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, "completed", row.Status, "first completion still auto-completes")
	assert.Equal(t, 0, row.RewatchCount, "a normal first completion must NOT touch rewatch_count")
}

func TestRewatch_IncrementIsIdempotentOnRepeatedFinaleMarks(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedCompletedFullyWatched(t, db, "u1", "anime-1", 0)
	_, err := svc.Rewatch(ctx, "u1", "anime-1")
	require.NoError(t, err)
	for ep := 1; ep <= 12; ep++ {
		_, err := svc.MarkEpisodeWatched(ctx, "u1", "anime-1", markReq(ep))
		require.NoError(t, err)
	}
	// User taps "mark watched" on the finale again after it already completed.
	_, err = svc.MarkEpisodeWatched(ctx, "u1", "anime-1", markReq(12))
	require.NoError(t, err)

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, 1, row.RewatchCount, "re-marking an already-completed finale must not double-count")
}

func TestRewatch_TwoSequentialRewatches_CountClimbs(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	ctx := context.Background()
	seedCompletedFullyWatched(t, db, "u1", "anime-1", 0)

	for cycle := 1; cycle <= 2; cycle++ {
		_, err := svc.Rewatch(ctx, "u1", "anime-1")
		require.NoError(t, err)
		for ep := 1; ep <= 12; ep++ {
			_, err := svc.MarkEpisodeWatched(ctx, "u1", "anime-1", markReq(ep))
			require.NoError(t, err)
		}
	}

	row := readListRow(t, db, "u1", "anime-1")
	assert.Equal(t, 2, row.RewatchCount, "two completed rewatches → rewatch_count = 2")
}
