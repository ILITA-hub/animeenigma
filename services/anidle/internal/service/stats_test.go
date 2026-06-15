package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

type fakeStatsStore struct{ stats map[string]*domain.UserStats }

func newFakeStatsStore() *fakeStatsStore { return &fakeStatsStore{stats: map[string]*domain.UserStats{}} }
func (f *fakeStatsStore) GetUserStats(_ context.Context, u string) (*domain.UserStats, error) {
	return f.stats[u], nil
}
func (f *fakeStatsStore) SaveUserStats(_ context.Context, st *domain.UserStats) error {
	f.stats[st.UserID] = st
	return nil
}

func TestStats_StreakIncrementsOnConsecutiveDays(t *testing.T) {
	store := newFakeStatsStore()
	svc := NewStatsService(store)
	ctx := context.Background()

	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-14", true, 3))
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-15", true, 2))

	st, _ := store.GetUserStats(ctx, "u1")
	assert.Equal(t, 2, st.GamesWon)
	assert.Equal(t, 2, st.CurrentStreak)
	assert.Equal(t, 2, st.MaxStreak)
	assert.Equal(t, 1, st.GuessDistribution["2"])
	assert.Equal(t, 1, st.GuessDistribution["3"])
}

func TestStats_StreakResetsAfterGap(t *testing.T) {
	store := newFakeStatsStore()
	svc := NewStatsService(store)
	ctx := context.Background()
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-10", true, 1))
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-15", true, 1)) // 5-day gap
	st, _ := store.GetUserStats(ctx, "u1")
	assert.Equal(t, 1, st.CurrentStreak)
	assert.Equal(t, 1, st.MaxStreak)
}

func TestStats_LossBreaksStreak(t *testing.T) {
	store := newFakeStatsStore()
	svc := NewStatsService(store)
	ctx := context.Background()
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-14", true, 1))
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-15", false, 0)) // gave up
	st, _ := store.GetUserStats(ctx, "u1")
	assert.Equal(t, 0, st.CurrentStreak)
	assert.Equal(t, 1, st.GamesWon)
	assert.Equal(t, 2, st.GamesPlayed)
}
