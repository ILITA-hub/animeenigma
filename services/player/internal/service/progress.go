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
	log          *logger.Logger
}

func NewProgressService(progressRepo *repo.ProgressRepository, log *logger.Logger) *ProgressService {
	return &ProgressService{
		progressRepo: progressRepo,
		log:          log,
	}
}

// UpdateProgress updates or creates watch progress
func (s *ProgressService) UpdateProgress(ctx context.Context, userID string, req *domain.UpdateProgressRequest) (*domain.WatchProgress, error) {
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       req.AnimeID,
		EpisodeNumber: req.EpisodeNumber,
		Progress:      req.Progress,
		Duration:      req.Duration,
		Completed:     req.Progress >= req.Duration-5, // Consider completed if within 5 seconds
		LastWatchedAt: time.Now(),
	}

	if err := s.progressRepo.Upsert(ctx, progress); err != nil {
		return nil, err
	}

	return progress, nil
}

// GetProgress returns watch progress for an anime
func (s *ProgressService) GetProgress(ctx context.Context, userID, animeID string) ([]*domain.WatchProgress, error) {
	return s.progressRepo.GetByUserAndAnime(ctx, userID, animeID)
}
