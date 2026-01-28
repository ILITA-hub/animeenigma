package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/google/uuid"
)

type VideoRepository struct {
	db *database.DB
}

func NewVideoRepository(db *database.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func (r *VideoRepository) Create(ctx context.Context, video *domain.Video) error {
	video.ID = uuid.New().String()
	video.CreatedAt = time.Now()

	query := `
		INSERT INTO videos (
			id, anime_id, type, episode_number, name, source_type,
			source_url, storage_key, quality, language, duration, thumbnail_url, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := r.db.ExecContext(ctx, query,
		video.ID, video.AnimeID, video.Type, video.EpisodeNumber, video.Name,
		video.SourceType, video.SourceURL, video.StorageKey, video.Quality,
		video.Language, video.Duration, video.ThumbnailURL, video.CreatedAt)

	if err != nil {
		return fmt.Errorf("create video: %w", err)
	}

	return nil
}

func (r *VideoRepository) GetByID(ctx context.Context, id string) (*domain.Video, error) {
	query := `
		SELECT v.id, v.anime_id, a.name as anime_name, v.type, v.episode_number, v.name,
			v.source_type, v.source_url, v.storage_key, v.quality, v.language,
			v.duration, v.thumbnail_url, v.created_at
		FROM videos v
		INNER JOIN anime a ON v.anime_id = a.id
		WHERE v.id = $1
	`

	var video domain.Video
	err := r.db.GetContext(ctx, &video, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFound("video")
		}
		return nil, fmt.Errorf("get video by id: %w", err)
	}

	return &video, nil
}

func (r *VideoRepository) GetForAnime(ctx context.Context, animeID string, videoType domain.VideoType) ([]*domain.Video, error) {
	query := `
		SELECT id, anime_id, type, episode_number, name, source_type,
			source_url, storage_key, quality, language, duration, thumbnail_url, created_at
		FROM videos
		WHERE anime_id = $1
	`

	args := []interface{}{animeID}
	if videoType != "" {
		query += " AND type = $2"
		args = append(args, videoType)
	}

	query += " ORDER BY episode_number, type"

	var videos []*domain.Video
	if err := r.db.SelectContext(ctx, &videos, query, args...); err != nil {
		return nil, fmt.Errorf("get videos for anime: %w", err)
	}

	return videos, nil
}

func (r *VideoRepository) GetForEpisode(ctx context.Context, animeID string, episodeNumber int) ([]*domain.Video, error) {
	query := `
		SELECT id, anime_id, type, episode_number, name, source_type,
			source_url, storage_key, quality, language, duration, thumbnail_url, created_at
		FROM videos
		WHERE anime_id = $1 AND episode_number = $2 AND type = 'episode'
		ORDER BY quality
	`

	var videos []*domain.Video
	if err := r.db.SelectContext(ctx, &videos, query, animeID, episodeNumber); err != nil {
		return nil, fmt.Errorf("get videos for episode: %w", err)
	}

	return videos, nil
}

func (r *VideoRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM videos WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete video: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.NotFound("video")
	}

	return nil
}

func (r *VideoRepository) GetRandomVideos(ctx context.Context, videoType domain.VideoType, count int, excludeIDs []string) ([]*domain.Video, error) {
	query := `
		SELECT v.id, v.anime_id, a.name as anime_name, v.type, v.episode_number, v.name,
			v.source_type, v.source_url, v.storage_key, v.quality, v.language,
			v.duration, v.thumbnail_url, v.created_at
		FROM videos v
		INNER JOIN anime a ON v.anime_id = a.id
		WHERE v.type = $1
	`

	args := []interface{}{videoType}
	argIdx := 2

	if len(excludeIDs) > 0 {
		query += fmt.Sprintf(" AND v.id NOT IN (SELECT unnest($%d::text[]))", argIdx)
		args = append(args, excludeIDs)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY RANDOM() LIMIT $%d", argIdx)
	args = append(args, count)

	var videos []*domain.Video
	if err := r.db.SelectContext(ctx, &videos, query, args...); err != nil {
		return nil, fmt.Errorf("get random videos: %w", err)
	}

	return videos, nil
}

func (r *VideoRepository) HasVideosForAnime(ctx context.Context, animeID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM videos WHERE anime_id = $1 AND type = 'episode')`

	var exists bool
	if err := r.db.GetContext(ctx, &exists, query, animeID); err != nil {
		return false, fmt.Errorf("check has videos: %w", err)
	}

	return exists, nil
}
