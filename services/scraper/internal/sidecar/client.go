// Package sidecar is the HTTP client to the Camoufox stealth-scraper sidecar
// (services/stealth-scraper). Providers whose DB `engine` column is "browser"
// delegate stream extraction here: a curl-class Go client cannot reach players
// whose stream id + CDN host are built at runtime in JS (e.g. gogoanime →
// megaplay), so the sidecar drives a real browser and returns a stream session.
//
// The returned Stream's single Source points at the sidecar's own /hls proxy
// path (it restreams the playlist + segments through the same browser context
// that solved the player — same exit IP, cookies, TLS), so downstream consumers
// fetch a normal HLS playlist.
//
// Error mapping: 404 → ErrNotFound, 5xx / transport → ErrProviderDown.
package sidecar

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/userkey"
)

const maxBody = 1 << 20 // 1 MiB — resolve responses are small JSON.

const maxFetchBody = 16 << 20 // 16 MiB — discovery pages (HTML/JSON) base64-inflated.

// wedgedKinds is the set of sidecar `kind` values that mean "this provider is
// wedged / over budget" (as opposed to a transient upstream challenge or a
// genuine stream failure). The Go circuit breaker (internal/health.Breaker)
// counts these per provider; the legacy `exhausted` alias is normalized to
// `pool_exhausted` so the breaker works whether or not the Phase-1 sidecar
// rename has shipped yet.
//
// Phase 1 sidecar kinds: provider_wedged, pool_exhausted (was `exhausted`).
// Phase 2 sidecar kinds: capacity, user_quota.
var wedgedKinds = map[string]string{
	"provider_wedged": "provider_wedged",
	"pool_exhausted":  "pool_exhausted",
	"exhausted":       "pool_exhausted", // legacy alias (pre-Phase-1 sidecar)
	"capacity":        "capacity",
	"user_quota":      "user_quota",
	// Phase 3 (graceful degradation): sidecar refuses NEW resolves while the
	// host is at Critical pressure. Treated as wedged so the breaker parks the
	// provider (half-open retry re-probes once pressure clears).
	"degraded": "degraded",
}

// ProviderWedgedError wraps domain.ErrProviderDown for the subset of sidecar
// failures that indicate the browser pool is wedged or over budget for this
// provider. It still satisfies errors.Is(err, domain.ErrProviderDown) (so the
// orchestrator's failover classifier treats it as retryable exactly as before),
// but it ALSO carries the machine-readable Kind so the circuit breaker can
// inspect the cause via sidecar.IsWedged / errors.As.
type ProviderWedgedError struct {
	Kind string
	err  error // the underlying domain.WrapProviderDown(...) value
}

func (e *ProviderWedgedError) Error() string {
	return fmt.Sprintf("sidecar provider wedged (kind=%s): %v", e.Kind, e.err)
}

// Unwrap exposes the wrapped domain.ErrProviderDown chain so errors.Is keeps
// matching the sentinel.
func (e *ProviderWedgedError) Unwrap() error { return e.err }

// IsWedged reports whether err is (or wraps) a *ProviderWedgedError and returns
// its normalized Kind. The breaker uses this; non-wedged errors return ("",false).
func IsWedged(err error) (string, bool) {
	var pwe *ProviderWedgedError
	if errors.As(err, &pwe) {
		return pwe.Kind, true
	}
	return "", false
}

// classifyDown builds the error for a sidecar non-OK response. When `kind` is a
// wedged kind it returns a *ProviderWedgedError (wrapping the ErrProviderDown
// value `base`); otherwise it returns `base` unchanged. Centralizes the wedged
// decision so resolve() and Fetch() stay in sync.
func classifyDown(kind string, base error) error {
	if norm, ok := wedgedKinds[kind]; ok {
		return &ProviderWedgedError{Kind: norm, err: base}
	}
	return base
}

// Client is a thin HTTP wrapper around the stealth-scraper sidecar.
type Client struct {
	baseURL string
	http    *http.Client
}

