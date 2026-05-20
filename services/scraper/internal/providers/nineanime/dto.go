package nineanime

// dto.go — Internal DTOs for the WordPress REST API search response.
// Per 28-RESEARCH.md Code Examples + Pitfall 4 (default ?s= search broken).
//
// Shape observed during 2026-05-20 live recon against ?search=frieren:
//
//	[
//	  {
//	    "id": 9314,
//	    "title": "Frieren: Beyond Journey's End Season 2",
//	    "url": "https://9anime.me.uk/series/frieren-beyond-journeys-end-season-2/",
//	    "type": "post",
//	    "subtype": "series"
//	  },
//	  ...
//	]
//
// We filter `subtype == "series"` client-side to drop episode-stub and
// page-stub noise (Pitfall 4 — the WP default `?s=` returns 19 irrelevant
// episode-7 stubs).

// wpSearchResult is one element of the WP REST search JSON array
// `/wp-json/wp/v2/search?search=<term>&per_page=20`.
type wpSearchResult struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Type    string `json:"type"`    // always "post" for the dramastream theme
	Subtype string `json:"subtype"` // "series" | "post" | "page" — filter to "series"
}

// episodeRef is the minimal shape persisted to the episodes cache. We
// store the canonical episode `href` from the series page (per Pitfall 5
// — irregular slugs, never reconstruct) along with the parsed episode
// number for sort-key.
type episodeRef struct {
	URL    string `json:"url"`    // full canonical episode URL (the Episode.ID)
	Number int    `json:"number"` // parsed `data-number` attr (sort key)
	Title  string `json:"title,omitempty"`
}
