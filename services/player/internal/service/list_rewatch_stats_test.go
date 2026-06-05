package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Group 3 — list stats with rewatches (design 2026-06-05).
// Lifetime "episodes watched" = SUM(episodes * (1 + rewatch_count)). For a
// completed entry episodes == total, so a completed rewatch doubles its
// contribution. During an active rewatch (status='watching', episodes reset)
// the value transiently dips — documented and accepted.

func seedStatsEntry(t *testing.T, db *gorm.DB, animeID, status string, episodes, rewatchCount int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes, is_rewatching, rewatch_count, created_at, updated_at)
		 VALUES (?,?,?,?,0,?,0,?,now(),now())`,
		"al-"+animeID, "u1", animeID, status, episodes, rewatchCount).Error)
}

func TestStats_NoRewatch_CountsEpisodesOnce(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedStatsEntry(t, db, "anime-1", "completed", 12, 0)

	stats, err := svc.GetPublicWatchlistStats(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Equal(t, 12, stats.TotalEpisodes)
}

func TestStats_OneCompletedRewatch_DoublesEpisodes(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedStatsEntry(t, db, "anime-1", "completed", 12, 1)

	stats, err := svc.GetPublicWatchlistStats(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Equal(t, 24, stats.TotalEpisodes, "12 episodes watched twice = 24")
}

func TestStats_TwoRewatches_TriplesEpisodes(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedStatsEntry(t, db, "anime-1", "completed", 12, 2)

	stats, err := svc.GetPublicWatchlistStats(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Equal(t, 36, stats.TotalEpisodes)
}

func TestStats_MixedLibrary_SumsPerEntry(t *testing.T) {
	svc, db := setupListServiceTestDB(t)
	seedStatsEntry(t, db, "anime-1", "completed", 12, 0) // 12
	seedStatsEntry(t, db, "anime-2", "completed", 24, 1) // 48
	seedStatsEntry(t, db, "anime-3", "watching", 5, 0)   // 5

	stats, err := svc.GetPublicWatchlistStats(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Equal(t, 65, stats.TotalEpisodes, "12 + 24*2 + 5")
}

func TestStats_ActiveRewatchDip_IsDocumentedBehavior(t *testing.T) {
	// Mid-rewatch: status reset to 'watching', episodes=3, rewatch_count not yet
	// bumped (increments on completion). Contribution is 3*1=3 — a transient dip
	// from the pre-rewatch 12 that self-heals to 24 when the rewatch completes.
	svc, db := setupListServiceTestDB(t)
	seedStatsEntry(t, db, "anime-1", "watching", 3, 0)

	stats, err := svc.GetPublicWatchlistStats(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.TotalEpisodes, "documents the accepted transient dip during an active rewatch")
}
