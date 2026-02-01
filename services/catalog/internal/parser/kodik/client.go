package kodik

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	APIEndpoint      = "https://kodikapi.com"
	TokenSourceURL   = "https://raw.githubusercontent.com/nb557/plugins/refs/heads/main/online_mod.js"
	FallbackTokenURL = "https://kodik-add.com/add-players.min.js?v=2"
)

type Client struct {
	httpClient   *http.Client
	token        string
	tokenExpires time.Time
}

// SearchResult represents an anime search result from Kodik
type SearchResult struct {
	ID            string       `json:"id"`
	Type          string       `json:"type"`
	Link          string       `json:"link"`
	Title         string       `json:"title"`
	TitleOrig     string       `json:"title_orig"`
	OtherTitle    string       `json:"other_title,omitempty"`
	Year          int          `json:"year"`
	LastSeason    int          `json:"last_season,omitempty"`
	LastEpisode   int          `json:"last_episode,omitempty"`
	EpisodesCount int          `json:"episodes_count,omitempty"`
	ShikimoriID   string       `json:"shikimori_id,omitempty"`
	KinopoiskID   string       `json:"kinopoisk_id,omitempty"`
	ImdbID        string       `json:"imdb_id,omitempty"`
	Quality       string       `json:"quality"`
	Translation   *Translation `json:"translation"`
	Screenshots   []string     `json:"screenshots,omitempty"`
	Seasons       map[string]*Season `json:"seasons,omitempty"`
}

// Translation represents dubbing/subtitle info
type Translation struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Type          string `json:"type"`           // "voice" or "subtitles"
	EpisodesCount int    `json:"episodes_count"` // Number of available episodes
}

// Season represents season data with episodes
type Season struct {
	Link     string                  `json:"link"`
	Episodes map[string]interface{} `json:"episodes,omitempty"` // Can be string or Episode object
}

// Episode represents a single episode
type Episode struct {
	Link        string   `json:"link"`
	Title       string   `json:"title,omitempty"`
	Screenshots []string `json:"screenshots,omitempty"`
}

// GetEpisodeLink gets the link for a specific episode from a season
func (s *Season) GetEpisodeLink(episodeNum string) string {
	if s.Episodes == nil {
		return ""
	}

	ep, ok := s.Episodes[episodeNum]
	if !ok {
		return ""
	}

	// Episode can be a string (just the link) or a map with link property
	if link, ok := ep.(string); ok {
		return link
	}

	if epMap, ok := ep.(map[string]interface{}); ok {
		if link, ok := epMap["link"].(string); ok {
			return link
		}
	}

	return ""
}

// SearchResponse is the API response structure
type SearchResponse struct {
	Time    string         `json:"time"`
	Total   int            `json:"total"`
	Results []SearchResult `json:"results"`
}

// VideoSource represents a video source from Kodik
type VideoSource struct {
	URL           string `json:"url"`
	Quality       int    `json:"quality"`
	TranslationID int    `json:"translation_id"`
	Translation   string `json:"translation"`
	Episode       int    `json:"episode"`
}

// NewClient creates a new Kodik API client
func NewClient() (*Client, error) {
	client := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Try to get token automatically
	token, err := client.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get kodik token: %w", err)
	}
	client.token = token
	// Token is valid for 24 hours (refresh every 12 hours to be safe)
	client.tokenExpires = time.Now().Add(12 * time.Hour)

	return client, nil
}

// refreshTokenIfNeeded refreshes the token if it's expired or about to expire
func (c *Client) refreshTokenIfNeeded() error {
	if time.Now().Before(c.tokenExpires) {
		return nil
	}

	token, err := c.getToken()
	if err != nil {
		return err
	}
	c.token = token
	c.tokenExpires = time.Now().Add(12 * time.Hour)
	return nil
}

// NewClientWithToken creates a client with a pre-defined token
func NewClientWithToken(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

// getToken retrieves the API token from public sources
func (c *Client) getToken() (string, error) {
	// Try main source first
	resp, err := c.httpClient.Get(TokenSourceURL)
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			token := c.extractTokenFromSource(string(body))
			if token != "" {
				return token, nil
			}
		}
	}

	// Fallback to alternative source
	resp, err = c.httpClient.Get(FallbackTokenURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch token source: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token source: %w", err)
	}

	// Extract token from minified JS
	text := string(body)
	idx := strings.Index(text, "token=")
	if idx == -1 {
		return "", fmt.Errorf("token not found in source")
	}

	tokenStart := idx + 7 // skip 'token="'
	tokenEnd := strings.Index(text[tokenStart:], "\"")
	if tokenEnd == -1 {
		return "", fmt.Errorf("token end not found")
	}

	return text[tokenStart : tokenStart+tokenEnd], nil
}

