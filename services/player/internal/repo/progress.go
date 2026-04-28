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

// UpsertProgress writes heartbeat progress for an episode (called from
// UpdateProgress on every player save). It updates progress, duration, and
// last_watched_at, but intentionally does NOT touch the completed flag —
// completion is a discrete event written via MarkCompleted. This makes
// completed sticky against heartbeat saves: once an episode is marked
// completed, subsequent progress saves do not reset it to false.
func (r *ProgressRepository) UpsertProgress(ctx context.Context, progress *domain.WatchProgress) error {
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
			"last_watched_at": progress.LastWatchedAt,
			"updated_at":      progress.UpdatedAt,
		}),
	}).Create(progress).Error
}

// MarkCompleted sets watch_progress.completed=true for an episode.
// Called from MarkEpisodeWatched (both the 20-min auto-mark and the manual
// mark-watched button). Idempotent: safe to call when the row exists with
// completed=true (no-op), safe to call when the row is missing (creates it
// with progress=0, duration=0). Existing progress and duration are preserved
// on conflict.
func (r *ProgressRepository) MarkCompleted(ctx context.Context, userID, animeID string, episodeNumber int) error {
	now := time.Now()
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       animeID,
		EpisodeNumber: episodeNumber,
		Progress:      0,
		Duration:      0,
		Completed:     true,
		LastWatchedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}, {Name: "episode_number"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"completed":       true,
			"last_watched_at": now,
			"updated_at":      now,
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
