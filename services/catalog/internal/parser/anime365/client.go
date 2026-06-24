package anime365

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://smotret-anime.org"

// Config configures the anime365 client. Empty fields fall back to defaults.
type Config struct {
	BaseURL   string
	Enabled   bool
	UserAgent string
	Timeout   time.Duration
	Transport http.RoundTripper // egress-recording transport (AR-EGRESS-03)
}

// Client is the anime365 read-only HTTP client.
type Client struct {
	baseURL    string
	enabled    bool
	userAgent  string
	httpClient *http.Client
}

// NewClient builds a Client, applying safe defaults.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "AnimeEnigma/1.0 (+https://animeenigma.org)"
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		enabled:    cfg.Enabled,
		userAgent:  cfg.UserAgent,
		httpClient: &http.Client{Timeout: cfg.Timeout, Transport: cfg.Transport},
	}
}

// IsConfigured reports whether the provider is enabled. anime365 needs no key,
// so this is just the enable flag; the aggregator skips it when false.
func (c *Client) IsConfigured() bool { return c != nil && c.enabled }

// getJSON GETs path and decodes the JSON body into out.
func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("anime365: GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchSeriesByMAL finds the anime365 series id whose myAnimeListId matches
// malID. title is used as the search query. Returns (0, nil) when no result
// matches — the caller treats that as "not on anime365", not an error.
func (c *Client) SearchSeriesByMAL(ctx context.Context, malID, title string) (int, error) {
	mal, err := strconv.Atoi(strings.TrimSpace(malID))
	if err != nil || mal <= 0 {
		// Unresolvable mal id → treat as "not on anime365" (fail-soft), not an
		// error, so the aggregator doesn't mark the provider down.
		return 0, nil
	}
	q := url.Values{}
	q.Set("query", title)
	q.Set("limit", "20") // top matches; we filter by exact myAnimeListId below
	var env struct {
		Data []series `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/series?"+q.Encode(), &env); err != nil {
		return 0, err
	}
	for _, s := range env.Data {
		if s.MyAnimeListID == mal {
			return s.ID, nil
		}
	}
	return 0, nil
}

// ListEpisodes returns all episodes for a series.
func (c *Client) ListEpisodes(ctx context.Context, seriesID int) ([]Episode, error) {
	q := url.Values{}
	q.Set("seriesId", strconv.Itoa(seriesID))
	q.Set("limit", "1000") // covers the longest series; revisit if a series exceeds this
	var env struct {
		Data []Episode `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/episodes?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// ListTranslations returns the translations for one episode.
func (c *Client) ListTranslations(ctx context.Context, episodeID int) ([]Translation, error) {
	var env struct {
		Data episodeDetail `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/episodes/"+strconv.Itoa(episodeID), &env); err != nil {
		return nil, err
	}
	return env.Data.Translations, nil
}

// DownloadSubtitle fetches the subtitle file for a translation. It prefers the
// ASS form (preserves styling; rendered by the frontend SubtitleOverlay via
// ass-compiler) and falls back to the pre-converted VTT when the ASS request
// fails or returns a body that is not valid ASS.
func (c *Client) DownloadSubtitle(ctx context.Context, transID int) ([]byte, string, error) {
	assBody, assErr := c.fetchRaw(ctx, fmt.Sprintf("/episodeTranslations/%d.ass?willcache", transID))
	if assErr == nil && isValidASS(assBody) {
		return assBody, "ass", nil
	}
	vttBody, vttErr := c.fetchRaw(ctx, fmt.Sprintf("/translations/vtt/%d", transID))
	if vttErr != nil {
		if assErr != nil {
			return nil, "", fmt.Errorf("anime365: ass failed (%v) and vtt failed: %w", assErr, vttErr)
		}
		return nil, "", fmt.Errorf("anime365: ass invalid and vtt failed: %w", vttErr)
	}
	return vttBody, "vtt", nil
}

// Ping checks anime365 reachability via a cheap search query. Returns latency.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	q := url.Values{}
	q.Set("query", "naruto")
	q.Set("limit", "1")
	var env struct {
		Data []series `json:"data"`
	}
	err := c.getJSON(ctx, "/api/series?"+q.Encode(), &env)
	return time.Since(start), err
}

func (c *Client) fetchRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anime365: GET %s: status %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// isValidASS does a cheap structural check so an HTML error/paywall page never
// reaches the player as a "subtitle".
func isValidASS(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "[Script Info]") && strings.Contains(s, "Dialogue:")
}
