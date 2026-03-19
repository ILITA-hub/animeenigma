package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PreferenceRepository struct {
	db *gorm.DB
}

func NewPreferenceRepository(db *gorm.DB) *PreferenceRepository {
	return &PreferenceRepository{db: db}
}

// UpsertAnimePreference creates or updates the user's per-anime preference
func (r *PreferenceRepository) UpsertAnimePreference(ctx context.Context, pref *domain.UserAnimePreference) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"player", "language", "watch_type", "translation_id", "translation_title", "updated_at"}),
		}).
		Create(pref).Error
}

// GetAnimePreference returns the user's saved preference for a specific anime
func (r *PreferenceRepository) GetAnimePreference(ctx context.Context, userID, animeID string) (*domain.UserAnimePreference, error) {
	var pref domain.UserAnimePreference
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&pref).Error
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

// GetUserGlobalFavorite returns the user's #1 most-watched combo from watch_history
func (r *PreferenceRepository) GetUserGlobalFavorite(ctx context.Context, userID string) (*domain.ComboCount, error) {
	var result domain.ComboCount
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_title, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("player, language, watch_type, translation_title").
		Order("count DESC").
		Limit(1).
		Scan(&result).Error
	if err != nil || result.Count == 0 {
		return nil, err
	}
	return &result, nil
}

// GetUserTopCombos returns the user's top combos ranked by watch count
func (r *PreferenceRepository) GetUserTopCombos(ctx context.Context, userID string, limit int) ([]domain.ComboCount, error) {
	var results []domain.ComboCount
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_title, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("player, language, watch_type, translation_title").
		Order("count DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

// GetCommunityPopularity returns the most popular combos for a specific anime
func (r *PreferenceRepository) GetCommunityPopularity(ctx context.Context, animeID string) ([]domain.CommunityCombo, error) {
	var results []domain.CommunityCombo
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_id, translation_title, COUNT(DISTINCT user_id) as viewers").
		Where("anime_id = ?", animeID).
		Group("player, language, watch_type, translation_id, translation_title").
		Order("viewers DESC").
		Scan(&results).Error
	return results, err
}

// GetPinnedTranslations queries catalog's pinned_translations table (shared DB)
func (r *PreferenceRepository) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	var results []domain.PinnedTranslation
	err := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Find(&results).Error
	return results, err
}

// CreateWatchHistory inserts a watch_history row with full combo context
func (r *PreferenceRepository) CreateWatchHistory(ctx context.Context, history *domain.WatchHistory) error {
	return r.db.WithContext(ctx).Create(history).Error
}
