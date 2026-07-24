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
	string(domain.TypeNewEpisode):         true,
	string(domain.TypeFeedbackCreated):    true,
	string(domain.TypeFeedbackInProgress): true,
	string(domain.TypeFeedbackAIDone):     true,
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

// NewEpisodeDedupeKey builds the canonical per-anime dedupe key for a
// new_episode notification:
//
//	new_episode:<anime_id>
//
// AUTO-660 deliberately excludes provider/team fields. The detector still
// scans and snapshots every watched combo, but a user gets one active
// notification for an anime regardless of which combo discovers the episode.
func NewEpisodeDedupeKey(animeID string) string {
	return fmt.Sprintf("new_episode:%s", animeID)
}

// LegacyNewEpisodeDedupeKey returns the pre-AUTO-660 combo-scoped key. The
// detector supplies these keys when it creates the canonical per-anime row so
// any still-unread legacy rows are invalidated instead of remaining visible
// beside the grouped notification.
func LegacyNewEpisodeDedupeKey(animeID, player, language, watchType, translationID string) string {
	return fmt.Sprintf("new_episode:%s:%s:%s:%s:%s",
		animeID, player, language, watchType, translationID)
}

// FeedbackDedupeKey builds the canonical dedupe key for a feedback_* stage
// notification: feedback:<report_id>:<stage>. Stage is one of
// created | in_progress | ai_done.
func FeedbackDedupeKey(reportID, stage string) string {
	return fmt.Sprintf("feedback:%s:%s", reportID, stage)
}

// UpsertRequest is the input to the producer path.
//
// InvalidateDedupeKeys (optional) lists dedupe keys whose still-unread
// notifications for the same user should be stamped invalidated_at after
// the upsert succeeds — the feedback triage loop uses this so a new stage
// supersedes the previous one's unread row. Already-read rows are left
// untouched (the user saw them; history stays honest).
type UpsertRequest struct {
	UserID               string          `json:"user_id"`
	Type                 string          `json:"type"`
	DedupeKey            string          `json:"dedupe_key"`
	Payload              json.RawMessage `json:"payload"`
	InvalidateDedupeKeys []string        `json:"invalidate_dedupe_keys,omitempty"`
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

// Delete delegates (bin from the history modal — soft-remove everywhere).
func (s *NotificationService) Delete(ctx context.Context, userID, id string) error {
	return s.repo.Delete(ctx, userID, id)
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

	// Best-effort supersede: invalidate the listed dedupe keys' unread rows.
	// A failure here must not fail the producer call — the new notification
	// already exists; worst case the user briefly sees two stages.
	if len(req.InvalidateDedupeKeys) > 0 {
		if _, err := s.repo.InvalidateUnreadByDedupeKeys(ctx, req.UserID, req.InvalidateDedupeKeys); err != nil {
			s.log.Warnw("failed to invalidate superseded notifications",
				"user_id", req.UserID,
				"dedupe_keys", req.InvalidateDedupeKeys,
				"error", err,
			)
		}
	}
	return row, nil
}

// InvalidateUnread stamps invalidated_at on the user's unread notifications
// matching the given dedupe keys, without creating anything new. Used when a
// feedback report is closed as not_relevant — pending stage notifications
// stop being actual but no replacement notification is warranted.
func (s *NotificationService) InvalidateUnread(
	ctx context.Context,
	userID string,
	dedupeKeys []string,
) (int64, error) {
	if userID == "" {
		return 0, apperrors.InvalidInput("user_id required")
	}
	if len(dedupeKeys) == 0 {
		return 0, apperrors.InvalidInput("dedupe_keys required")
	}
	return s.repo.InvalidateUnreadByDedupeKeys(ctx, userID, dedupeKeys)
}
