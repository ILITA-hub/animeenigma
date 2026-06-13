package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ListService struct {
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	prefRepo     *repo.PreferenceRepository
	progressRepo *repo.ProgressRepository
	recsHint     *RecsHintProducer    // Recs extraction Phase 1 — fire-and-forget recompute hint to recs:8094; nil-safe, may be nil in tests
	gachaCredit  *GachaCreditProducer // Phase 4 — fire-and-forget Энигмы credits; nil-safe, may be nil in tests
	log          *logger.Logger
}

// NewListService wires the list service. The recsHint and gachaCredit
// arguments may be nil in test environments that don't exercise the
// respective paths; MarkEpisodeWatched and UpdateListEntry nil-guard each
// before invoking.
func NewListService(
	listRepo *repo.ListRepository,
	activityRepo *repo.ActivityRepository,
	prefRepo *repo.PreferenceRepository,
	progressRepo *repo.ProgressRepository,
	recsHint *RecsHintProducer,
	gachaCredit *GachaCreditProducer,
	log *logger.Logger,
) *ListService {
	return &ListService{
		listRepo:     listRepo,
		activityRepo: activityRepo,
		prefRepo:     prefRepo,
		progressRepo: progressRepo,
		recsHint:     recsHint,
		gachaCredit:  gachaCredit,
		log:          log,
	}
}

// GetUserList returns user's anime list with optional status filter
func (s *ListService) GetUserList(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	if status != "" {
		return s.listRepo.GetByUserAndStatus(ctx, userID, status)
	}
	return s.listRepo.GetByUser(ctx, userID, false)
}

// GetUserListPaginated returns user's anime list with pagination.
// search filters entries by anime title (name / name_ru / name_jp, case-insensitive). Empty = no filter.
func (s *ListService) GetUserListPaginated(ctx context.Context, userID, status, search string, filters domain.ListFilters, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	return s.listRepo.GetByUserPaginated(ctx, userID, status, search, false, filters, params)
}

// GetUserStatuses returns lightweight anime_id+status pairs for the entire list
func (s *ListService) GetUserStatuses(ctx context.Context, userID string) ([]domain.AnimeStatusEntry, error) {
	return s.listRepo.GetByUserStatuses(ctx, userID)
}

// GetUserListByStatusesWithProgress returns the joined anime_list × animes ×
// watch_progress projection used by the workstream hero-spotlight v1.0 Phase 3
// internal endpoint. One round-trip per resolver, LIMIT 200 row cap. Empty
// `statuses` returns a non-nil empty slice with no error — the spotlight
// `not_time_yet` / `continue_watching_new` resolvers occasionally pass an
// empty filter when their card is ineligible and we want a fast 200 + [].
//
// Errors are wrapped with libs/errors so the handler emits a stable
// INTERNAL code and the log line carries `GetUserListByStatusesWithProgress`
// in the message for grep-ability.
func (s *ListService) GetUserListByStatusesWithProgress(
	ctx context.Context,
	userID string,
	statuses []string,
) ([]domain.InternalListItem, error) {
	if len(statuses) == 0 {
		return []domain.InternalListItem{}, nil
	}
	items, err := s.listRepo.GetByUserAndStatusesWithProgress(ctx, userID, statuses)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "GetUserListByStatusesWithProgress: query")
	}
	if items == nil {
		items = []domain.InternalListItem{}
	}
	return items, nil
}

// GetPublicWatchlistPaginated returns user's public watchlist with pagination.
// search filters entries by anime title (name / name_ru / name_jp, case-insensitive). Empty = no filter.
// Enforces the target user's activity_visibility server-side: 'none' returns
// an empty page, 'non_hentai' drops 18+ entries. The output for 'non_hentai'
// must stay indistinguishable from 'all' minus those rows — no hints.
func (s *ListService) GetPublicWatchlistPaginated(ctx context.Context, userID string, statuses []string, search string, filters domain.ListFilters, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	visibility := s.listRepo.GetUserActivityVisibility(ctx, userID)
	if visibility == repo.ActivityVisibilityNone {
		return []*domain.AnimeListEntry{}, 0, nil
	}
	excludeHentai := visibility == repo.ActivityVisibilityNonHentai
	if len(statuses) == 0 {
		return s.listRepo.GetByUserPaginated(ctx, userID, "", search, excludeHentai, filters, params)
	}
	return s.listRepo.GetByUserAndStatusesPaginated(ctx, userID, statuses, search, excludeHentai, filters, params)
}

