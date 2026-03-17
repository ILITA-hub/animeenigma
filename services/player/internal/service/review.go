package service

import (
	"context"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ReviewService struct {
	reviewRepo   *repo.ReviewRepository
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

func NewReviewService(reviewRepo *repo.ReviewRepository, listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *ReviewService {
	return &ReviewService{
		reviewRepo:   reviewRepo,
		listRepo:     listRepo,
		activityRepo: activityRepo,
		log:          log,
	}
}

// CreateOrUpdateReview creates or updates a user's review
func (s *ReviewService) CreateOrUpdateReview(ctx context.Context, userID, username string, req *domain.CreateReviewRequest) (*domain.Review, error) {
	if req.Score < 1 || req.Score > 10 {
		return nil, errors.InvalidInput("score must be between 1 and 10")
	}

	// Check if review already exists (for activity dedup + "new" vs "update")
	existingReview, _ := s.reviewRepo.GetByUserAndAnime(ctx, userID, req.AnimeID)

	review := &domain.Review{
		UserID:     userID,
		AnimeID:    req.AnimeID,
		Username:   username,
		Score:      req.Score,
		ReviewText: req.ReviewText,
	}

	if err := s.reviewRepo.Upsert(ctx, review); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to save review")
	}

	// Record review activity event (deduplicated per day)
	// The review event already includes the score, so no separate score event needed
	isNewReview := existingReview == nil
	// Truncate review text for activity preview (full text stays in reviews table)
	contentPreview := req.ReviewText
	if len([]rune(contentPreview)) > 300 {
		contentPreview = string([]rune(contentPreview)[:300]) + "…"
	}
	reviewEvent := &domain.ActivityEvent{
		UserID:   userID,
		Username: username,
		AnimeID:  req.AnimeID,
		Type:     "review",
		NewValue: strconv.Itoa(req.Score),
		Content:  contentPreview,
	}
	if req.ReviewText == "" {
		reviewEvent.OldValue = "score"
	} else if isNewReview {
		reviewEvent.OldValue = "new"
	} else {
		reviewEvent.OldValue = "update"
	}
	// Check for existing review event today — update it instead of creating a new one
	existingEvent, _ := s.activityRepo.GetTodayByUserAnimeType(ctx, userID, req.AnimeID, "review")
	if existingEvent != nil {
		existingEvent.NewValue = reviewEvent.NewValue
		existingEvent.OldValue = reviewEvent.OldValue
		existingEvent.Content = reviewEvent.Content
		if err := s.activityRepo.Update(ctx, existingEvent); err != nil {
			s.log.Errorw("failed to update review activity", "user_id", userID, "anime_id", req.AnimeID, "error", err)
		}
	} else {
		if err := s.activityRepo.Create(ctx, reviewEvent); err != nil {
			s.log.Errorw("failed to record review activity", "user_id", userID, "anime_id", req.AnimeID, "error", err)
		}
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

// GetBatchAnimeRatings returns average ratings for multiple anime
func (s *ReviewService) GetBatchAnimeRatings(ctx context.Context, animeIDs []string) (map[string]*domain.AnimeRating, error) {
	return s.reviewRepo.GetBatchAnimeRatings(ctx, animeIDs)
}

// DeleteReview removes a user's review
func (s *ReviewService) DeleteReview(ctx context.Context, userID, animeID string) error {
	return s.reviewRepo.Delete(ctx, userID, animeID)
}
