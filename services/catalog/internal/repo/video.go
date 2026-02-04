package repo

import (
	"context"
	"errors"
	"fmt"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

type VideoRepository struct {
	db *gorm.DB
}

func NewVideoRepository(db *gorm.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func (r *VideoRepository) Create(ctx context.Context, video *domain.Video) error {
	if err := r.db.WithContext(ctx).Create(video).Error; err != nil {
		return fmt.Errorf("create video: %w", err)
	}
	return nil
}

func (r *VideoRepository) GetByID(ctx context.Context, id string) (*domain.Video, error) {
	var video domain.Video
	if err := r.db.WithContext(ctx).First(&video, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("video")
		}
		return nil, fmt.Errorf("get video by id: %w", err)
	}
	return &video, nil
}

func (r *VideoRepository) GetForAnime(ctx context.Context, animeID string, videoType domain.VideoType) ([]*domain.Video, error) {
	query := r.db.WithContext(ctx).Where("anime_id = ?", animeID)
	if videoType != "" {
		query = query.Where("type = ?", videoType)
	}

	var videos []*domain.Video
	if err := query.Order("episode_number, type").Find(&videos).Error; err != nil {
		return nil, fmt.Errorf("get videos for anime: %w", err)
	}
	return videos, nil
}

func (r *VideoRepository) GetForEpisode(ctx context.Context, animeID string, episodeNumber int) ([]*domain.Video, error) {
	var videos []*domain.Video
	err := r.db.WithContext(ctx).
		Where("anime_id = ? AND episode_number = ? AND type = ?", animeID, episodeNumber, "episode").
		Order("quality").
		Find(&videos).Error
	if err != nil {
		return nil, fmt.Errorf("get videos for episode: %w", err)
	}
	return videos, nil
}

func (r *VideoRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&domain.Video{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete video: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("video")
	}
	return nil
}

func (r *VideoRepository) GetRandomVideos(ctx context.Context, videoType domain.VideoType, count int, excludeIDs []string) ([]*domain.Video, error) {
	query := r.db.WithContext(ctx).Where("type = ?", videoType)
	if len(excludeIDs) > 0 {
		query = query.Where("id NOT IN ?", excludeIDs)
	}

	var videos []*domain.Video
	err := query.Order("RANDOM()").Limit(count).Find(&videos).Error
	if err != nil {
		return nil, fmt.Errorf("get random videos: %w", err)
	}
	return videos, nil
}

func (r *VideoRepository) HasVideosForAnime(ctx context.Context, animeID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Video{}).
		Where("anime_id = ? AND type = ?", animeID, "episode").
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check has videos: %w", err)
	}
	return count > 0, nil
}
