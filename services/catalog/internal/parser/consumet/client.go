package consumet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultProvider is the provider to use (zoro = HiAnime)
	DefaultProvider = "zoro"

	// Retry configuration
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond
)

// PreferredServers defines the order of servers to try
var PreferredServers = []string{"vidcloud", "streamsb", "vidstreaming"}

// Client is the Consumet API client
type Client struct {
	httpClient *http.Client
	baseURL    string
	provider   string
}

// NewClient creates a new Consumet client
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://consumet:3000"
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		provider: DefaultProvider,
	}
}

// SearchResult represents a search result from Consumet
type SearchResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Image    string `json:"image"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	SubOrDub string `json:"subOrDub"`
}

// Episode represents an episode from Consumet
type Episode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	IsFiller bool   `json:"isFiller"`
	URL      string `json:"url"`
}

// AnimeInfo represents anime information with episodes
type AnimeInfo struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Image       string    `json:"image"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	TotalEps    int       `json:"totalEpisodes"`
	Episodes    []Episode `json:"episodes"`
	SubOrDub    string    `json:"subOrDub"`
}

// Server represents a streaming server
type Server struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Stream represents stream source data
type Stream struct {
	URL       string            `json:"url"`
	IsM3U8    bool              `json:"isM3U8"`
	Quality   string            `json:"quality"`
	Headers   map[string]string `json:"headers,omitempty"`
	Subtitles []Subtitle        `json:"subtitles,omitempty"`
}

// Subtitle represents a subtitle track
type Subtitle struct {
	URL  string `json:"url"`
	Lang string `json:"lang"`
}

// StreamResponse represents the full stream response
type StreamResponse struct {
	Headers   map[string]string `json:"headers"`
	Sources   []Stream          `json:"sources"`
	Subtitles []Subtitle        `json:"subtitles"`
}

// Search searches for anime by title
func (c *Client) Search(title string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/anime/%s/%s", c.baseURL, c.provider, url.QueryEscape(title))

	body, err := c.doRequest(searchURL)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var response struct {
		Results []SearchResult `json:"results"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	return response.Results, nil
}

// GetAnimeInfo gets anime info including episodes
func (c *Client) GetAnimeInfo(animeID string) (*AnimeInfo, error) {
	infoURL := fmt.Sprintf("%s/anime/%s/info?id=%s", c.baseURL, c.provider, url.QueryEscape(animeID))

	body, err := c.doRequest(infoURL)
	if err != nil {
		return nil, fmt.Errorf("anime info request failed: %w", err)
	}

	var info AnimeInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("failed to parse anime info: %w", err)
	}

	return &info, nil
}

// GetEpisodes gets all episodes for an anime
func (c *Client) GetEpisodes(animeID string) ([]Episode, error) {
	info, err := c.GetAnimeInfo(animeID)
	if err != nil {
		return nil, err
	}
	return info.Episodes, nil
}

// GetServers returns available servers for streaming
// Consumet doesn't have a dedicated servers endpoint, so we return predefined servers
func (c *Client) GetServers() []Server {
	return []Server{
		{Name: "vidcloud", URL: ""},
		{Name: "streamsb", URL: ""},
		{Name: "vidstreaming", URL: ""},
	}
}

// GetStream gets the stream URL for an episode
func (c *Client) GetStream(episodeID string, serverName string) (*StreamResponse, error) {
	// Build list of servers to try
	serversToTry := []string{serverName}
	if serverName == "" {
		serversToTry = PreferredServers
	}

	var lastErr error
	for _, server := range serversToTry {
		stream, err := c.getStreamWithRetry(episodeID, server)
		if err == nil && stream != nil && len(stream.Sources) > 0 {
			return stream, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to get stream after trying servers %v: %w", serversToTry, lastErr)
	}
	return nil, fmt.Errorf("no stream available from any server")
}

// getStreamWithRetry attempts to fetch stream with retry logic
func (c *Client) getStreamWithRetry(episodeID string, serverName string) (*StreamResponse, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := retryBaseWait * time.Duration(1<<(attempt-1))
			time.Sleep(wait)
		}

		stream, err := c.getStreamDirect(episodeID, serverName)
		if err == nil && stream != nil && len(stream.Sources) > 0 {
			return stream, nil
		}
		lastErr = err

		// Don't retry on 404 errors
		if err != nil && strings.Contains(err.Error(), "404") {
			return nil, err
		}
	}

	return nil, lastErr
}

// getStreamDirect fetches stream from a specific server
func (c *Client) getStreamDirect(episodeID string, serverName string) (*StreamResponse, error) {
	watchURL := fmt.Sprintf("%s/anime/%s/watch?episodeId=%s",
		c.baseURL, c.provider, url.QueryEscape(episodeID))

	if serverName != "" {
		watchURL += "&server=" + serverName
	}

	body, err := c.doRequest(watchURL)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	var response StreamResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse stream response: %w", err)
	}

	return &response, nil
}

// doRequest performs an HTTP GET request and returns the body
func (c *Client) doRequest(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AnimeEnigma/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
