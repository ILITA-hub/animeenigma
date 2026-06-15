package service

import (
	"context"
	"fmt"
)

// ZEntry is one sorted-set member+score.
type ZEntry struct {
	Member string
	Score  float64
}

type zsetStore interface {
	ZAdd(ctx context.Context, key, member string, score float64) error
	ZRangeAsc(ctx context.Context, key string, n int) ([]ZEntry, error)
}

type LeaderboardService struct{ z zsetStore }

func NewLeaderboardService(z zsetStore) *LeaderboardService { return &LeaderboardService{z: z} }

// LeaderEntry is one row of the daily leaderboard.
type LeaderEntry struct {
	Username string `json:"username"`
	Attempts int    `json:"attempts"`
}

const attemptsWeight = 1e10 // attempts dominate; solve-time breaks ties

func lbKey(date string) string { return "anidle:leaderboard:" + date }

// RecordSolve adds a solver. Score packs (attempts, solveUnix) so ascending
// ZRange = fewest attempts first, earliest solve first within a tie.
func (s *LeaderboardService) RecordSolve(ctx context.Context, date, username string, attempts int, solveUnix int64) error {
	score := float64(attempts)*attemptsWeight + float64(solveUnix)
	return s.z.ZAdd(ctx, lbKey(date), username, score)
}

func (s *LeaderboardService) Top(ctx context.Context, date string, n int) ([]LeaderEntry, error) {
	entries, err := s.z.ZRangeAsc(ctx, lbKey(date), n)
	if err != nil {
		return nil, fmt.Errorf("leaderboard top: %w", err)
	}
	out := make([]LeaderEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, LeaderEntry{
			Username: e.Member,
			Attempts: int(e.Score / attemptsWeight),
		})
	}
	return out, nil
}
