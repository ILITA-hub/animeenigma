package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// EpisodesLookupCacheTTL is the literal 5-minute TTL mandated by
// NOTIF-DET-01. Repeated lookups within the window served from Redis (no
// upstream parser hit). 5 minutes is the design-doc choice — long enough to
// absorb a worst-case detector run, short enough that a snapshot UPDATE
// during manual SC verification falls out of cache within one make-target
// round-trip.
const EpisodesLookupCacheTTL = 5 * time.Minute

// EpisodesLookupResult is the JSON payload the catalog's internal
// /episodes endpoint returns and the value cached in Redis.
//
// TranslationTitle is the per-player display title for the combo's
// translation (Kodik Translation.Title / AnimeLib Team.Name). It rides
// along in the same upstream parser response, so it's free. Optional:
// pre-existing cached entries (5-min TTL) deserialize with an empty title
// and self-heal on the next cache miss.
type EpisodesLookupResult struct {
	LatestAvailableEpisode int       `json:"latest_available_episode"`
	TranslationTitle       string    `json:"translation_title,omitempty"`
	CheckedAt              time.Time `json:"checked_at"`
}

// EpisodesLookupService routes the notifications detector's per-combo
// "latest available episode" question to the right parser (kodik or
// animelib), with a 5-minute Redis cache absorbing repeated lookups.
//
// Phase 2 v1.0 Notifications Engine — NOTIF-DET-01.
// Mounted at GET /internal/anime/{shikimori_id}/episodes by
// services/catalog/internal/handler/internal_episodes.go.
type EpisodesLookupService struct {
	cache          *cache.RedisCache
	kodikClient    *kodik.Client
	animelibClient *animelib.Client
	animeRepo      *repo.AnimeRepository
	catalogService *CatalogService // for ResolveAnimeLibID
	animeLevel     *animeLevelResolver
	log            *logger.Logger
}

// NewEpisodesLookupService constructs the service. The catalogService param
// is required only for AnimeLib lookups (slug → AnimeLib internal numeric
// ID). For Kodik-only deployments the dependency is benign — the kodik path
// never touches it.
func NewEpisodesLookupService(
	c *cache.RedisCache,
	kodikClient *kodik.Client,
	animelibClient *animelib.Client,
	animeRepo *repo.AnimeRepository,
	catalogService *CatalogService,
	rawResolver *RawResolver,
	log *logger.Logger,
) *EpisodesLookupService {
	return &EpisodesLookupService{
		cache:          c,
		kodikClient:    kodikClient,
		animelibClient: animelibClient,
		animeRepo:      animeRepo,
		catalogService: catalogService,
		animeLevel:     &animeLevelResolver{finder: animeRepo, scraper: catalogService, raw: rawResolver},
		log:            log,
	}
}

// LatestAvailable returns the latest episode number available for the given
// (shikimori_id, player, translation_id, watch_type, language) combo,
// served from Redis when available and otherwise resolved via the per-player
// parser adapter.
//
// Cache key (literal NOTIF-DET-01 mandate):
//
//	notifications:episodes:{shikimori_id}:{player}:{translation_id}:{watch_type}
//
// `language` is part of the upstream contract but NOT part of the cache key
// because Kodik + AnimeLib do not branch on it (it's downstream of
// translation_id selection). Included in the route signature for forward
// compatibility with English players added later.
func (s *EpisodesLookupService) LatestAvailable(
	ctx context.Context,
	shikimoriID, player, translationID, watchType, language string,
) (EpisodesLookupResult, error) {
	if shikimoriID == "" {
		return EpisodesLookupResult{}, apperrors.InvalidInput("shikimori_id required")
	}
	animeLevel := isAnimeLevelPlayer(player)

	cacheKey := fmt.Sprintf("notifications:episodes:%s:%s:%s:%s",
		shikimoriID, player, translationID, watchType)

	// Cache hit — return immediately without hitting any parser.
	var cached EpisodesLookupResult
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	var (
		latest           int
		translationTitle string
		err              error
	)

	switch {
	case animeLevel:
		latest, translationTitle, err = s.animeLevel.Latest(ctx, shikimoriID, player, watchType)
	case player == "kodik" && translationID == "":
		latest, translationTitle, err = s.latestKodikAnyTeam(ctx, shikimoriID)
	case player == "animelib" && translationID == "":
		latest, translationTitle, err = s.latestAnimeLibAnyTeam(ctx, shikimoriID)
	case player == "kodik":
		tid, parseErr := strconv.Atoi(translationID)
		if parseErr != nil {
			return EpisodesLookupResult{}, apperrors.InvalidInput(
				fmt.Sprintf("translation_id must be numeric for kodik: %v", parseErr))
		}
		latest, translationTitle, err = s.kodikClient.LatestEpisodeForTranslation(ctx, shikimoriID, tid)
	case player == "animelib":
		if watchType != "sub" && watchType != "dub" {
			return EpisodesLookupResult{}, apperrors.InvalidInput(
				"watch_type must be sub or dub for animelib")
		}
		teamID, parseErr := strconv.Atoi(translationID)
		if parseErr != nil {
			return EpisodesLookupResult{}, apperrors.InvalidInput(
				fmt.Sprintf("translation_id must be numeric for animelib: %v", parseErr))
		}
		latest, translationTitle, err = s.lookupAnimeLib(ctx, shikimoriID, teamID, watchType)
	default:
		return EpisodesLookupResult{}, apperrors.InvalidInput(
			"player not supported by detector in v1.0")
	}

	if err != nil {
		// Distinguish "no episode available for this combo" (NotFound)
		// from infrastructure errors (Internal). Parser adapters return
		// a plain error today — we wrap as NotFound when the error
		// surface is "no match found" semantics.
		if isNotFoundLike(err) {
			return EpisodesLookupResult{}, apperrors.Wrap(err,
				apperrors.CodeNotFound, "no episode available for combo")
		}
		return EpisodesLookupResult{}, apperrors.Wrap(err,
			apperrors.CodeInternal, "latest episode lookup")
	}

	result := EpisodesLookupResult{
		LatestAvailableEpisode: latest,
		TranslationTitle:       translationTitle,
		CheckedAt:              time.Now().UTC(),
	}

	// Cache success only — failed lookups should retry on the next call.
	if cacheErr := s.cache.Set(ctx, cacheKey, result, EpisodesLookupCacheTTL); cacheErr != nil && s.log != nil {
		s.log.Warnw("episodes lookup: cache set failed",
			"key", cacheKey, "error", cacheErr)
	}
	return result, nil
}

