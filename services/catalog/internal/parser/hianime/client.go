package hianime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Retry configuration
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond
)

// PreferredServers defines the order of servers to try (hd-2 works, hd-1 is often blocked)
var PreferredServers = []string{"hd-2", "hd-1"}

// serverNameMap maps server names returned by the aniwatch servers endpoint
// to the names the sources endpoint actually accepts.
// The servers endpoint returns new names (vidsrc, megacloud, t-cloud) but
// the sources endpoint only works with legacy names (hd-1, hd-2).
var serverNameMap = map[string]string{
	"megacloud": "hd-2",
	"vidsrc":    "hd-1",
}

// Client is the HiAnime API client
type Client struct {
	httpClient     *http.Client
	aniwatchAPIURL string
}

// NewClient creates a new HiAnime client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		aniwatchAPIURL: "http://aniwatch:4000",
	}
}

// NewClientWithAniwatch creates a new HiAnime client with custom Aniwatch API URL
func NewClientWithAniwatch(aniwatchAPIURL string) *Client {
	c := NewClient()
	if aniwatchAPIURL != "" {
		c.aniwatchAPIURL = aniwatchAPIURL
	}
	return c
}

// SearchResult represents a search result from HiAnime
type SearchResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	JName    string `json:"jname"`
	Poster   string `json:"poster"`
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// Episode represents an episode from HiAnime
type Episode struct {
	ID       string `json:"id"`       // e.g., "death-note-60?ep=1234"
	Number   int    `json:"number"`
	Title    string `json:"title"`
	IsFiller bool   `json:"is_filler"`
}

// Server represents a streaming server
type Server struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "sub", "dub", "raw"
}

// Stream represents stream source data
type Stream struct {
	URL       string            `json:"url"`
	Type      string            `json:"type"` // "hls" or "mp4"
	Subtitles []Subtitle        `json:"subtitles"`
	Headers   map[string]string `json:"headers,omitempty"`
	Intro     *TimeRange        `json:"intro,omitempty"`
	Outro     *TimeRange        `json:"outro,omitempty"`
	AnilistID int               `json:"anilist_id,omitempty"`
	MalID     int               `json:"mal_id,omitempty"`
}

// Subtitle represents a subtitle track
type Subtitle struct {
	URL     string `json:"url"`
	Lang    string `json:"lang"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// TimeRange for intro/outro markers
type TimeRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Search searches for anime by title via aniwatch API.
func (c *Client) Search(title string) ([]SearchResult, error) {
	apiURL := fmt.Sprintf("%s/api/v2/hianime/search?q=%s&page=1",
		c.aniwatchAPIURL, url.QueryEscape(title))

	body, err := c.doGet(apiURL)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var resp struct {
		Data struct {
			Animes []SearchResult `json:"animes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	return resp.Data.Animes, nil
}

