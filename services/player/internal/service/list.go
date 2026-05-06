package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
)

// recsRepoForListService is the narrow surface ListService needs from the
// recs repo. Production wires *repo.RecsRepository; tests inject a fake.
// Phase 13 (REC-INFRA-03) — synchronous S6 seed update inside
// MarkEpisodeWatched. May be nil in tests that don't exercise the seed
// path; the hot path nil-guards before invoking.
type recsRepoForListService interface {
	UpdateS6Seed(ctx context.Context, userID, animeID string, completedAt time.Time, score int) error
}

// listServiceCache is the narrow Delete-only cache surface ListService
// needs to invalidate the user's recs:user:{id}:topN key after a seed
// update. Phase 13 (REC-INFRA-03) — fire-and-forget. May be nil in tests.
type listServiceCache interface {
	Delete(ctx context.Context, keys ...string) error
}

type ListService struct {
	listRepo         *repo.ListRepository
	activityRepo     *repo.ActivityRepository
	prefRepo         *repo.PreferenceRepository
	progressRepo     *repo.ProgressRepository
	userOrchestrator *recs.UserOrchestrator   // Phase 11 (REC-INFRA-02) — debounced trigger; may be nil in tests
	recsRepo         recsRepoForListService   // Phase 13 (REC-INFRA-03) — synchronous S6 seed update; may be nil in tests
	cache            listServiceCache         // Phase 13 (REC-INFRA-03) — cache invalidation after seed update; may be nil in tests
	log              *logger.Logger
}

// NewListService wires the list service. The userOrchestrator, recsRepo, and
// cache arguments may be nil in test environments that don't exercise the
// recs trigger / seed-update / cache-bust paths; MarkEpisodeWatched
// nil-guards each before invoking.
func NewListService(
	listRepo *repo.ListRepository,
	activityRepo *repo.ActivityRepository,
	prefRepo *repo.PreferenceRepository,
	progressRepo *repo.ProgressRepository,
	userOrchestrator *recs.UserOrchestrator,
	recsRepo recsRepoForListService,
	cache listServiceCache,
	log *logger.Logger,
) *ListService {
	return &ListService{
		listRepo:         listRepo,
		activityRepo:     activityRepo,
		prefRepo:         prefRepo,
		progressRepo:     progressRepo,
		userOrchestrator: userOrchestrator,
		recsRepo:         recsRepo,
		cache:            cache,
		log:              log,
	}
}

// GetUserList returns user's anime list with optional status filter
func (s *ListService) GetUserList(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	if status != "" {
		return s.listRepo.GetByUserAndStatus(ctx, userID, status)
	}
	return s.listRepo.GetByUser(ctx, userID)
}

// GetUserListPaginated returns user's anime list with pagination
func (s *ListService) GetUserListPaginated(ctx context.Context, userID, status string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	return s.listRepo.GetByUserPaginated(ctx, userID, status, params)
}

// GetUserStatuses returns lightweight anime_id+status pairs for the entire list
func (s *ListService) GetUserStatuses(ctx context.Context, userID string) ([]domain.AnimeStatusEntry, error) {
	return s.listRepo.GetByUserStatuses(ctx, userID)
}

// GetPublicWatchlistPaginated returns user's public watchlist with pagination
func (s *ListService) GetPublicWatchlistPaginated(ctx context.Context, userID string, statuses []string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	if len(statuses) == 0 {
		return s.listRepo.GetByUserPaginated(ctx, userID, "", params)
	}
	return s.listRepo.GetByUserAndStatusesPaginated(ctx, userID, statuses, params)
}