// GetUserAnimeEntry returns a single anime entry from user's list
func (s *ListService) GetUserAnimeEntry(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	return s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// GetAnimeMALID returns the anime's MAL id from the catalog-owned animes
// table ("" when unknown). Used by the viewer-context aggregate to resolve
// legacy mal_{id} list entries without the frontend supplying the id.
func (s *ListService) GetAnimeMALID(ctx context.Context, animeID string) (string, error) {
	return s.listRepo.GetAnimeMALID(ctx, animeID)
}

// clampRewatchCount bounds a manually-supplied rewatch_count to
// [0, MaxRewatchCount]. Design 2026-06-05.
func clampRewatchCount(n int) int {
	if n < 0 {
		return 0
	}
	if n > domain.MaxRewatchCount {
		return domain.MaxRewatchCount
	}
	return n
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

	// IsRewatching: apply when provided, else preserve existing so a PATCH that
	// omits it can't silently end an in-progress rewatch (which would also skip
	// the finale rewatch_count bump).
	if req.IsRewatching != nil {
		entry.IsRewatching = *req.IsRewatching
	} else if existingEntry != nil {
		entry.IsRewatching = existingEntry.IsRewatching
	}

	// RewatchCount: apply (clamped) when provided, else preserve existing so a
	// PATCH that omits it doesn't clobber the tally to 0. Design 2026-06-05.
	if req.RewatchCount != nil {
		entry.RewatchCount = clampRewatchCount(*req.RewatchCount)
	} else if existingEntry != nil {
		entry.RewatchCount = existingEntry.RewatchCount
	}

	// Manually completing an in-progress rewatch mirrors the IncrementEpisodes
	// finale branch: bump the tally once and clear the flag. Fires only on the
	// non-completed → completed transition; an explicit rewatch_count in the
	// same request is authoritative (imports) and suppresses the auto-bump.
	if req.Status == "completed" && entry.IsRewatching &&
		(existingEntry == nil || existingEntry.Status != "completed") {
		if req.RewatchCount == nil {
			entry.RewatchCount = clampRewatchCount(entry.RewatchCount + 1)
		}
		entry.IsRewatching = false
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

	// Handle CompletedAt - use provided value, preserve existing, or auto-set.
	// newlyCompleted is set true only in the auto-set arm so the gacha hook
	// fires only when this call is the one that first marks the title done.
	var newlyCompleted bool
	if req.CompletedAt != nil {
		entry.CompletedAt = req.CompletedAt
	} else if existingEntry != nil && existingEntry.CompletedAt != nil {
		entry.CompletedAt = existingEntry.CompletedAt
	} else if req.Status == "completed" {
		now := time.Now()
		entry.CompletedAt = &now
		newlyCompleted = true
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

	// Phase 4 (gacha): fire non-blocking title-completed credit when this call
	// is the one that first sets CompletedAt. Nil-safe; gacha deduplicates on
	// (user_id, "title_completed", animeID) so rewatches don't double-pay.
	if newlyCompleted {
		s.gachaCredit.TitleCompleted(userID, req.AnimeID)
	}

	return entry, nil
}

// DeleteListEntry removes an anime from user's list
func (s *ListService) DeleteListEntry(ctx context.Context, userID, animeID string) error {
	return s.listRepo.Delete(ctx, userID, animeID)
}

// Rewatch starts a fresh rewatch cycle for a completed anime. Design 2026-06-05:
// status→'watching', episodes→0, is_rewatching→true, and this anime's
// watch_progress rows reset to completed=false/progress=0 (rows kept;
// watch_history audit trail preserved). rewatch_count is NOT bumped here — it
// increments when the rewatch reaches the finale (watching→completed while
// is_rewatching).
func (s *ListService) Rewatch(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	reset, err := s.listRepo.StartRewatch(ctx, userID, animeID)
	if err != nil {
		return nil, err
	}
	if !reset {
		// Not in the list or not completed — refuse rather than wiping the
		// watch_progress of an in-flight first watch.
		return nil, errors.NotFound("completed list entry to rewatch")
	}
	// Reset per-episode progress so the resume state machine sees a fresh cycle
	// (0 → partial → full). Best-effort: a reset failure must not strand the
	// already-applied list-state change, mirroring MarkEpisodeWatched's
	// best-effort progress writes.
	if err := s.progressRepo.ResetForAnime(ctx, userID, animeID); err != nil {
		s.log.Errorw("failed to reset watch_progress for rewatch",
			"user_id", userID,
			"anime_id", animeID,
			"error", err,
		)
	}
	return s.listRepo.GetByUserAndAnime(ctx, userID, animeID)
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
			// Phase 4 (gacha): fire non-blocking episode-watched credit.
			// Nil-safe; gacha outage never fails this branch.
			s.gachaCredit.EpisodeWatched(userID, animeID, req.Episode)
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
	// Phase 4 (gacha): fire non-blocking episode-watched credit on main path.
	// Nil-safe; gacha outage never fails MarkEpisodeWatched. Gacha deduplicates
	// on (user_id, reason, ref) so firing on every invocation is safe.
	s.gachaCredit.EpisodeWatched(userID, animeID, req.Episode)

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
	}

	// Recs extraction Phase 1 (spec 2026-06-11): one fire-and-forget hint
	// replaces both the in-process debounce trigger and the synchronous S6
	// seed update — the recs service derives the seed from anime_list on
	// receipt. Drop-on-full; never blocks or fails this request.
	//
	// Placement note: the old S6 seed block ran OUTSIDE the `req.Player != ""`
	// branch (it fired on every MarkEpisodeWatched call), while the old
	// userOrchestrator trigger ran INSIDE it. Firing the single hint here (at
	// the broader level) preserves every case in which either old mechanism
	// fired.
	s.recsHint.Hint(userID, animeID)

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

// GetPublicWatchlistStats returns aggregate stats for a user's public watchlist.
// Mirrors GetPublicWatchlistPaginated's activity_visibility enforcement so the
// stats card can't leak what the list itself hides.
func (s *ListService) GetPublicWatchlistStats(ctx context.Context, userID string, statuses []string) (*domain.WatchlistStats, error) {
	visibility := s.listRepo.GetUserActivityVisibility(ctx, userID)
	if visibility == repo.ActivityVisibilityNone {
		return &domain.WatchlistStats{}, nil
	}
	return s.listRepo.GetUserWatchlistStats(ctx, userID, statuses, visibility == repo.ActivityVisibilityNonHentai)
}

// GetWatchersCount returns how many users have the given anime in their list
// with status='watching'. Powers the Phase 14 / UX-28 social-proof badge on
// the anime detail view. Public endpoint, no auth required.
func (s *ListService) GetWatchersCount(ctx context.Context, animeID string) (int64, error) {
	return s.listRepo.CountWatchers(ctx, animeID)
}

// GetPublicWatchlist returns user's public watchlist filtered by allowed statuses
func (s *ListService) GetPublicWatchlist(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	visibility := s.listRepo.GetUserActivityVisibility(ctx, userID)
	if visibility == repo.ActivityVisibilityNone {
		return []*domain.AnimeListEntry{}, nil
	}
	excludeHentai := visibility == repo.ActivityVisibilityNonHentai
	if len(statuses) == 0 {
		return s.listRepo.GetByUser(ctx, userID, excludeHentai)
	}
	return s.listRepo.GetByUserAndStatuses(ctx, userID, statuses, excludeHentai)
}

// GetListFacets returns the filter facets for the caller's own list.
func (s *ListService) GetListFacets(ctx context.Context, userID string) (*domain.ListFacets, error) {
	facets, err := s.listRepo.GetListFacets(ctx, userID, false)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "GetListFacets: query")
	}
	return facets, nil
}

// GetPublicListFacets returns filter facets for a public profile, honoring the
// target user's activity_visibility (none → empty; non_hentai → 18+ excluded).
func (s *ListService) GetPublicListFacets(ctx context.Context, userID string) (*domain.ListFacets, error) {
	visibility := s.listRepo.GetUserActivityVisibility(ctx, userID)
	if visibility == repo.ActivityVisibilityNone {
		return &domain.ListFacets{Genres: []domain.FacetGenre{}, Kinds: []domain.FacetKind{}}, nil
	}
	excludeHentai := visibility == repo.ActivityVisibilityNonHentai
	facets, err := s.listRepo.GetListFacets(ctx, userID, excludeHentai)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "GetPublicListFacets: query")
	}
	return facets, nil
}
