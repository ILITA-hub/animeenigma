package service

import (
	"encoding/json"
	"fmt"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
)

// BuildNewEpisodePayload marshals a domain.NewEpisodePayload (the JSON
// shape stored in UserNotification.Payload for type=new_episode) from the
// detector's per-combo inputs.
//
// Inputs:
//   - combo            — the (anime, player, language, watch_type,
//     translation_id) key shared with the dedupe key.
//   - anime            — read-only AnimeView projection (Russian-first title
//     selection, poster URL).
//   - maxWatched       — user's highest episode_number already watched on
//     this combo.
//   - latestAvail      — parser's reported latest episode number (the
//     value that drove the detector's diff).
//   - translationTitle — optional per-player display title for the
//     translation (Kodik Translation.Title / AnimeLib Team.Name), resolved
//     by catalog's /internal/episodes lookup in the same parser call as the
//     episode count. May be empty (e.g. served from a pre-upgrade cache
//     entry) — frontend NotificationCard treats the field as optional.
//
// Returns the marshaled JSON bytes ready to feed straight into
// NotificationService.Upsert.payload.
func BuildNewEpisodePayload(
	combo domain.Combo,
	anime *repo.AnimeView,
	maxWatched, latestAvail int,
	translationTitle string,
) ([]byte, error) {
	if anime == nil {
		return nil, apperrors.InvalidInput("anime view required")
	}

	title := anime.NameRU
	if title == "" {
		// Russian-first per project convention; fall back to original.
		title = anime.Name
	}

	payload := domain.NewEpisodePayload{
		AnimeID:                anime.ID,
		ShikimoriID:            anime.ShikimoriID,
		AnimeTitle:             title,
		AnimePosterURL:         anime.PosterURL,
		FirstUnwatchedEpisode:  maxWatched + 1,
		LatestAvailableEpisode: latestAvail,
		Player:                 combo.Player,
		Language:               combo.Language,
		WatchType:              combo.WatchType,
		TranslationID:          combo.TranslationID,
		TranslationTitle:       translationTitle,
		WatchURL:               BuildWatchURL(anime.ID),
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "marshal new_episode payload")
	}
	return out, nil
}

// BuildWatchURL formats the new-episode link consumed by the frontend
// NotificationCard / store. It is deliberately a bare anime-page link with NO
// query params: the old `?provider&team&episode` deep-link baked in a stale
// `episode = maxWatched+1` (computed at notification-creation time) that the
// frontend treated as a HARD override of its live resume state — so it kept
// landing users on the wrong episode. Dropping the params lets the anime page's
// unified watchState auto-select the correct episode on load (a caught-up
// viewer lands on the newest episode naturally):
//
//	/anime/{anime_id}
func BuildWatchURL(animeID string) string {
	return fmt.Sprintf("/anime/%s", animeID)
}
