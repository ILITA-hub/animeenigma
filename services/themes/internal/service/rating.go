package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/repo"
)

type RatingService struct {
	ratingRepo *repo.RatingRepository
	themeRepo  *repo.ThemeRepository
	log        *logger.Logger
}

func NewRatingService(ratingRepo *repo.RatingRepository, themeRepo *repo.ThemeRepository, log *logger.Logger) *RatingService {
	return &RatingService{
		ratingRepo: ratingRepo,
		themeRepo:  themeRepo,
		log:        log,
	}
}

// Rate upserts a user's rating for a theme.
func (s *RatingService) Rate(ctx context.Context, userID, themeID string, score int) error {
	if score < 1 || score > 10 {
		return errors.InvalidInput("score must be between 1 and 10")
	}

	// Verify theme exists
	theme, err := s.themeRepo.GetByID(ctx, themeID)
	if err != nil || theme == nil {
		return errors.NotFound("theme")
	}

	rating := &domain.ThemeRating{
		UserID:  userID,
		ThemeID: themeID,
		Score:   score,
	}

	if err := s.ratingRepo.Upsert(ctx, rating); err != nil {
		return errors.Wrap(err, errors.CodeInternal, "save rating")
	}

	s.log.Infow("theme rated",
		"user_id", userID,
		"theme_id", themeID,
		"score", score,
	)
	return nil
}

// Unrate removes a user's rating for a theme.
func (s *RatingService) Unrate(ctx context.Context, userID, themeID string) error {
	if err := s.ratingRepo.Delete(ctx, userID, themeID); err != nil {
		return errors.NotFound("rating")
	}
	return nil
}

// GetUserRatings returns a user's ratings, optionally filtered by year/season.
func (s *RatingService) GetUserRatings(ctx context.Context, userID string, year int, season string) ([]domain.ThemeRating, error) {
	ratings, err := s.ratingRepo.GetUserRatings(ctx, userID, year, season)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "get user ratings")
	}
	return ratings, nil
}
