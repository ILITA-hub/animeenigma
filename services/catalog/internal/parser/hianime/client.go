package hianime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	BaseURL      = "https://hianime.to"
	AjaxURL      = "https://hianime.to/ajax"
	MegaCloudURL = "https://megacloud.tv/embed-2/ajax/e-1/getSources"

	// Retry configuration
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond
)

// PreferredServers defines the order of servers to try (hd-2 works, hd-1 is often blocked)
var PreferredServers = []string{"hd-2", "hd-1", "vidstreaming"}

// Client is the HiAnime API client
type Client struct {
	httpClient     *http.Client
	baseURL        string
	aniwatchAPIURL string
}

// NewClient creates a new HiAnime client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:        BaseURL,
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

// AnimeInfo represents detailed anime information
type AnimeInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Poster        string   `json:"poster"`
	Description   string   `json:"description"`
	Type          string   `json:"type"`
	Quality       string   `json:"quality"`
	Rating        string   `json:"rating"`
	Duration      string   `json:"duration"`
	Status        string   `json:"status"`
	EpisodesSub   int      `json:"episodes_sub"`
	EpisodesDub   int      `json:"episodes_dub"`
	EpisodesTotal int      `json:"episodes_total"`
	Genres        []string `json:"genres"`
	MALId         string   `json:"mal_id"`
}

// Search searches for anime by title
func (c *Client) Search(title string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/search?keyword=%s", c.baseURL, url.QueryEscape(title))

	doc, err := c.fetchDocument(searchURL)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var results []SearchResult
	doc.Find(".film_list-wrap .flw-item").Each(func(i int, s *goquery.Selection) {
		filmDetail := s.Find(".film-detail")
		filmPoster := s.Find(".film-poster")

		name := strings.TrimSpace(filmDetail.Find(".film-name a").Text())
		href, _ := filmPoster.Find("a").Attr("href")

		// Extract ID from href (e.g., "/watch/death-note-60" -> "death-note-60")
		id := strings.TrimPrefix(href, "/watch/")
		id = strings.TrimPrefix(id, "/")

		poster, _ := filmPoster.Find("img").Attr("data-src")
		if poster == "" {
			poster, _ = filmPoster.Find("img").Attr("src")
		}

		animeType := strings.TrimSpace(filmDetail.Find(".fd-infor .fdi-item:first-child").Text())
		duration := strings.TrimSpace(filmDetail.Find(".fd-infor .fdi-duration").Text())

		if id != "" && name != "" {
			results = append(results, SearchResult{
				ID:       id,
				Name:     name,
				Poster:   poster,
				Type:     animeType,
				Duration: duration,
			})
		}
	})

	return results, nil
}

// SearchByMALID searches for anime by MAL ID
func (c *Client) SearchByMALID(malID string) (*AnimeInfo, error) {
	// HiAnime doesn't have direct MAL ID lookup, so we need to search
	// This is a limitation - we might need to search by title instead
	return nil, fmt.Errorf("MAL ID search not directly supported, use Search by title")
}

// GetAnimeInfo gets detailed anime information
func (c *Client) GetAnimeInfo(animeID string) (*AnimeInfo, error) {
	infoURL := fmt.Sprintf("%s/watch/%s", c.baseURL, animeID)

	doc, err := c.fetchDocument(infoURL)
	if err != nil {
		return nil, fmt.Errorf("anime info request failed: %w", err)
	}

	info := &AnimeInfo{ID: animeID}

	// Parse anime details
	info.Name = strings.TrimSpace(doc.Find(".anis-content .film-name").Text())
	info.Poster, _ = doc.Find(".anis-content .film-poster img").Attr("src")
	info.Description = strings.TrimSpace(doc.Find(".film-description .text").Text())

	// Parse meta info
	doc.Find(".anisc-info .item").Each(func(i int, s *goquery.Selection) {
		label := strings.TrimSpace(s.Find(".item-head").Text())
		value := strings.TrimSpace(s.Find(".name").Text())

		switch strings.ToLower(strings.TrimSuffix(label, ":")) {
		case "type":
			info.Type = value
		case "quality":
			info.Quality = value
		case "status":
			info.Status = value
		case "duration":
			info.Duration = value
		case "mal":
			// Extract MAL ID from link
			if link, exists := s.Find("a").Attr("href"); exists {
				if strings.Contains(link, "myanimelist.net/anime/") {
					parts := strings.Split(link, "/anime/")
					if len(parts) > 1 {
						info.MALId = strings.Split(parts[1], "/")[0]
					}
				}
			}
		}
	})

	// Parse genres
	doc.Find(".anisc-info .item-list a").Each(func(i int, s *goquery.Selection) {
		genre := strings.TrimSpace(s.Text())
		if genre != "" {
			info.Genres = append(info.Genres, genre)
		}
	})

	// Parse episode counts from tick items
	subText := doc.Find(".tick-sub").Text()
	dubText := doc.Find(".tick-dub").Text()
	epsText := doc.Find(".tick-eps").Text()

	if n, err := strconv.Atoi(strings.TrimSpace(subText)); err == nil {
		info.EpisodesSub = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(dubText)); err == nil {
		info.EpisodesDub = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(epsText)); err == nil {
		info.EpisodesTotal = n
	}

	return info, nil
}

