package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/google/uuid"
)

type ProgressRepository struct {
	db *database.DB
}

func NewProgressRepository(db *database.DB) *ProgressRepository {
	return &ProgressRepository{
		db: db,
	}
}

// Upsert creates or updates watch progress
func (r *ProgressRepository) Upsert(ctx context.Context, progress *domain.WatchProgress) error {
	query := `
		INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, anime_id, episode_number)
		DO UPDATE SET
			progress = EXCLUDED.progress,
			duration = GREATEST(watch_progress.duration, EXCLUDED.duration),
			completed = EXCLUDED.completed,
			last_watched_at = EXCLUDED.last_watched_at,
			updated_at = EXCLUDED.updated_at
	`

	id := progress.ID
	if id == "" {
		id = uuid.New().String()
	}

	now := time.Now()

	_, err := r.db.ExecContext(ctx, query,
		id,
		progress.UserID,
		progress.AnimeID,
		progress.EpisodeNumber,
		progress.Progress,
		progress.Duration,
		progress.Completed,
		now,
		now,
		now,
	)

	return err
}

// GetByUserAndAnime returns watch progress for a user's anime
func (r *ProgressRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) ([]*domain.WatchProgress, error) {
	query := `
		SELECT id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at
		FROM watch_progress
		WHERE user_id = $1 AND anime_id = $2
		ORDER BY episode_number
	`

	var results []*domain.WatchProgress
	err := r.db.SelectContext(ctx, &results, query, userID, animeID)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetByUserAnimeEpisode returns progress for specific episode
func (r *ProgressRepository) GetByUserAnimeEpisode(ctx context.Context, userID, animeID string, episode int) (*domain.WatchProgress, error) {
	query := `
		SELECT id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at
		FROM watch_progress
		WHERE user_id = $1 AND anime_id = $2 AND episode_number = $3
	`

	var p domain.WatchProgress
	err := r.db.GetContext(ctx, &p, query, userID, animeID, episode)
	if err != nil {
		return nil, err
	}

	return &p, nil
}
