// Package anime365 is a read-only client for the smotret-anime / anime365
// public API. It resolves anime → series → episode → Russian-subtitle
// translations and downloads the subtitle files. No API key is required.
package anime365

// Episode is one episode row from GET /api/episodes?seriesId=.
type Episode struct {
	ID          int    `json:"id"`
	EpisodeInt  string `json:"episodeInt"`
	EpisodeType string `json:"episodeType"`
	IsActive    bool   `json:"isActive"`
}

// Translation is one translation (sub/voice, per language) from
// GET /api/episodes/{id}.
type Translation struct {
	ID             int    `json:"id"`
	TypeKind       string `json:"typeKind"` // "sub" | "voice" | "raw"
	TypeLang       string `json:"typeLang"` // "ru" | "en" | "ja"
	AuthorsSummary string `json:"authorsSummary"`
}

// series is the minimal shape we need from GET /api/series search results.
type series struct {
	ID            int `json:"id"`
	MyAnimeListID int `json:"myAnimeListId"`
}

// episodeDetail is the GET /api/episodes/{id} payload (data object).
type episodeDetail struct {
	ID           int           `json:"id"`
	Translations []Translation `json:"translations"`
}
