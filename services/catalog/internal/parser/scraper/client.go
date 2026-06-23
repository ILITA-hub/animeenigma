// Package scraper is a thin HTTP wrapper around the scraper microservice
// at SCRAPER_API_URL (default http://scraper:8088). The wrapper is
// deliberately dumb: it forwards request to the scraper, returns the
// raw response status + body verbatim, and lets the catalog handler
// passthrough 503/200 responses without re-interpreting them.
//
// Design notes (Phase 15 plan 04):
//   - HTTP 503 from the scraper is the canonical "not-yet-implemented"
//     contract for /scraper/{episodes,servers,stream} during Phase 15.
//     We return it as (503, body, nil) — a legitimate response shape,
//     not an error worth signaling.
//   - HTTP 5xx other than 503 means the scraper itself is unhealthy.
//     We return (status, body, wrapped ErrScraperUpstream) so the
//     handler maps it to 502 instead of forwarding 500 verbatim.
//   - 2xx/3xx/4xx are returned with err==nil. The catalog handler decides
//     what to do based on the status.
//   - All requests are context-cancellable via http.NewRequestWithContext.
package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrScraperUpstream is the sentinel for unexpected scraper failures (5xx
// other than 503). The catalog handler can use errors.Is to map it to a
// 502 Bad Gateway response.
var ErrScraperUpstream = errors.New("scraper upstream error")

// maxScraperBody caps the response body the scraper service may stream
// back to the catalog client. Real scraper responses are <50 KiB; a
// misbehaving scraper streaming gigabytes would OOM the catalog without
// this guard. See REVIEW.md WR-04.
const maxScraperBody = 4 << 20 // 4 MiB

// Client is the thin HTTP client targeting the scraper service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client. timeout==0 falls back to a 15s default.
//
// Production wiring: NewClient(cfg.Scraper.APIURL, cfg.Scraper.Timeout)
// where cfg.Scraper.APIURL defaults to http://scraper:8088 and the
// timeout defaults to 15s.
//
// REVIEW.md WR-01: baseURL has trailing slashes trimmed so request URLs
// like baseURL + "/scraper/episodes" never produce "//scraper/episodes"
// (which chi normalizes, but proxies/IDS in the middle may not).
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetEpisodes forwards GET /scraper/episodes?mal_id=<id>&title=<title>&prefer=<prefer>.
// Title is required for providers whose malsync entries are missing (e.g. gogoanime),
// which fall back to fuzzy title search; AnimePahe also uses it for its own fuzzy fallback.
// When exclusive is true, ?exclusive=true is appended so the scraper skips failover.
func (c *Client) GetEpisodes(ctx context.Context, malID int, title string, altTitles []string, prefer string, exclusive bool) (int, []byte, error) {
	q := url.Values{}
	q.Set("mal_id", strconv.Itoa(malID))
	if title != "" {
		q.Set("title", title)
	}
	setAltTitles(q, altTitles)
	if prefer != "" {
		q.Set("prefer", prefer)
	}
	if exclusive {
		q.Set("exclusive", "true")
	}
	return c.doGET(ctx, "/scraper/episodes", q)
}

// setAltTitles sets the comma-joined `title_alt` query param when alternate
// title forms are present (ISS-017). The scraper handler splits on commas.
func setAltTitles(q url.Values, altTitles []string) {
	if len(altTitles) > 0 {
		q.Set("title_alt", strings.Join(altTitles, ","))
	}
}

// GetServers forwards GET /scraper/servers?mal_id=<id>&title=<title>&episode=<ep>&prefer=<prefer>.
// When exclusive is true, ?exclusive=true is appended so the scraper skips failover.
func (c *Client) GetServers(ctx context.Context, malID int, title string, altTitles []string, episodeID, prefer string, exclusive bool) (int, []byte, error) {
	q := url.Values{}
	q.Set("mal_id", strconv.Itoa(malID))
	if title != "" {
		q.Set("title", title)
	}
	setAltTitles(q, altTitles)
	q.Set("episode", episodeID)
	if prefer != "" {
		q.Set("prefer", prefer)
	}
	if exclusive {
		q.Set("exclusive", "true")
	}
	return c.doGET(ctx, "/scraper/servers", q)
}

// GetStream forwards GET /scraper/stream?mal_id=...&title=...&episode=...&server=...&category=...&prefer=...
// When exclusive is true, ?exclusive=true is appended so the scraper skips failover.
// When userKey is non-empty, an X-AE-User header is added so the sidecar can
// enforce per-user session quotas (Phase 2 RAM-budgeted pool).
func (c *Client) GetStream(ctx context.Context, malID int, title string, altTitles []string, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error) {
	q := url.Values{}
	q.Set("mal_id", strconv.Itoa(malID))
	if title != "" {
		q.Set("title", title)
	}
	setAltTitles(q, altTitles)
	q.Set("episode", episodeID)
	q.Set("server", serverID)
	if category != "" {
		q.Set("category", category)
	}
	if prefer != "" {
		q.Set("prefer", prefer)
	}
	if exclusive {
		q.Set("exclusive", "true")
	}
	var hdr http.Header
	if userKey != "" {
		hdr = http.Header{"X-AE-User": []string{userKey}}
	}
	return c.doGET(ctx, "/scraper/stream", q, hdr)
}

// GetHealth forwards GET /scraper/health (no query params).
func (c *Client) GetHealth(ctx context.Context) (int, []byte, error) {
	return c.doGET(ctx, "/scraper/health", nil)
}

// --- 18+ group (/anime18/*) — title-searched (the 18anime provider matches by
// title, not MAL id). The scraper handler still requires a non-empty mal_id
// query param for shape parity, so callers pass the catalog's ShikimoriID
// (the provider ignores it). Served by the scraper's separate adult orchestrator. ---

// GetAnime18Episodes forwards GET /anime18/episodes?mal_id=&title=&title_alt=.
func (c *Client) GetAnime18Episodes(ctx context.Context, malID, title string, altTitles []string) (int, []byte, error) {
	q := url.Values{}
	q.Set("mal_id", malID)
	if title != "" {
		q.Set("title", title)
	}
	setAltTitles(q, altTitles)
	return c.doGET(ctx, "/anime18/episodes", q)
}

// GetAnime18Stream forwards GET /anime18/stream?mal_id=&title=&title_alt=&episode=&server=.
// An empty serverID lets the provider failover mp4upload->turbovid.
func (c *Client) GetAnime18Stream(ctx context.Context, malID, title string, altTitles []string, episodeSlug, serverID string) (int, []byte, error) {
	q := url.Values{}
	q.Set("mal_id", malID)
	if title != "" {
		q.Set("title", title)
	}
	setAltTitles(q, altTitles)
	q.Set("episode", episodeSlug)
	if serverID != "" {
		q.Set("server", serverID)
	}
	return c.doGET(ctx, "/anime18/stream", q)
}

// doGET issues a single GET and returns (status, body, err).
//
// Error semantics:
//   - 503: returned verbatim with err==nil (Phase 15 not-yet-implemented contract).
//   - 5xx other than 503: returned with err wrapping ErrScraperUpstream so the
//     caller can errors.Is-match it and map to 502 Bad Gateway.
//   - 2xx/3xx/4xx: returned with err==nil.
//   - Transport / context errors: returned with status=0, body=nil, err=cause.
//
// The optional hdr argument merges extra headers into the outbound request.
// Callers that need no extra headers may omit it entirely (variadic keeps
// the existing GetEpisodes/GetServers/GetHealth/anime18 call-sites unchanged).
func (c *Client) doGET(ctx context.Context, path string, q url.Values, hdr ...http.Header) (int, []byte, error) {
	full := c.baseURL + path
	if q != nil && len(q) > 0 {
		full += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build scraper request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if len(hdr) > 0 && hdr[0] != nil {
		for k, vs := range hdr[0] {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("scraper http: %w", err)
	}
	defer func() {
		// REVIEW.md WR-04: drain any unread bytes so the keep-alive
		// connection can be reused even on partial-body failures.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Cap the body so a misbehaving scraper cannot OOM the catalog.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxScraperBody))
	if readErr != nil {
		return resp.StatusCode, nil, fmt.Errorf("scraper read body: %w", readErr)
	}

	// 503 is the canonical not-yet-implemented contract — forward verbatim.
	if resp.StatusCode == http.StatusServiceUnavailable {
		return resp.StatusCode, body, nil
	}
	// Other 5xx is a real upstream failure worth signaling.
	if resp.StatusCode >= 500 {
		return resp.StatusCode, body, fmt.Errorf("scraper upstream %d: %w", resp.StatusCode, ErrScraperUpstream)
	}
	return resp.StatusCode, body, nil
}
