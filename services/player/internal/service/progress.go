package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// progressUpserter is the heartbeat-persistence seam (satisfied by
// *repo.ProgressRepository). Narrowed to an interface so the Logic-B fire path
// can be unit-tested without a DB.
type progressUpserter interface {
	UpsertProgress(ctx context.Context, progress *domain.WatchProgress) error
}

// logicBLookup is the Phase-9 Logic-B gating seam (satisfied by
// *repo.ProgressRepository): one query returning the anime's shikimori_id,
// episodes_aired, and whether the user is actively watching it.
type logicBLookup interface {
	LogicBContext(ctx context.Context, userID, animeID string) (shikimoriID string, episodesAired int, watching bool, err error)
}

// demandFirer is the fire-and-forget autocache-demand seam (satisfied by
// *DemandProducer). Kept narrow so UpdateProgress's Logic-B branch is testable.
type demandFirer interface {
	Want(malID string, episode int, reason string)
}

type ProgressService struct {
	progressRepo *repo.ProgressRepository
	prefService  *PreferenceService
	// upsert is the heartbeat-persistence seam — the same concrete object as
	// progressRepo in production, held as an interface for unit-testability.
	upsert progressUpserter
	// logicB resolves the (shikimori_id, episodes_aired, watching) gating tuple
	// for the Phase-9 Logic-B next_ep demand. Same concrete object as
	// progressRepo today; held as an interface for testability.
	logicB logicBLookup
	// demand is the fire-and-forget player→library autocache demand producer
	// (Phase 9 / TRIG-02). Nil-safe: when nil/disabled, no demand is fired.
	demand demandFirer
	log    *logger.Logger
}

func NewProgressService(progressRepo *repo.ProgressRepository, prefService *PreferenceService, demand *DemandProducer, log *logger.Logger) *ProgressService {
	return &ProgressService{
		progressRepo: progressRepo,
		prefService:  prefService,
		upsert:       progressRepo,
		logicB:       progressRepo,
		demand:       demand,
		log:          log,
	}
}

// prefersRawAudio reports whether a resolved combo wants original Japanese
// audio — i.e. whether watching it should pre-cache the RAW pool (Phase-9
// Logic-B / TRIG-02). ANY sub combo carries original Japanese audio (the
// subtitles overlay on the raw JP video) regardless of the subtitle language or
// the source provider: kodik/ru/sub, english/en/sub (gogoanime), hianime/en/sub
// and consumet/en/sub are all raw-audio. The AE and Raw players are always raw.
// Only DUB combos (replaced audio) are excluded — "gate on RAW and SUB-preferring
// combos, skip DUB entirely". (Corrected 2026-06-17: the original gate keyed on
// player∈{ae,raw}||lang=='ja', which wrongly dropped every sub combo.)
func prefersRawAudio(player, watchType string) bool {
	return watchType == "sub" || player == "ae" || player == "raw"
}

// UpdateProgress updates or creates watch progress (heartbeat saves).
// Does not mark the episode as completed — that is a discrete event written
// via ProgressRepository.MarkCompleted from ListService.MarkEpisodeWatched.
func (s *ProgressService) UpdateProgress(ctx context.Context, userID string, req *domain.UpdateProgressRequest) (*domain.WatchProgress, error) {
	progress := &domain.WatchProgress{
		UserID:        userID,
		AnimeID:       req.AnimeID,
		EpisodeNumber: req.EpisodeNumber,
		Progress:      req.Progress,
		Duration:      req.Duration,
		LastWatchedAt: time.Now(),
	}

	if err := s.upsert.UpsertProgress(ctx, progress); err != nil {
		return nil, err
	}

	// Upsert anime preference if combo fields are present
	if req.Player != "" && s.prefService != nil {
		s.prefService.UpsertAnimePreference(ctx, userID, req)
	}

	// Phase 9 / TRIG-02 — Logic B: pull the NEXT episode (N+1) into the autocache
	// pool ahead of an active JP-audio watcher. This is the first-heartbeat fire
	// point (max lead time, per CONTEXT decision 2). The library demand PK-dedup
	// collapses repeated per-heartbeat fires, so an in-memory guard is
	// unnecessary. Best-effort throughout: a lookup error or a slow/down library
	// must NEVER block or fail the heartbeat — log WARN and move on.
	s.maybeFireNextEpDemand(ctx, userID, req)

	return progress, nil
}

