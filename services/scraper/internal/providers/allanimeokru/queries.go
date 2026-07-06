package allanimeokru

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GraphQL operations sent to AllAnime's Apollo server. Lifted VERBATIM from
// services/catalog/internal/parser/allanime/queries.go (CONTEXT.md D1) — the
// upstream-defined contract is identical for raw-jp and EN-sub consumption.
//
// Apollo's APQ (Automatic Persisted Queries) flow: the client sends both the
// query string AND the persistedQuery extension. On cache miss the server
// auto-registers the operation under our provided SHA, so we never chase
// rotations as long as we control the query strings.
const (
	apiHost    = "api.allanime.day"
	apiReferer = "https://allmanga.to"
	apiUA      = "AnimeEnigma/1.0"

	SearchQuery   = `query SearchShows($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) { shows(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) { edges { _id name englishName nativeName thumbnail availableEpisodes } } }`
	EpisodesQuery = `query EpisodesByID($_id: String!) { show(_id: $_id) { _id availableEpisodesDetail } }`
	SourcesQuery  = `query SourceUrls($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) { episode(showId: $showId, translationType: $translationType, episodeString: $episodeString) { episodeString sourceUrls } }`
)

// Persisted-query SHA256 hashes. Search and Episodes use derived SHAs
// (Apollo APQ auto-registers). Sources pins to a known-stable SHA used by
// AllAnime's own clients — the auto-registered path hits a buggier resolver
// that errors out on `countryOfOrigin`.
var (
	SHASearchFallback   = computeSHA(SearchQuery)
	SHAEpisodesFallback = computeSHA(EpisodesQuery)
	SHASourcesFallback  = "d405d0edd690624b66baba3068e0edc3ac90f1597d898a1ec8db4e5c43c00fec"
)

func computeSHA(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
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

// buildSearchVariables encodes the `variables` payload for a shows search by
// title or MAL ID. translationType=sub for the scraper-side EN catalog
// (CONTEXT.md — diverges from the catalog-side raw-jp which uses "raw"
// pre-commit 102c590, "sub" post).
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
		TranslationType: "sub",
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
	v := map[string]any{"_id": showID}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("buildEpisodesVariables: %w", err)
	}
	return string(b), nil
}

// buildSourcesVariables encodes the variables payload for the sources query.
// translationType is one of "sub" | "dub" | "raw" (empty defaults to "sub").
func buildSourcesVariables(showID, episodeString, translationType string) (string, error) {
	if translationType == "" {
		translationType = "sub"
	}
	v := map[string]any{
		"showId":          showID,
		"translationType": translationType,
		"episodeString":   episodeString,
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("buildSourcesVariables: %w", err)
	}
	return string(b), nil
}