// New constructs a Client. baseURL is the sidecar service URL (e.g.
// http://stealth-scraper:3000). timeout bounds one /resolve call (browser
// resolution is slow — allow generously).
func New(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

type resolveRequest struct {
	Provider string `json:"provider"`
	EmbedURL string `json:"embed_url,omitempty"`
	Title    string `json:"title,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Category string `json:"category,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	UserKey  string `json:"user_key,omitempty"`
}

type subtitle struct {
	URL     string `json:"url"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

type timeRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type sessionData struct {
	SessionID         string     `json:"session_id"`
	MasterURL         string     `json:"master_url"`
	PlaylistProxyPath string     `json:"playlist_proxy_path"`
	Referer           string     `json:"referer"`
	Subtitles         []subtitle `json:"subtitles"`
	Intro             *timeRange `json:"intro"`
	Outro             *timeRange `json:"outro"`
}

type resolveResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error"`
	Kind    string      `json:"kind"`
	Data    sessionData `json:"data"`
}

type fetchRequest struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Method   string `json:"method,omitempty"`
	UserKey  string `json:"user_key,omitempty"`
}

type fetchResponse struct {
	Success bool              `json:"success"`
	Error   string            `json:"error"`
	Kind    string            `json:"kind"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"` // allowlisted response headers (lowercase keys)
	Body    string            `json:"body"`    // base64
}

// ResolveEmbed resolves a known embed/wrapper URL (a provider server ID) to a
// playable Stream via the sidecar. provider is the recipe key (e.g. "gogoanime").
func (c *Client) ResolveEmbed(
	ctx context.Context, provider, embedURL string, category domain.Category, baseURL string,
) (*domain.Stream, error) {
	return c.resolve(ctx, resolveRequest{
		Provider: provider,
		EmbedURL: embedURL,
		Category: string(category),
		BaseURL:  baseURL,
		UserKey:  userkey.FromContext(ctx),
	})
}

// Fetch routes one GET through the sidecar's warm browser session for `provider`
// (a site whose discovery is challenge-gated to curl/Go) and returns the RAW
// body + the UPSTREAM status. Only sidecar-level failures (challenge / pool
// exhausted / host denied / transport) return an error; an upstream 4xx/5xx is
// returned as (status, body, nil) so the provider keeps its own status handling.
//
// This is a thin wrapper over FetchWithHeaders for callers (gogoanime/nineanime/
// animepahe) that only need status+body.
func (c *Client) Fetch(ctx context.Context, provider, rawURL string) (int, []byte, error) {
	status, _, body, err := c.FetchWithHeaders(ctx, provider, rawURL)
	return status, body, err
}

// FetchWithHeaders is Fetch plus the sidecar-returned allowlisted response
// headers (lowercase keys; see stealth-scraper _FETCH_HEADER_ALLOWLIST). Some
// providers carry response semantics in a header rather than the body — miruro's
// secure-pipe marks its transport encoding in `x-obfuscated`, which its Go
// decoder needs. The map is never nil on success (empty when no allowlisted
// header was present).
func (c *Client) FetchWithHeaders(ctx context.Context, provider, rawURL string) (int, map[string]string, []byte, error) {
	reqBody, err := json.Marshal(fetchRequest{
		Provider: provider, URL: rawURL, Method: "GET", UserKey: userkey.FromContext(ctx),
	})
	if err != nil {
		return 0, nil, nil, domain.WrapProviderDown(err, "sidecar: marshal fetch")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/fetch", bytes.NewReader(reqBody))
	if err != nil {
		return 0, nil, nil, domain.WrapProviderDown(err, "sidecar: build fetch request")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, nil, nil, domain.WrapProviderDown(err, "sidecar: fetch request")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
	if err != nil {
		return 0, nil, nil, domain.WrapProviderDown(err, "sidecar: read fetch body")
	}

	var out fetchResponse
	decodeErr := json.Unmarshal(raw, &out)

	if resp.StatusCode == http.StatusNotFound {
		return 0, nil, nil, domain.WrapNotFound(
			fmt.Errorf("sidecar fetch 404 (kind=%s): %s", out.Kind, snippet(raw)), "sidecar: fetch not found")
	}
	if resp.StatusCode != http.StatusOK {
		base := domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: fetch")
		return 0, nil, nil, classifyDown(out.Kind, base)
	}
	if decodeErr != nil {
		return 0, nil, nil, domain.WrapProviderDown(decodeErr, "sidecar: decode fetch response")
	}
	if !out.Success {
		return 0, nil, nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch unsuccessful (kind=%s): %s", out.Kind, out.Error), "sidecar: fetch")
	}
	body, err := base64.StdEncoding.DecodeString(out.Body)
	if err != nil {
		return 0, nil, nil, domain.WrapProviderDown(err, "sidecar: decode fetch body b64")
	}
	headers := out.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	return out.Status, headers, body, nil
}

func (c *Client) resolve(ctx context.Context, req resolveRequest) (*domain.Stream, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "sidecar: marshal request")
	}
	url := c.baseURL + "/resolve"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "sidecar: build request")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "sidecar: request")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "sidecar: read body")
	}

	// Best-effort pre-parse so the sidecar's `kind`
	// (challenge|error|internal|exhausted|not_found) is surfaced in the error —
	// a Cloudflare challenge storm reads differently from a recipe regression on
	// the playback-health dashboard, even though both map to ErrProviderDown for
	// failover.
	var out resolveResponse
	decodeErr := json.Unmarshal(raw, &out)

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.WrapNotFound(
			fmt.Errorf("sidecar 404 (kind=%s): %s", out.Kind, snippet(raw)), "sidecar: not found")
	}
	if resp.StatusCode != http.StatusOK {
		base := domain.WrapProviderDown(
			fmt.Errorf("sidecar status %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: resolve",
		)
		return nil, classifyDown(out.Kind, base)
	}
	if decodeErr != nil {
		return nil, domain.WrapProviderDown(decodeErr, "sidecar: decode response")
	}
	if !out.Success || out.Data.PlaylistProxyPath == "" {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar unsuccessful (kind=%s): %s", out.Kind, out.Error), "sidecar: resolve",
		)
	}

	return c.toStream(out.Data), nil
}

