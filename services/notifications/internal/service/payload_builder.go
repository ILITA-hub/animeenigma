package service

import (
	"encoding/json"
	"fmt"
	"net/url"

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
		WatchURL:               BuildWatchURL(anime.ID, combo.Player, maxWatched+1, translationTitle),
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "marshal new_episode payload")
	}
	return out, nil
}

// BuildWatchURL formats the new-episode deep-link consumed by the frontend
// NotificationCard / store. aePlayer reads `provider` (its source id) and
// `team` (the team TITLE, e.g. a Kodik translation title) to preselect the
// source on mount; `episode` lands the user on the new episode:
//
//	/anime/{anime_id}/watch?provider={provider}&team={team}&episode={ep}
func BuildWatchURL(animeID, provider string, episode int, team string) string {
	return fmt.Sprintf("/anime/%s/watch?provider=%s&team=%s&episode=%d",
		animeID, url.QueryEscape(provider), url.QueryEscape(team), episode)
}
