package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ReviewService struct {
	reviewRepo *repo.ReviewRepository
	listRepo   *repo.ListRepository
	log        *logger.Logger
}

func NewReviewService(reviewRepo *repo.ReviewRepository, listRepo *repo.ListRepository, log *logger.Logger) *ReviewService {
	return &ReviewService{
		reviewRepo: reviewRepo,
		listRepo:   listRepo,
		log:        log,
	}
}

// CreateOrUpdateReview creates or updates a user's review
func (s *ReviewService) CreateOrUpdateReview(ctx context.Context, userID, username string, req *domain.CreateReviewRequest) (*domain.Review, error) {
	if req.Score < 1 || req.Score > 10 {
		return nil, errors.InvalidInput("score must be between 1 and 10")
	}

	review := &domain.Review{
		UserID:     userID,
		AnimeID:    req.AnimeID,
		AnimeTitle: req.AnimeTitle,
		AnimeCover: req.AnimeCover,
		Username:   username,
		Score:      req.Score,
		ReviewText: req.ReviewText,
	}

	if err := s.reviewRepo.Upsert(ctx, review); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to save review")
	}

	// Sync score to watchlist entry
	entry, _ := s.listRepo.GetByUserAndAnime(ctx, userID, req.AnimeID)
	if entry != nil {
		entry.Score = req.Score
		if err := s.listRepo.Upsert(ctx, entry); err != nil {
			s.log.Errorw("failed to sync review score to watchlist",
				"user_id", userID,
				"anime_id", req.AnimeID,
				"error", err,
			)
		}
	}

	return review, nil
}

// GetAnimeReviews returns all reviews for an anime
func (s *ReviewService) GetAnimeReviews(ctx context.Context, animeID string) ([]*domain.Review, error) {
	return s.reviewRepo.GetByAnime(ctx, animeID)
}

// GetUserReviews returns all reviews by a user
func (s *ReviewService) GetUserReviews(ctx context.Context, userID string) ([]*domain.Review, error) {
	return s.reviewRepo.GetByUser(ctx, userID)
}

// GetUserReview returns a specific user's review for an anime
func (s *ReviewService) GetUserReview(ctx context.Context, userID, animeID string) (*domain.Review, error) {
	return s.reviewRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// GetAnimeRating returns the average rating for an anime
func (s *ReviewService) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	return s.reviewRepo.GetAnimeRating(ctx, animeID)
}

// DeleteReview removes a user's review
func (s *ReviewService) DeleteReview(ctx context.Context, userID, animeID string) error {
	return s.reviewRepo.Delete(ctx, userID, animeID)
}
