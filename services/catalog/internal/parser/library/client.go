// Package library is a thin HTTP client for the self-hosted library
// service (workstream raw-jp / v0.2 / Phase 06). It exposes only the
// surface the catalog's hybrid raw resolver needs:
//
//   - GetEpisode(ctx, shikimoriID, episode) — the lookup the resolver
//     runs FIRST on every /raw/stream request. 404 is treated as a
//     legitimate empty state (return nil, nil); 5xx / transport
//     errors / timeouts are returned wrapped.
//   - Ping(ctx)                              — used by an external
//     health-check goroutine to short-circuit when the library is
//     known-down. NOT on the request path.
//
// Per-request timeout is bounded by http.Client.Timeout (defaults to
// 2 seconds — the library is on the same docker network, so any
// longer wait means it's actually down). The library envelope wraps
// the payload as { "success": true, "data": {...} } via
// libs/httputil.OK; we decode into an envelope struct and return the
// inner pointer.
package library

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

// Config controls the library client. APIURL is the base URL such as
// "http://library:8089"; a trailing slash is trimmed in NewClient.
// Timeout defaults to 2 seconds when zero.
type Config struct {
	APIURL  string
	Timeout time.Duration
}

// Client is the thin HTTP wrapper around the library service.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// EpisodeResponse is the shape returned by GET /api/library/episodes/
// {shikimori_id}/{episode}. The library service wraps it in libs/
// httputil's standard envelope; we decode into the envelope and
// return the inner payload.
type EpisodeResponse struct {
	MinIOURL    string `json:"minio_url"`
	DurationSec int    `json:"duration_sec"`
	SizeBytes   int64  `json:"size_bytes"`
}

// envelope mirrors libs/httputil.Response — Success + Data only; we
// ignore error/meta because GetEpisode only inspects 2xx bodies.
type envelope struct {
	Success bool             `json:"success"`
	Data    *EpisodeResponse `json:"data"`
}

// NewClient constructs a Client from a Config. Empty timeout falls
// back to 2 seconds (SPEC-locked per-request cap). Trailing slash on
// APIURL is trimmed so URL composition never produces a double slash.
func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	cfg.APIURL = strings.TrimRight(cfg.APIURL, "/")
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// GetEpisode fetches the library_episodes row for (shikimoriID,
// episode) over HTTP. Behavior:
//
//   - 200 → returns a non-nil *EpisodeResponse parsed from the
//     envelope. MinIOURL is validated non-empty (defensive against
//     partial server bugs).
//   - 404 → returns (nil, nil): legitimate empty state (no library
//     row yet).
//   - 5xx → returns (nil, wrapped error) — transient; caller
//     should NOT cache the decision.
//   - other 4xx → returns (nil, wrapped error).
//   - transport / decode error / timeout → returns (nil, wrapped
//     error). Timeouts are NEVER silently treated as 404.
//
// Empty shikimoriID and non-positive episode are rejected before any
// request is issued.
func (c *Client) GetEpisode(ctx context.Context, shikimoriID string, episode int) (*EpisodeResponse, error) {
	if shikimoriID == "" {
		return nil, fmt.Errorf("library: empty shikimori_id")
	}
	if episode <= 0 {
		return nil, fmt.Errorf("library: episode must be > 0, got %d", episode)
	}

	u := fmt.Sprintf("%s/api/library/episodes/%s/%s",
		c.cfg.APIURL,
		url.PathEscape(shikimoriID),
		strconv.Itoa(episode),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("library: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("library: do request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusOK:
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return nil, fmt.Errorf("library: decode 200 body: %w", err)
		}
		if env.Data == nil {
			return nil, fmt.Errorf("library: 200 body has nil data")
		}
		if env.Data.MinIOURL == "" {
			return nil, fmt.Errorf("library: empty minio_url in 200 body")
		}
		return env.Data, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("library: upstream %d", resp.StatusCode)
	default:
		return nil, fmt.Errorf("library: unexpected status %d", resp.StatusCode)
	}
}

// Ping issues a GET /health on the configured library APIURL. Returns
// nil on a 2xx response within the configured Timeout, otherwise a
// wrapped error. NOT on the request path — used by an external
// health-check goroutine (out of scope here; wired in main.go).
func (c *Client) Ping(ctx context.Context) error {
	u := c.cfg.APIURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("library: ping build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("library: ping do: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("library: ping non-2xx %d", resp.StatusCode)
	}
	return nil
}
