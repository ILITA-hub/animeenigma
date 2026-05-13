package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// allowedSortFields is a whitelist of column names that can appear in ORDER BY.
var allowedSortFields = map[string]bool{
	"updated_at": true,
	"created_at": true,
	"score":      true,
	"status":     true,
	"episodes":   true,
	"title":      true, // handled specially via JOIN
}

// sanitizedOrderClause returns a safe ORDER BY clause built from validated
// sort field and direction. If either value is invalid, it falls back to
// "updated_at DESC".
func sanitizedOrderClause(sort, order string) string {
	if !allowedSortFields[sort] {
		return "updated_at DESC"
	}
	dir := strings.ToUpper(order)
	if dir != "ASC" && dir != "DESC" {
		dir = "DESC"
	}
	if sort == "title" {
		return "animes.name " + dir
	}
	return "anime_list." + sort + " " + dir
}

// isTitleSort returns true when the requested sort requires a JOIN with animes.
func isTitleSort(sort string) bool {
	return sort == "title"
}

type ListRepository struct {
	db *gorm.DB
}

func NewListRepository(db *gorm.DB) *ListRepository {
	return &ListRepository{db: db}
}

func (r *ListRepository) Upsert(ctx context.Context, entry *domain.AnimeListEntry) error {
	now := time.Now()
	entry.UpdatedAt = now
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":        entry.Status,
			"score":         entry.Score,
			"episodes":      entry.Episodes,
			"notes":         gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.notes)", entry.Notes),
			"tags":          gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.tags)", entry.Tags),
			"is_rewatching": entry.IsRewatching,
			"priority":      gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.priority)", entry.Priority),
			"mal_id":        gorm.Expr("COALESCE(?, anime_list.mal_id)", entry.MalID),
			"started_at":    gorm.Expr("COALESCE(?, anime_list.started_at)", entry.StartedAt),
			"completed_at":  gorm.Expr("COALESCE(?, anime_list.completed_at)", entry.CompletedAt),
			"updated_at":    entry.UpdatedAt,
		}),
	}).Create(entry).Error
}

func (r *ListRepository) GetByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndStatus(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ? AND status = ?", userID, status).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	var entry domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &entry, err
}

func (r *ListRepository) Delete(ctx context.Context, userID, animeID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Delete(&domain.AnimeListEntry{}).Error
}

