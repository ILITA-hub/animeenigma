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
	"bytes"
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
	MinIOURL      string `json:"minio_url"`
	DurationSec   int    `json:"duration_sec"`
	SizeBytes     int64  `json:"size_bytes"`
	StoryboardURL string `json:"storyboard_url,omitempty"`
}

// envelope mirrors libs/httputil.Response — Success + Data only; we
// ignore error/meta because GetEpisode only inspects 2xx bodies.
type envelope struct {
	Success bool             `json:"success"`
	Data    *EpisodeResponse `json:"data"`
}

// EpisodeListItem is one entry in the List response (episode number +
// playlist URL). Mirrors services/library/internal/handler.episodeListItem.
type EpisodeListItem struct {
	EpisodeNumber int    `json:"episode_number"`
	MinIOURL      string `json:"minio_url"`
	DurationSec   int    `json:"duration_sec"`
}

// listEnvelope wraps the {episodes:[...]} list payload.
type listEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Episodes []EpisodeListItem `json:"episodes"`
	} `json:"data"`
}

// RecentEpisode is one entry from GET /internal/library/recent-episodes —
// the (anime, episode) the playback probe should target.
type RecentEpisode struct {
	ShikimoriID   string `json:"shikimori_id"`
	EpisodeNumber int    `json:"episode_number"`
}

// recentEnvelope wraps the {episodes:[{shikimori_id,episode_number}]} payload.
type recentEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Episodes []RecentEpisode `json:"episodes"`
	} `json:"data"`
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

// ListEpisodes fetches every locally-encoded episode for an anime via
// GET /api/library/episodes/{shikimoriID}. Behavior mirrors GetEpisode:
//
//   - 200 → returns the (possibly empty) slice. An empty list is a
//     legitimate "nothing local yet" state, NOT an error.
//   - 5xx → (nil, wrapped error) — transient; caller should not cache.
//   - other non-2xx → (nil, wrapped error).
//   - transport / decode error / timeout → (nil, wrapped error).
//
// Empty shikimoriID is rejected before any request is issued.
func (c *Client) ListEpisodes(ctx context.Context, shikimoriID string) ([]EpisodeListItem, error) {
	if shikimoriID == "" {
		return nil, fmt.Errorf("library: empty shikimori_id")
	}
	u := fmt.Sprintf("%s/api/library/episodes/%s", c.cfg.APIURL, url.PathEscape(shikimoriID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("library: build list request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("library: do list request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusOK:
		var env listEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return nil, fmt.Errorf("library: decode list 200 body: %w", err)
		}
		return env.Data.Episodes, nil
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("library: list upstream %d", resp.StatusCode)
	default:
		return nil, fmt.Errorf("library: list unexpected status %d", resp.StatusCode)
	}
}

// postInternal POSTs a JSON body to a Docker-network-only library
// /internal/* endpoint on the existing cfg.APIURL base (reusing the
// bounded httpClient.Timeout). It drains+closes the body — the
// endpoints reply {ok:true}, so only the status code is inspected — and
// returns a wrapped error on any non-2xx so a best-effort caller can
// log+drop it. NO response-body parsing.
//
// Phase 08-03 (workstream auto-torrent / serve-signal): the serve-signal
// producers (RecordFetch / RecordDemand) are best-effort — the caller in
// raw_resolver.go discards the returned error and never fails a playback
// resolution because this call errored.
func (c *Client) postInternal(ctx context.Context, path string, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("library: marshal %s body: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("library: build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("library: do %s request: %w", path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("library: %s non-2xx %d", path, resp.StatusCode)
	}
	return nil
}

// RecordFetch fires the HIT serve-signal: POST /internal/library/
// autocache/fetch {mal_id, episode}. Best-effort — the library side
// bumps last_fetch_at/fetch_count + counts serve_total{hit}. The caller
// drops the returned error; a non-2xx/transport error never fails the
// playback resolution. Docker-network-only (NOT gateway-proxied).
func (c *Client) RecordFetch(ctx context.Context, malID string, episode int) error {
	return c.postInternal(ctx, "/internal/library/autocache/fetch", map[string]any{
		"mal_id":  malID,
		"episode": episode,
	})
}

// RecordDemand fires the MISS serve-signal: POST /internal/library/
// autocache/demand {mal_id, episode, reason, titles}. Best-effort — the library
// side records a wanted item + counts serve_total{miss}. titles is the ordered
// fallback title list (name_jp → romaji → name_en) the library Planner searches
// trackers with (the library has no anime titles of its own); empty entries are
// dropped server-side. The caller drops the returned error; a non-2xx/transport
// error never fails the playback resolution + failover. Docker-network-only.
func (c *Client) RecordDemand(ctx context.Context, malID string, episode int, reason string, titles []string, trigger *DemandTrigger) error {
	body := map[string]any{
		"mal_id":  malID,
		"episode": episode,
		"reason":  reason,
		"titles":  titles,
	}
	if trigger != nil {
		body["trigger"] = map[string]any{
			"user_id":         trigger.UserID,
			"username":        trigger.Username,
			"player":          trigger.Player,
			"language":        trigger.Language,
			"watch_type":      trigger.WatchType,
			"watched_episode": trigger.WatchedEpisode,
		}
	}
	return c.postInternal(ctx, "/internal/library/autocache/demand", body)
}

// DemandTrigger is the cause→effect watcher context attached to a backfill demand
// (the user who hit an ae serve-MISS for this episode). The library appends it to
// autocache_trigger_log so the dashboard can show the playback that caused the
// download. For backfill the watched + target episode are the same (the requested
// episode).
type DemandTrigger struct {
	UserID         string
	Username       string
	Player         string
	Language       string
	WatchType      string
	WatchedEpisode int
}

// RecentEpisodes fetches the newest distinct-anime library uploads via
// GET /internal/library/recent-episodes?limit=N, for the analytics playback
// probe's ae target set. Behavior mirrors the other readers:
//
//   - 200 → returns the (possibly empty) slice.
//   - 404 / unconfigured client → (nil, nil): legitimate empty state.
//   - 5xx / other non-2xx / transport / decode error → (nil, wrapped error).
func (c *Client) RecentEpisodes(ctx context.Context, limit int) ([]RecentEpisode, error) {
	if c == nil || c.cfg.APIURL == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	u := fmt.Sprintf("%s/internal/library/recent-episodes?limit=%d", c.cfg.APIURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("library: build recent request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("library: recent do request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusOK:
		var env recentEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return nil, fmt.Errorf("library: decode recent 200 body: %w", err)
		}
		return env.Data.Episodes, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("library: recent upstream %d", resp.StatusCode)
	default:
		return nil, fmt.Errorf("library: recent unexpected status %d", resp.StatusCode)
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
