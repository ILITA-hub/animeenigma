// Package animejoy is the catalog-side discovery core for the AnimeJoy RU-sub
// video provider (Sibnet + AllVideo legs). Phase 1 covers DISCOVERY ONLY:
// resolving a title to an animejoy news_id via the DLE search page, and parsing
// the playlist AJAX tree into per-team episodes carrying the Sibnet + AllVideo
// embed URLs. Final-leg (Sibnet/AllVideo → mp4) resolution and all DB / streaming
// / capability wiring are deferred to later phases.
//
// Design note: the pure parsing/scoring functions (parseSearchResults,
// scoreAndPick, parsePlaylist) take []byte and return plain values so they are
// fully unit-testable offline against the captured testdata/ fixtures, with thin
// http wrappers (ResolveNewsID, FetchPlaylist) layered on top — mirroring the
// Kodik catalog parser's shape.
package animejoy

// Episode is a single episode within a team, carrying the two legs we keep
// (Sibnet primary, AllVideo fallback). The fields hold the EMBED URLs as found
// in the playlist (e.g. iv.sibnet.ru/shell.php?videoid=… / fsst.online/embed/…),
// NOT the resolved final mp4 — leg resolution is Phase 2. Either may be empty if
// that player is absent for the episode.
type Episode struct {
	Num      int    // parsed from the "N серия" label (per-series number)
	Sibnet   string // Sibnet embed URL (iv.sibnet.ru/…), or "" if absent
	AllVideo string // AllVideo embed URL (fsst.online/…), or "" if absent
}

// Team is one fansub/voiceover team (a top-level group in the playlist tree).
// AnimeJoy's playlist groups players by a "<group>_<player>" data-id; the group
// index is the team. Most series have a single team.
type Team struct {
	ID       string    // team group index as a string ("0", "1", …)
	Name     string    // team display name when available; "" for the implicit single team
	Episodes []Episode // episodes that carry at least one of Sibnet/AllVideo, ascending by Num
}

// searchHit is one deduped DLE search result row.
type searchHit struct {
	NewsID  string // the numeric news_id ("3647")
	Title   string // the human title from the result card (Russian, may include an alt title)
	Section string // URL section: "tv-serialy", "anime-films", "ova", …
}

// Query is the catalog-side lookup request used by scoreAndPick / ResolveNewsID.
// Titles holds the primary title plus any synonyms (romaji/english/alt). Season
// and Year are best-effort disambiguators; Kind is the catalog kind ("TV",
// "Movie", "OVA", …) used for the section filter.
type Query struct {
	Titles []string
	Year   int
	Season int
	Kind   string
}
