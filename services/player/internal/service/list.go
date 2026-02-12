package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ListService struct {
	listRepo   *repo.ListRepository
	log        *logger.Logger
	catalogURL string
	httpClient *http.Client
}

func NewListService(listRepo *repo.ListRepository, log *logger.Logger) *ListService {
	return &ListService{
		listRepo:   listRepo,
		log:        log,
		catalogURL: "http://catalog:8081",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetUserList returns user's anime list with optional status filter
func (s *ListService) GetUserList(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	var err error
	if status != "" {
		entries, err = s.listRepo.GetByUserAndStatus(ctx, userID, status)
	} else {
		entries, err = s.listRepo.GetByUser(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	s.enrichEntries(ctx, entries)
	return entries, nil
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

	// When marking as completed, set episodes to total if not explicitly provided
	if req.Status == "completed" && req.Episodes == nil {
		totalEpisodes := entry.AnimeTotalEpisodes
		if totalEpisodes == 0 && existingEntry != nil {
			totalEpisodes = existingEntry.AnimeTotalEpisodes
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
func (s *ListService) MigrateListEntry(ctx context.Context, userID, oldAnimeID, newAnimeID, newTitle, newCover string) (*domain.AnimeListEntry, error) {
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

	title := newTitle
	if title == "" {
		title = oldEntry.AnimeTitle
	}
	cover := newCover
	if cover == "" {
		cover = oldEntry.AnimeCover
	}

	newEntry := &domain.AnimeListEntry{
		UserID:             userID,
		AnimeID:            newAnimeID,
		AnimeTitle:         title,
		AnimeCover:         cover,
		Status:             oldEntry.Status,
		Score:              oldEntry.Score,
		Episodes:           oldEntry.Episodes,
		Notes:              oldEntry.Notes,
		Tags:               oldEntry.Tags,
		IsRewatching:       oldEntry.IsRewatching,
		Priority:           oldEntry.Priority,
		AnimeType:          oldEntry.AnimeType,
		AnimeTotalEpisodes: oldEntry.AnimeTotalEpisodes,
		MalID:              oldEntry.MalID,
		StartedAt:          oldEntry.StartedAt,
		CompletedAt:        oldEntry.CompletedAt,
	}

	if err := s.listRepo.Upsert(ctx, newEntry); err != nil {
		return nil, err
	}

	return newEntry, nil
}

// GetPublicWatchlist returns user's public watchlist filtered by allowed statuses
func (s *ListService) GetPublicWatchlist(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	var err error
	if len(statuses) == 0 {
		entries, err = s.listRepo.GetByUser(ctx, userID)
	} else {
		entries, err = s.listRepo.GetByUserAndStatuses(ctx, userID, statuses)
	}
	if err != nil {
		return nil, err
	}
	s.enrichEntries(ctx, entries)
	return entries, nil
}

// enrichEntries fills in missing anime titles/covers by fetching from the catalog service.
// Also persists the data back to the DB so subsequent requests don't need to fetch again.
func (s *ListService) enrichEntries(ctx context.Context, entries []*domain.AnimeListEntry) {
	var missing []*domain.AnimeListEntry
	for _, e := range entries {
		if e.AnimeTitle == "" {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return
	}

	for _, e := range missing {
		info := s.fetchAnimeFromCatalog(ctx, e.AnimeID)
		if info == nil {
			continue
		}
		e.AnimeTitle = info.Name
		if info.PosterURL != "" {
			e.AnimeCover = info.PosterURL
		}
		// Persist to DB in background so next request has the data
		entry := *e
		go func() {
			if err := s.listRepo.Upsert(context.Background(), &entry); err != nil {
				s.log.Warnw("failed to backfill anime title", "anime_id", entry.AnimeID, "error", err)
			}
		}()
	}
}

type catalogAnimeInfo struct {
	Name      string `json:"name"`
	NameRU    string `json:"name_ru"`
	PosterURL string `json:"poster_url"`
}

func (s *ListService) fetchAnimeFromCatalog(ctx context.Context, animeID string) *catalogAnimeInfo {
	url := fmt.Sprintf("%s/api/anime/%s", s.catalogURL, animeID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		Data catalogAnimeInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	// Prefer Russian name, fall back to romanized
	name := result.Data.NameRU
	if name == "" {
		name = result.Data.Name
	}
	if name == "" {
		return nil
	}
	result.Data.Name = name

	return &result.Data
}
