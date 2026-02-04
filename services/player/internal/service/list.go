package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ListService struct {
	listRepo *repo.ListRepository
	log      *logger.Logger
}

func NewListService(listRepo *repo.ListRepository, log *logger.Logger) *ListService {
	return &ListService{
		listRepo: listRepo,
		log:      log,
	}
}

// GetUserList returns user's anime list with optional status filter
func (s *ListService) GetUserList(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	if status != "" {
		return s.listRepo.GetByUserAndStatus(ctx, userID, status)
	}
	return s.listRepo.GetByUser(ctx, userID)
}

// GetUserAnimeEntry returns a single anime entry from user's list
func (s *ListService) GetUserAnimeEntry(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	return s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// UpdateListEntry updates or creates an anime list entry
func (s *ListService) UpdateListEntry(ctx context.Context, userID string, req *domain.UpdateListRequest) (*domain.AnimeListEntry, error) {
	// Check if entry already exists to preserve dates
	existingEntry, _ := s.listRepo.GetByUserAndAnime(ctx, userID, req.AnimeID)

	entry := &domain.AnimeListEntry{
		UserID:             userID,
		AnimeID:            req.AnimeID,
		AnimeTitle:         req.AnimeTitle,
		AnimeCover:         req.AnimeCover,
		Status:             req.Status,
		AnimeType:          req.AnimeType,
	}

	if req.Score != nil {
		entry.Score = *req.Score
	}

	if req.Episodes != nil {
		entry.Episodes = *req.Episodes
	}

	if req.Notes != nil {
		entry.Notes = *req.Notes
	}

	if req.Tags != nil {
		entry.Tags = *req.Tags
	}

	if req.IsRewatching != nil {
		entry.IsRewatching = *req.IsRewatching
	}

	if req.Priority != nil {
		entry.Priority = *req.Priority
	}

	if req.AnimeTotalEpisodes != nil {
		entry.AnimeTotalEpisodes = *req.AnimeTotalEpisodes
	}

	if req.MalID != nil {
		entry.MalID = req.MalID
	}

	// Handle StartedAt - use provided value, preserve existing, or auto-set
	if req.StartedAt != nil {
		entry.StartedAt = req.StartedAt
	} else if existingEntry != nil && existingEntry.StartedAt != nil {
		entry.StartedAt = existingEntry.StartedAt
	} else if req.Status == "watching" {
		now := time.Now()
		entry.StartedAt = &now
	}

	// Handle CompletedAt - use provided value, preserve existing, or auto-set
	if req.CompletedAt != nil {
		entry.CompletedAt = req.CompletedAt
	} else if existingEntry != nil && existingEntry.CompletedAt != nil {
		entry.CompletedAt = existingEntry.CompletedAt
	} else if req.Status == "completed" {
		now := time.Now()
		entry.CompletedAt = &now
	}

	if err := s.listRepo.Upsert(ctx, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

// DeleteListEntry removes an anime from user's list
func (s *ListService) DeleteListEntry(ctx context.Context, userID, animeID string) error {
	return s.listRepo.Delete(ctx, userID, animeID)
}

// MarkEpisodeWatched marks an episode as watched and updates the episodes count
func (s *ListService) MarkEpisodeWatched(ctx context.Context, userID, animeID string, episode int) (*domain.AnimeListEntry, error) {
	updated, err := s.listRepo.IncrementEpisodes(ctx, userID, animeID, episode)
	if err != nil {
		return nil, err
	}

	if !updated {
		s.log.Infow("episode already marked or anime not in list",
			"user_id", userID,
			"anime_id", animeID,
			"episode", episode,
		)
	}

	// Return updated entry
	return s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// GetPublicWatchlist returns user's public watchlist filtered by allowed statuses
func (s *ListService) GetPublicWatchlist(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	if len(statuses) == 0 {
		// If no statuses specified, return all
		return s.listRepo.GetByUser(ctx, userID)
	}
	return s.listRepo.GetByUserAndStatuses(ctx, userID, statuses)
}