func (r *ListRepository) IncrementEpisodes(ctx context.Context, userID, animeID string, episodeNumber int) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE anime_list SET
			episodes = ?,
			status = CASE
				WHEN a.episodes_count > 0 AND ? >= a.episodes_count THEN 'completed'
				WHEN anime_list.status = 'plan_to_watch' THEN 'watching'
				ELSE anime_list.status END,
			started_at = COALESCE(anime_list.started_at, NOW()),
			completed_at = CASE
				WHEN a.episodes_count > 0 AND ? >= a.episodes_count THEN NOW()
				ELSE anime_list.completed_at END,
			updated_at = NOW()
		FROM animes a
		WHERE anime_list.anime_id = a.id
		  AND anime_list.user_id = ? AND anime_list.anime_id = ?
		  AND anime_list.episodes < ?`,
		episodeNumber, episodeNumber, episodeNumber, userID, animeID, episodeNumber)
	return result.RowsAffected > 0, result.Error
}

// CountWatchers returns the number of anime_list rows where the given anime is
// being actively watched (status='watching'). Used by Phase 14 / UX-28 to
// render a soft social-proof badge on the anime detail view. Soft-deleted
// rows are excluded automatically by GORM's gorm.DeletedAt handling on the
// model.
func (r *ListRepository) CountWatchers(ctx context.Context, animeID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Where("anime_id = ? AND status = ?", animeID, "watching").
		Count(&count).Error
	return count, err
}

func (r *ListRepository) GetByUserAndStatuses(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("anime_list.user_id = ? AND anime_list.status IN ?", userID, statuses).
		Order("anime_list.updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserPaginated(ctx context.Context, userID, status, search string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("anime_list.user_id = ?", userID)
	if status != "" {
		base = base.Where("anime_list.status = ?", status)
	}

	needsAnimesJoin := isTitleSort(params.Sort) || search != ""
	if needsAnimesJoin {
		base = base.Joins("LEFT JOIN animes ON animes.id = anime_list.anime_id")
	}
	if search != "" {
		like := "%" + search + "%"
		base = base.Where(
			"animes.name ILIKE ? OR animes.name_ru ILIKE ? OR animes.name_jp ILIKE ?",
			like, like, like,
		)
	}

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(sanitizedOrderClause(params.Sort, params.Order)).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}

func (r *ListRepository) GetByUserStatuses(ctx context.Context, userID string) ([]domain.AnimeStatusEntry, error) {
	var entries []domain.AnimeStatusEntry
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Select("anime_id, status, score, episodes").
		Where("user_id = ?", userID).
		Scan(&entries).Error
	return entries, err
}

func (r *ListRepository) GetUserWatchlistStats(ctx context.Context, userID string, statuses []string) (*domain.WatchlistStats, error) {
	var stats domain.WatchlistStats

	base := r.db.WithContext(ctx).Model(&domain.AnimeListEntry{}).Where("anime_list.user_id = ?", userID)
	if len(statuses) > 0 {
		base = base.Where("anime_list.status IN ?", statuses)
	}

	err := base.Select(
		"COALESCE(AVG(NULLIF(anime_list.score, 0)), 0) as avg_score, "+
			"COALESCE(SUM(anime_list.episodes), 0) as total_episodes, "+
			"COUNT(*) as total_entries, "+
			"COUNT(CASE WHEN anime_list.status = 'completed' THEN 1 END) as completed",
	).Scan(&stats).Error

	return &stats, err
}

// --- Phase 1 (workstream: social) plan 02 — review-shaped queries -------
// These methods power the six review endpoints by operating on the same
// anime_list table that powers the watchlist, filtered by
// `(score > 0 OR review_text != '')` so MAL-imported score=8 rows and
// written-review rows both qualify as "reviews".

// GetReviewsByAnime returns every anime_list row for `animeID` that has
// either a non-zero score OR a non-empty review_text. Preloads Anime so the
// handler can include the existing JSON `anime` field unchanged.
func (r *ListRepository) GetReviewsByAnime(ctx context.Context, animeID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").
		Where("anime_id = ? AND (score > 0 OR review_text <> '')", animeID).
		Order("created_at DESC").
		Find(&entries).Error
	return entries, err
}

// GetReviewsByUser returns every anime_list row for `userID` that qualifies
// as a review (score>0 OR review_text!=''). Newest-first, preloads Anime.
func (r *ListRepository) GetReviewsByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").
		Where("user_id = ? AND (score > 0 OR review_text <> '')", userID).
		Order("created_at DESC").
		Find(&entries).Error
	return entries, err
}

// GetUserReview returns the single anime_list row for the given (user, anime)
// pair if it carries a review (score>0 OR review_text!=''). Returns
// errors.NotFound when the row is absent OR exists with empty review.
func (r *ListRepository) GetUserReview(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	var entry domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").
		Where("user_id = ? AND anime_id = ? AND (score > 0 OR review_text <> '')", userID, animeID).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("review")
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// UpsertReview writes the (score, review_text, username) triple onto an
// existing anime_list row OR creates a fresh row with status='completed' when
// none exists. On the update path it ONLY assigns score, review_text,
// username, updated_at — status / episodes / notes / tags / etc. on the
// pre-existing watchlist row are preserved. Returns the resulting entry.
func (r *ListRepository) UpsertReview(ctx context.Context, userID, animeID, username string, score int, reviewText string) (*domain.AnimeListEntry, error) {
	now := time.Now()
	entry := &domain.AnimeListEntry{
		UserID:     userID,
		AnimeID:    animeID,
		Status:     "completed", // only takes effect on INSERT path
		Score:      score,
		ReviewText: reviewText,
		Username:   username,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"score":       score,
			"review_text": reviewText,
			"username":    username,
			"updated_at":  now,
		}),
	}).Create(entry).Error
	if err != nil {
		return nil, err
	}
	// Reload to capture any preserved fields (status, episodes, etc.).
	// REVIEW.md CR-03: propagate the reload error rather than returning
	// the locally-constructed `entry` — that entry has no ID (Postgres
	// gen_random_uuid() default fires server-side), no canonical
	// CreatedAt, and no preserved pre-existing fields on the conflict
	// path. Returning it would let clients PATCH/DELETE with an empty ID.
	var fresh domain.AnimeListEntry
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&fresh).Error; err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "failed to reload review after upsert")
	}
	return &fresh, nil
}

// ClearReview sets score=0 + review_text='' on the matching row (the row
// stays in anime_list — it just drops out of the reviews filter). Idempotent
// — no error when no row matches.
func (r *ListRepository) ClearReview(ctx context.Context, userID, animeID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Updates(map[string]interface{}{
			"score":       0,
			"review_text": "",
			"updated_at":  time.Now(),
		}).Error
}

// GetAnimeRating returns the average score and total scoring-row count for an
// anime, only considering anime_list rows where score>0.
func (r *ListRepository) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	var result struct {
		AverageScore float64 `gorm:"column:average_score"`
		TotalReviews int64   `gorm:"column:total_reviews"`
	}
	err := r.db.WithContext(ctx).
		Raw(`SELECT COALESCE(AVG(score), 0) AS average_score, COUNT(*) AS total_reviews
		     FROM anime_list WHERE anime_id = ? AND score > 0`, animeID).
		Scan(&result).Error
	if err != nil {
		// Propagate the error so callers can distinguish "no ratings yet"
		// (legitimate AverageScore=0, TotalReviews=0) from "DB hiccup
		// during lookup" — REVIEW.md CR-02. Previously the error was
		// swallowed and the handler served a fake zero rating that was
		// indistinguishable from a real one.
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "failed to get anime rating")
	}
	return &domain.AnimeRating{
		AnimeID:      animeID,
		AverageScore: result.AverageScore,
		TotalReviews: int(result.TotalReviews),
	}, nil
}

// GetBatchAnimeRatings returns a map keyed by anime_id, only for anime that
// have at least one anime_list row with score>0.
func (r *ListRepository) GetBatchAnimeRatings(ctx context.Context, animeIDs []string) (map[string]*domain.AnimeRating, error) {
	var rows []struct {
		AnimeID      string  `gorm:"column:anime_id"`
		AverageScore float64 `gorm:"column:average_score"`
		TotalReviews int64   `gorm:"column:total_reviews"`
	}
	err := r.db.WithContext(ctx).
		Raw(`SELECT anime_id,
		            COALESCE(AVG(score), 0) AS average_score,
		            COUNT(*) AS total_reviews
		     FROM anime_list
		     WHERE anime_id IN ? AND score > 0
		     GROUP BY anime_id`, animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]*domain.AnimeRating, len(rows))
	for _, row := range rows {
		out[row.AnimeID] = &domain.AnimeRating{
			AnimeID:      row.AnimeID,
			AverageScore: row.AverageScore,
			TotalReviews: int(row.TotalReviews),
		}
	}
	return out, nil
}

func (r *ListRepository) GetByUserAndStatusesPaginated(ctx context.Context, userID string, statuses []string, search string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("anime_list.user_id = ? AND anime_list.status IN ?", userID, statuses)

	needsAnimesJoin := isTitleSort(params.Sort) || search != ""
	if needsAnimesJoin {
		base = base.Joins("LEFT JOIN animes ON animes.id = anime_list.anime_id")
	}
	if search != "" {
		like := "%" + search + "%"
		base = base.Where(
			"animes.name ILIKE ? OR animes.name_ru ILIKE ? OR animes.name_jp ILIKE ?",
			like, like, like,
		)
	}

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(sanitizedOrderClause(params.Sort, params.Order)).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}
