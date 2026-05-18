package idmapping

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const kitsuBaseURL = "https://kitsu.io/api/edge"

// ExtraIDs holds external-site IDs that don't live in the ARM response.
// Kitsu's /anime/{id}?include=mappings endpoint surfaces these.
type ExtraIDs struct {
	IMDbID *string
	TMDBID *string
}

// KitsuMappings resolves IMDb and TMDB IDs for an anime via Kitsu's mappings
// endpoint. Returns (nil, nil) when Kitsu has no record for the ID (404) —
// callers should treat this as "no mapping available", not as a hard failure.
//
// Kitsu returns JSON:API. The `included` array carries Mapping resources;
// each has `attributes.external_site` (e.g. "imdb", "themoviedb/movie",
// "themoviedb/tv") and `attributes.external_id` (the foreign ID string).
func (c *Client) KitsuMappings(ctx context.Context, kitsuID int) (*ExtraIDs, error) {
	if kitsuID <= 0 {
		return nil, fmt.Errorf("idmapping: invalid kitsu id %d", kitsuID)
	}

	endpoint := fmt.Sprintf("%s/anime/%d?include=mappings", kitsuBaseURL, kitsuID)

	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(rctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("kitsu: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kitsu: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kitsu: status %d", resp.StatusCode)
	}

	var doc struct {
		Included []struct {
			Type       string `json:"type"`
			Attributes struct {
				ExternalSite string `json:"externalSite"`
				ExternalID   string `json:"externalId"`
			} `json:"attributes"`
		} `json:"included"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("kitsu: decode: %w", err)
	}

	out := &ExtraIDs{}
	for _, inc := range doc.Included {
		if inc.Type != "mappings" {
			continue
		}
		switch inc.Attributes.ExternalSite {
		case "imdb":
			v := inc.Attributes.ExternalID
			out.IMDbID = &v
		case "themoviedb/movie", "themoviedb/tv":
			v := inc.Attributes.ExternalID
			// First TMDB hit wins; movies and TV shouldn't both appear.
			if out.TMDBID == nil {
				out.TMDBID = &v
			}
		}
	}

	if out.IMDbID == nil && out.TMDBID == nil {
		return nil, nil
	}
	return out, nil
}
