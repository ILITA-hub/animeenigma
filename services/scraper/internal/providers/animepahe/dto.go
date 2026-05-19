// Package animepahe implements the AnimePahe scraper provider (domain.Provider).
//
// SCRAPER-PAHE-01 / SCRAPER-PAHE-02 / SCRAPER-PAHE-04 / SCRAPER-NF-02.
// Wave-2 of Phase 16. Builds on:
//
//   - Plan 16-01: BaseHTTPClient.Jar() accessor (DDoS-Guard cookie inspection)
//     and on-disk goldens at services/scraper/testdata/animepahe/.
//   - Plan 16-02: KwikExtractor in services/scraper/internal/embeds — registered
//     on the embeds.Registry so this provider's GetStream() can route kwik.cx
//     URLs via registry.Find(embedURL).
//
// dto.go holds JSON DTOs derived from the goldens and RESEARCH.md "Code
// Examples". DTO field names follow upstream JSON wire shapes (snake_case)
// rather than Go convention so the json tags can be omitted where the field
// names match — and so the DTOs are recognizable when audit-diffed against
// the on-disk fixtures.
//
// DTO shapes verified verbatim against testdata/animepahe/frieren-{search,
// release}.json (Phase 27 D4 / A1+A2) — no struct field changes required
// when the parser migrated to the resolver transport.
package animepahe

// epDTO is one entry inside /api?m=release&id={id}.data[]. The upstream emits
// floats for episode numbers occasionally (e.g. 12.5 specials) — we round at
// map time in Provider.ListEpisodes.
//
// `Filler` may be absent on older episodes; the zero value (0) is the correct
// default ("not a filler episode") per RESEARCH.md Assumption A8.
type epDTO struct {
	Session       string  `json:"session"`
	EpisodeNumber float64 `json:"episode"`
	Title         string  `json:"title"`
	Filler        int     `json:"filler"`
	CreatedAt     string  `json:"created_at"`
}

// releaseResponse is the top-level shape of /api?m=release&id={id}&page={n}.
// `CurrentPage` and `LastPage` drive the pagination loop in Provider.ListEpisodes
// (loop until current_page >= last_page, capped at 50 pages).
type releaseResponse struct {
	Total       int     `json:"total"`
	PerPage     int     `json:"per_page"`
	CurrentPage int     `json:"current_page"`
	LastPage    int     `json:"last_page"`
	From        int     `json:"from"`
	To          int     `json:"to"`
	Data        []epDTO `json:"data"`
}

// searchEntry is one row from /api?m=search&q=... — the fuzzy-fallback path
// when malsync has no record of the (mal_id → animepahe_id) mapping.
type searchEntry struct {
	ID       int    `json:"id"`
	Session  string `json:"session"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Episodes int    `json:"episodes"`
	Status   string `json:"status"`
	Season   string `json:"season"`
	Year     int    `json:"year"`
	Score    float64 `json:"score"`
	Poster   string `json:"poster"`
}

// searchResponse is the top-level shape of /api?m=search&q=...
type searchResponse struct {
	Total       int           `json:"total"`
	PerPage     int           `json:"per_page"`
	CurrentPage int           `json:"current_page"`
	LastPage    int           `json:"last_page"`
	Data        []searchEntry `json:"data"`
}

// malSyncEntry is one provider mapping inside the Sites["animepahe"] map.
// Identifier may arrive as a string (typical) or a number (rare) — handle both
// in MalSyncClient.Lookup by stringifying via fmt.Sprintf("%v", ...).
type malSyncEntry struct {
	Identifier any    `json:"identifier"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// malSyncResponse is the top-level shape of https://api.malsync.moe/mal/anime/{mal_id}.
// Sites is keyed by lowercase provider slug ("animepahe", "zoro", ...).
// Each provider can list multiple entries (sub/dub variants) — we pick the
// first entry for the animepahe key.
type malSyncResponse struct {
	ID    int                                `json:"id"`
	Title string                             `json:"title"`
	Sites map[string]map[string]malSyncEntry `json:"Sites"`
}
