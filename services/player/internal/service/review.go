package service

import (
	"context"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// ReviewService is the business-logic layer for the six reviews endpoints.
// Phase 1 (workstream: social) plan 02: refactored to consume the unified
// ListRepository exclusively. The public method signatures still match what
// the handler layer calls; only the return types change from the legacy
// review type to AnimeListEntry — consumers project to the 7-field
// reviewResponse struct in handler/review.go to keep the wire shape
// byte-identical.
type ReviewService struct {
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

// NewReviewService wires the refactored review service.
func NewReviewService(listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *ReviewService {
	return &ReviewService{
		listRepo:     listRepo,
		activityRepo: activityRepo,
		log:          log,
	}
}

// CreateOrUpdateReview creates or updates a user's review. The activity-
// emission block matches the pre-refactor behavior verbatim — per-day
// dedup via ActivityRepository.GetTodayByUserAnimeType, OldValue carries
// "new"/"update"/"score" markers as before.
func (s *ReviewService) CreateOrUpdateReview(ctx context.Context, userID, username string, isAdmin bool, req *domain.CreateReviewRequest) (*domain.AnimeListEntry, error) {
	// Validation rule unchanged from pre-refactor: score must be 1..10.
	if req.Score < 1 || req.Score > 10 {
		return nil, errors.InvalidInput("score must be between 1 and 10")
	}

	// Detect new-vs-update for the activity event's OldValue marker. A row
	// with score=0 + review_text='' counts as "new review" from the
	// activity-feed's perspective — functionally equivalent to "no review
	// yet" since the row has no review content.
	existing, _ := s.listRepo.GetUserReview(ctx, userID, req.AnimeID)

	entry, err := s.listRepo.UpsertReview(ctx, userID, req.AnimeID, username, req.Score, req.ReviewText)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to save review")
	}

	// AUTO-408 — admin-authored reviews get an automatic System «AnimeEnigma» 👍
	// (idempotent). Best-effort: a seed failure must never fail the review write.
	if isAdmin && entry != nil {
		if err := s.listRepo.SeedSystemReaction(ctx, entry.ID); err != nil {
			s.log.Errorw("failed to seed system reaction on admin review", "review_id", entry.ID, "error", err)
		}
	}

	// Record review activity event (deduplicated per day) — logic preserved
	// line-for-line from the pre-refactor service/review.go.
	isNewReview := existing == nil
	contentPreview := req.ReviewText
	if len([]rune(contentPreview)) > 300 {
		contentPreview = string([]rune(contentPreview)[:300]) + "…"
	}
	reviewEvent := &domain.ActivityEvent{
		UserID:   userID,
		Username: username,
		AnimeID:  req.AnimeID,
		Type:     "review",
		NewValue: strconv.Itoa(req.Score),
		Content:  contentPreview,
	}
	if req.ReviewText == "" {
		reviewEvent.OldValue = "score"
	} else if isNewReview {
		reviewEvent.OldValue = "new"
	} else {
		reviewEvent.OldValue = "update"
	}
	existingEvent, _ := s.activityRepo.GetTodayByUserAnimeType(ctx, userID, req.AnimeID, "review")
	if existingEvent != nil {
		existingEvent.NewValue = reviewEvent.NewValue
		existingEvent.OldValue = reviewEvent.OldValue
		existingEvent.Content = reviewEvent.Content
		if err := s.activityRepo.Update(ctx, existingEvent); err != nil {
			s.log.Errorw("failed to update review activity", "user_id", userID, "anime_id", req.AnimeID, "error", err)
		}
	} else {
		if err := s.activityRepo.Create(ctx, reviewEvent); err != nil {
			s.log.Errorw("failed to record review activity", "user_id", userID, "anime_id", req.AnimeID, "error", err)
		}
	}

	// Pre-refactor code synced the review score to the watchlist via a
	// second Upsert. After the schema merge that sync is now a no-op
	// because UpsertReview already wrote score into the same anime_list
	// row. Drop the redundant call.
	return entry, nil
}

// GetAnimeReviews returns every anime_list row for the anime that qualifies
// as a "review" (score>0 OR review_text!=''), each with its emoji reactions
// attached. viewerUserID (nil for anonymous) drives the per-emoji
// ReactedByMe flag. AUTO-408.
func (s *ReviewService) GetAnimeReviews(ctx context.Context, animeID string, viewerUserID *string) ([]*domain.AnimeListEntry, error) {
	entries, err := s.listRepo.GetReviewsByAnime(ctx, animeID)
	if err != nil {
		return nil, err
	}
	// Best-effort: a passive-watcher episode-count failure must never break the
	// reviews list — fall back to the raw anime_list.episodes value.
	if err := s.listRepo.ApplyEffectiveEpisodes(ctx, entries); err != nil {
		s.log.Warnw("apply effective episodes failed", "anime_id", animeID, "error", err)
	}
	return s.attachReactions(ctx, entries, viewerUserID)
}

// GetUserReview returns the current user's review (or errors.NotFound when
// the row is absent or empty-on-both), with reactions attached and
// ReactedByMe resolved for the requesting user. AUTO-408.
func (s *ReviewService) GetUserReview(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	entry, err := s.listRepo.GetUserReview(ctx, userID, animeID)
	if err != nil {
		return nil, err
	}
	single := []*domain.AnimeListEntry{entry}
	if err := s.listRepo.ApplyEffectiveEpisodes(ctx, single); err != nil {
		s.log.Warnw("apply effective episodes failed", "anime_id", animeID, "error", err)
	}
	viewer := userID
	if _, err := s.attachReactions(ctx, single, &viewer); err != nil {
		return nil, err
	}
	return entry, nil
}

// attachReactions populates entry.Reactions in-place for each entry by fetching
// aggregated counts in a single batched query. A nil/empty input is a no-op.
// AUTO-408.
func (s *ReviewService) attachReactions(ctx context.Context, entries []*domain.AnimeListEntry, viewerUserID *string) ([]*domain.AnimeListEntry, error) {
	if len(entries) == 0 {
		return entries, nil
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	counts, err := s.listRepo.GetReactionCounts(ctx, ids, viewerUserID)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to load review reactions")
	}
	for _, e := range entries {
		e.Reactions = counts[e.ID]
	}
	return entries, nil
}

// ToggleReaction sets/replaces/removes the caller's single reaction on a review
// (one reaction per person — see repo.ToggleReaction) and returns the added flag
// plus the review's fresh reaction counts. Rejects emojis outside the fixed
// 12-emoji palette and blocks reacting to your own review. username is
// denormalized onto the reaction for the who-reacted popover. AUTO-408.
func (s *ReviewService) ToggleReaction(ctx context.Context, animeID, reviewID, userID, username, emoji string) (bool, []domain.ReactionCount, error) {
	if !domain.AllowedReactionEmojis[emoji] {
		return false, nil, errors.InvalidInput("unsupported reaction emoji")
	}
	// No self-reactions. AUTO-408.
	authorID, err := s.listRepo.GetReviewAuthorID(ctx, reviewID)
	if err != nil {
		return false, nil, errors.Wrap(err, errors.CodeInternal, "failed to resolve review author")
	}
	if authorID == "" {
		return false, nil, errors.NotFound("review not found")
	}
	if authorID == userID {
		return false, nil, errors.Forbidden("cannot react to your own review")
	}
	added, err := s.listRepo.ToggleReaction(ctx, reviewID, userID, username, emoji)
	if err != nil {
		return false, nil, errors.Wrap(err, errors.CodeInternal, "failed to toggle reaction")
	}
	viewer := userID
	counts, err := s.listRepo.GetReactionCounts(ctx, []string{reviewID}, &viewer)
	if err != nil {
		return false, nil, errors.Wrap(err, errors.CodeInternal, "failed to load reaction counts")
	}
	return added, counts[reviewID], nil
}

// GetUserReviews returns every anime_list row for the user that qualifies
// as a review (score>0 OR review_text!=''). Used by GET /api/users/reviews.
func (s *ReviewService) GetUserReviews(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	entries, err := s.listRepo.GetReviewsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.listRepo.ApplyEffectiveEpisodes(ctx, entries); err != nil {
		s.log.Warnw("apply effective episodes failed", "user_id", userID, "error", err)
	}
	return entries, nil
}

// GetAnimeRating returns the average rating + scoring-row count.
func (s *ReviewService) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	return s.listRepo.GetAnimeRating(ctx, animeID)
}

// GetBatchAnimeRatings returns average ratings for multiple anime.
func (s *ReviewService) GetBatchAnimeRatings(ctx context.Context, animeIDs []string) (map[string]*domain.AnimeRating, error) {
	return s.listRepo.GetBatchAnimeRatings(ctx, animeIDs)
}

// DeleteReview clears the user's review (score=0, review_text='') — the
// underlying anime_list row stays. Idempotent on missing rows.
func (s *ReviewService) DeleteReview(ctx context.Context, userID, animeID string) error {
	return s.listRepo.ClearReview(ctx, userID, animeID)
}
