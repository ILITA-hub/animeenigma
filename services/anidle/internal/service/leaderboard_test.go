package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeZSet struct{ members map[string]map[string]float64 }

func newFakeZSet() *fakeZSet { return &fakeZSet{members: map[string]map[string]float64{}} }
func (f *fakeZSet) ZAdd(_ context.Context, key, member string, score float64) error {
	if f.members[key] == nil {
		f.members[key] = map[string]float64{}
	}
	f.members[key][member] = score
	return nil
}
func (f *fakeZSet) ZRangeAsc(_ context.Context, key string, n int) ([]ZEntry, error) {
	type kv struct {
		m string
		s float64
	}
	var all []kv
	for m, s := range f.members[key] {
		all = append(all, kv{m, s})
	}
	// simple insertion sort ascending by score
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].s < all[j-1].s; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	out := []ZEntry{}
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, ZEntry{Member: all[i].m, Score: all[i].s})
	}
	return out, nil
}

func TestLeaderboard_RanksByFewerAttemptsThenEarlier(t *testing.T) {
	z := newFakeZSet()
	lb := NewLeaderboardService(z)
	ctx := context.Background()

	require.NoError(t, lb.RecordSolve(ctx, "2026-06-15", "alice", 4, 1000))
	require.NoError(t, lb.RecordSolve(ctx, "2026-06-15", "bob", 2, 2000))
	require.NoError(t, lb.RecordSolve(ctx, "2026-06-15", "carol", 2, 1500))

	top, err := lb.Top(ctx, "2026-06-15", 10)
	require.NoError(t, err)
	require.Len(t, top, 3)
	// 2 attempts beats 4; among 2-attempt solvers, earlier solve (1500) beats later (2000)
	assert.Equal(t, "carol", top[0].Username)
	assert.Equal(t, "bob", top[1].Username)
	assert.Equal(t, "alice", top[2].Username)
	assert.Equal(t, 2, top[0].Attempts)
}
