package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type HistoryService struct {
	historyRepo *repo.HistoryRepository
	log         *logger.Logger
}

func NewHistoryService(historyRepo *repo.HistoryRepository, log *logger.Logger) *HistoryService {
	return &HistoryService{
		historyRepo: historyRepo,
		log:         log,
	}
}

// GetWatchHistory returns user's watch history
func (s *HistoryService) GetWatchHistory(ctx context.Context, userID string, limit int) ([]*domain.WatchHistory, error) {
	return s.historyRepo.GetByUser(ctx, userID, limit)
}
