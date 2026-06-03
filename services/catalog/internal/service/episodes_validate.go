package service

// Watch-Together workstream / Phase 04 (state switching) — WT-STATE-02.
//
// EpisodesValidateService backs the catalog-side validation surface
// consumed by services/watch-together/'s state:change_* inbound
// handlers. Returns {Valid bool, Reason string} for a (anime, player,
// translation, episode) tuple so the WT service can decide whether to
// broadcast room:state_changed or send EPISODE_UNAVAILABLE /
// PLAYER_UNAVAILABLE / TRANSLATION_UNAVAILABLE to the sender only.
//
// Mounted at GET /internal/anime/{shikimoriId}/episodes/validate by
// services/catalog/internal/handler/internal_episodes_validate.go.
//
// Design references:
//   - .planning/workstreams/watch-together/phases/04-state-switching/04.1-PLAN.md
//   - .planning/workstreams/watch-together/phases/04-state-switching/04-CONTEXT.md
//
// D-04 "smallest change": kodik/animelib delegate to the existing
// EpisodesLookupService.LatestAvailable so we reuse the parser + 5-min
// Redis cache that NOTIF-DET-01 already pays for. The remaining three
// players (ourenglish/hanime/raw) ship permissive v1.0 validation —
// tightening is deferred to v1.1 (see TODO inline).

