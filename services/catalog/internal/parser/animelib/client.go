package animelib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	BaseURL  = "https://hapi.hentaicdn.org/api"
	VideoCDN = "https://video1.cdnlibs.org/.\u0430s/" // .аs/ with Cyrillic "а"
	Referer  = "https://v3.animelib.org"
)

// Client is the AnimeLib API client
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string // optional Bearer token
}

// NewClient creates a new AnimeLib client
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: BaseURL,
		token:   token,
	}
}

// SearchResult represents a search result from AnimeLib
type SearchResult struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	RusName string `json:"rus_name"`
	EngName string `json:"eng_name"`
	SlugURL string `json:"slug_url"`
	Cover   *Cover `json:"cover"`
}

// Cover represents a cover/poster image
type Cover struct {
	Default   string `json:"default"`
	Thumbnail string `json:"thumbnail"`
}

// AnimeDetail represents detailed anime information from AnimeLib
type AnimeDetail struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	RusName string `json:"rus_name"`
	EngName string `json:"eng_name"`
	SlugURL string `json:"slug_url"`
}

// Episode represents an episode from AnimeLib
type Episode struct {
	ID     int    `json:"id"`
	Number string `json:"number"`
	Name   string `json:"name"`
}

// EpisodeDetail represents detailed episode information with player data
type EpisodeDetail struct {
	ID      int          `json:"id"`
	Number  string       `json:"number"`
	Name    string       `json:"name"`
	Players []PlayerData `json:"players"`
}

// PlayerData represents a player/translation for an episode (actual API structure)
type PlayerData struct {
	ID              int             `json:"id"`
	EpisodeID       int             `json:"episode_id"`
	Player          string          `json:"player"`           // "Kodik" or "Animelib"
	TranslationType TranslationType `json:"translation_type"` // {"id": 2, "label": "Озвучка"}
	Team            Team            `json:"team"`
	Src             string          `json:"src"`             // iframe URL for Kodik players
	Video           *VideoData      `json:"video,omitempty"`     // direct video for Animelib players
	Subtitles       []SubtitleFile  `json:"subtitles,omitempty"` // external subtitle files (ASS, VTT)
	Views           int             `json:"views"`
}

// TranslationType represents the translation type from AnimeLib API
type TranslationType struct {
	ID    int    `json:"id"`
	Label string `json:"label"` // "Озвучка" (voice), "Субтитры" (subtitles)
}

// Team represents a translation/dubbing team
type Team struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SubtitleFile represents an external subtitle file from the AnimeLib API
type SubtitleFile struct {
	ID     int    `json:"id"`
	Format string `json:"format"` // "ass", "vtt"
	Src    string `json:"src"`    // full URL to subtitle file
}

// VideoData holds quality variants for a direct video
type VideoData struct {
	ID      int            `json:"id"`
	Quality []VideoQuality `json:"quality"`
}

// VideoQuality represents a single quality variant
type VideoQuality struct {
	Quality int    `json:"quality"` // 360, 720, 1080, 2160
	Href    string `json:"href"`   // path to mp4
	Bitrate int    `json:"bitrate"`
}

// Search searches for anime by title on AnimeLib
func (c *Client) Search(query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/anime?q=%s&site_id[]=1&site_id[]=3&fields[]=rate_avg&fields[]=releaseDate",
		c.baseURL, url.QueryEscape(query))

	body, err := c.doRequest(searchURL)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var response struct {
		Data []SearchResult `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	return response.Data, nil
}

// GetAnimeBySlug gets detailed anime information by slug URL
func (c *Client) GetAnimeBySlug(slugURL string) (*AnimeDetail, error) {
	detailURL := fmt.Sprintf("%s/anime/%s", c.baseURL, url.PathEscape(slugURL))

	body, err := c.doRequest(detailURL)
	if err != nil {
		return nil, fmt.Errorf("anime detail request failed: %w", err)
	}

	var response struct {
		Data AnimeDetail `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse anime detail response: %w", err)
	}

	return &response.Data, nil
}

// GetEpisodes gets all episodes for an anime by its AnimeLib ID
func (c *Client) GetEpisodes(animeID int) ([]Episode, error) {
	episodesURL := fmt.Sprintf("%s/episodes?anime_id=%d", c.baseURL, animeID)

	body, err := c.doRequest(episodesURL)
	if err != nil {
		return nil, fmt.Errorf("episodes request failed: %w", err)
	}

	var response struct {
		Data []Episode `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse episodes response: %w", err)
	}

	return response.Data, nil
}

// GetEpisodeStreams gets detailed episode data including player/stream info
func (c *Client) GetEpisodeStreams(episodeID int) (*EpisodeDetail, error) {
	episodeURL := fmt.Sprintf("%s/episodes/%d", c.baseURL, episodeID)

	body, err := c.doRequest(episodeURL)
	if err != nil {
		return nil, fmt.Errorf("episode detail request failed: %w", err)
	}

	var response struct {
		Data EpisodeDetail `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse episode detail response: %w", err)
	}

	return &response.Data, nil
}

// BuildVideoURL constructs a full video URL from a quality href path
func BuildVideoURL(href string) string {
	return VideoCDN + href
}

// doRequest performs an HTTP GET request and returns the body.
// If a token is set and the request returns 401/403, it retries without
// the Authorization header (self-healing against expired tokens).
func (c *Client) doRequest(reqURL string) ([]byte, error) {
	maxAttempts := 1
	if c.token != "" {
		maxAttempts = 2 // retry without token on auth failure
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Referer", Referer)
		req.Header.Set("Origin", Referer)

		// First attempt uses token; retry skips it
		if c.token != "" && attempt == 0 {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr != nil {
			return nil, readErr
		}

		// On auth failure with token, retry without it
		if (resp.StatusCode == 401 || resp.StatusCode == 403) && c.token != "" && attempt == 0 {
			lastErr = fmt.Errorf("API returned status %d with token, retrying without", resp.StatusCode)
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, lastErr
}
