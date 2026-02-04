package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewRepository struct {
	db *gorm.DB
}

func NewReviewRepository(db *gorm.DB) *ReviewRepository {
	return &ReviewRepository{db: db}
}

func (r *ReviewRepository) Upsert(ctx context.Context, review *domain.Review) error {
	now := time.Now()
	review.UpdatedAt = now
	if review.CreatedAt.IsZero() {
		review.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"anime_title":  gorm.Expr("COALESCE(NULLIF(?, ''), reviews.anime_title)", review.AnimeTitle),
			"anime_cover":  gorm.Expr("COALESCE(NULLIF(?, ''), reviews.anime_cover)", review.AnimeCover),
			"username":     gorm.Expr("COALESCE(NULLIF(?, ''), reviews.username)", review.Username),
			"score":        review.Score,
			"review_text":  review.ReviewText,
			"updated_at":   review.UpdatedAt,
		}),
	}).Create(review).Error
}

func (r *ReviewRepository) GetByAnime(ctx context.Context, animeID string) ([]*domain.Review, error) {
	var reviews []*domain.Review
	err := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Order("created_at DESC").
		Find(&reviews).Error
	return reviews, err
}

func (r *ReviewRepository) GetByUser(ctx context.Context, userID string) ([]*domain.Review, error) {
	var reviews []*domain.Review
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&reviews).Error
	return reviews, err
}

func (r *ReviewRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) (*domain.Review, error) {
	var review domain.Review
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &review, err
}

func (r *ReviewRepository) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	var result struct {
		AverageScore float64
		TotalReviews int64
	}
	err := r.db.WithContext(ctx).
		Model(&domain.Review{}).
		Select("COALESCE(AVG(score), 0) as average_score, COUNT(*) as total_reviews").
		Where("anime_id = ?", animeID).
		Scan(&result).Error
	if err != nil {
		return &domain.AnimeRating{AnimeID: animeID}, nil
	}
	return &domain.AnimeRating{
		AnimeID:      animeID,
		AverageScore: result.AverageScore,
		TotalReviews: int(result.TotalReviews),
	}, nil
}

func (r *ReviewRepository) Delete(ctx context.Context, userID, animeID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Delete(&domain.Review{}).Error
}