// toStream maps a sidecar resolution to a domain.Stream. The Source URL is the
// sidecar's OWN /hls restream path (not the real CDN): the megaplay CDNs sit
// behind Cloudflare bot-management that gates on the TLS/HTTP2 fingerprint, so
// the playlist + segments can ONLY be fetched through the resolving Camoufox
// browser's in-page fetch — a direct Go/curl fetch gets a 403 "Attention
// Required" page (verified 2026-06-20). The downstream streaming HLS proxy
// fetches this sidecar /hls URL (the `stealth-scraper` host is allowlisted) and
// rewrites the returned playlist's child URIs to route segment GETs back
// through the same path.
func (c *Client) toStream(d sessionData) *domain.Stream {
	tracks := make([]domain.Track, 0, len(d.Subtitles))
	for _, s := range d.Subtitles {
		if s.URL == "" {
			continue
		}
		tracks = append(tracks, domain.Track{
			File: s.URL, Label: s.Label, Kind: "captions", Default: s.Default,
		})
	}
	st := &domain.Stream{
		Sources: []domain.Source{{URL: c.baseURL + d.PlaylistProxyPath, Type: "hls"}},
		Tracks:  tracks,
	}
	if d.Intro != nil {
		st.Intro = &domain.TimeRange{Start: d.Intro.Start, End: d.Intro.End}
	}
	if d.Outro != nil {
		st.Outro = &domain.TimeRange{Start: d.Outro.Start, End: d.Outro.End}
	}
	return st
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

// SIDFromProxyURL extracts the stealth-scraper session id from a sidecar
// stream-proxy URL (http://stealth-scraper:3000/hls?sid=...&url=...). Returns
// ok=false for every non-sidecar URL so callers can gate cheaply.
func SIDFromProxyURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path != "/hls" {
		return "", false
	}
	sid := u.Query().Get("sid")
	return sid, sid != ""
}

// SessionAlive reports the sidecar's liveness verdict for sid: "alive",
// "rehydratable" or "gone". FAIL-OPEN: any transport/decode error returns
// "alive" — a sidecar hiccup must not stampede cache re-resolves.
func (c *Client) SessionAlive(ctx context.Context, sid string) string {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/session/"+url.PathEscape(sid)+"/alive", nil)
	if err != nil {
		return "alive"
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "alive"
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "alive"
	}
	var out struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&out); err != nil {
		return "alive"
	}
	switch out.State {
	case "alive", "rehydratable", "gone":
		return out.State
	}
	return "alive"
}
