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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/userkey"
)

const maxBody = 1 << 20 // 1 MiB — resolve responses are small JSON.

const maxFetchBody = 16 << 20 // 16 MiB — discovery pages (HTML/JSON) base64-inflated.

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
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Kind    string `json:"kind"`
	Status  int    `json:"status"`
	Body    string `json:"body"` // base64
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
func (c *Client) Fetch(ctx context.Context, provider, rawURL string) (int, []byte, error) {
	reqBody, err := json.Marshal(fetchRequest{
		Provider: provider, URL: rawURL, Method: "GET", UserKey: userkey.FromContext(ctx),
	})
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: marshal fetch")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/fetch", bytes.NewReader(reqBody))
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: build fetch request")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: fetch request")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: read fetch body")
	}

	var out fetchResponse
	decodeErr := json.Unmarshal(raw, &out)

	if resp.StatusCode == http.StatusNotFound {
		return 0, nil, domain.WrapNotFound(
			fmt.Errorf("sidecar fetch 404 (kind=%s): %s", out.Kind, snippet(raw)), "sidecar: fetch not found")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: fetch")
	}
	if decodeErr != nil {
		return 0, nil, domain.WrapProviderDown(decodeErr, "sidecar: decode fetch response")
	}
	if !out.Success {
		return 0, nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch unsuccessful (kind=%s): %s", out.Kind, out.Error), "sidecar: fetch")
	}
	body, err := base64.StdEncoding.DecodeString(out.Body)
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: decode fetch body b64")
	}
	return out.Status, body, nil
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
		return nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar status %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: resolve",
		)
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
