package service

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// commentBodyMaxRunes caps the body at 2000 UTF-8 runes — counted via
// utf8.RuneCountInString so a 2000-character Japanese / Cyrillic comment
// is not falsely rejected because its byte length exceeds 2000.
const commentBodyMaxRunes = 2000

// commentPreviewMaxRunes is how many runes of the body fit in the activity
// event's `content` column; the rest is replaced with a single "…" suffix.
const commentPreviewMaxRunes = 300

// commentListDefaultLimit / commentListMaxLimit guard ListComments.
const (
	commentListDefaultLimit = 50
	commentListMaxLimit     = 100
)

// rate-limit knobs — per-(user, anime), sliding 1-hour window. Acceptable
// for v0.1 single-replica per CONTEXT.md.
const (
	rateLimitMax    = 10
	rateLimitWindow = time.Hour
)

// rateBucket is a per-process in-memory rate limiter keyed by
// (userID, animeID). Constructed inside NewCommentService so tests get a
// fresh bucket and 429s from one test don't leak into the next.
type rateBucket struct {
	mu      sync.Mutex
	entries map[string][]time.Time
}

func newRateBucket() *rateBucket {
	return &rateBucket{entries: map[string][]time.Time{}}
}

// allow prunes entries older than rateLimitWindow, then either records
// `now` and returns true OR returns false when the window already holds
// `rateLimitMax` events.
func (b *rateBucket) allow(userID, animeID string) bool {
	key := userID + "|" + animeID
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rateLimitWindow)

	// Prune in-place using a single forward pass.
	existing := b.entries[key]
	keep := existing[:0]
	for _, t := range existing {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}

	if len(keep) >= rateLimitMax {
		b.entries[key] = keep
		return false
	}
	keep = append(keep, now)
	b.entries[key] = keep
	return true
}

// CommentService coordinates comment writes, soft deletes, listing and
// the activity-event emission that powers ActivityFeed.vue.
type CommentService struct {
	commentRepo  *repo.CommentRepository
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
	rateBucket   *rateBucket
}

// NewCommentService constructs the service with a fresh in-memory rate bucket.
func NewCommentService(commentRepo *repo.CommentRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *CommentService {
	return &CommentService{
		commentRepo:  commentRepo,
		activityRepo: activityRepo,
		log:          log,
		rateBucket:   newRateBucket(),
	}
}

// validateBody applies the trim + non-empty + ≤2000-rune contract.
// Returns the trimmed body on success.
func validateBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", errors.InvalidInput("comment body cannot be empty")
	}
	if utf8.RuneCountInString(trimmed) > commentBodyMaxRunes {
		return "", errors.InvalidInput("comment body cannot exceed 2000 characters")
	}
	return trimmed, nil
}

// truncatePreview returns the first `commentPreviewMaxRunes` runes of body
// suffixed with "…" if truncation occurred. Body that already fits is
// returned unchanged.
func truncatePreview(body string) string {
	runes := []rune(body)
	if len(runes) <= commentPreviewMaxRunes {
		return body
	}
	return string(runes[:commentPreviewMaxRunes]) + "…"
}

// CreateComment validates body (1..2000 UTF-8 runes after trim), gates on
// the rate bucket, persists the row, and emits one `type='comment'`
// activity event per successful create. NO per-day dedup — every create
// emits a separate row (this is the divergence from review events).
func (s *CommentService) CreateComment(ctx context.Context, userID, username, animeID string, req *domain.CreateCommentRequest) (*domain.Comment, error) {
	if req == nil {
		return nil, errors.InvalidInput("missing request body")
	}

	body, err := validateBody(req.Body)
	if err != nil {
		return nil, err
	}

	if !s.rateBucket.allow(userID, animeID) {
		return nil, errors.RateLimited()
	}

	c := &domain.Comment{
		UserID:   userID,
		AnimeID:  animeID,
		Username: username,
		Body:     body,
		// ParentID intentionally NIL in v0.1 — DTO does not expose it and
		// the service does not read it from anywhere (Pitfall 8 guard).
	}
	if err := s.commentRepo.Create(ctx, c); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to save comment")
	}

	// Emit one activity event per successful create. Failure is non-fatal —
	// the comment is already persisted; we just log and return.
	preview := truncatePreview(body)
	event := &domain.ActivityEvent{
		UserID:   userID,
		Username: username,
		AnimeID:  animeID,
		Type:     "comment",
		Content:  preview,
		// OldValue / NewValue intentionally empty — comments have no
		// score/status delta. The dedup branch from review.go is stripped.
	}
	if err := s.activityRepo.Create(ctx, event); err != nil {
		s.log.Errorw("failed to record comment activity",
			"user_id", userID, "anime_id", animeID, "error", err)
	}

	return c, nil
}

// UpdateComment edits the body of a comment owned by the caller (or by
// anyone if isAdmin is true — the backend allows admin edit for tooling
// parity even though the frontend hides the pencil from admins on
// non-owned comments per CONTEXT.md).
func (s *CommentService) UpdateComment(ctx context.Context, userID, commentID string, isAdmin bool, req *domain.UpdateCommentRequest) (*domain.Comment, error) {
	if req == nil {
		return nil, errors.InvalidInput("missing request body")
	}

	body, err := validateBody(req.Body)
	if err != nil {
		return nil, err
	}

	existing, err := s.commentRepo.GetByID(ctx, commentID)
	if err != nil {
		return nil, err
	}

	if existing.UserID != userID && !isAdmin {
		return nil, errors.Forbidden("not the comment owner")
	}

	if err := s.commentRepo.Update(ctx, commentID, body); err != nil {
		return nil, err
	}

	// Reload to return the canonical row (gets the fresh UpdatedAt).
	return s.commentRepo.GetByID(ctx, commentID)
}

// DeleteComment soft-deletes a comment owned by the caller (or by anyone
// if isAdmin is true).
func (s *CommentService) DeleteComment(ctx context.Context, userID, commentID string, isAdmin bool) error {
	existing, err := s.commentRepo.GetByID(ctx, commentID)
	if err != nil {
		return err
	}
	if existing.UserID != userID && !isAdmin {
		return errors.Forbidden("not the comment owner")
	}
	return s.commentRepo.SoftDelete(ctx, commentID)
}

// ListComments returns one page of the anime's comments newest-first.
// Limit defaults to 50 when 0; clamped to commentListMaxLimit.
func (s *CommentService) ListComments(ctx context.Context, animeID, cursor string, limit int) (*domain.CommentsListResponse, error) {
	if limit <= 0 {
		limit = commentListDefaultLimit
	}
	if limit > commentListMaxLimit {
		limit = commentListMaxLimit
	}

	comments, nextCursor, err := s.commentRepo.ListByAnime(ctx, animeID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []*domain.Comment{}
	}

	return &domain.CommentsListResponse{
		Comments:   comments,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}, nil
}
