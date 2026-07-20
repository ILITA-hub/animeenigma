package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// Next-episode date provenance values stored in domain.Anime.NextEpisodeSource.
const (
	sourceShikimori = "shikimori"
	sourceAniList   = "anilist"

	// AniList currently permits 30 requests per minute. Keep a small margin
	// above the exact two-second interval so a rolling rate-limit window cannot
	// reject the tail of a calendar sync. The scheduler allows ten minutes for
	// this job, so reconciling the roughly 100 weekly entries remains safe.
	defaultAniListReconcilePacing = 2100 * time.Millisecond
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
// leaves the Shikimori value untouched. Calls are paced below AniList's public
// 30 req/min limit and abort promptly on context cancellation.
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
		// Spec scope: corroborate ONLY ongoing anime. The Shikimori calendar
		// also lists announced ("anons") titles; later-wins is justified only
		// for ongoing shows (Shikimori under-reports their dates across
		// broadcast hiatuses), so skip the rest — no fetch, no metric — keeping
		// their Shikimori value.
		if info.status != "ongoing" {
			continue
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

// defendAniListNextEpisode preserves an AniList-corroborated next-episode date on
// `fresh` (the Shikimori-rebuilt row from the nightly batch refresh) when the
// stored `existing` row was AniList-sourced and still holds the later date.
// Same later-wins rule as the calendar reconciler: Shikimori only wins if it now
// reports an even-later date (the show resumed and slipped further), in which
// case the source reverts to shikimori. Always stamps a provenance value on
// `fresh` so the full-row Update never writes an empty source.
func defendAniListNextEpisode(fresh, existing *domain.Anime) {
	if fresh.NextEpisodeSource == "" {
		fresh.NextEpisodeSource = sourceShikimori
	}
	if existing.NextEpisodeSource == sourceAniList && existing.NextEpisodeAt != nil &&
		(fresh.NextEpisodeAt == nil || existing.NextEpisodeAt.After(*fresh.NextEpisodeAt)) {
		fresh.NextEpisodeAt = existing.NextEpisodeAt
		fresh.NextEpisodeSource = existing.NextEpisodeSource
	}
}