// GetEpisodes gets all episodes for an anime via aniwatch API.
func (c *Client) GetEpisodes(animeID string) ([]Episode, error) {
	apiURL := fmt.Sprintf("%s/api/v2/hianime/anime/%s/episodes",
		c.aniwatchAPIURL, url.PathEscape(animeID))

	body, err := c.doGet(apiURL)
	if err != nil {
		return nil, fmt.Errorf("episodes request failed: %w", err)
	}

	var resp struct {
		Data struct {
			TotalEpisodes int `json:"totalEpisodes"`
			Episodes      []struct {
				EpisodeID string `json:"episodeId"`
				Number    int    `json:"number"`
				Title     string `json:"title"`
				IsFiller  bool   `json:"isFiller"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse episodes response: %w", err)
	}

	episodes := make([]Episode, len(resp.Data.Episodes))
	for i, ep := range resp.Data.Episodes {
		episodes[i] = Episode{
			ID:       ep.EpisodeID,
			Number:   ep.Number,
			Title:    ep.Title,
			IsFiller: ep.IsFiller,
		}
	}

	return episodes, nil
}

// GetServers gets available servers for an episode via aniwatch API.
func (c *Client) GetServers(episodeID string) ([]Server, error) {
	apiURL := fmt.Sprintf("%s/api/v2/hianime/episode/servers?animeEpisodeId=%s",
		c.aniwatchAPIURL, url.QueryEscape(episodeID))

	body, err := c.doGet(apiURL)
	if err != nil {
		return nil, fmt.Errorf("servers request failed: %w", err)
	}

	var resp struct {
		Data struct {
			Sub []struct {
				ServerName string `json:"serverName"`
				ServerID   int    `json:"serverId"`
			} `json:"sub"`
			Dub []struct {
				ServerName string `json:"serverName"`
				ServerID   int    `json:"serverId"`
			} `json:"dub"`
			Raw []struct {
				ServerName string `json:"serverName"`
				ServerID   int    `json:"serverId"`
			} `json:"raw"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse servers response: %w", err)
	}

	var servers []Server
	for _, s := range resp.Data.Sub {
		name := mapServerName(s.ServerName)
		servers = append(servers, Server{
			ID:   strconv.Itoa(s.ServerID),
			Name: name,
			Type: "sub",
		})
	}
	for _, s := range resp.Data.Dub {
		name := mapServerName(s.ServerName)
		servers = append(servers, Server{
			ID:   strconv.Itoa(s.ServerID),
			Name: name,
			Type: "dub",
		})
	}
	for _, s := range resp.Data.Raw {
		name := mapServerName(s.ServerName)
		servers = append(servers, Server{
			ID:   strconv.Itoa(s.ServerID),
			Name: name,
			Type: "raw",
		})
	}

	return servers, nil
}

// GetStream gets the stream URL for an episode from a specific server.
// Uses Aniwatch API to get decrypted HLS streams.
// Includes retry logic with exponential backoff and server fallback.
func (c *Client) GetStream(episodeID string, serverID string, category string) (*Stream, error) {
	// Build list of servers to try: requested server first, then fallbacks
	serversToTry := []string{serverID}

	// Add fallback servers if the requested one isn't in the preferred list
	// or if it's hd-1 (which is often blocked), prioritize hd-2
	if serverID == "hd-1" {
		serversToTry = []string{"hd-2", "hd-1"}
	} else if serverID == "" {
		serversToTry = PreferredServers
	}

	var lastErr error
	for _, server := range serversToTry {
		stream, err := c.getStreamWithRetry(episodeID, server, category)
		if err == nil && stream != nil && stream.URL != "" {
			return stream, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to get HLS stream after trying servers %v: %w", serversToTry, lastErr)
	}
	return nil, fmt.Errorf("no HLS stream available from any server")
}

// getStreamWithRetry attempts to fetch stream with exponential backoff retry
func (c *Client) getStreamWithRetry(episodeID string, serverID string, category string) (*Stream, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s, 2s
			wait := retryBaseWait * time.Duration(1<<(attempt-1))
			time.Sleep(wait)
		}

		stream, err := c.getStreamFromAniwatch(episodeID, serverID, category)
		if err == nil && stream != nil && stream.URL != "" {
			return stream, nil
		}

		lastErr = err

		// Don't retry on certain errors (invalid format, etc.)
		if err != nil && strings.Contains(err.Error(), "invalid episode ID format") {
			return nil, err
		}

		// Don't retry if we got a 403 (blocked) - try next server instead
		if err != nil && strings.Contains(err.Error(), "status 403") {
			return nil, err
		}
	}

	return nil, lastErr
}

// getStreamFromAniwatch fetches stream from Aniwatch API.
// Aniwatch API handles MegaCloud decryption and returns direct HLS URLs.
func (c *Client) getStreamFromAniwatch(episodeID string, serverID string, category string) (*Stream, error) {
	// Validate episode ID format: "anime-slug?ep=12345"
	if !strings.Contains(episodeID, "?ep=") {
		return nil, fmt.Errorf("invalid episode ID format: %s", episodeID)
	}

	apiURL := fmt.Sprintf("%s/api/v2/hianime/episode/sources?animeEpisodeId=%s",
		c.aniwatchAPIURL,
		url.QueryEscape(episodeID),
	)

	if serverID != "" {
		apiURL += "&server=" + mapServerName(serverID)
	}

	if category != "" {
		apiURL += "&category=" + category
	} else {
		apiURL += "&category=sub"
	}

	body, err := c.doGet(apiURL)
	if err != nil {
		return nil, fmt.Errorf("aniwatch API request failed: %w", err)
	}

	var aniwatchResp struct {
		Status int `json:"status"`
		Data   struct {
			Headers map[string]string `json:"headers"`
			Sources []struct {
				URL    string `json:"url"`
				IsM3U8 bool   `json:"isM3U8"`
				Type   string `json:"type"`
			} `json:"sources"`
			Tracks []struct {
				URL  string `json:"url"`
				Lang string `json:"lang"`
			} `json:"tracks"`
			Intro *struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"intro"`
			Outro *struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"outro"`
			AnilistID int `json:"anilistID"`
			MalID     int `json:"malID"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &aniwatchResp); err != nil {
		return nil, fmt.Errorf("failed to parse aniwatch API response: %w", err)
	}

	if aniwatchResp.Status != 200 || len(aniwatchResp.Data.Sources) == 0 {
		return nil, fmt.Errorf("no sources found in aniwatch API response (status: %d)", aniwatchResp.Status)
	}

	source := aniwatchResp.Data.Sources[0]
	if source.URL == "" {
		return nil, fmt.Errorf("no valid source URL found")
	}

	stream := &Stream{
		URL:       source.URL,
		Type:      "hls",
		Headers:   aniwatchResp.Data.Headers,
		AnilistID: aniwatchResp.Data.AnilistID,
		MalID:     aniwatchResp.Data.MalID,
	}

	// Add subtitles from tracks (filter out thumbnails)
	for _, track := range aniwatchResp.Data.Tracks {
		if track.Lang == "thumbnails" {
			continue
		}
		isDefault := strings.ToLower(track.Lang) == "english"
		stream.Subtitles = append(stream.Subtitles, Subtitle{
			URL:     track.URL,
			Lang:    track.Lang,
			Label:   track.Lang,
			Default: isDefault,
		})
	}

	// Add intro/outro markers
	if aniwatchResp.Data.Intro != nil && aniwatchResp.Data.Intro.End > 0 {
		stream.Intro = &TimeRange{
			Start: aniwatchResp.Data.Intro.Start,
			End:   aniwatchResp.Data.Intro.End,
		}
	}
	if aniwatchResp.Data.Outro != nil && aniwatchResp.Data.Outro.End > 0 {
		stream.Outro = &TimeRange{
			Start: aniwatchResp.Data.Outro.Start,
			End:   aniwatchResp.Data.Outro.End,
		}
	}

	return stream, nil
}

// mapServerName translates new aniwatch API server names to the legacy names
// that the sources endpoint accepts.
func mapServerName(name string) string {
	if mapped, ok := serverNameMap[name]; ok {
		return mapped
	}
	return name
}

// doGet performs an HTTP GET and returns the response body.
func (c *Client) doGet(apiURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
