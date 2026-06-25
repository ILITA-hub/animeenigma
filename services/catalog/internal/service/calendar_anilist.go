package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// Next-episode date provenance values stored in domain.Anime.NextEpisodeSource.
const (
	sourceShikimori = "shikimori"
	sourceAniList   = "anilist"
)

// AniListAiringFetcher is the slice of *idmapping.Client the calendar reconciler
// needs. Declared as an interface so tests supply a handwritten fake.
type AniListAiringFetcher interface {
	AniListAiringByMALID(ctx context.Context, malID string) (*idmapping.AniListAiring, error)
}

// laterWins picks the later of the Shikimori and AniList next-episode times,
// treating nil as "earliest". It returns the chosen time and whether AniList
// won. AniList is adopted only when it is strictly after Shikimori's date (or
// when Shikimori has no date at all): Shikimori's failure mode is reporting a
// date that is too EARLY (it ignores broadcast hiatuses), so only a later
// AniList date carries new information.
func laterWins(shikimori, anilist *time.Time) (chosen *time.Time, fromAniList bool) {
	if anilist == nil {
		return shikimori, false
	}
	if shikimori == nil || anilist.After(*shikimori) {
		return anilist, true
	}
	return shikimori, false
}

// reconcileCalendarWithAniList corroborates each calendar anime's Shikimori
// next-episode time against AniList's broadcaster schedule, adopting AniList's
// date only when it is strictly later (later-wins). It mutates seen in place
// (nextEpisodeAt + source) and never returns an error — any AniList failure
// leaves the Shikimori value untouched. Calls are paced (~2 req/s) and abort
// promptly on context cancellation.
func (s *CatalogService) reconcileCalendarWithAniList(ctx context.Context, seen map[string]*calendarInfo) {
	if s.aniListAiring == nil {
		return
	}
	first := true
	for _, info := range seen {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !first && s.aniListReconcilePacing > 0 {
			time.Sleep(s.aniListReconcilePacing)
		}
		first = false

		airing, err := s.aniListAiring.AniListAiringByMALID(ctx, info.shikimoriID)
		if err != nil {
			s.log.Debugw("anilist airing lookup failed, keeping shikimori date",
				"shikimori_id", info.shikimoriID, "error", err)
			info.source = sourceShikimori
			metrics.NextEpisodeSourceTotal.WithLabelValues(sourceShikimori).Inc()
			continue
		}

		var aniListAt *time.Time
		if airing != nil {
			aniListAt = airing.NextAiringAt
		}
		chosen, fromAniList := laterWins(info.nextEpisodeAt, aniListAt)
		info.nextEpisodeAt = chosen
		if fromAniList {
			info.source = sourceAniList
			s.log.Infow("adopted anilist next-episode date (later than shikimori)",
				"shikimori_id", info.shikimoriID,
				"anilist_episode", airing.NextEpisode,
				"next_episode_at", chosen)
			metrics.NextEpisodeSourceTotal.WithLabelValues(sourceAniList).Inc()
		} else {
			info.source = sourceShikimori
			metrics.NextEpisodeSourceTotal.WithLabelValues(sourceShikimori).Inc()
		}
	}
}
