package repo

import (
	"context"
	"errors"
	"strings"
	"time"

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
	return sort + " " + dir
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

func (r *ListRepository) GetByUserAndStatuses(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ? AND status IN ?", userID, statuses).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserPaginated(ctx context.Context, userID, status string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if status != "" {
		base = base.Where("status = ?", status)
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

	base := r.db.WithContext(ctx).Model(&domain.AnimeListEntry{}).Where("user_id = ?", userID)
	if len(statuses) > 0 {
		base = base.Where("status IN ?", statuses)
	}

	err := base.Select(
		"COALESCE(AVG(NULLIF(score, 0)), 0) as avg_score, "+
			"COALESCE(SUM(episodes), 0) as total_episodes, "+
			"COUNT(*) as total_entries, "+
			"COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed",
	).Scan(&stats).Error

	return &stats, err
}

func (r *ListRepository) GetByUserAndStatusesPaginated(ctx context.Context, userID string, statuses []string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("user_id = ? AND status IN ?", userID, statuses)

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
