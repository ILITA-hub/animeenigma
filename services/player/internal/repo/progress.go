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

// ListContinueWatching returns the user's most-recent in-progress episode per
// anime (one row per anime), ordered by last_watched_at DESC. Uses a window
// function (ROW_NUMBER() OVER (PARTITION BY anime_id ORDER BY
// last_watched_at DESC)) so the JOIN against animes is one-shot.
//
// "In-progress" for MVP = watch_progress.completed = false. The
// "completed-but-next-episode-exists" Crunchyroll case is deferred to Phase 9.
//
// limit is clamped to [1, 20]; default (limit<=0) is 10.
//
// Phase 8 (UX-15 / UA-061).
func (r *ProgressRepository) ListContinueWatching(
	ctx context.Context, userID string, limit int,
) ([]*domain.ContinueWatchingItem, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}

	type scanRow struct {
		AnimeID       string    `gorm:"column:anime_id"`
		EpisodeNumber int       `gorm:"column:episode_number"`
		Progress      int       `gorm:"column:progress"`
		Duration      int       `gorm:"column:duration"`
		LastWatchedAt time.Time `gorm:"column:last_watched_at"`
		DroppedOffAt  *int      `gorm:"column:dropped_off_at"`
		AnimeName     string    `gorm:"column:anime_name"`
		AnimeNameRU   string    `gorm:"column:anime_name_ru"`
		AnimeNameJP   string    `gorm:"column:anime_name_jp"`
		AnimePoster   string    `gorm:"column:anime_poster"`
		AnimeEpisodes int       `gorm:"column:anime_episodes"`
	}

	sqlStr := `
        WITH ranked AS (
            SELECT
                wp.anime_id,
                wp.episode_number,
                wp.progress,
                wp.duration,
                wp.last_watched_at,
                wp.dropped_off_at,
                ROW_NUMBER() OVER (
                    PARTITION BY wp.anime_id
                    ORDER BY wp.last_watched_at DESC
                ) AS rn
            FROM watch_progress wp
            WHERE wp.user_id = ?
              AND wp.completed = false
        )
        SELECT
            r.anime_id,
            r.episode_number,
            r.progress,
            r.duration,
            r.last_watched_at,
            r.dropped_off_at,
            a.name           AS anime_name,
            a.name_ru        AS anime_name_ru,
            a.name_jp        AS anime_name_jp,
            a.poster_url     AS anime_poster,
            a.episodes_count AS anime_episodes
        FROM ranked r
        JOIN animes a ON a.id = r.anime_id
        WHERE r.rn = 1
          AND a.deleted_at IS NULL
        ORDER BY r.last_watched_at DESC
        LIMIT ?`

	var rows []scanRow
	if err := r.db.WithContext(ctx).
		Raw(sqlStr, userID, limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]*domain.ContinueWatchingItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, &domain.ContinueWatchingItem{
			Anime: domain.AnimeInfo{
				ID:            row.AnimeID,
				Name:          row.AnimeName,
				NameRU:        row.AnimeNameRU,
				NameJP:        row.AnimeNameJP,
				PosterURL:     row.AnimePoster,
				EpisodesCount: row.AnimeEpisodes,
			},
			EpisodeNumber: row.EpisodeNumber,
			Progress:      row.Progress,
			Duration:      row.Duration,
			LastWatchedAt: row.LastWatchedAt,
			DroppedOffAt:  row.DroppedOffAt,
		})
	}
	return items, nil
}
