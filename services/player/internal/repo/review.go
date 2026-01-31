package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/google/uuid"
)

type ReviewRepository struct {
	db *database.DB
}

func NewReviewRepository(db *database.DB) *ReviewRepository {
	return &ReviewRepository{
		db: db,
	}
}

// Upsert creates or updates a review
func (r *ReviewRepository) Upsert(ctx context.Context, review *domain.Review) error {
	query := `
		INSERT INTO reviews (id, user_id, anime_id, anime_title, anime_cover, username, score, review_text, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, anime_id)
		DO UPDATE SET
			anime_title = COALESCE(NULLIF(EXCLUDED.anime_title, ''), reviews.anime_title),
			anime_cover = COALESCE(NULLIF(EXCLUDED.anime_cover, ''), reviews.anime_cover),
			username = COALESCE(NULLIF(EXCLUDED.username, ''), reviews.username),
			score = EXCLUDED.score,
			review_text = EXCLUDED.review_text,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	if review.ID == "" {
		review.ID = uuid.New().String()
	}
	review.CreatedAt = now
	review.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		review.ID,
		review.UserID,
		review.AnimeID,
		review.AnimeTitle,
		review.AnimeCover,
		review.Username,
		review.Score,
		review.ReviewText,
		review.CreatedAt,
		review.UpdatedAt,
	)
	return err
}

// GetByAnime returns all reviews for an anime
func (r *ReviewRepository) GetByAnime(ctx context.Context, animeID string) ([]*domain.Review, error) {
	query := `
		SELECT id, user_id, anime_id, anime_title, anime_cover, username, score, review_text, created_at, updated_at
		FROM reviews
		WHERE anime_id = $1
		ORDER BY created_at DESC
	`

	var reviews []*domain.Review
	err := r.db.SelectContext(ctx, &reviews, query, animeID)
	if err != nil {
		return nil, err
	}
	return reviews, nil
}

// GetByUser returns all reviews by a user
func (r *ReviewRepository) GetByUser(ctx context.Context, userID string) ([]*domain.Review, error) {
	query := `
		SELECT id, user_id, anime_id, anime_title, anime_cover, username, score, review_text, created_at, updated_at
		FROM reviews
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	var reviews []*domain.Review
	err := r.db.SelectContext(ctx, &reviews, query, userID)
	if err != nil {
		return nil, err
	}
	return reviews, nil
}

// GetByUserAndAnime returns a specific review
func (r *ReviewRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) (*domain.Review, error) {
	query := `
		SELECT id, user_id, anime_id, anime_title, anime_cover, username, score, review_text, created_at, updated_at
		FROM reviews
		WHERE user_id = $1 AND anime_id = $2
	`

	var review domain.Review
	err := r.db.GetContext(ctx, &review, query, userID, animeID)
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// GetAnimeRating returns the average rating for an anime
func (r *ReviewRepository) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	query := `
		SELECT
			anime_id,
			COALESCE(AVG(score)::numeric, 0) as average_score,
			COUNT(*) as total_reviews
		FROM reviews
		WHERE anime_id = $1
		GROUP BY anime_id
	`

	var rating domain.AnimeRating
	err := r.db.GetContext(ctx, &rating, query, animeID)
	if err != nil {
		// No reviews yet
		return &domain.AnimeRating{
			AnimeID:      animeID,
			AverageScore: 0,
			TotalReviews: 0,
		}, nil
	}
	return &rating, nil
}

// Delete removes a review
func (r *ReviewRepository) Delete(ctx context.Context, userID, animeID string) error {
	query := `DELETE FROM reviews WHERE user_id = $1 AND anime_id = $2`
	_, err := r.db.ExecContext(ctx, query, userID, animeID)
	return err
}
