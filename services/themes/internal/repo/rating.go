package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RatingRepository struct {
	db *gorm.DB
}

func NewRatingRepository(db *gorm.DB) *RatingRepository {
	return &RatingRepository{db: db}
}

// Upsert creates or updates a user's rating for a theme.
func (r *RatingRepository) Upsert(ctx context.Context, rating *domain.ThemeRating) error {
	now := time.Now()
	rating.UpdatedAt = now
	if rating.CreatedAt.IsZero() {
		rating.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "theme_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"score", "updated_at",
		}),
	}).Create(rating).Error
}

// Delete removes a user's rating for a theme.
func (r *RatingRepository) Delete(ctx context.Context, userID, themeID string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND theme_id = ?", userID, themeID).
		Delete(&domain.ThemeRating{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetByUserAndTheme returns a user's rating for a specific theme.
func (r *RatingRepository) GetByUserAndTheme(ctx context.Context, userID, themeID string) (*domain.ThemeRating, error) {
	var rating domain.ThemeRating
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND theme_id = ?", userID, themeID).
		First(&rating).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rating, err
}

// GetUserRatings returns all of a user's ratings, optionally filtered by year/season.
func (r *RatingRepository) GetUserRatings(ctx context.Context, userID string, year int, season string) ([]domain.ThemeRating, error) {
	query := r.db.WithContext(ctx).
		Table("theme_ratings").
		Joins("JOIN anime_themes ON anime_themes.id = theme_ratings.theme_id").
		Where("theme_ratings.user_id = ? AND anime_themes.deleted_at IS NULL", userID)

	if year > 0 {
		query = query.Where("anime_themes.year = ?", year)
	}
	if season != "" {
		query = query.Where("anime_themes.season = ?", season)
	}

	var ratings []domain.ThemeRating
	err := query.Select("theme_ratings.*").Find(&ratings).Error
	return ratings, err
}

// GetUserScoresMap returns a map of theme_id -> score for a user's ratings.
func (r *RatingRepository) GetUserScoresMap(ctx context.Context, userID string, themeIDs []string) (map[string]int, error) {
	if len(themeIDs) == 0 {
		return map[string]int{}, nil
	}

	var ratings []domain.ThemeRating
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND theme_id IN ?", userID, themeIDs).
		Find(&ratings).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]int, len(ratings))
	for _, r := range ratings {
		result[r.ThemeID] = r.Score
	}
	return result, nil
}
