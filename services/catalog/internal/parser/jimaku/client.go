package jimaku

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	BaseURL = "https://jimaku.cc/api"
)

// Client is the Jimaku API client
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

// Entry represents a subtitle entry (anime) on Jimaku
type Entry struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	JapaneseName *string `json:"japanese_name,omitempty"`
	EnglishName  *string `json:"english_name,omitempty"`
	AnilistID    *int    `json:"anilist_id,omitempty"`
}

// SubtitleFile represents a subtitle file available for download
type SubtitleFile struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// NewClient creates a new Jimaku client
func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey:  apiKey,
		baseURL: BaseURL,
	}
}

// IsConfigured returns true if the client has an API key
func (c *Client) IsConfigured() bool {
	return c.apiKey != ""
}

// SearchByAnilistID searches for subtitle entries by AniList ID
func (c *Client) SearchByAnilistID(anilistID int) ([]Entry, error) {
	params := url.Values{}
	params.Set("anilist_id", strconv.Itoa(anilistID))

	body, err := c.doRequest("GET", "/entries/search", params)
	if err != nil {
		return nil, fmt.Errorf("search by anilist ID %d: %w", anilistID, err)
	}

	var entries []Entry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	return entries, nil
}

// GetFiles returns subtitle files for an entry, optionally filtered by episode
func (c *Client) GetFiles(entryID int64, episode *int) ([]SubtitleFile, error) {
	params := url.Values{}
	if episode != nil {
		params.Set("episode", strconv.Itoa(*episode))
	}

	body, err := c.doRequest("GET", fmt.Sprintf("/entries/%d/files", entryID), params)
	if err != nil {
		return nil, fmt.Errorf("get files for entry %d: %w", entryID, err)
	}

	var files []SubtitleFile
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("parse files response: %w", err)
	}

	return files, nil
}

// GetSubtitlesForEpisode combines search + file listing into a single call
func (c *Client) GetSubtitlesForEpisode(anilistID int, episode int) ([]SubtitleFile, string, error) {
	entries, err := c.SearchByAnilistID(anilistID)
	if err != nil {
		return nil, "", err
	}

	if len(entries) == 0 {
		return nil, "", nil
	}

	// Use the first matching entry
	entry := entries[0]

	files, err := c.GetFiles(entry.ID, &episode)
	if err != nil {
		return nil, entry.Name, err
	}

	return files, entry.Name, nil
}

// FileFormat returns the subtitle format based on file extension
func FileFormat(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".ass", ".ssa":
		return "ass"
	case ".srt":
		return "srt"
	case ".vtt":
		return "vtt"
	default:
		return "unknown"
	}
}

func (c *Client) doRequest(method, path string, params url.Values) ([]byte, error) {
	reqURL := c.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
