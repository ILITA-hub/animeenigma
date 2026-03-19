package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ProgressService struct {
	progressRepo *repo.ProgressRepository
	prefService  *PreferenceService
	log          *logger.Logger
}

func NewProgressService(progressRepo *repo.ProgressRepository, prefService *PreferenceService, log *logger.Logger) *ProgressService {
	return &ProgressService{
		progressRepo: progressRepo,
		prefService:  prefService,
		log:          log,
	}
}

// UpdateProgress updates or creates watch progress (time tracking only)
func (s *ProgressService) UpdateProgress(ctx context.Context, userID string, req *domain.UpdateProgressRequest) (*domain.WatchProgress, error) {
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       req.AnimeID,
		EpisodeNumber: req.EpisodeNumber,
		Progress:      req.Progress,
		Duration:      req.Duration,
		Completed:     false, // User marks manually
		LastWatchedAt: time.Now(),
	}

	if err := s.progressRepo.Upsert(ctx, progress); err != nil {
		return nil, err
	}

	// Upsert anime preference if combo fields are present
	if req.Player != "" {
		s.prefService.UpsertAnimePreference(ctx, userID, req)
	}

	return progress, nil
}

// GetProgress returns watch progress for an anime
func (s *ProgressService) GetProgress(ctx context.Context, userID, animeID string) ([]*domain.WatchProgress, error) {
	return s.progressRepo.GetByUserAndAnime(ctx, userID, animeID)
}