// maybeFireNextEpDemand fires a next_ep autocache demand for episode N+1 iff the
// resolved combo prefers raw audio (any sub combo, or the ae/raw players), the
// anime is status=watching for this user, its shikimori_id is known, and N+1 has
// aired. Never returns/raises — the caller's heartbeat result is unaffected by
// any failure here.
func (s *ProgressService) maybeFireNextEpDemand(ctx context.Context, userID string, req *domain.UpdateProgressRequest) {
	// Cheapest gate first: skip the DB lookup entirely for dub-only combos.
	if !prefersRawAudio(req.Player, req.WatchType) {
		return
	}
	if s.logicB == nil || s.demand == nil {
		return
	}

	shikimoriID, episodesAired, watching, err := s.logicB.LogicBContext(ctx, userID, req.AnimeID)
	if err != nil {
		s.log.Warnw("logic-b: next_ep gating lookup failed; skipping demand",
			"user_id", userID, "anime_id", req.AnimeID, "error", err)
		return
	}
	if !watching || shikimoriID == "" {
		return
	}

	next := req.EpisodeNumber + 1
	if next > episodesAired {
		// N+1 has not aired yet — nothing to pre-download.
		//
		// WR-06 (Phase-09 review) — ACCEPTED as-is: episodesAired is
		// animes.episodes_aired (catalog/Shikimori-sourced). When Shikimori lags
		// the actual airing (common for simulcasts — an episode is on torrents
		// hours before Shikimori bumps the count) this gate is stale-low and Logic
		// B does not fire for the genuinely-aired N+1, so the trigger's freshness
		// is bounded by Shikimori sync latency rather than torrent availability.
		// This is an inherent data-source coupling (not an off-by-one — the bound
		// admits exactly next ∈ [1, episodesAired]); treating episodes_aired as a
		// soft hint with one episode of lookahead is a possible future enhancement.
		return
	}

	// Fire-and-forget; nil-safe on the producer side.
	s.demand.Want(shikimoriID, next, "next_ep")
}

// GetProgress returns watch progress for an anime
func (s *ProgressService) GetProgress(ctx context.Context, userID, animeID string) ([]*domain.WatchProgress, error) {
	return s.progressRepo.GetByUserAndAnime(ctx, userID, animeID)
}

// MarkDropOff records that the user closed the page mid-episode at the given
// playback position (seconds). Phase 5 (G-01).
func (s *ProgressService) MarkDropOff(ctx context.Context, userID, animeID string, req *domain.DropOffRequest) error {
	return s.progressRepo.MarkDropOff(ctx, userID, animeID, req.EpisodeNumber, req.Progress)
}

// ListContinueWatching returns the user's most-recent in-progress episodes,
// one row per anime, ordered by last_watched_at DESC. Phase 8 (UX-15 / UA-061).
func (s *ProgressService) ListContinueWatching(
	ctx context.Context, userID string, limit int,
) ([]*domain.ContinueWatchingItem, error) {
	return s.progressRepo.ListContinueWatching(ctx, userID, limit)
}

// GetBulkProgress returns a map keyed by anime_id with the user's furthest
// episode reached + completion flags. Used by AnimeCardNew (via the
// /users/anime-progress endpoint) to render a per-card progress badge.
// Pure read-through delegate; the repo enforces the empty-input fast-path
// and the JOIN semantics. Phase 9 (UX-16).
func (s *ProgressService) GetBulkProgress(
	ctx context.Context, userID string, animeIDs []string,
) (domain.BulkAnimeProgressMap, error) {
	return s.progressRepo.GetBulkProgress(ctx, userID, animeIDs)
}
