package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
			"anime_title":          gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.anime_title)", entry.AnimeTitle),
			"anime_cover":          gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.anime_cover)", entry.AnimeCover),
			"status":               entry.Status,
			"score":                entry.Score,
			"episodes":             entry.Episodes,
			"notes":                gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.notes)", entry.Notes),
			"tags":                 gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.tags)", entry.Tags),
			"is_rewatching":        entry.IsRewatching,
			"priority":             gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.priority)", entry.Priority),
			"anime_type":           gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.anime_type)", entry.AnimeType),
			"anime_total_episodes": gorm.Expr("CASE WHEN ? > 0 THEN ? ELSE anime_list.anime_total_episodes END", entry.AnimeTotalEpisodes, entry.AnimeTotalEpisodes),
			"mal_id":               gorm.Expr("COALESCE(?, anime_list.mal_id)", entry.MalID),
			"started_at":           gorm.Expr("COALESCE(?, anime_list.started_at)", entry.StartedAt),
			"completed_at":         gorm.Expr("COALESCE(?, anime_list.completed_at)", entry.CompletedAt),
			"updated_at":           entry.UpdatedAt,
		}),
	}).Create(entry).Error
}

func (r *ListRepository) GetByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndStatus(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, status).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	var entry domain.AnimeListEntry
	err := r.db.WithContext(ctx).
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
	result := r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Where("user_id = ? AND anime_id = ? AND episodes < ?", userID, animeID, episodeNumber).
		Updates(map[string]interface{}{
			"episodes":   episodeNumber,
			"status":     gorm.Expr("CASE WHEN anime_total_episodes > 0 AND ? >= anime_total_episodes THEN 'completed' WHEN status = 'plan_to_watch' THEN 'watching' ELSE status END", episodeNumber),
			"started_at": gorm.Expr("COALESCE(started_at, NOW())"),
			"completed_at": gorm.Expr("CASE WHEN anime_total_episodes > 0 AND ? >= anime_total_episodes THEN NOW() ELSE completed_at END", episodeNumber),
			"updated_at": time.Now(),
		})
	return result.RowsAffected > 0, result.Error
}

func (r *ListRepository) GetByUserAndStatuses(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status IN ?", userID, statuses).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}
