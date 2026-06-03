package eighteenanime

// EpisodeURL builds the canonical 18anime episode page URL from an episode
// slug (the full "<id>-<slug>-episode-N" form returned by ListEpisodes).
// The service layer uses this to turn the frontend's ?ep=<slug> back into the
// page URL that GetStream fetches.
func EpisodeURL(slug string) string {
	return baseURL + "/hentai/" + slug + ".html"
}