// lookupAnimeLib resolves the AnimeLib-side ID for the catalog row and
// delegates to the parser adapter.
func (s *EpisodesLookupService) lookupAnimeLib(
	ctx context.Context,
	shikimoriID string,
	teamID int,
	watchType string,
) (int, string, error) {
	anime, err := s.animeRepo.GetByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, "", fmt.Errorf("anime lookup by shikimori_id %q: %w", shikimoriID, err)
	}
	if anime == nil {
		return 0, "", fmt.Errorf("anime not found for shikimori_id %q", shikimoriID)
	}

	animelibID, err := s.catalogService.ResolveAnimeLibID(ctx, anime)
	if err != nil {
		return 0, "", fmt.Errorf("animelib id resolve: %w", err)
	}

	return s.animelibClient.LatestEpisodeForTeam(ctx, animelibID, teamID, watchType)
}

// latestKodikAnyTeam resolves the anime-level latest episode for an aePlayer
// kodik combo (empty translation_id) — the max across any translation.
func (s *EpisodesLookupService) latestKodikAnyTeam(ctx context.Context, shikimoriID string) (int, string, error) {
	n, err := s.kodikClient.LatestEpisodeAnyTranslation(ctx, shikimoriID)
	if err != nil {
		return 0, "", err
	}
	if n == 0 {
		return 0, "", apperrors.NotFound("no kodik episodes for anime")
	}
	return n, "", nil
}

// latestAnimeLibAnyTeam resolves the anime-level latest episode for an aePlayer
// animelib combo (empty translation_id) — the max across the full episode list.
func (s *EpisodesLookupService) latestAnimeLibAnyTeam(ctx context.Context, shikimoriID string) (int, string, error) {
	anime, err := s.animeRepo.GetByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, "", fmt.Errorf("anime lookup by shikimori_id %q: %w", shikimoriID, err)
	}
	if anime == nil {
		return 0, "", apperrors.NotFound("anime not found")
	}
	animelibID, err := s.catalogService.ResolveAnimeLibID(ctx, anime)
	if err != nil {
		return 0, "", fmt.Errorf("animelib id resolve: %w", err)
	}
	n, err := s.animelibClient.LatestEpisodeAnyTeam(ctx, animelibID)
	if err != nil {
		return 0, "", err
	}
	if n == 0 {
		return 0, "", apperrors.NotFound("no animelib episodes for anime")
	}
	return n, "", nil
}

// isNotFoundLike inspects an error message for the parser adapters'
// "no match" sentinels. Cheap pattern match — both adapters produce stable
// error messages with the substring "no translation" or "no episode".
func isNotFoundLike(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "no translation", "no episode", "not found", "no episodes returned")
}

// containsAny is a tiny helper that avoids pulling in strings just for the
// repeated Contains check.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	if m > n {
		return -1
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}
