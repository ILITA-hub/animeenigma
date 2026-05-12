package gogoanime

// dto.go — DTOs for the Gogoanime/Anitaku scraper provider.
//
// HTML-driven shapes (search, category, episode pages) come from the
// anitaku.to goldens captured in services/scraper/testdata/gogoanime/ via the
// Phase 18 Plan 18-01 capture script. The JSON-driven shapes mirror the
// api.malsync.moe payload (forward-compat probe — malsync currently has no
// Gogoanime/Anitaku key, see malsync_no_gogo.json).

// searchResult is one <p class="name"><a href="/category/<slug>"> entry from
// /search.html?keyword=<title>. Slug is the path-tail after /category/, used
// downstream as the gogoanime providerID.
type searchResult struct {
	Slug  string
	Title string
}

// episodeRow is one <a href="/<slug>-episode-N"> entry on the /category/<slug>
// page. URLSlug is the trailing href path: "one-piece-episode-1" or
// "one-piece-dub-episode-1". Number is parsed from the trailing -episode-N
// segment.
type episodeRow struct {
	Number  int
	URLSlug string
	Title   string
}

// serverRow is one <li class="server"><a data-video> entry inside the
// <div class="anime_muti_link"> block on /<slug>-episode-N. Name is derived
// from the visible label (HD-1, HD-2, StreamHG, Earnvids); EmbedURL is the
// raw data-video attribute value, normalized to an absolute https:// URL.
type serverRow struct {
	Name     string
	EmbedURL string
}

// malSyncEntry is one provider mapping inside Sites[<provider>] in the
// api.malsync.moe response. Identifier may arrive as a string (typical) or
// a number (rare) — handle both at extract time via fmt.Sprintf("%v", ...).
type malSyncEntry struct {
	Identifier any    `json:"identifier"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// malSyncResponse is the top-level shape of https://api.malsync.moe/mal/anime/{mal_id}.
// Sites is keyed by the malsync.moe provider slug — for our purposes
// "Gogoanime" (note capitalization, per RESEARCH.md Sources). Each provider
// can list multiple entries (sub/dub variants); we pick the first non-empty
// identifier deterministically (sorted-key iteration in malsync.go).
type malSyncResponse struct {
	ID    int                                `json:"id"`
	Title string                             `json:"title"`
	Sites map[string]map[string]malSyncEntry `json:"Sites"`
}
