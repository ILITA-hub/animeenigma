package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/domain"
)

type LeaderboardService struct {
	log *logger.Logger
}

func NewLeaderboardService(log *logger.Logger) *LeaderboardService {
	return &LeaderboardService{
		log: log,
	}
}

// GetGlobalLeaderboard returns the top players globally
func (s *LeaderboardService) GetGlobalLeaderboard(ctx context.Context, limit int) ([]*domain.LeaderboardEntry, error) {
	// In a real implementation, this would fetch from database
	// For now, return sample data
	leaderboard := []*domain.LeaderboardEntry{
		{
			UserID:      "1",
			Username:    "player1",
			TotalScore:  1500,
			GamesPlayed: 50,
			GamesWon:    25,
		},
		{
			UserID:      "2",
			Username:    "player2",
			TotalScore:  1200,
			GamesPlayed: 40,
			GamesWon:    15,
		},
	}

	return leaderboard, nil
}

// UpdatePlayerStats updates a player's leaderboard stats
func (s *LeaderboardService) UpdatePlayerStats(ctx context.Context, userID string, score int, won bool) error {
	// In a real implementation, this would update the database
	s.log.Infow("updating player stats", "user_id", userID, "score", score, "won", won)
	return nil
}