// GetEpisodes gets all episodes for an anime
func (c *Client) GetEpisodes(animeID string) ([]Episode, error) {
	// First, get the anime page to find the data-id
	infoURL := fmt.Sprintf("%s/watch/%s", c.baseURL, animeID)

	doc, err := c.fetchDocument(infoURL)
	if err != nil {
		return nil, fmt.Errorf("anime page request failed: %w", err)
	}

	// Find the data-id attribute
	dataID, exists := doc.Find("#watch-main").Attr("data-id")
	if !exists {
		// Try alternative selector
		dataID, exists = doc.Find("[data-id]").First().Attr("data-id")
		if !exists {
			return nil, fmt.Errorf("could not find anime data-id")
		}
	}

	// Fetch episodes via AJAX
	episodesURL := fmt.Sprintf("%s/v2/episode/list/%s", AjaxURL, dataID)

	req, err := http.NewRequest("GET", episodesURL, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("episodes ajax request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var ajaxResp struct {
		Status bool   `json:"status"`
		HTML   string `json:"html"`
	}

	if err := json.Unmarshal(body, &ajaxResp); err != nil {
		return nil, fmt.Errorf("failed to parse episodes response: %w", err)
	}

	if !ajaxResp.Status {
		return nil, fmt.Errorf("episodes request returned false status")
	}

	// Parse HTML from response
	epDoc, err := goquery.NewDocumentFromReader(strings.NewReader(ajaxResp.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse episodes HTML: %w", err)
	}

	var episodes []Episode
	epDoc.Find(".ep-item").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		dataNumber, _ := s.Attr("data-number")
		title, _ := s.Attr("title")
		isFiller := s.HasClass("filler")

		epNum, _ := strconv.Atoi(dataNumber)

		// Extract episode ID from href
		// href format: /watch/anime-slug?ep=12345
		epID := strings.TrimPrefix(href, "/watch/")

		if epID != "" {
			episodes = append(episodes, Episode{
				ID:       epID,
				Number:   epNum,
				Title:    title,
				IsFiller: isFiller,
			})
		}
	})

	return episodes, nil
}