// GetUserAnimeEntry returns a single anime entry from user's list
func (s *ListService) GetUserAnimeEntry(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	return s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// UpdateListEntry updates or creates an anime list entry
func (s *ListService) UpdateListEntry(ctx context.Context, userID, username string, req *domain.UpdateListRequest) (*domain.AnimeListEntry, error) {
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

	// Record activity event if status changed (skip for imports with no username)
	oldStatus := ""
	if existingEntry != nil {
		oldStatus = existingEntry.Status
	}
	if oldStatus != req.Status && username != "" {
		activityEvent := &domain.ActivityEvent{
			UserID:   userID,
			Username: username,
			AnimeID:  req.AnimeID,
			Type:     "status_change",
			OldValue: oldStatus,
			NewValue: req.Status,
		}
		if err := s.activityRepo.Create(ctx, activityEvent); err != nil {
			s.log.Errorw("failed to record status change activity",
				"user_id", userID,
				"anime_id", req.AnimeID,
				"error", err,
			)
		}
	}

	return entry, nil
}

// DeleteListEntry removes an anime from user's list
func (s *ListService) DeleteListEntry(ctx context.Context, userID, animeID string) error {
	return s.listRepo.Delete(ctx, userID, animeID)
}

// MarkEpisodeWatched marks an episode as watched and updates the episodes count.
// Called by both the 20-min auto-mark and the manual mark-watched button across
// all four players. After bumping anime_list.episodes, also flips
// watch_progress.completed=true for the episode (single source of truth — Phase 3).
// The MarkCompleted call is best-effort: a failure is logged but does not fail
// the request, mirroring the existing watch_history pattern below.
func (s *ListService) MarkEpisodeWatched(ctx context.Context, userID, animeID string, req *domain.MarkEpisodeWatchedRequest) (*domain.AnimeListEntry, error) {
	updated, err := s.listRepo.IncrementEpisodes(ctx, userID, animeID, req.Episode)
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
				"episode", req.Episode,
			)
			now := time.Now()
			entry := &domain.AnimeListEntry{
				UserID:    userID,
				AnimeID:   animeID,
				Status:    "watching",
				Episodes:  req.Episode,
				StartedAt: &now,
			}
			if err := s.listRepo.Upsert(ctx, entry); err != nil {
				return nil, err
			}
			// Single source of truth: also mark watch_progress.completed=true
			if err := s.progressRepo.MarkCompleted(ctx, userID, animeID, req.Episode); err != nil {
				s.log.Errorw("failed to mark watch_progress completed (auto-create path)",
					"user_id", userID,
					"anime_id", animeID,
					"episode", req.Episode,
					"error", err,
				)
			}
			return entry, nil
		}

		s.log.Infow("episode already marked",
			"user_id", userID,
			"anime_id", animeID,
			"episode", req.Episode,
		)
	}

	// Single source of truth: flip watch_progress.completed=true for this episode.
	// Idempotent — safe whether IncrementEpisodes updated, was a no-op, or hit
	// the auto-create branch above (which has its own MarkCompleted call).
	if err := s.progressRepo.MarkCompleted(ctx, userID, animeID, req.Episode); err != nil {
		s.log.Errorw("failed to mark watch_progress completed",
			"user_id", userID,
			"anime_id", animeID,
			"episode", req.Episode,
			"error", err,
		)
	}

	// Create watch history with combo context if present
	if req.Player != "" {
		progress, _ := s.progressRepo.GetByUserAnimeEpisode(ctx, userID, animeID, req.Episode)
		durationWatched := 0
		if progress != nil {
			durationWatched = progress.Progress
		}
		history := &domain.WatchHistory{
			UserID:           userID,
			AnimeID:          animeID,
			EpisodeNumber:    req.Episode,
			Player:           req.Player,
			Language:         req.Language,
			WatchType:        req.WatchType,
			TranslationID:    req.TranslationID,
			TranslationTitle: req.TranslationTitle,
			DurationWatched:  durationWatched,
			SessionID:        req.SessionID,
			WatchedAt:        time.Now(),
		}
		if err := s.prefRepo.CreateWatchHistory(ctx, history); err != nil {
			s.log.Errorw("failed to create watch history",
				"user_id", userID,
				"anime_id", animeID,
				"episode", req.Episode,
				"error", err,
			)
		}
		metrics.WatchEpisodesTotal.WithLabelValues(req.Player, req.Language, req.WatchType).Inc()

		// Phase 11 (REC-INFRA-02): fire-and-forget debounced trigger so the
		// user's recs row reflects the new watch within ~5 minutes. The
		// orchestrator handles the SetNX debounce + spawning internally —
		// this call is non-blocking and never returns an error to us. We use
		// context.Background() so a cancelled request context (the HTTP
		// handler returning) doesn't kill the goroutine before SetNX runs.
		if s.userOrchestrator != nil {
			go func() {
				_ = s.userOrchestrator.TriggerForUser(context.Background(), userID)
			}()
		}
	}

	// Phase 13 (REC-INFRA-03): SYNCHRONOUS S6 seed update when this
	// completion qualifies — status='completed' AND score>=7 AND
	// completed_at is set. The pin must appear on the next /api/users/recs
	// call, so we update inside the request path (NOT fire-and-forget like
	// the user orchestrator above). Failure is logged but does NOT fail
	// the request — same contract as CreateWatchHistory above.
	//
	// Latency budget per Decision §B1: < 5ms p95 added overhead. The
	// UpdateS6Seed UPDATE touches a single PK row + 4 columns; expected
	// 1-2ms. The follow-up cache.Delete is fire-and-forget (goroutine).
	if s.recsRepo != nil {
		freshEntry, _ := s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
		if freshEntry != nil &&
			freshEntry.Status == "completed" &&
			freshEntry.Score >= 7 &&
			freshEntry.CompletedAt != nil {
			if err := s.recsRepo.UpdateS6Seed(ctx, userID, animeID, *freshEntry.CompletedAt, freshEntry.Score); err != nil {
				s.log.Errorw("failed to update s6 seed (non-fatal)",
					"user_id", userID,
					"anime_id", animeID,
					"error", err,
				)
			} else if s.cache != nil {
				// Fire-and-forget cache bust so the next recs call rebuilds
				// with the pin. Use context.Background() so a cancelled
				// request context doesn't kill the goroutine before the
				// Delete runs.
				go func() {
					if err := s.cache.Delete(context.Background(), recs.UserTopNKey(recs.UserID(userID))); err != nil {
						s.log.Warnw("failed to bust recs cache after s6 seed update (non-fatal)",
							"user_id", userID, "error", err)
					}
				}()
			}
		}
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

// GetPublicWatchlistStats returns aggregate stats for a user's public watchlist
func (s *ListService) GetPublicWatchlistStats(ctx context.Context, userID string, statuses []string) (*domain.WatchlistStats, error) {
	return s.listRepo.GetUserWatchlistStats(ctx, userID, statuses)
}

// GetPublicWatchlist returns user's public watchlist filtered by allowed statuses
func (s *ListService) GetPublicWatchlist(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	if len(statuses) == 0 {
		return s.listRepo.GetByUser(ctx, userID)
	}
	return s.listRepo.GetByUserAndStatuses(ctx, userID, statuses)
}
