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
// mark-watched button). Idempotent: safe to call when the row is missing
// (creates it with progress=0, duration=0, watch_count=1), safe to call when
// the row exists with completed=true (rewatch — increments watch_count).
// Existing progress and duration are preserved on conflict.
//
// Rewatch detection (Phase 5 G-02): on conflict, watch_count increments only
// if the existing row was already completed=true. The first
// completed-flip leaves watch_count at its default of 1.
func (r *ProgressRepository) MarkCompleted(ctx context.Context, userID, animeID string, episodeNumber int) error {
	now := time.Now()
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       animeID,
		EpisodeNumber: episodeNumber,
		Progress:      0,
		Duration:      0,
		Completed:     true,
		WatchCount:    1,
		LastWatchedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}, {Name: "episode_number"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"completed": true,
			// CASE handles three states:
			//   1. row was completed=true → rewatch, increment by 1
			//   2. row was completed=false (heartbeat existed first) → first completion, set to 1
			//   3. row missing → INSERT path uses watch_count=1 from struct
			"watch_count":     gorm.Expr("CASE WHEN watch_progress.completed THEN watch_progress.watch_count + 1 ELSE 1 END"),
			"last_watched_at": now,
			"updated_at":      now,
		}),
	}).Create(progress).Error
}

// MarkDropOff records that the user closed the page mid-episode without
// completing. Idempotent: safe whether the row exists or not. Does NOT touch
// the completed flag — drop-off only annotates an in-progress watch.
//
// Phase 5 gap-fill (G-01). Called from the dropoff beacon endpoint, which
// receives navigator.sendBeacon payloads on pagehide / beforeunload.
func (r *ProgressRepository) MarkDropOff(ctx context.Context, userID, animeID string, episodeNumber, droppedAt int) error {
	now := time.Now()
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       animeID,
		EpisodeNumber: episodeNumber,
		Progress:      droppedAt,
		Duration:      0,
		DroppedOffAt:  &droppedAt,
		LastWatchedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}, {Name: "episode_number"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			// Preserve the highest progress seen — if the user paused at 800s,
			// then resumed and dropped at 600s, the meaningful drop point is
			// still 800s. Same logic as duration in UpsertProgress.
			"progress":        gorm.Expr("GREATEST(watch_progress.progress, ?)", droppedAt),
			"dropped_off_at":  droppedAt,
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
