package service

import (
	"context"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// ReviewService is the business-logic layer for the six reviews endpoints.
// Phase 1 (workstream: social) plan 02: refactored to consume the unified
// ListRepository exclusively. The public method signatures still match what
// the handler layer calls; only the return types change from the legacy
// review type to AnimeListEntry — consumers project to the 7-field
// reviewResponse struct in handler/review.go to keep the wire shape
// byte-identical.
type ReviewService struct {
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

// NewReviewService wires the refactored review service.
func NewReviewService(listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *ReviewService {
	return &ReviewService{
		listRepo:     listRepo,
		activityRepo: activityRepo,
		log:          log,
	}
}

// CreateOrUpdateReview creates or updates a user's review. The activity-
// emission block matches the pre-refactor behavior verbatim — per-day
// dedup via ActivityRepository.GetTodayByUserAnimeType, OldValue carries
// "new"/"update"/"score" markers as before.
func (s *ReviewService) CreateOrUpdateReview(ctx context.Context, userID, username string, req *domain.CreateReviewRequest) (*domain.AnimeListEntry, error) {
	// Validation rule unchanged from pre-refactor: score must be 1..10.
	if req.Score < 1 || req.Score > 10 {
		return nil, errors.InvalidInput("score must be between 1 and 10")
	}

	// Detect new-vs-update for the activity event's OldValue marker. A row
	// with score=0 + review_text='' counts as "new review" from the
	// activity-feed's perspective — functionally equivalent to "no review
	// yet" since the row has no review content.
	existing, _ := s.listRepo.GetUserReview(ctx, userID, req.AnimeID)

	entry, err := s.listRepo.UpsertReview(ctx, userID, req.AnimeID, username, req.Score, req.ReviewText)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to save review")
	}

	// Record review activity event (deduplicated per day) — logic preserved
	// line-for-line from the pre-refactor service/review.go.
	isNewReview := existing == nil
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

	// Pre-refactor code synced the review score to the watchlist via a
	// second Upsert. After the schema merge that sync is now a no-op
	// because UpsertReview already wrote score into the same anime_list
	// row. Drop the redundant call.
	return entry, nil
}

// GetAnimeReviews returns every anime_list row for the anime that qualifies
// as a "review" (score>0 OR review_text!='').
func (s *ReviewService) GetAnimeReviews(ctx context.Context, animeID string) ([]*domain.AnimeListEntry, error) {
	return s.listRepo.GetReviewsByAnime(ctx, animeID)
}

// GetUserReview returns the current user's review (or errors.NotFound when
// the row is absent or empty-on-both).
func (s *ReviewService) GetUserReview(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	return s.listRepo.GetUserReview(ctx, userID, animeID)
}

// GetUserReviews returns every anime_list row for the user that qualifies
// as a review (score>0 OR review_text!=''). Used by GET /api/users/reviews.
func (s *ReviewService) GetUserReviews(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	return s.listRepo.GetReviewsByUser(ctx, userID)
}

// GetAnimeRating returns the average rating + scoring-row count.
func (s *ReviewService) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	return s.listRepo.GetAnimeRating(ctx, animeID)
}

// GetBatchAnimeRatings returns average ratings for multiple anime.
func (s *ReviewService) GetBatchAnimeRatings(ctx context.Context, animeIDs []string) (map[string]*domain.AnimeRating, error) {
	return s.listRepo.GetBatchAnimeRatings(ctx, animeIDs)
}

// DeleteReview clears the user's review (score=0, review_text='') — the
// underlying anime_list row stays. Idempotent on missing rows.
func (s *ReviewService) DeleteReview(ctx context.Context, userID, animeID string) error {
	return s.listRepo.ClearReview(ctx, userID, animeID)
}
