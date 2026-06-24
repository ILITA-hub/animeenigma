// Package anime365 is a read-only client for the smotret-anime / anime365
// public API. It resolves anime → series → episode → Russian-subtitle
// translations and downloads the subtitle files. No API key is required.
package anime365

import (
	"bytes"
	"fmt"
	"strconv"
)

// Episode is one episode row from GET /api/episodes?seriesId=.
type Episode struct {
	ID          int      `json:"id"`
	EpisodeInt  string   `json:"episodeInt"`
	EpisodeType string   `json:"episodeType"`
	IsActive    flexBool `json:"isActive"`
}

// Active reports whether the episode is active. anime365 returns isActive as a
// JSON number (1/0), not a bool — flexBool tolerates both forms.
func (e Episode) Active() bool { return bool(e.IsActive) }

// flexBool decodes a JSON bool OR number. anime365 sends isActive (and
// isFirstUploaded) as 1/0 rather than true/false; any non-zero number or
// `true` decodes to true.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch s := string(bytes.TrimSpace(data)); s {
	case "true":
		*b = true
	case "false", "null":
		*b = false
	default:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("anime365: flexBool: cannot parse %q", s)
		}
		*b = n != 0
	}
	return nil
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
