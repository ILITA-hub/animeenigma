package service

import (
	"context"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

type statsStore interface {
	GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error)
	SaveUserStats(ctx context.Context, st *domain.UserStats) error
}

type StatsService struct{ store statsStore }

func NewStatsService(s statsStore) *StatsService { return &StatsService{store: s} }

func (s *StatsService) Get(ctx context.Context, userID string) (*domain.UserStats, error) {
	st, err := s.store.GetUserStats(ctx, userID)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return &domain.UserStats{UserID: userID, GuessDistribution: map[string]int{}}, nil
	}
	return st, nil
}

// RecordDailyResult updates aggregates + streak for one finished daily game.
func (s *StatsService) RecordDailyResult(ctx context.Context, userID, date string, won bool, attempts int) error {
	st, err := s.store.GetUserStats(ctx, userID)
	if err != nil {
		return err
	}
	if st == nil {
		st = &domain.UserStats{UserID: userID, GuessDistribution: map[string]int{}}
	}
	if st.GuessDistribution == nil {
		st.GuessDistribution = map[string]int{}
	}

	st.GamesPlayed++
	if won {
		st.GamesWon++
		st.GuessDistribution[strconv.Itoa(attempts)]++
		if st.LastPlayedDate != "" && isYesterday(st.LastPlayedDate, date) {
			st.CurrentStreak++
		} else {
			st.CurrentStreak = 1
		}
		if st.CurrentStreak > st.MaxStreak {
			st.MaxStreak = st.CurrentStreak
		}
	} else {
		st.CurrentStreak = 0
	}
	st.LastPlayedDate = date
	st.UpdatedAt = time.Now().UTC()
	return s.store.SaveUserStats(ctx, st)
}

// isYesterday reports whether `prev` is exactly one day before `cur` (both "2006-01-02").
func isYesterday(prev, cur string) bool {
	p, err1 := time.Parse("2006-01-02", prev)
	c, err2 := time.Parse("2006-01-02", cur)
	if err1 != nil || err2 != nil {
		return false
	}
	return c.Sub(p) == 24*time.Hour
}
