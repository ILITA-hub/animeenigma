package animejoy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the AnimeJoy origin. It is RKN-flagged and a DLE moving
	// target, so the base is overridable on the Client (mirror fallback
	// animejoya.ru is a later-phase concern).
	DefaultBaseURL = "https://animejoy.ru"

	// defaultUserAgent is a plain desktop UA; AnimeJoy serves search + playlist
	// AJAX to ordinary clients (no CF challenge from our egress per AUTO-084).
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0 Safari/537.36"

	// maxBodyBytes caps search/playlist response reads (search pages run ~70 KiB;
	// 4 MiB is a generous ceiling that still guards against a runaway body).
	maxBodyBytes = 4 << 20
)

// Client is the catalog-side AnimeJoy discovery client. It holds the configured
// base URL and a shared *http.Client; like the Kodik client it is a singleton
// reused across concurrent catalog requests, so it carries no mutable per-request
// state. Phase 1 exposes the discovery surface (ResolveNewsID / FetchPlaylist)
// plus the pure parsers; leg resolution and caching arrive in later phases.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client against DefaultBaseURL with a 30s timeout (matching
// the Kodik client).
func NewClient() *Client {
	return &Client{
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithBaseURL builds a Client against a custom base (used by tests to
// point at an httptest server, and by the mirror-fallback path later).
func NewClientWithBaseURL(baseURL string) *Client {
	c := NewClient()
	if strings.TrimSpace(baseURL) != "" {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
	return c
}

// base returns the configured base URL, trimmed of any trailing slash.
func (c *Client) base() string {
	if c.baseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.baseURL, "/")
}

// getBody issues a GET to url with the default User-Agent plus any extra headers,
// requires a 200, and returns the response body read through the shared
// maxBodyBytes LimitReader. It is the single HTTP-fetch primitive shared by the
// search / playlist / leg-resolver wrappers; each caller layers its own
// parse-step error context on top. Transport, status, and read errors carry a
// generic "animejoy:" prefix.
func (c *Client) getBody(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("animejoy: build request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("animejoy: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("animejoy: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("animejoy: read body: %w", err)
	}
	return body, nil
}
