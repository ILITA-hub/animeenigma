package allanime

import (
	"encoding/json"
	"fmt"
)

// Persisted-query SHA256 hashes used by the AllAnime web client. These are
// MIRRORED in the Config struct so the operator can rotate them via env
// without a code change. The constants below are NOT used at runtime;
// they're documented here as a paper trail of the SHAs known to work as of
// 2026-05.
//
// These hashes are the long-standing values used by upstream reference
// projects (pystardust/ani-cli, justfoolingaround/animdl) — they have been
// stable for >18 months because Apollo persisted-query manifests rarely
// invalidate. If AllAnime breaks them, swap via env (`ALLANIME_QUERY_*_SHA`).
const (
	// SHASearchFallback — search shows by title with translationType filter.
	SHASearchFallback = "06327bc10dd682e1ee7e07b6db9c16e9ad2fd56c1b769e47513128cd5c9fc77a"
	// SHAEpisodesFallback — list episodes for a show by translationType.
	SHAEpisodesFallback = "0ac09728ee9d556967c1a60bbcf55a9f58b4112006d09a258356aeafe1c33889"
	// SHASourcesFallback — resolve playable source URLs for an episode.
	SHASourcesFallback = "0ac09728ee9d556967c1a60bbcf55a9f58b4112006d09a258356aeafe1c33889"
)

// effectiveSearchSHA returns the configured SHA, falling back to the known
// good value if the env is unset.
func (c *Client) effectiveSearchSHA() string {
	if c.cfg.QuerySearchSHA != "" {
		return c.cfg.QuerySearchSHA
	}
	return SHASearchFallback
}

func (c *Client) effectiveEpisodesSHA() string {
	if c.cfg.QueryEpisodesSHA != "" {
		return c.cfg.QueryEpisodesSHA
	}
	return SHAEpisodesFallback
}

func (c *Client) effectiveSourcesSHA() string {
	if c.cfg.QuerySourcesSHA != "" {
		return c.cfg.QuerySourcesSHA
	}
	return SHASourcesFallback
}

// buildExtensions returns the JSON-encoded Apollo persistedQuery extension
// for a given SHA, suitable for the `extensions` query-string param.
func buildExtensions(sha string) string {
	type persistedQuery struct {
		Version    int    `json:"version"`
		SHA256Hash string `json:"sha256Hash"`
	}
	type extensions struct {
		PersistedQuery persistedQuery `json:"persistedQuery"`
	}
	b, _ := json.Marshal(extensions{
		PersistedQuery: persistedQuery{Version: 1, SHA256Hash: sha},
	})
	return string(b)
}

// buildSearchVariables encodes the `variables` payload for a shows search.
//   - search:  free-text query terms.
//   - translationType: "raw" — we only want original-audio sources.
func buildSearchVariables(query string) (string, error) {
	type searchVars struct {
		Query           string `json:"query"`
		AllowAdult      bool   `json:"allowAdult"`
		AllowUnknown    bool   `json:"allowUnknown"`
		IsManga         bool   `json:"isManga"`
		TranslationType string `json:"translationType"`
		CountryOrigin   string `json:"countryOrigin"`
	}
	type limitVars struct {
		Search      searchVars `json:"search"`
		Limit       int        `json:"limit"`
		Page        int        `json:"page"`
		TranslationType string `json:"translationType"`
		CountryOrigin   string `json:"countryOrigin"`
	}
	v := limitVars{
		Search: searchVars{
			Query:           query,
			AllowAdult:      false,
			AllowUnknown:    false,
			TranslationType: "raw",
			CountryOrigin:   "ALL",
		},
		Limit:           10,
		Page:            1,
		TranslationType: "raw",
		CountryOrigin:   "ALL",
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("buildSearchVariables: %w", err)
	}
	return string(b), nil
}

// buildEpisodesVariables encodes the variables payload for an episodes query.
func buildEpisodesVariables(showID string) (string, error) {
	v := map[string]any{
		"_id": showID,
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("buildEpisodesVariables: %w", err)
	}
	return string(b), nil
}

// buildSourcesVariables encodes the variables payload for the sources query.
// AllAnime's sources resolver expects a composite ID built from (showID,
// translationType, episodeString).
func buildSourcesVariables(showID, episodeString string) (string, error) {
	v := map[string]any{
		"showId":          showID,
		"translationType": "raw",
		"episodeString":   episodeString,
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("buildSourcesVariables: %w", err)
	}
	return string(b), nil
}
