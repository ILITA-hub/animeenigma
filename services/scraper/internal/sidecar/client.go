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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

const maxBody = 1 << 20 // 1 MiB — resolve responses are small JSON.

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
	})
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
	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.WrapNotFound(fmt.Errorf("sidecar 404: %s", snippet(raw)), "sidecar: not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar status %d: %s", resp.StatusCode, snippet(raw)), "sidecar: resolve",
		)
	}

	var out resolveResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, domain.WrapProviderDown(err, "sidecar: decode response")
	}
	if !out.Success || out.Data.MasterURL == "" {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar unsuccessful: %s", out.Error), "sidecar: resolve",
		)
	}

	return c.toStream(out.Data), nil
}

// toStream maps a sidecar resolution to a domain.Stream. The Source URL is the
// REAL CDN master the browser resolved. The CDN serves the playlist + segments
// to any client bearing the megaplay Referer (no browser/clearance needed for
// streaming — verified 2026-06-20), so the downstream streaming HLS proxy
// fetches it directly using the Referer header; catalog signs the URL so the
// proxy's provenance gate trusts it (same pattern as the other scraper CDNs).
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
		Sources: []domain.Source{{URL: d.MasterURL, Type: "hls"}},
		Tracks:  tracks,
	}
	if d.Referer != "" {
		st.Headers = map[string]string{"Referer": d.Referer}
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
