package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProgressRepository struct {
	db *gorm.DB
}

func NewProgressRepository(db *gorm.DB) *ProgressRepository {
	return &ProgressRepository{db: db}
}

func (r *ProgressRepository) Upsert(ctx context.Context, progress *domain.WatchProgress) error {
	now := time.Now()
	progress.LastWatchedAt = now
	progress.UpdatedAt = now
	if progress.CreatedAt.IsZero() {
		progress.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}, {Name: "episode_number"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"progress":        progress.Progress,
			"duration":        gorm.Expr("GREATEST(watch_progress.duration, ?)", progress.Duration),
			"completed":       progress.Completed,
			"last_watched_at": progress.LastWatchedAt,
			"updated_at":      progress.UpdatedAt,
		}),
	}).Create(progress).Error
}

func (r *ProgressRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) ([]*domain.WatchProgress, error) {
	var results []*domain.WatchProgress
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Order("episode_number").
		Find(&results).Error
	return results, err
}

func (r *ProgressRepository) GetByUserAnimeEpisode(ctx context.Context, userID, animeID string, episode int) (*domain.WatchProgress, error) {
	var p domain.WatchProgress
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ? AND episode_number = ?", userID, animeID, episode).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}