import (
	"context"
	"strconv"
	"strings"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// ValidateResult is the JSON payload returned by /episodes/validate.
// Reason is empty on Valid=true; otherwise one of:
//   - "EPISODE_UNAVAILABLE"      — episode_id does not exist for combo
//   - "PLAYER_UNAVAILABLE"       — anime has no episodes on this player
//   - "TRANSLATION_UNAVAILABLE"  — translation_id invalid for player+anime
type ValidateResult struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`
}

// Validation reason codes (match the WT inbound error codes from
// 04-CONTEXT.md so the WT service can pass-through without translation).
const (
	ReasonEpisodeUnavailable     = "EPISODE_UNAVAILABLE"
	ReasonPlayerUnavailable      = "PLAYER_UNAVAILABLE"
	ReasonTranslationUnavailable = "TRANSLATION_UNAVAILABLE"
)

// validPlayers is the closed set of v1.0 players. Mirrors CLAUDE.md
// §Video Player Architecture. Unknown player → 400 InvalidInput.
var validPlayers = map[string]struct{}{
	"kodik":      {},
	"animelib":   {},
	"ourenglish": {},
	"hanime":     {},
	"raw":        {},
}

// IsValidPlayer reports whether p is one of the 5 known players.
// Exported for handler-side input validation symmetry.
func IsValidPlayer(p string) bool {
	_, ok := validPlayers[p]
	return ok
}

// episodesLookupAdapter is the narrow surface EpisodesValidateService
// needs from EpisodesLookupService. The real *EpisodesLookupService
// satisfies it structurally — tests inject a closure-backed fake.
type episodesLookupAdapter interface {
	LatestAvailable(
		ctx context.Context,
		shikimoriID, player, translationID, watchType, language string,
	) (EpisodesLookupResult, error)
}

// animeRepoAdapter is the narrow surface needed for the
// translation-omitted (player-change) mode and for resolving a
// Watch-Together-supplied catalog UUID to its shikimori_id. The
// production *repo.AnimeRepository satisfies it structurally.
type animeRepoAdapter interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
}

// EpisodesValidateService is a pure-Go validation engine. No HTTP, no
// JSON — that lives in the handler.
type EpisodesValidateService struct {
	lookup    episodesLookupAdapter
	animeRepo animeRepoAdapter
	log       *logger.Logger
}

// NewEpisodesValidateService constructs the service. lookup may be a
// real *EpisodesLookupService; animeRepo a real *repo.AnimeRepository.
// Both are taken as narrow interfaces to keep tests light.
func NewEpisodesValidateService(
	lookup episodesLookupAdapter,
	animeRepo animeRepoAdapter,
	log *logger.Logger,
) *EpisodesValidateService {
	return &EpisodesValidateService{
		lookup:    lookup,
		animeRepo: animeRepo,
		log:       log,
	}
}

// ValidateEpisode returns whether the given (shikimoriID, player,
// episodeID, translationID, watchType) combo is playable.
//
// Modes:
//
//  1. Translation-omitted (translationID == "") — used by
//     state:change_player. Validates that the player is supported and
//     for kodik/animelib that an anime row exists. The permissive trio
//     (ourenglish/hanime/raw) returns Valid=true unconditionally.
//
//  2. Full validation (translationID != "") — used by
//     state:change_episode and state:change_translation. For
//     kodik/animelib this consults EpisodesLookupService and asserts
//     episodeID is numeric, > 0, and ≤ LatestAvailableEpisode. For
//     ourenglish/hanime/raw it only checks that episodeID is non-empty
//     (TODO v1.1 below).
//
// Error vs. ValidateResult contract:
//   - apperrors.InvalidInput — caller bug (missing/unknown input).
//     Maps to HTTP 400 via httputil.Error.
//   - apperrors.Internal — infrastructure failure (DB down etc.).
//   - ValidateResult{Valid:false, Reason:...}, nil — successful
//     negative answer. The HTTP layer returns 200; the WT service
//     turns Reason into a sender-only error frame.
func (s *EpisodesValidateService) ValidateEpisode(
	ctx context.Context,
	shikimoriID, player, episodeID, translationID, watchType string,
) (ValidateResult, error) {
	if shikimoriID == "" {
		return ValidateResult{}, apperrors.InvalidInput("shikimori_id required")
	}
	if !IsValidPlayer(player) {
		return ValidateResult{}, apperrors.InvalidInput("player not supported")
	}

	switch player {
	case "kodik", "animelib":
		return s.validateKodikOrAnimeLib(ctx, shikimoriID, player, episodeID, translationID, watchType)
	default:
		// TODO(v1.1): tighten validation via scraper/parser
		// episode-listing methods for ourenglish, hanime, and raw.
		// Phase 4 ships permissive validation per D-04 "smallest
		// change" + the workstream §Claude's Discretion note.
		return s.validatePermissive(episodeID, translationID), nil
	}
}

// resolveShikimoriID maps the incoming anime identifier to the canonical
// shikimori_id the Kodik/AnimeLib parsers key off.
//
// Watch Together creates rooms with the internal catalog UUID as anime_id
// (InviteButton passes anime.id) and forwards that UUID here as the
// {shikimoriId} path segment. The Kodik/AnimeLib lookups, however, query
// upstream by the real shikimori_id — so a raw UUID never matched and
// every in-room dub/episode/player switch was rejected as
// TRANSLATION_UNAVAILABLE / PLAYER_UNAVAILABLE (ISS-024). This mirrors the
// resolution CatalogService.GetKodikTranslations already does for the
// player's own /anime/{id}/kodik/translations route.
//
// A raw shikimori_id (digits only — what the unit tests and any direct
// caller pass) is used as-is. A UUID (contains a hyphen; shikimori_ids
// never do) is resolved via GetByID. Returns "" when the UUID matches no
// anime row, so the caller can short-circuit to a clean negative answer
// instead of a misleading parser miss.
func (s *EpisodesValidateService) resolveShikimoriID(ctx context.Context, idOrShikimori string) (string, error) {
	if !strings.Contains(idOrShikimori, "-") {
		return idOrShikimori, nil
	}
	anime, err := s.animeRepo.GetByID(ctx, idOrShikimori)
	if err != nil {
		// GetByID returns apperrors.NotFound on a miss — treat an unknown
		// UUID as "no such anime" (empty result), not infrastructure.
		if appErr, ok := apperrors.IsAppError(err); ok && appErr.Code == apperrors.CodeNotFound {
			return "", nil
		}
		return "", apperrors.Wrap(err, apperrors.CodeInternal, "resolve anime uuid")
	}
	if anime == nil || anime.ShikimoriID == "" {
		return "", nil
	}
	return anime.ShikimoriID, nil
}

// validateKodikOrAnimeLib runs the strict kodik|animelib path: catalog
// row exists + EpisodesLookupService confirms episode_id within the
// available range.
func (s *EpisodesValidateService) validateKodikOrAnimeLib(
	ctx context.Context,
	shikimoriID, player, episodeID, translationID, watchType string,
) (ValidateResult, error) {
	// Resolve a Watch-Together-supplied catalog UUID to the real
	// shikimori_id before any parser lookup. An unknown/unresolvable id
	// short-circuits to PLAYER_UNAVAILABLE (the anime simply isn't
	// playable on this surface).
	resolved, err := s.resolveShikimoriID(ctx, shikimoriID)
	if err != nil {
		return ValidateResult{}, err
	}
	if resolved == "" {
		return ValidateResult{Valid: false, Reason: ReasonPlayerUnavailable}, nil
	}
	shikimoriID = resolved

	// Translation-omitted (player-change) mode: only verify the anime
	// row exists. We do NOT call LatestAvailable because that path
	// requires translation_id (it returns InvalidInput otherwise).
	if translationID == "" {
		anime, err := s.animeRepo.GetByShikimoriID(ctx, shikimoriID)
		if err != nil {
			return ValidateResult{}, apperrors.Wrap(err,
				apperrors.CodeInternal, "anime repo lookup")
		}
		if anime == nil {
			return ValidateResult{Valid: false, Reason: ReasonPlayerUnavailable}, nil
		}
		// Anime exists; the player is structurally supported.
		// Phase 4 cannot probe upstream availability without a
		// translation, so we trust this and return Valid=true. The
		// upcoming change_episode call after a successful
		// change_player will catch unplayable combos.
		return ValidateResult{Valid: true}, nil
	}

	// Full validation: hit the underlying parser via the lookup
	// service. The 5-min Redis cache absorbs rapid-fire validations
	// when a user mashes the episode picker.
	result, err := s.lookup.LatestAvailable(ctx, shikimoriID, player, translationID, watchType, "")
	if err != nil {
		appErr, isApp := apperrors.IsAppError(err)
		if isApp {
			switch appErr.Code {
			case apperrors.CodeNotFound:
				// "no episode available for combo" — translation
				// is structurally invalid for this anime+player.
				return ValidateResult{Valid: false, Reason: ReasonTranslationUnavailable}, nil
			case apperrors.CodeInvalidInput:
				// Bubble up — caller passed e.g. non-numeric
				// translation_id and we can't soften this into a
				// false-positive valid=false.
				return ValidateResult{}, appErr
			}
		}
		// Anything else is infrastructure — bubble up as Internal.
		return ValidateResult{}, err
	}

	// Episode-id arithmetic. Empty / non-numeric / out-of-range
	// collapses to EPISODE_UNAVAILABLE (NOT an error — the WT
	// service expects a 200 with valid=false here).
	if episodeID == "" {
		return ValidateResult{Valid: false, Reason: ReasonEpisodeUnavailable}, nil
	}
	n, err := strconv.Atoi(episodeID)
	if err != nil || n <= 0 || n > result.LatestAvailableEpisode {
		return ValidateResult{Valid: false, Reason: ReasonEpisodeUnavailable}, nil
	}
	return ValidateResult{Valid: true}, nil
}

// validatePermissive is the v1.0 ourenglish/hanime/raw branch. No
// upstream probe — we only sanity-check the request shape.
//
// Three WT call shapes reach here (see the inbound state:* handlers):
//   - change_episode     → episode set, translation = room's (maybe "")
//   - change_translation → episode = room's (set), translation set
//   - change_player      → BOTH episode_id and translation_id empty
//
// change_player's "both empty" shape is the player-change mode: a valid
// "switch to this player" request whose real first episode the frontend
// resolves on mount. We MUST accept it (mirrors the kodik/animelib
// translation-omitted branch) — rejecting it broke every in-room switch
// to OurEnglish/Hanime/Raw with a bogus EPISODE_UNAVAILABLE (ISS-025).
// Full mode still requires a non-empty episode_id.
//
// TODO(v1.1): tighten full-mode validation by calling the scraper
// microservice's /episodes endpoint (ourenglish) and the AllAnime /
// Hanime parser episode listings. Skipped in v1.0 per D-04 "smallest
// change" — see the workstream §Claude's Discretion paragraph.
func (s *EpisodesValidateService) validatePermissive(episodeID, translationID string) ValidateResult {
	if episodeID == "" && translationID == "" {
		// Player-change mode — permissively accept the switch.
		return ValidateResult{Valid: true}
	}
	if episodeID == "" {
		return ValidateResult{Valid: false, Reason: ReasonEpisodeUnavailable}
	}
	return ValidateResult{Valid: true}
}
