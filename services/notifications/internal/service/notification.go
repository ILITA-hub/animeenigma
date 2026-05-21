package service

import (
	"context"
	"encoding/json"
	"fmt"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
)

// allowedTypes is the v1.0 whitelist of notification types. Anything else
// rejected at the Upsert boundary so a buggy producer can't pollute the
// table with unrecognised types that the frontend won't render.
var allowedTypes = map[string]bool{
	string(domain.TypeNewEpisode): true,
}

// NotificationService is the thin orchestration layer between the HTTP
// handler and the repo. Owns input validation, payload-JSON marshalling,
// and dedupe-key construction helpers. All cross-cutting concerns
// (transactions, retries, metrics) live here so the repo stays a clean
// SQL layer.
type NotificationService struct {
	repo *repo.NotificationRepository
	log  *logger.Logger
}

// NewNotificationService constructs the service.
func NewNotificationService(r *repo.NotificationRepository, log *logger.Logger) *NotificationService {
	return &NotificationService{repo: r, log: log}
}

// NewEpisodeDedupeKey builds the canonical dedupe key for a new_episode
// notification per the design-doc Dedupe Key spec:
//
//	new_episode:<anime_id>:<player>:<language>:<watch_type>:<translation_id>
//
// Stable order so two callers (Phase 2 detector + this service's UPSERT)
// always agree on the same string.
func NewEpisodeDedupeKey(animeID, player, language, watchType, translationID string) string {
	return fmt.Sprintf("new_episode:%s:%s:%s:%s:%s",
		animeID, player, language, watchType, translationID)
}

// UpsertRequest is the input to the producer path.
type UpsertRequest struct {
	UserID    string          `json:"user_id"`
	Type      string          `json:"type"`
	DedupeKey string          `json:"dedupe_key"`
	Payload   json.RawMessage `json:"payload"`
}

// List delegates to the repo with default args translated.
func (s *NotificationService) List(
	ctx context.Context,
	userID string,
	status repo.ListStatus,
	limit, offset int,
) ([]domain.UserNotification, int64, int64, error) {
	return s.repo.List(ctx, userID, status, limit, offset)
}

// UnreadCount delegates.
func (s *NotificationService) UnreadCount(ctx context.Context, userID string) (int64, error) {
	return s.repo.UnreadCount(ctx, userID)
}

// MarkRead delegates.
func (s *NotificationService) MarkRead(ctx context.Context, userID, id string) error {
	return s.repo.MarkRead(ctx, userID, id)
}

// MarkAllRead delegates.
func (s *NotificationService) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	return s.repo.MarkAllRead(ctx, userID)
}

// Dismiss delegates.
func (s *NotificationService) Dismiss(ctx context.Context, userID, id string) error {
	return s.repo.Dismiss(ctx, userID, id)
}

// Click delegates.
func (s *NotificationService) Click(ctx context.Context, userID, id string) error {
	return s.repo.Click(ctx, userID, id)
}

// Upsert validates the request, marshals the payload, and calls the repo.
// This is the only path the internal producer endpoint (and Phase 2's
// detector) use to create or refresh notifications.
func (s *NotificationService) Upsert(
	ctx context.Context,
	req UpsertRequest,
) (*domain.UserNotification, error) {
	if req.UserID == "" {
		return nil, apperrors.InvalidInput("user_id required")
	}
	if !allowedTypes[req.Type] {
		return nil, apperrors.InvalidInput(fmt.Sprintf("unknown notification type: %q", req.Type))
	}
	if req.DedupeKey == "" {
		return nil, apperrors.InvalidInput("dedupe_key required")
	}
	if len(req.Payload) == 0 {
		return nil, apperrors.InvalidInput("payload required")
	}

	// Validate the payload is well-formed JSON (the column is jsonb NOT
	// NULL — bad JSON would surface as a 500 from Postgres rather than a
	// proper 400 if we didn't catch it here).
	var probe interface{}
	if err := json.Unmarshal(req.Payload, &probe); err != nil {
		return nil, apperrors.InvalidInput(fmt.Sprintf("payload is not valid JSON: %v", err))
	}

	row, err := s.repo.Upsert(ctx, req.UserID, req.Type, req.DedupeKey, []byte(req.Payload))
	if err != nil {
		return nil, err
	}
	return row, nil
}
