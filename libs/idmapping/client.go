package idmapping

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://arm.haglund.dev/api/v2"

// MappingResult holds the ID mapping response from ARM (anime-relations mapping).
type MappingResult struct {
	AniList   *int    `json:"anilist"`
	MAL       *int    `json:"myanimelist"`
	AniDB     *int    `json:"anidb"`
	Kitsu     *int    `json:"kitsu"`
	LiveChart *int    `json:"livechart"`
	IMDB      *string `json:"imdb"`
}

// Client interacts with the ARM anime ID mapping API (arm.haglund.dev).
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new ARM mapping client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    defaultBaseURL,
	}
}

// ResolveByShikimoriID resolves anime IDs from a Shikimori ID.
// Shikimori IDs equal MAL IDs, so we query with source=myanimelist.
func (c *Client) ResolveByShikimoriID(id string) (*MappingResult, error) {
	return c.resolve("myanimelist", id)
}

// ResolveByMALID resolves anime IDs from a MyAnimeList ID.
func (c *Client) ResolveByMALID(id string) (*MappingResult, error) {
	return c.resolve("myanimelist", id)
}

func (c *Client) resolve(source, id string) (*MappingResult, error) {
	endpoint := fmt.Sprintf("%s/ids?source=%s&id=%s", c.baseURL, url.QueryEscape(source), url.QueryEscape(id))

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("ARM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No mapping found
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ARM returned status %d: %s", resp.StatusCode, string(body))
	}

	var result MappingResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ARM decode response: %w", err)
	}

	return &result, nil
}
