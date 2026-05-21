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
//   - translationTitle — optional title for the translation (empty in v1.0;
//     populated in a future patch — see TODO below).
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
		// TODO(v1.0.x): populate translation_title via a per-player title
		// resolver. The detector does NOT have cheap access to it today —
		// kodik exposes it on Translation.Title, animelib on Team.Name, but
		// both require an extra parser hit per combo. Frontend NotificationCard
		// already handles this field being optional.
		TranslationTitle: translationTitle,
		WatchURL:         BuildWatchURL(anime.ID, combo.Player, maxWatched+1, combo.TranslationID),
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "marshal new_episode payload")
	}
	return out, nil
}

// BuildWatchURL formats the deep-link URL pattern from the design doc:
//
//	/anime/{anime_id}/watch?player={player}&episode={ep}&translation={translation_id}
//
// The Phase 3 frontend NotificationCard consumes this URL verbatim.
func BuildWatchURL(animeID, player string, episode int, translationID string) string {
	return fmt.Sprintf("/anime/%s/watch?player=%s&episode=%d&translation=%s",
		animeID, player, episode, translationID)
}
