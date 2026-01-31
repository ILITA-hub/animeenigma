package aniboom

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	DefaultDomain = "animego.me"
	AniboomDomain = "aniboom.one"
	UserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type Client struct {
	httpClient *http.Client
	domain     string
}

type SearchResult struct {
	Title      string `json:"title"`
	Year       string `json:"year"`
	OtherTitle string `json:"other_title"`
	Type       string `json:"type"`
	Link       string `json:"link"`
	AnimegoID  string `json:"animego_id"`
}

type Translation struct {
	Name          string `json:"name"`
	TranslationID string `json:"translation_id"`
}

type EpisodeInfo struct {
	Number string `json:"number"`
	Title  string `json:"title"`
	Date   string `json:"date"`
	Status string `json:"status"`
}

type AnimeInfo struct {
	Title        string        `json:"title"`
	OtherTitles  []string      `json:"other_titles"`
	Status       string        `json:"status"`
	Type         string        `json:"type"`
	Genres       []string      `json:"genres"`
	Description  string        `json:"description"`
	Episodes     string        `json:"episodes"`
	EpisodesInfo []EpisodeInfo `json:"episodes_info"`
	Translations []Translation `json:"translations"`
	PosterURL    string        `json:"poster_url"`
	Link         string        `json:"link"`
	AnimegoID    string        `json:"animego_id"`
}

type VideoSource struct {
	URL           string `json:"url"`
	Type          string `json:"type"` // "mpd" or "m3u8"
	TranslationID string `json:"translation_id"`
	Translation   string `json:"translation"`
	Episode       int    `json:"episode"`
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		domain: DefaultDomain,
	}
}

func NewClientWithMirror(mirror string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		domain: mirror,
	}
}

// warmupSession makes an initial request to get cookies from DDoS-Guard
func (c *Client) warmupSession() error {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/", c.domain), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Just consume the body to complete the request
	io.Copy(io.Discard, resp.Body)
	return nil
}

// FastSearch performs a quick search on animego.me
func (c *Client) FastSearch(title string) ([]SearchResult, error) {
	// First, warm up the session to get DDoS-Guard cookies
	if err := c.warmupSession(); err != nil {
		return nil, fmt.Errorf("warmup failed: %w", err)
	}

	reqURL := fmt.Sprintf("https://%s/search/all", c.domain)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("type", "small")
	q.Add("q", title)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", fmt.Sprintf("https://%s/", c.domain))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed: status %d, body: %s", resp.StatusCode, string(body)[:min(200, len(body))])
	}

	var response struct {
		Status  string `json:"status"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("search failed: status %s", response.Status)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response.Content))
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	doc.Find(".result-search-item").Each(func(i int, s *goquery.Selection) {
		result := SearchResult{}

		if h5 := s.Find("h5"); h5.Length() > 0 {
			result.Title = strings.TrimSpace(h5.Text())
			if a := h5.Find("a"); a.Length() > 0 {
				if href, exists := a.Attr("href"); exists {
					result.Link = fmt.Sprintf("https://%s%s", c.domain, href)
					// Extract animego_id from link
					lastDash := strings.LastIndex(result.Link, "-")
					if lastDash != -1 {
						result.AnimegoID = result.Link[lastDash+1:]
					}
				}
			}
		}

		if yearSpan := s.Find(".anime-year"); yearSpan.Length() > 0 {
			result.Year = strings.TrimSpace(yearSpan.Text())
		}

		if otherTitle := s.Find(".text-truncate"); otherTitle.Length() > 0 {
			result.OtherTitle = strings.TrimSpace(otherTitle.Text())
		}

		if typeLink := s.Find("a[href*='anime/type']"); typeLink.Length() > 0 {
			result.Type = strings.TrimSpace(typeLink.Text())
		}

		results = append(results, result)
	})

	return results, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetTranslations gets available translations for an anime
func (c *Client) GetTranslations(animegoID string) ([]Translation, error) {
	reqURL := fmt.Sprintf("https://%s/anime/%s/player", c.domain, animegoID)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("_allow", "true")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", fmt.Sprintf("https://%s/search/all?q=anime", c.domain))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body)[:min(200, len(body))])
	}

	var response struct {
		Status  string `json:"status"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response.Content))
	if err != nil {
		return nil, err
	}

	// Check if content is blocked
	if doc.Find(".player-blocked").Length() > 0 {
		reason := doc.Find(".h5").Text()
		return nil, fmt.Errorf("content blocked: %s", reason)
	}

	// Map dubbing ID to translation name
	translations := make(map[string]*Translation)

	doc.Find("#video-dubbing .video-player-toggle-item").Each(func(i int, s *goquery.Selection) {
		if dubbing, exists := s.Attr("data-dubbing"); exists {
			name := strings.TrimSpace(s.Text())
			translations[dubbing] = &Translation{Name: name}
		}
	})

	// Get translation IDs from aniboom player
	doc.Find("#video-players .video-player-toggle-item").Each(func(i int, s *goquery.Selection) {
		provider, _ := s.Attr("data-provider")
		if provider == "24" { // Aniboom provider
			dubbing, _ := s.Attr("data-provide-dubbing")
			player, _ := s.Attr("data-player")

			// Extract translation ID from player URL
			if idx := strings.LastIndex(player, "="); idx != -1 {
				translationID := player[idx+1:]
				if t, ok := translations[dubbing]; ok {
					t.TranslationID = translationID
				}
			}
		}
	})

	var result []Translation
	for _, t := range translations {
		if t.TranslationID != "" {
			result = append(result, *t)
		}
	}

	return result, nil
}

