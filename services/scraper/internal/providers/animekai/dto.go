package animekai

// dto.go — placeholder shapes for v3.1 fill-in. The escape-hatch stub does
// not parse any upstream responses, but these types pre-allocate the DTO
// surface so SCRAPER-KAI-01..04 fill-in is a body-only PR.
//
// Shapes are lifted from gogoanime/dto.go (Phase 18 analog). When the v3.1
// implementation lands, the JSON tags and field set may evolve — they are
// only present here for forward-compat.

// searchResult is one result row from AnimeKai's /search page. Slug is the
// path-tail used downstream as the AnimeKai providerID.
type searchResult struct {
	Slug  string
	Title string
}

// episodeRow is one row from the per-anime episode listing page. URLSlug is
// the trailing href path; Number is parsed from the trailing -episode-N
// segment.
type episodeRow struct {
	Number  int
	URLSlug string
	Title   string
}

// serverRow is one embed-host entry inside the per-episode server panel.
// EmbedURL is normalized to an absolute https:// URL at parse time.
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

// malSyncResponse is the top-level shape of
// https://api.malsync.moe/mal/anime/{mal_id}. Sites is keyed by the
// malsync.moe provider slug — for our purposes the key is "AnimeKai" (note
// capitalization). The Sites map is left generic since malsync's response
// shape is provider-agnostic.
type malSyncResponse struct {
	ID    int                                `json:"id"`
	Title string                             `json:"title"`
	Sites map[string]map[string]malSyncEntry `json:"Sites"`
}
