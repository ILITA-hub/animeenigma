package miruro

// Upstream JSON shapes for the three Miruro endpoints we consume. Field
// names mirror what the SPA receives — see SPIKE-MIRURO.md §"Live Integration
// Probe" for the captured payload shapes (Frieren AniList 154587, 2026-05-20).
//
// We capture only the fields we actually consume; the upstream payloads
// carry far more metadata (covers, descriptions, character lists, etc.) that
// we don't surface.

// infoResponse models the decoded body of `info/<aniListID>`. We only
// extract what FindID / ListEpisodes need to cross-validate the ID; the
// authoritative per-episode listing is the `episodes` endpoint below.
//
// Source: SPIKE-MIRURO.md §"Live Integration Probe", Gate 2 sample.
type infoResponse struct {
	Media struct {
		ID    int `json:"id"`    // AniList ID — should match the request
		IDMal int `json:"idMal"` // MAL ID == Shikimori ID
		Title struct {
			Native  string `json:"native"`
			Romaji  string `json:"romaji"`
			English string `json:"english"`
		} `json:"title"`
	} `json:"media"`
}

// episodesResponse models the decoded body of `episodes?anilistId=<id>`.
// Top level is `providers` keyed by provider name (e.g. "dune", "kiwi",
// "hop", "bee", "ANIMEKAI"). Each provider exposes its episode arrays
// bucketed by audio category ("sub", "dub").
//
// Source: SPIKE-MIRURO.md §"Live Integration Probe", episodes_154587 run.
type episodesResponse struct {
	Providers map[string]providerEpisodeBlock `json:"providers"`
}

// providerEpisodeBlock is one inner provider's episode listing on the
// episodes endpoint.
type providerEpisodeBlock struct {
	Episodes map[string][]rawEpisode `json:"episodes"`
}

// rawEpisode is one episode entry. ID is the opaque upstream identifier
// — base64-looking string the SPA passes back verbatim on the sources
// follow-up call. Number is a 1-indexed episode number.
//
// Per the spike's Gate 4 sample (kiwi block, ep 1):
//
//	{"id":"YW5pbWVwYWhlOjUzMTk6NjAwNTk6MQ","number":1,
//	 "title":"Shall We Go, Then?","airDate":"2026-01-16",
//	 "duration":1561,"audio":"sub","filler":false,...}
type rawEpisode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Filler   bool   `json:"filler"`
	Audio    string `json:"audio"`
	AirDate  string `json:"airDate"`
	Duration int    `json:"duration"`
}

// sourcesResponse models the decoded body of
// `sources?episodeId=<eid>&provider=<p>[&category=<c>]`. Each stream
// entry typically carries a direct HLS m3u8 URL (1080p) plus an optional
// Kwik embed mirror; we prefer the direct URL.
//
// Source: SPIKE-MIRURO.md §Gate 2 sample for kiwi/ep1.
type sourcesResponse struct {
	Streams []rawStream `json:"streams"`
	// Embeds is occasionally populated when the inner provider doesn't
	// expose a direct stream URL. We don't currently follow these
	// (Kwik / hianime / etc. extractors live in the catalog parser
	// today, not this scraper provider).
	Embeds []rawEmbed `json:"embeds,omitempty"`
}

// rawStream is one playable URL exposed under sources.streams[]. Type
// is "hls" or "mp4"; Quality is the upstream's label ("1080p" / "auto").
// Referer is the header the CDN expects to whitelist requests.
type rawStream struct {
	URL     string `json:"url"`
	Type    string `json:"type"`    // "hls" / "mp4"
	Quality string `json:"quality"` // "1080p" / "720p" / "auto"
	Referer string `json:"referer,omitempty"`
}

// rawEmbed describes an indirect player URL. Currently unused — present
// for forward-compat with provider blocks that don't ship direct streams.
type rawEmbed struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

// rawSubtitle is one external subtitle track attached to a stream. Some
// inner providers (Crunchyroll-shaped) embed soft-subs alongside the
// HLS URL.
type rawSubtitle struct {
	URL  string `json:"url"`
	Lang string `json:"lang"`
}
