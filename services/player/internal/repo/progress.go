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

// ResetForAnime clears per-episode completion for a fresh rewatch cycle:
// completed=false and progress=0 for every row of (user, anime). Rows are KEPT
// (not deleted) and the append-only watch_history audit trail is untouched, so
// only the resume state machine's "highest completed episode" is rewound.
// Design 2026-06-05.
func (r *ProgressRepository) ResetForAnime(ctx context.Context, userID, animeID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.WatchProgress{}).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Updates(map[string]interface{}{
			"completed":  false,
			"progress":   0,
			"updated_at": now,
		}).Error
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

// LogicBContext returns the gating data the player Logic-B producer needs to
// decide whether to fire a next_ep autocache demand for an active JP-audio
// watcher (Phase 9 / TRIG-02), in a single query keyed by (userID, animeID):
//
//   - shikimoriID    — animes.shikimori_id (the library's mal_id target)
//   - episodesAired  — animes.episodes_aired (the aired cap for N+1)
//   - watching       — true iff anime_list.status = 'watching' for this user
//
// When the user has no watching-list row for the anime (or the anime row is
// absent/soft-deleted), it returns watching=false with a nil error — "no
// demand", NOT a failure. The fire path treats this as a no-op. The INNER JOIN
// to animes mirrors the existing animes JOINs in this file.
func (r *ProgressRepository) LogicBContext(
	ctx context.Context, userID, animeID string,
) (shikimoriID string, episodesAired int, watching bool, err error) {
	type scanRow struct {
		ShikimoriID   string `gorm:"column:shikimori_id"`
		EpisodesAired int    `gorm:"column:episodes_aired"`
	}

	sqlStr := `
        SELECT
            a.shikimori_id                AS shikimori_id,
            COALESCE(a.episodes_aired, 0) AS episodes_aired
        FROM anime_list al
        JOIN animes a ON a.id = al.anime_id AND a.deleted_at IS NULL
        WHERE al.user_id = ?
          AND al.anime_id = ?
          AND al.status = 'watching'
        LIMIT 1`

	var rows []scanRow
	if err := r.db.WithContext(ctx).Raw(sqlStr, userID, animeID).Scan(&rows).Error; err != nil {
		return "", 0, false, err
	}
	if len(rows) == 0 {
		// No watching-list row → not a watching anime for this user. Not an error.
		return "", 0, false, nil
	}
	return rows[0].ShikimoriID, rows[0].EpisodesAired, true, nil
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

// ListContinueWatching returns the "continue watching" rail for the user.
//
// Semantics (rewritten 2026-06-01): the rail is driven by the user's LIST
// (anime_list.status = 'watching') intersected with REAL available episodes —
// not by stale watch_progress.completed flags alone. For each watching-list
// anime the user has started, we take their most-recent watch_progress row and:
//
//   - if that row is still in progress (completed=false) → resume that episode
//     with its saved progress/duration;
//   - if that row is completed → advance to the next episode (progress 0), but
//     ONLY when a real next episode exists (next <= available episodes, where
//     available = episodes_aired for ongoing shows, else episodes_count).
//
// This means: anime the user dropped/completed (status != 'watching'), anime
// they never added to a list, and anime they've fully caught up on are all
// excluded — which is what users expect from a "continue watching" rail.
// One row per anime, ordered by last_watched_at DESC.
//
// limit is clamped to [1, 20]; default (limit<=0) is 10.
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
        WITH latest AS (
            SELECT
                wp.anime_id,
                wp.episode_number,
                wp.progress,
                wp.duration,
                wp.completed,
                wp.last_watched_at,
                wp.dropped_off_at,
                ROW_NUMBER() OVER (
                    PARTITION BY wp.anime_id
                    ORDER BY wp.last_watched_at DESC
                ) AS rn
            FROM watch_progress wp
            WHERE wp.user_id = ?
        )
        SELECT
            l.anime_id,
            CASE WHEN l.completed THEN l.episode_number + 1 ELSE l.episode_number END AS episode_number,
            CASE WHEN l.completed THEN 0 ELSE l.progress END                          AS progress,
            CASE WHEN l.completed THEN 0 ELSE l.duration END                          AS duration,
            l.last_watched_at,
            CASE WHEN l.completed THEN NULL ELSE l.dropped_off_at END                 AS dropped_off_at,
            a.name           AS anime_name,
            a.name_ru        AS anime_name_ru,
            a.name_jp        AS anime_name_jp,
            a.poster_url     AS anime_poster,
            a.episodes_count AS anime_episodes
        FROM anime_list al
        JOIN animes a ON a.id = al.anime_id AND a.deleted_at IS NULL
        JOIN latest l ON l.anime_id = al.anime_id AND l.rn = 1
        WHERE al.user_id = ?
          AND al.status = 'watching'
          AND (
                l.completed = false
                OR (
                    (CASE WHEN COALESCE(a.episodes_aired, 0) > 0
                          THEN a.episodes_aired ELSE COALESCE(a.episodes_count, 0) END) > 0
                    AND (l.episode_number + 1) <=
                        (CASE WHEN COALESCE(a.episodes_aired, 0) > 0
                              THEN a.episodes_aired ELSE COALESCE(a.episodes_count, 0) END)
                )
              )
        ORDER BY l.last_watched_at DESC
        LIMIT ?`

	var rows []scanRow
	if err := r.db.WithContext(ctx).
		Raw(sqlStr, userID, userID, limit).
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
