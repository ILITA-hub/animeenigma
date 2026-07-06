package allanimeokru

// DTOs for parsing AllAnime GraphQL responses.

// searchShowsResponse is the response from the SearchShows persisted query.
type searchShowsResponse struct {
	Data struct {
		Shows struct {
			Edges []searchEdge `json:"edges"`
		} `json:"shows"`
	} `json:"data"`
	Errors []map[string]any `json:"errors,omitempty"`
}

type searchEdge struct {
	ID                string         `json:"_id"`
	Name              string         `json:"name"`
	EnglishName       string         `json:"englishName"`
	NativeName        string         `json:"nativeName"`
	Thumbnail         string         `json:"thumbnail"`
	AvailableEpisodes map[string]int `json:"availableEpisodes"`
}

// showResponse is the response from the EpisodesByID persisted query.
type showResponse struct {
	Data struct {
		Show struct {
			ID                      string                  `json:"_id"`
			AvailableEpisodesDetail availableEpisodesDetail `json:"availableEpisodesDetail"`
		} `json:"show"`
	} `json:"data"`
	Errors []map[string]any `json:"errors,omitempty"`
}

type availableEpisodesDetail struct {
	Sub []string `json:"sub"`
	Dub []string `json:"dub"`
	Raw []string `json:"raw"`
}

// episodeEnvelope is the response from the SourceUrls persisted query. May
// contain either the legacy direct `episode.sourceUrls` or the current
// encrypted `tobeparsed` blob (decrypts to the same inner shape).
type episodeEnvelope struct {
	Data struct {
		Episode    *episodeData `json:"episode"`
		Tobeparsed string       `json:"tobeparsed"`
	} `json:"data"`
	Errors []map[string]any `json:"errors,omitempty"`
}

// episodeData is the inner `episode` payload of the sources GraphQL response,
// shared by the legacy direct-JSON shape and the AES-CTR `tobeparsed` shape
// (the decrypted blob has the same schema).
type episodeData struct {
	EpisodeString string      `json:"episodeString"`
	SourceUrls    []sourceURL `json:"sourceUrls"`
}

type sourceURL struct {
	SourceURL     string  `json:"sourceUrl"`
	SourceName    string  `json:"sourceName"`
	Type          string  `json:"type"`
	Priority      float64 `json:"priority"`
	Sandbox       string  `json:"sandbox"`
	FileExtension string  `json:"fileExtenstion"` // AllAnime spelling — sic
	Subtitles     []struct {
		SourceURL string `json:"src"`
		Lang      string `json:"lang"`
		Label     string `json:"label"`
	} `json:"subtitles"`
}
