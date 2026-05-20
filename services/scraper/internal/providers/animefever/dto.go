package animefever

// dto.go — Internal DTOs for AnimeFever's `/ajax/anime/load_episodes_v2`
// JSON response. Per 28-RESEARCH.md Code Examples / AnimeFever AJAX iframe
// extraction.
//
// Shape observed during 2026-05-20 live recon against Frieren ep28:
//
//	{
//	  "status": true,
//	  "value": "<iframe src=\"https://am.vidstream.vip/...\" ...></iframe>",
//	  "embed": true,
//	  // additional fields may appear; we only consume the three above.
//	}
//
// When status=false or embed=false, the upstream is signalling that the
// `ctk` token is stale or the requested server has no embed for this
// episode. Caller MUST handle this as a recoverable extract failure
// (WrapExtractFailed) so the orchestrator falls through to the next server.

// ajaxLoadEpisodeResponse is the shape returned by POST
// /ajax/anime/load_episodes_v2?s=<server>.
type ajaxLoadEpisodeResponse struct {
	Status bool   `json:"status"`
	Value  string `json:"value"` // HTML fragment containing the iframe tag
	Embed  bool   `json:"embed"`
}

// episodeRef is the minimal shape we persist to the episodes cache. We
// store only the per-episode identifier (the AnimeFever ?ep= numeric value)
// and the parsed episode number; reconstruction into a domain.Episode
// happens in client.go via materializeEpisodes.
type episodeRef struct {
	EID    string `json:"eid"`    // AnimeFever ?ep=<eid> token
	Number int    `json:"number"` // parsed episode number (sort key)
	Title  string `json:"title,omitempty"`
}
