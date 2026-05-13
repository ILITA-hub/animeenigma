package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProgressRepository struct {
	db  *gorm.DB
	log *logger.Logger // optional; nil-safe via logIfPresent
}

func NewProgressRepository(db *gorm.DB) *ProgressRepository {
	return &ProgressRepository{db: db}
}

// WithLogger attaches an optional logger. Used by main.go for observability
// (Phase 8 / WR-02: warn when ListContinueWatching's INNER JOIN drops rows
// whose anime row is missing or soft-deleted). Tests call NewProgressRepository
// without a logger, in which case observability warnings are silently skipped.
func (r *ProgressRepository) WithLogger(l *logger.Logger) *ProgressRepository {
	r.log = l
	return r
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

	// WR-02 (Phase 8): the INNER JOIN against animes silently drops any
	// in-progress watch_progress rows whose anime row is missing or
	// soft-deleted (a.deleted_at IS NOT NULL). User-facing impact is small
	// (missing cards the user can't act on anyway) but the silent filter
	// masks data-integrity issues — a real bug elsewhere could be losing
	// watch_progress entries and this query would hide the symptom.
	//
	// Compare the JOIN result to the underlying "distinct anime_id in
	// watch_progress" count for this user; warn on drift. This is a
	// separate count query rather than a CTE-on-CTE rewrite because the
	// observability check must succeed even when the main query returns
	// fewer rows than expected — wrapping it inside the same statement
	// would hide the symptom we're trying to surface.
	if r.log != nil {
		var distinctAnimeCount int64
		// Count animes that the user has any in-progress (completed=false)
		// progress against. This is the upper bound for ListContinueWatching
		// output before the JOIN-induced filtering.
		countErr := r.db.WithContext(ctx).Raw(
			`SELECT COUNT(DISTINCT anime_id)
             FROM watch_progress
             WHERE user_id = ? AND completed = false`,
			userID,
		).Scan(&distinctAnimeCount).Error
		if countErr == nil {
			// Cap the upper bound at the request limit — the query was
			// asked for at most `limit` rows, so dropping below `limit`
			// when the upper bound is < limit is not "drift".
			expected := distinctAnimeCount
			if expected > int64(limit) {
				expected = int64(limit)
			}
			if int64(len(rows)) < expected {
				r.log.Warnw(
					"continue-watching INNER JOIN dropped rows; orphaned or soft-deleted anime in watch_progress",
					"user_id", userID,
					"expected", expected,
					"returned", len(rows),
					"dropped", expected-int64(len(rows)),
				)
			}
		}
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

// GetBulkProgress returns the per-anime progress map for the given user and
// anime IDs. One entry per anime — the row with the highest episode_number
// (breaking ties on last_watched_at DESC) joined against the animes table
// for episodes_count + episodes_aired. Animes the user has no progress on
// are omitted from the map; callers treat absence as "no badge".
//
// Empty-input fast-path: returns an empty map without hitting the DB.
//
// Phase 9 (UX-16).
func (r *ProgressRepository) GetBulkProgress(
	ctx context.Context, userID string, animeIDs []string,
) (domain.BulkAnimeProgressMap, error) {
	if len(animeIDs) == 0 {
		return domain.BulkAnimeProgressMap{}, nil
	}

	type scanRow struct {
		AnimeID       string `gorm:"column:anime_id"`
		LatestEpisode int    `gorm:"column:latest_episode"`
		Completed     bool   `gorm:"column:completed"`
		DroppedOffAt  *int   `gorm:"column:dropped_off_at"`
		EpisodesCount int    `gorm:"column:episodes_count"`
		EpisodesAired int    `gorm:"column:episodes_aired"`
	}

	sqlStr := `
        WITH ranked AS (
            SELECT
                wp.anime_id,
                wp.episode_number,
                wp.completed,
                wp.dropped_off_at,
                wp.last_watched_at,
                ROW_NUMBER() OVER (
                    PARTITION BY wp.anime_id
                    ORDER BY wp.episode_number DESC, wp.last_watched_at DESC
                ) AS rn
            FROM watch_progress wp
            WHERE wp.user_id = ?
              AND wp.anime_id IN (?)
        )
        SELECT
            r.anime_id              AS anime_id,
            r.episode_number        AS latest_episode,
            r.completed             AS completed,
            r.dropped_off_at        AS dropped_off_at,
            a.episodes_count        AS episodes_count,
            COALESCE(a.episodes_aired, 0) AS episodes_aired
        FROM ranked r
        JOIN animes a ON a.id = r.anime_id
        WHERE r.rn = 1
          AND a.deleted_at IS NULL`

	var rows []scanRow
	if err := r.db.WithContext(ctx).
		Raw(sqlStr, userID, animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(domain.BulkAnimeProgressMap, len(rows))
	for _, row := range rows {
		// "Completed" for badge purposes is stricter than just row.completed=true.
		// A user who marked only E1 of a 12-episode show as completed is still
		// "in progress" — the badge should still show. The Completed flag
		// flips true only when the user marked their FURTHEST episode completed
		// AND that furthest episode is at least episodes_count (or episodes_aired
		// when count is unknown / not-yet-finished show).
		reachedAll := false
		if row.EpisodesCount > 0 && row.LatestEpisode >= row.EpisodesCount {
			reachedAll = true
		} else if row.EpisodesCount == 0 && row.EpisodesAired > 0 && row.LatestEpisode >= row.EpisodesAired {
			reachedAll = true
		}
		out[row.AnimeID] = domain.BulkAnimeProgressEntry{
			LatestEpisode: row.LatestEpisode,
			EpisodesCount: row.EpisodesCount,
			EpisodesAired: row.EpisodesAired,
			Completed:     row.Completed && reachedAll,
			Dropped:       row.DroppedOffAt != nil && !row.Completed,
		}
	}
	return out, nil
}