// GetEmbedLink gets the embed link for an anime
func (c *Client) GetEmbedLink(animegoID string) (string, error) {
	reqURL := fmt.Sprintf("https://%s/anime/%s/player", c.domain, animegoID)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("_allow", "true")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return "", fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var response struct {
		Status  string `json:"status"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}

	if response.Status != "success" {
		return "", fmt.Errorf("failed to get embed: status %s", response.Status)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response.Content))
	if err != nil {
		return "", err
	}

	// Check if content is blocked
	if doc.Find(".player-blocked").Length() > 0 {
		reason := doc.Find(".h5").Text()
		return "", fmt.Errorf("content blocked: %s", reason)
	}

	// Find aniboom player link (provider 24)
	link := ""
	doc.Find("#video-players .video-player-toggle-item[data-provider='24']").Each(func(i int, s *goquery.Selection) {
		if player, exists := s.Attr("data-player"); exists {
			// Remove query params
			if idx := strings.Index(player, "?"); idx != -1 {
				link = "https:" + player[:idx]
			} else {
				link = "https:" + player
			}
		}
	})

	if link == "" {
		return "", fmt.Errorf("no aniboom player found for id %s", animegoID)
	}

	return link, nil
}

// GetVideoSource gets the video source URL for a specific episode
func (c *Client) GetVideoSource(animegoID string, episode int, translationID string) (*VideoSource, error) {
	embedLink, err := c.GetEmbedLink(animegoID)
	if err != nil {
		return nil, err
	}

	// Build embed URL with params
	embedURL, err := url.Parse(embedLink)
	if err != nil {
		return nil, err
	}

	q := embedURL.Query()
	if episode != 0 {
		q.Set("episode", fmt.Sprintf("%d", episode))
	}
	q.Set("translation", translationID)
	embedURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", embedURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Referer", fmt.Sprintf("https://%s/", c.domain))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	// Find video element with data-parameters
	dataParams, exists := doc.Find("#video").Attr("data-parameters")
	if !exists {
		return nil, fmt.Errorf("video data not found")
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(dataParams), &params); err != nil {
		return nil, err
	}

	// Get dash or hls source
	var videoURL, videoType string

	if dash, ok := params["dash"].(string); ok {
		var dashData map[string]interface{}
		if err := json.Unmarshal([]byte(dash), &dashData); err == nil {
			if src, ok := dashData["src"].(string); ok {
				videoURL = src
				videoType = "mpd"
			}
		}
	}

	if videoURL == "" {
		if hls, ok := params["hls"].(string); ok {
			var hlsData map[string]interface{}
			if err := json.Unmarshal([]byte(hls), &hlsData); err == nil {
				if src, ok := hlsData["src"].(string); ok {
					videoURL = src
					videoType = "m3u8"
				}
			}
		}
	}

	if videoURL == "" {
		return nil, fmt.Errorf("no video source found")
	}

	return &VideoSource{
		URL:           videoURL,
		Type:          videoType,
		TranslationID: translationID,
		Episode:       episode,
	}, nil
}

// SearchByShikimoriName searches for anime by Shikimori name
func (c *Client) SearchByShikimoriName(nameRU, nameJP, nameEN string) (*SearchResult, error) {
	// Try Russian name first
	if nameRU != "" {
		results, err := c.FastSearch(nameRU)
		if err == nil && len(results) > 0 {
			return &results[0], nil
		}
	}

	// Try Japanese name
	if nameJP != "" {
		results, err := c.FastSearch(nameJP)
		if err == nil && len(results) > 0 {
			return &results[0], nil
		}
	}

	// Try English name
	if nameEN != "" {
		results, err := c.FastSearch(nameEN)
		if err == nil && len(results) > 0 {
			return &results[0], nil
		}
	}

	return nil, fmt.Errorf("anime not found on animego")
}

// GetMPDPlaylist gets the full MPD playlist with corrected URLs
func (c *Client) GetMPDPlaylist(videoURL string) (string, error) {
	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Origin", "https://aniboom.one")
	req.Header.Set("Referer", "https://aniboom.one/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	playlist := string(body)

	// Fix relative URLs in playlist
	if strings.Contains(playlist, "<MPD") {
		// MPD format - fix base URLs
		re := regexp.MustCompile(`<BaseURL>([^<]+)</BaseURL>`)
		serverPath := videoURL[:strings.LastIndex(videoURL, "/")+1]
		playlist = re.ReplaceAllString(playlist, fmt.Sprintf("<BaseURL>%s$1</BaseURL>", serverPath))
	}

	return playlist, nil
}
