package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

type HistoryRepository struct {
	db *gorm.DB
}

func NewHistoryRepository(db *gorm.DB) *HistoryRepository {
	return &HistoryRepository{db: db}
}

func (r *HistoryRepository) GetByUser(ctx context.Context, userID string, limit int) ([]*domain.WatchHistory, error) {
	var history []*domain.WatchHistory
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("watched_at DESC").
		Limit(limit).
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) Create(ctx context.Context, history *domain.WatchHistory) error {
	return r.db.WithContext(ctx).Create(history).Error
}