// GetServers gets available servers for an episode
func (c *Client) GetServers(episodeID string) ([]Server, error) {
	// episodeID format: "anime-slug?ep=12345" - extract ep number
	var epNum string
	if idx := strings.Index(episodeID, "?ep="); idx != -1 {
		epNum = episodeID[idx+4:]
	} else {
		return nil, fmt.Errorf("invalid episode ID format: %s", episodeID)
	}

	serversURL := fmt.Sprintf("%s/v2/episode/servers?episodeId=%s", AjaxURL, epNum)

	req, err := http.NewRequest("GET", serversURL, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("servers request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ajaxResp struct {
		Status bool   `json:"status"`
		HTML   string `json:"html"`
	}

	if err := json.Unmarshal(body, &ajaxResp); err != nil {
		return nil, fmt.Errorf("failed to parse servers response: %w", err)
	}

	if !ajaxResp.Status {
		return nil, fmt.Errorf("servers request returned false status")
	}

	// Parse HTML
	serversDoc, err := goquery.NewDocumentFromReader(strings.NewReader(ajaxResp.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse servers HTML: %w", err)
	}

	var servers []Server

	// Parse sub servers
	serversDoc.Find(".servers-sub .server-item").Each(func(i int, s *goquery.Selection) {
		serverID, _ := s.Attr("data-id")
		serverName := strings.TrimSpace(s.Find("a").Text())
		if serverName == "" {
			serverName = strings.TrimSpace(s.Text())
		}

		if serverID != "" {
			servers = append(servers, Server{
				ID:   serverID,
				Name: serverName,
				Type: "sub",
			})
		}
	})

	// Parse dub servers
	serversDoc.Find(".servers-dub .server-item").Each(func(i int, s *goquery.Selection) {
		serverID, _ := s.Attr("data-id")
		serverName := strings.TrimSpace(s.Find("a").Text())
		if serverName == "" {
			serverName = strings.TrimSpace(s.Text())
		}

		if serverID != "" {
			servers = append(servers, Server{
				ID:   serverID,
				Name: serverName,
				Type: "dub",
			})
		}
	})

	// Parse raw servers if any
	serversDoc.Find(".servers-raw .server-item").Each(func(i int, s *goquery.Selection) {
		serverID, _ := s.Attr("data-id")
		serverName := strings.TrimSpace(s.Find("a").Text())
		if serverName == "" {
			serverName = strings.TrimSpace(s.Text())
		}

		if serverID != "" {
			servers = append(servers, Server{
				ID:   serverID,
				Name: serverName,
				Type: "raw",
			})
		}
	})

	return servers, nil
}

// GetStream gets the stream URL for an episode from a specific server
// Uses Aniwatch API to get decrypted HLS streams instead of iframe embeds
// Includes retry logic with exponential backoff and server fallback
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

// getStreamFromAniwatch fetches stream from Aniwatch API
// Aniwatch API handles MegaCloud decryption and returns direct HLS URLs
func (c *Client) getStreamFromAniwatch(episodeID string, serverID string, category string) (*Stream, error) {
	// Validate episode ID format: "anime-slug?ep=12345"
	if !strings.Contains(episodeID, "?ep=") {
		return nil, fmt.Errorf("invalid episode ID format: %s", episodeID)
	}

	// Build Aniwatch API URL
	// Format: /api/v2/hianime/episode/sources?animeEpisodeId={animeSlug}?ep={epNum}&server={server}&category={category}
	// The aniwatch-api expects the full episodeId in the format "anime-slug?ep=12345"
	apiURL := fmt.Sprintf("%s/api/v2/hianime/episode/sources?animeEpisodeId=%s",
		c.aniwatchAPIURL,
		url.QueryEscape(episodeID),
	)

	// Add server if provided (hd-1, hd-2, etc.)
	// serverID should be the server name like "hd-1", "hd-2" from frontend
	if serverID != "" {
		apiURL += "&server=" + serverID
	}

	// Add category (sub, dub, raw)
	if category != "" {
		apiURL += "&category=" + category
	} else {
		apiURL += "&category=sub"
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aniwatch API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aniwatch API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse Aniwatch API response
	// Format: {"status":200,"data":{"headers":{},"tracks":[],"intro":{},"outro":{},"sources":[]}}
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

	// Get the first source (usually the best quality HLS stream)
	source := aniwatchResp.Data.Sources[0]
	if source.URL == "" {
		return nil, fmt.Errorf("no valid source URL found")
	}

	stream := &Stream{
		URL:     source.URL,
		Type:    "hls",
		Headers: aniwatchResp.Data.Headers,
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

// getStreamDirect gets stream using direct HiAnime scraping (fallback)
func (c *Client) getStreamDirect(serverID string) (*Stream, error) {
	sourcesURL := fmt.Sprintf("%s/v2/episode/sources?id=%s", AjaxURL, serverID)

	req, err := http.NewRequest("GET", sourcesURL, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sources request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var sourcesResp struct {
		Type   string `json:"type"`
		Link   string `json:"link"`
		Server int    `json:"server"`
	}

	if err := json.Unmarshal(body, &sourcesResp); err != nil {
		return nil, fmt.Errorf("failed to parse sources response: %w", err)
	}

	if sourcesResp.Link == "" {
		return nil, fmt.Errorf("no stream source available")
	}

	// If it's an iframe type, we need to extract the actual stream
	if sourcesResp.Type == "iframe" {
		return c.extractStreamFromEmbed(sourcesResp.Link)
	}

	// Direct link
	return &Stream{
		URL:  sourcesResp.Link,
		Type: "hls",
	}, nil
}

// extractStreamFromEmbed extracts stream URL from embed page
func (c *Client) extractStreamFromEmbed(embedURL string) (*Stream, error) {
	// Determine the provider from URL
	if strings.Contains(embedURL, "megacloud") || strings.Contains(embedURL, "rapid-cloud") {
		return c.extractMegaCloudStream(embedURL)
	}

	// For other providers, return the embed URL as iframe
	return &Stream{
		URL:  embedURL,
		Type: "iframe",
	}, nil
}

// extractMegaCloudStream extracts stream from MegaCloud/RapidCloud
func (c *Client) extractMegaCloudStream(embedURL string) (*Stream, error) {
	// Fetch the embed page
	req, err := http.NewRequest("GET", embedURL, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)
	req.Header.Set("Referer", BaseURL+"/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed page request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// Extract the video ID from embed URL
	// Format: https://megacloud.tv/embed-2/e-1/XXXXXX?k=1
	videoIDRegex := regexp.MustCompile(`/e-\d+/([^?]+)`)
	matches := videoIDRegex.FindStringSubmatch(embedURL)
	if len(matches) < 2 {
		// Try alternative pattern
		videoIDRegex = regexp.MustCompile(`embed[^/]*/([^?/]+)`)
		matches = videoIDRegex.FindStringSubmatch(embedURL)
	}

	if len(matches) < 2 {
		// Return embed URL as fallback
		return &Stream{
			URL:  embedURL,
			Type: "iframe",
		}, nil
	}

	videoID := matches[1]

	// Get sources from MegaCloud API
	sourcesURL := fmt.Sprintf("https://megacloud.tv/embed-2/ajax/e-1/getSources?id=%s", videoID)

	req, err = http.NewRequest("GET", sourcesURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", embedURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("megacloud sources request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse the sources response
	var megaResp struct {
		Sources   interface{} `json:"sources"`
		Tracks    []struct {
			File    string `json:"file"`
			Kind    string `json:"kind"`
			Label   string `json:"label"`
			Default bool   `json:"default"`
		} `json:"tracks"`
		Intro struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"intro"`
		Outro struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"outro"`
		Encrypted bool `json:"encrypted"`
	}

	if err := json.Unmarshal(body, &megaResp); err != nil {
		// If parsing fails, check for error in HTML
		if strings.Contains(html, "not available") {
			return nil, fmt.Errorf("video not available")
		}
		return &Stream{
			URL:  embedURL,
			Type: "iframe",
		}, nil
	}

	stream := &Stream{
		Type:    "hls",
		Headers: map[string]string{"Referer": "https://megacloud.tv/"},
	}

	// Handle sources (can be string or array)
	switch s := megaResp.Sources.(type) {
	case string:
		// Sources might be encrypted
		if megaResp.Encrypted {
			// For encrypted sources, return iframe as fallback
			return &Stream{
				URL:  embedURL,
				Type: "iframe",
			}, nil
		}
		stream.URL = s
	case []interface{}:
		if len(s) > 0 {
			if src, ok := s[0].(map[string]interface{}); ok {
				if file, ok := src["file"].(string); ok {
					stream.URL = file
				}
			}
		}
	}

	// Parse subtitles
	for _, track := range megaResp.Tracks {
		if track.Kind == "captions" || track.Kind == "subtitles" {
			stream.Subtitles = append(stream.Subtitles, Subtitle{
				URL:     track.File,
				Label:   track.Label,
				Default: track.Default,
			})
		}
	}

	// Parse intro/outro
	if megaResp.Intro.End > 0 {
		stream.Intro = &TimeRange{
			Start: megaResp.Intro.Start,
			End:   megaResp.Intro.End,
		}
	}
	if megaResp.Outro.End > 0 {
		stream.Outro = &TimeRange{
			Start: megaResp.Outro.Start,
			End:   megaResp.Outro.End,
		}
	}

	if stream.URL == "" {
		// Fallback to iframe
		return &Stream{
			URL:  embedURL,
			Type: "iframe",
		}, nil
	}

	return stream, nil
}

// fetchDocument fetches and parses HTML document
func (c *Client) fetchDocument(url string) (*goquery.Document, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	return goquery.NewDocumentFromReader(resp.Body)
}

// setHeaders sets common headers for requests
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
}
