package service

import "time"

// Next-episode date provenance values stored in domain.Anime.NextEpisodeSource.
const (
	sourceShikimori = "shikimori"
	sourceAniList   = "anilist"
)

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
