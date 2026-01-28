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

// UpdateListEntry updates or creates an anime list entry
func (s *ListService) UpdateListEntry(ctx context.Context, userID string, req *domain.UpdateListRequest) (*domain.AnimeListEntry, error) {
	entry := &domain.AnimeListEntry{
		UserID:  userID,
		AnimeID: req.AnimeID,
		Status:  req.Status,
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

	// Set timestamps based on status
	now := time.Now()
	if req.Status == "watching" && entry.StartedAt == nil {
		entry.StartedAt = &now
	}
	if req.Status == "completed" {
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
