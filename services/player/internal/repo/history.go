package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

type HistoryRepository struct {
	db *database.Database
}

func NewHistoryRepository(db *database.Database) *HistoryRepository {
	return &HistoryRepository{
		db: db,
	}
}

// GetByUser returns watch history for a user
func (r *HistoryRepository) GetByUser(ctx context.Context, userID string, limit int) ([]*domain.WatchHistory, error) {
	// Stub implementation
	return []*domain.WatchHistory{}, nil
}

// Create adds a new watch history entry
func (r *HistoryRepository) Create(ctx context.Context, history *domain.WatchHistory) error {
	// Stub implementation
	return nil
}
