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
		UserID:  userID,
		AnimeID: req.AnimeID,
		Status:  req.Status,
	}

	if req.Score != nil {
		entry.Score = *req.Score
	} else if existingEntry != nil {
		entry.Score = existingEntry.Score
	}

	if req.Episodes != nil {
		entry.Episodes = *req.Episodes
	} else if existingEntry != nil {
		entry.Episodes = existingEntry.Episodes
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

	// When marking as completed, set episodes to total if not explicitly provided
	if req.Status == "completed" && req.Episodes == nil {
		// Use Preloaded anime info for total episodes
		var totalEpisodes int
		if existingEntry != nil && existingEntry.Anime != nil {
			totalEpisodes = existingEntry.Anime.EpisodesCount
		}
		if totalEpisodes > 0 {
			entry.Episodes = totalEpisodes
		}
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
		// Check if the anime is in the user's list at all
		existing, err := s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
		if err != nil {
			return nil, err
		}

		if existing == nil {
			// Auto-create watchlist entry with status "watching"
			s.log.Infow("auto-creating watchlist entry for episode marking",
				"user_id", userID,
				"anime_id", animeID,
				"episode", episode,
			)
			now := time.Now()
			entry := &domain.AnimeListEntry{
				UserID:    userID,
				AnimeID:   animeID,
				Status:    "watching",
				Episodes:  episode,
				StartedAt: &now,
			}
			if err := s.listRepo.Upsert(ctx, entry); err != nil {
				return nil, err
			}
			return entry, nil
		}

		s.log.Infow("episode already marked",
			"user_id", userID,
			"anime_id", animeID,
			"episode", episode,
		)
	}

	// Return updated entry
	return s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// MigrateListEntry migrates a list entry from oldAnimeID to newAnimeID.
// Used when resolving mal_XXXXX entries to real UUID entries.
// Preserves status, score, episodes, dates, and all other fields.
func (s *ListService) MigrateListEntry(ctx context.Context, userID, oldAnimeID, newAnimeID string) (*domain.AnimeListEntry, error) {
	oldEntry, err := s.listRepo.GetByUserAndAnime(ctx, userID, oldAnimeID)
	if err != nil {
		return nil, err
	}
	if oldEntry == nil {
		return nil, nil
	}

	// If an entry with the new UUID already exists, just delete the old one
	existingNew, _ := s.listRepo.GetByUserAndAnime(ctx, userID, newAnimeID)
	if existingNew != nil {
		_ = s.listRepo.Delete(ctx, userID, oldAnimeID)
		return existingNew, nil
	}

	// Delete old entry, create new one with same data
	_ = s.listRepo.Delete(ctx, userID, oldAnimeID)

	newEntry := &domain.AnimeListEntry{
		UserID:       userID,
		AnimeID:      newAnimeID,
		Status:       oldEntry.Status,
		Score:        oldEntry.Score,
		Episodes:     oldEntry.Episodes,
		Notes:        oldEntry.Notes,
		Tags:         oldEntry.Tags,
		IsRewatching: oldEntry.IsRewatching,
		Priority:     oldEntry.Priority,
		MalID:        oldEntry.MalID,
		StartedAt:    oldEntry.StartedAt,
		CompletedAt:  oldEntry.CompletedAt,
	}

	if err := s.listRepo.Upsert(ctx, newEntry); err != nil {
		return nil, err
	}

	return newEntry, nil
}

// GetPublicWatchlist returns user's public watchlist filtered by allowed statuses
func (s *ListService) GetPublicWatchlist(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	if len(statuses) == 0 {
		return s.listRepo.GetByUser(ctx, userID)
	}
	return s.listRepo.GetByUserAndStatuses(ctx, userID, statuses)
}