// extractTokenFromSource extracts token using the decode algorithm
func (c *Client) extractTokenFromSource(source string) string {
	// Find the encoded secret in the source
	marker := "var embed = 'https://kodikapi.com/search';"
	startPos := strings.LastIndex(source, marker)
	if startPos == -1 {
		return ""
	}

	// Find decodeSecret call
	secretStart := strings.Index(source[startPos:], "Utils.decodeSecret([")
	if secretStart == -1 {
		return ""
	}
	secretStart += startPos + 20

	secretEnd := strings.Index(source[secretStart:], "],")
	if secretEnd == -1 {
		return ""
	}

	// Parse the secret array
	secretStr := source[secretStart : secretStart+secretEnd]
	parts := strings.Split(secretStr, ", ")
	secret := make([]int, len(parts))
	for i, p := range parts {
		fmt.Sscanf(strings.TrimSpace(p), "%d", &secret[i])
	}

	// Decode with password "kodik"
	return decodeSecret(secret, "kodik")
}

// salt function matches the Python implementation
func salt(input string) string {
	hash := 0
	for i := 0; i < len(input); i++ {
		c := int(input[i])
		hash = (hash << 5) - hash + c
		hash = hash & hash // Convert to 32-bit integer
	}

	result := ""
	i := 0
	for j := 29; j >= 0; {
		x := ((hash >> i & 7) << 3) + (hash >> j & 7)
		var cc int
		if x < 26 {
			cc = 97 + x
		} else if x < 52 {
			cc = 39 + x
		} else {
			cc = x - 4
		}
		result += string(rune(cc))
		i += 3
		j -= 3
	}
	return result
}

// decodeSecret decodes the secret array with password
func decodeSecret(input []int, password string) string {
	if len(input) == 0 || password == "" {
		return ""
	}

	hash := salt("123456789" + password)
	for len(hash) < len(input) {
		hash += hash
	}

	result := ""
	for i := 0; i < len(input); i++ {
		result += string(rune(input[i] ^ int(hash[i])))
	}
	return result
}

// SearchByShikimoriID searches for anime by Shikimori ID
func (c *Client) SearchByShikimoriID(shikimoriID string) ([]SearchResult, error) {
	// Refresh token if needed
	if err := c.refreshTokenIfNeeded(); err != nil {
		// Continue with existing token, it might still work
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry (exponential backoff)
			time.Sleep(time.Duration(attempt*500) * time.Millisecond)
		}

		params := url.Values{}
		params.Set("token", c.token)
		params.Set("shikimori_id", shikimoriID)
		params.Set("with_episodes", "true")
		params.Set("with_material_data", "true")

		resp, err := c.httpClient.PostForm(APIEndpoint+"/search", params)
		if err != nil {
			lastErr = fmt.Errorf("search request failed: %w", err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			// Token might be invalid, try to refresh
			if newToken, refreshErr := c.getToken(); refreshErr == nil {
				c.token = newToken
				c.tokenExpires = time.Now().Add(12 * time.Hour)
				lastErr = fmt.Errorf("token refresh needed, retrying")
				continue
			}
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("search failed: status %d, body: %s", resp.StatusCode, string(body)[:min(200, len(body))])
			// Don't retry on 4xx errors (except 401/403)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				break
			}
			continue
		}

		var result SearchResponse
		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = fmt.Errorf("decode error: %w", err)
			continue
		}

		if result.Total == 0 {
			return nil, fmt.Errorf("no results found for shikimori_id %s", shikimoriID)
		}

		return result.Results, nil
	}

	return nil, lastErr
}

// SearchByTitle searches for anime by title
func (c *Client) SearchByTitle(title string) ([]SearchResult, error) {
	// Refresh token if needed
	if err := c.refreshTokenIfNeeded(); err != nil {
		// Continue with existing token, it might still work
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*500) * time.Millisecond)
		}

		params := url.Values{}
		params.Set("token", c.token)
		params.Set("title", title)
		params.Set("types", "anime,anime-serial")
		params.Set("with_episodes", "true")
		params.Set("limit", "50")

		resp, err := c.httpClient.PostForm(APIEndpoint+"/search", params)
		if err != nil {
			lastErr = fmt.Errorf("search request failed: %w", err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			if newToken, refreshErr := c.getToken(); refreshErr == nil {
				c.token = newToken
				c.tokenExpires = time.Now().Add(12 * time.Hour)
				lastErr = fmt.Errorf("token refresh needed, retrying")
				continue
			}
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("search failed: status %d, body: %s", resp.StatusCode, string(body)[:min(200, len(body))])
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				break
			}
			continue
		}

		var result SearchResponse
		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = fmt.Errorf("decode error: %w", err)
			continue
		}

		if result.Total == 0 {
			return nil, fmt.Errorf("no results found for title: %s", title)
		}

		return result.Results, nil
	}

	return nil, lastErr
}

