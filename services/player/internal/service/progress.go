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

// UpdateProgress updates or creates watch progress (heartbeat saves).
// Does not mark the episode as completed — that is a discrete event written
// via ProgressRepository.MarkCompleted from ListService.MarkEpisodeWatched.
func (s *ProgressService) UpdateProgress(ctx context.Context, userID string, req *domain.UpdateProgressRequest) (*domain.WatchProgress, error) {
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       req.AnimeID,
		EpisodeNumber: req.EpisodeNumber,
		Progress:      req.Progress,
		Duration:      req.Duration,
		LastWatchedAt: time.Now(),
	}

	if err := s.progressRepo.UpsertProgress(ctx, progress); err != nil {
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

// MarkDropOff records that the user closed the page mid-episode at the given
// playback position (seconds). Phase 5 (G-01).
func (s *ProgressService) MarkDropOff(ctx context.Context, userID, animeID string, req *domain.DropOffRequest) error {
	return s.progressRepo.MarkDropOff(ctx, userID, animeID, req.EpisodeNumber, req.Progress)
}

// ListContinueWatching returns the user's most-recent in-progress episodes,
// one row per anime, ordered by last_watched_at DESC. Phase 8 (UX-15 / UA-061).
func (s *ProgressService) ListContinueWatching(
	ctx context.Context, userID string, limit int,
) ([]*domain.ContinueWatchingItem, error) {
	return s.progressRepo.ListContinueWatching(ctx, userID, limit)
}

// GetBulkProgress returns a map keyed by anime_id with the user's furthest
// episode reached + completion flags. Used by AnimeCardNew (via the
// /users/anime-progress endpoint) to render a per-card progress badge.
// Pure read-through delegate; the repo enforces the empty-input fast-path
// and the JOIN semantics. Phase 9 (UX-16).
func (s *ProgressService) GetBulkProgress(
	ctx context.Context, userID string, animeIDs []string,
) (domain.BulkAnimeProgressMap, error) {
	return s.progressRepo.GetBulkProgress(ctx, userID, animeIDs)
}
