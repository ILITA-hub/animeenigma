package service

import (
	"context"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/repo"
)

type ThemeService struct {
	themeRepo  *repo.ThemeRepository
	ratingRepo *repo.RatingRepository
	log        *logger.Logger
}

func NewThemeService(themeRepo *repo.ThemeRepository, ratingRepo *repo.RatingRepository, log *logger.Logger) *ThemeService {
	return &ThemeService{
		themeRepo:  themeRepo,
		ratingRepo: ratingRepo,
		log:        log,
	}
}

// List returns themes with filters, including user scores if userID is provided.
func (s *ThemeService) List(ctx context.Context, params domain.ThemeListParams) ([]domain.AnimeTheme, error) {
	// Normalize type to uppercase
	if params.Type != "" {
		params.Type = strings.ToUpper(params.Type)
	}

	themes, err := s.themeRepo.List(ctx, params)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "list themes")
	}

	// Attach user scores if authenticated
	if params.UserID != "" && len(themes) > 0 {
		themeIDs := make([]string, len(themes))
		for i, t := range themes {
			themeIDs[i] = t.ID
		}

		scores, err := s.ratingRepo.GetUserScoresMap(ctx, params.UserID, themeIDs)
		if err != nil {
			s.log.Errorw("failed to get user scores", "user_id", params.UserID, "error", err)
		} else {
			for i := range themes {
				if score, ok := scores[themes[i].ID]; ok {
					s := score
					themes[i].UserScore = &s
				}
			}
		}
	}

	return themes, nil
}

// GetByID returns a single theme, including user score if userID is provided.
func (s *ThemeService) GetByID(ctx context.Context, id, userID string) (*domain.AnimeTheme, error) {
	theme, err := s.themeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.NotFound("theme")
	}

	if userID != "" {
		rating, err := s.ratingRepo.GetByUserAndTheme(ctx, userID, id)
		if err == nil && rating != nil {
			theme.UserScore = &rating.Score
		}
	}

	return theme, nil
}