// GetTranslations returns all available translations for an anime by Shikimori ID
func (c *Client) GetTranslations(shikimoriID string) ([]Translation, error) {
	results, err := c.SearchByShikimoriID(shikimoriID)
	if err != nil {
		return nil, err
	}

	// Deduplicate translations and track episode count
	seen := make(map[int]*Translation)
	for _, r := range results {
		if r.Translation == nil {
			continue
		}

		// Determine episode count: use last_episode if available, otherwise episodes_count
		episodeCount := r.LastEpisode
		if episodeCount == 0 {
			episodeCount = r.EpisodesCount
		}
		// If still 0, count episodes from seasons data
		if episodeCount == 0 && r.Seasons != nil {
			for _, season := range r.Seasons {
				if season.Episodes != nil {
					episodeCount += len(season.Episodes)
				}
			}
		}

		if existing, ok := seen[r.Translation.ID]; ok {
			// Keep the higher episode count
			if episodeCount > existing.EpisodesCount {
				existing.EpisodesCount = episodeCount
			}
		} else {
			seen[r.Translation.ID] = &Translation{
				ID:            r.Translation.ID,
				Title:         r.Translation.Title,
				Type:          r.Translation.Type,
				EpisodesCount: episodeCount,
			}
		}
	}

	var translations []Translation
	for _, t := range seen {
		translations = append(translations, *t)
	}

	return translations, nil
}

// GetEpisodeLink returns the embed link for a specific episode
func (c *Client) GetEpisodeLink(shikimoriID string, episode int, translationID int) (string, error) {
	results, err := c.SearchByShikimoriID(shikimoriID)
	if err != nil {
		return "", err
	}

	// Find the result with matching translation
	for _, r := range results {
		if r.Translation != nil && r.Translation.ID == translationID {
			// Check if we have season/episode data
			if r.Seasons != nil {
				for _, season := range r.Seasons {
					epKey := fmt.Sprintf("%d", episode)
					epLink := season.GetEpisodeLink(epKey)
					if epLink != "" {
						if !strings.HasPrefix(epLink, "http") {
							epLink = "https:" + epLink
						}
						return epLink, nil
					}
				}
			}

			// Fallback: construct the link with episode parameter
			link := r.Link
			if !strings.HasPrefix(link, "http") {
				link = "https:" + link
			}
			// Add episode parameter
			if episode > 0 {
				if strings.Contains(link, "?") {
					link += fmt.Sprintf("&episode=%d", episode)
				} else {
					link += fmt.Sprintf("?episode=%d", episode)
				}
			}
			return link, nil
		}
	}

	return "", fmt.Errorf("translation %d not found for shikimori_id %s", translationID, shikimoriID)
}

// GetVideoURL extracts the actual video URL from a Kodik embed page
// This requires additional scraping of the embed page
func (c *Client) GetVideoURL(embedLink string) (*VideoSource, error) {
	req, err := http.NewRequest("GET", embedLink, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://animeenigma.ru/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// Extract urlParams from the page
	urlParamsMatch := regexp.MustCompile(`urlParams\s*=\s*'([^']+)'`).FindStringSubmatch(html)
	if len(urlParamsMatch) < 2 {
		// Try alternative pattern
		urlParamsMatch = regexp.MustCompile(`urlParams\s*=\s*\{([^}]+)\}`).FindStringSubmatch(html)
	}

	// Extract video hash and id
	videoType := extractBetween(html, ".type = '", "'")
	videoHash := extractBetween(html, ".hash = '", "'")
	videoID := extractBetween(html, ".id = '", "'")

	if videoType == "" || videoHash == "" || videoID == "" {
		return nil, fmt.Errorf("failed to extract video parameters from embed page")
	}

	// Get the POST endpoint from the script
	scriptSrc := extractBetween(html, `<script src="`, `"`)
	if scriptSrc == "" {
		return nil, fmt.Errorf("failed to find script source")
	}

	// For now, return the embed link - full video URL extraction requires
	// additional POST request to Kodik with the extracted parameters
	// This is complex and changes frequently, so embed link is more stable

	return &VideoSource{
		URL:     embedLink,
		Quality: 720,
	}, nil
}

// extractBetween extracts string between two markers
func extractBetween(s, start, end string) string {
	startIdx := strings.Index(s, start)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)
	endIdx := strings.Index(s[startIdx:], end)
	if endIdx == -1 {
		return ""
	}
	return s[startIdx : startIdx+endIdx]
}

// convertChar is used for ROT cipher decryption
func convertChar(char rune, num int) rune {
	alph := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	upper := strings.ToUpper(string(char))
	idx := strings.Index(alph, upper)
	if idx == -1 {
		return char
	}

	newIdx := (idx + num) % len(alph)
	result := rune(alph[newIdx])
	if char >= 'a' && char <= 'z' {
		return rune(strings.ToLower(string(result))[0])
	}
	return result
}

// decryptVideoURL decrypts the video URL using ROT cipher + base64
func decryptVideoURL(encrypted string) (string, error) {
	// Try different ROT values
	for rot := 0; rot < 26; rot++ {
		var converted strings.Builder
		for _, c := range encrypted {
			converted.WriteRune(convertChar(c, rot))
		}

		// Add base64 padding
		cryptedURL := converted.String()
		padding := (4 - (len(cryptedURL) % 4)) % 4
		cryptedURL += strings.Repeat("=", padding)

		decoded, err := base64.StdEncoding.DecodeString(cryptedURL)
		if err != nil {
			continue
		}

		result := string(decoded)
		if strings.Contains(result, "mp4:hls:manifest") {
			return result, nil
		}
	}

	return "", fmt.Errorf("failed to decrypt video URL")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
