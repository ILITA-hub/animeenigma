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
// translation-omitted (player-change) mode. The production
// *repo.AnimeRepository satisfies it structurally.
type animeRepoAdapter interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
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
		return s.validatePermissive(episodeID), nil
	}
}

// validateKodikOrAnimeLib runs the strict kodik|animelib path: catalog
// row exists + EpisodesLookupService confirms episode_id within the
// available range.
func (s *EpisodesValidateService) validateKodikOrAnimeLib(
	ctx context.Context,
	shikimoriID, player, episodeID, translationID, watchType string,
) (ValidateResult, error) {
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

// validatePermissive is the v1.0 ourenglish/hanime/raw branch. We
// only check episode_id is non-empty; no upstream probe.
//
// TODO(v1.1): tighten validation by calling the scraper microservice's
// /episodes endpoint (ourenglish) and the AllAnime / Hanime parser
// episode listings. Skipped in v1.0 per D-04 "smallest change" — see
// the workstream §Claude's Discretion paragraph for the rationale.
func (s *EpisodesValidateService) validatePermissive(episodeID string) ValidateResult {
	if episodeID == "" {
		return ValidateResult{Valid: false, Reason: ReasonEpisodeUnavailable}
	}
	return ValidateResult{Valid: true}
}
