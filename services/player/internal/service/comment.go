package service

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// rateBucket is a per-process in-memory rate limiter keyed by (userID, animeID).
// Per CONTEXT.md decision: rate limit is per-user-per-anime-per-hour. Single replica
// today, so process-local state is acceptable. Plan 03 will fill `allow` with the
// real 10-per-hour sliding-window check.
type rateBucket struct {
	mu      sync.Mutex
	entries map[string][]time.Time
}

func newRateBucket() *rateBucket {
	return &rateBucket{entries: map[string][]time.Time{}}
}

// allow is the rate-limit gate. Wave-0 stub always permits — plan 03 implements
// the 10-per-hour sliding-window prune.
func (b *rateBucket) allow(userID, animeID string) bool {
	return true
}

// CommentService coordinates comment writes, soft deletes, listing and the
// activity-event emission that powers ActivityFeed.vue.
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

// CreateComment validates body (1..2000 UTF-8 runes after trim), gates on the
// rate bucket, persists the row, and emits a `type='comment'` activity event.
func (s *CommentService) CreateComment(ctx context.Context, userID, username, animeID string, req *domain.CreateCommentRequest) (*domain.Comment, error) {
	return nil, errors.New(errors.CodeUnavailable, "comment service CreateComment: not implemented")
}

// UpdateComment edits the body of a comment owned by the caller (or by anyone
// if isAdmin is true).
func (s *CommentService) UpdateComment(ctx context.Context, userID, commentID string, isAdmin bool, req *domain.UpdateCommentRequest) (*domain.Comment, error) {
	return nil, errors.New(errors.CodeUnavailable, "comment service UpdateComment: not implemented")
}

// DeleteComment soft-deletes a comment owned by the caller (or by anyone if
// isAdmin is true).
func (s *CommentService) DeleteComment(ctx context.Context, userID, commentID string, isAdmin bool) error {
	return errors.New(errors.CodeUnavailable, "comment service DeleteComment: not implemented")
}

// ListComments returns one page of the anime's comments newest-first.
func (s *CommentService) ListComments(ctx context.Context, animeID, cursor string, limit int) (*domain.CommentsListResponse, error) {
	return nil, errors.New(errors.CodeUnavailable, "comment service ListComments: not implemented")
}
