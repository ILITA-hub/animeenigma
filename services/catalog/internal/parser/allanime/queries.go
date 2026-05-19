package allanime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GraphQL operations sent to AllAnime's Apollo server.
//
// AllAnime's edge is Cloudflare-fronted and requires the `extensions`
// Apollo persisted-query parameter to be present on every request (plain
// GraphQL queries get a JS challenge). We send the query string AND the
// extensions block — Apollo's Automatic Persisted Queries (APQ) flow lets
// the server cache new queries under the provided SHA on first hit, so we
// never have to chase SHA rotations: as long as we control the query
// strings, the SHAs derive from them.
const (
	SearchQuery   = `query SearchShows($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) { shows(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) { edges { _id name englishName nativeName thumbnail availableEpisodes } } }`
	EpisodesQuery = `query EpisodesByID($_id: String!) { show(_id: $_id) { _id availableEpisodesDetail } }`
	SourcesQuery  = `query SourceUrls($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) { episode(showId: $showId, translationType: $translationType, episodeString: $episodeString) { episodeString sourceUrls } }`
)

// Computed SHA256 hashes of the operations above. These are the Apollo
// persisted-query cache keys. If the operator overrides via env they win;
// otherwise we derive deterministically from the query string itself.
var (
	SHASearchFallback   = computeSHA(SearchQuery)
	SHAEpisodesFallback = computeSHA(EpisodesQuery)
	SHASourcesFallback  = computeSHA(SourcesQuery)
)

func computeSHA(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// effectiveSearchSHA returns the configured SHA, falling back to the
// SHA derived from the in-code query string.
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
// SearchInput accepts only query/allowAdult/allowUnknown/isManga.
// translationType and countryOrigin go at the outer level.
func buildSearchVariables(query string) (string, error) {
	type searchInput struct {
		Query        string `json:"query"`
		AllowAdult   bool   `json:"allowAdult"`
		AllowUnknown bool   `json:"allowUnknown"`
	}
	type vars struct {
		Search          searchInput `json:"search"`
		Limit           int         `json:"limit"`
		Page            int         `json:"page"`
		TranslationType string      `json:"translationType"`
		CountryOrigin   string      `json:"countryOrigin"`
	}
	v := vars{
		Search: searchInput{
			Query:        query,
			AllowAdult:   false,
			AllowUnknown: false,
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
