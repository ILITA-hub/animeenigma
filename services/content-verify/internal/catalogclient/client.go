// Package catalogclient resolves queue membership, capability structure, and
// per-provider streams. Public routes (/api/anime/*) go through the GATEWAY —
// the exact end-to-end path aePlayer uses — so a probe verdict can never
// diverge from what a real player request would get. Only the internal
// membership route talks to catalog directly (it has no gateway exposure).
// Scraper calls still pass prefer=<provider>&exclusive=true: same chain, but
// failover must not silently answer for a different provider than the unit
// being attributed.
package catalogclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotFound = errors.New("catalogclient: not found")

// Per-call deadlines. Metadata calls are cheap catalog lookups; stream
// resolution for engine=browser providers (gogoanime/animepahe/miruro/
// nineanime) runs a Camoufox session end-to-end and routinely needs 45-90s —
// a short client timeout here reports healthy providers as unreachable
// (2026-07-17 live-E2E finding: 20s cancelled resolves the player completes).
const (
	metaTimeout   = 30 * time.Second
	streamTimeout = 120 * time.Second
)

type Client struct {
	catalog string // internal catalog base (membership only)
	public  string // gateway base — same routes aePlayer calls
	hc      *http.Client
}

func New(catalogURL, publicURL string, hc *http.Client) *Client {
	if hc == nil {
		// Ceiling only — the per-call ctx deadlines above are the real bound.
		hc = &http.Client{Timeout: streamTimeout + 30*time.Second}
	}
	return &Client{catalog: strings.TrimRight(catalogURL, "/"),
		public: strings.TrimRight(publicURL, "/"), hc: hc}
}

type MembershipRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EpisodesAired int    `json:"episodes_aired"`
}
type Membership struct {
	Ongoing []MembershipRow `json:"ongoing"`
	Top     []MembershipRow `json:"top"`
}

type Cap struct {
	Provider string   `json:"provider"`
	State    string   `json:"state"`
	Group    string   `json:"group"`
	Lang     string   `json:"lang"`
	Audios   []string `json:"audios"`
}

type KodikTranslation struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Type          string `json:"type"` // voice | subtitles (claim only — we verify)
	EpisodesCount int    `json:"episodes_count"`
}

type ScraperEpisode struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
}
type ScraperServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type TimeRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type TrackInfo struct {
	File  string `json:"file"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}
type Stream struct {
	URL     string
	Exp     string
	Sig     string
	Referer string
	Type    string
	Intro   *TimeRange
	Outro   *TimeRange
	Tracks  []TrackInfo
}

// getJSON fetches u and decodes the {"success","data"} envelope into dst
// (dst receives the "data" value). 404 → ErrNotFound. timeout bounds this
// call even when the caller ctx is unbounded (the claim path's root ctx).
func (c *Client) getJSON(ctx context.Context, u string, timeout time.Duration, dst any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("catalogclient: %s -> %d", u, resp.StatusCode)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, dst)
}

func (c *Client) Membership(ctx context.Context) (*Membership, error) {
	var m Membership
	if err := c.getJSON(ctx, c.catalog+"/internal/verify/membership", metaTimeout, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) Capabilities(ctx context.Context, animeID string) ([]Cap, error) {
	var data struct {
		Families []struct {
			Providers []Cap `json:"providers"`
		} `json:"families"`
	}
	if err := c.getJSON(ctx, c.public+"/api/anime/"+url.PathEscape(animeID)+"/capabilities", metaTimeout, &data); err != nil {
		return nil, err
	}
	var caps []Cap
	for _, f := range data.Families {
		caps = append(caps, f.Providers...)
	}
	return caps, nil
}

func (c *Client) KodikTranslations(ctx context.Context, animeID string) ([]KodikTranslation, error) {
	var tr []KodikTranslation
	if err := c.getJSON(ctx, c.public+"/api/anime/"+url.PathEscape(animeID)+"/kodik/translations", metaTimeout, &tr); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return tr, nil
}

func (c *Client) ScraperEpisodes(ctx context.Context, animeID, provider string) ([]ScraperEpisode, error) {
	var data struct {
		Episodes []ScraperEpisode `json:"episodes"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/scraper/episodes?prefer=%s&exclusive=true", c.public, url.PathEscape(animeID), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, metaTimeout, &data); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil // provider has no match — not an error
		}
		return nil, err
	}
	return data.Episodes, nil
}

func (c *Client) ScraperServers(ctx context.Context, animeID, episodeID, provider string) ([]ScraperServer, error) {
	var data struct {
		Servers []ScraperServer `json:"servers"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/scraper/servers?episode=%s&prefer=%s&exclusive=true",
		c.public, url.PathEscape(animeID), url.QueryEscape(episodeID), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, metaTimeout, &data); err != nil {
		return nil, err
	}
	return data.Servers, nil
}

func (c *Client) ScraperStream(ctx context.Context, animeID, episodeID, serverID, category, provider string) (*Stream, error) {
	var data struct {
		Stream struct {
			Headers map[string]string `json:"headers"`
			Sources []struct {
				URL  string `json:"url"`
				Exp  string `json:"exp"`
				Sig  string `json:"sig"`
				Type string `json:"type"`
			} `json:"sources"`
			Tracks []TrackInfo `json:"tracks"`
			Intro  *TimeRange  `json:"intro"`
			Outro  *TimeRange  `json:"outro"`
		} `json:"stream"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/scraper/stream?episode=%s&server=%s&category=%s&prefer=%s&exclusive=true",
		c.public, url.PathEscape(animeID), url.QueryEscape(episodeID), url.QueryEscape(serverID), url.QueryEscape(category), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, streamTimeout, &data); err != nil {
		return nil, err
	}
	if len(data.Stream.Sources) == 0 {
		return nil, ErrNotFound
	}
	src := data.Stream.Sources[0]
	return &Stream{URL: src.URL, Exp: src.Exp, Sig: src.Sig, Type: src.Type,
		Referer: data.Stream.Headers["Referer"], Tracks: data.Stream.Tracks,
		Intro: data.Stream.Intro, Outro: data.Stream.Outro}, nil
}

func (c *Client) KodikStream(ctx context.Context, animeID string, episode, translation int) (*Stream, error) {
	var data struct {
		StreamURL string `json:"stream_url"`
		Referer   string `json:"referer"`
		Exp       string `json:"exp"`
		Sig       string `json:"sig"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/kodik/stream?episode=%d&translation=%d", c.public, url.PathEscape(animeID), episode, translation)
	if err := c.getJSON(ctx, u, streamTimeout, &data); err != nil {
		return nil, err
	}
	if data.StreamURL == "" {
		return nil, ErrNotFound
	}
	return &Stream{URL: data.StreamURL, Exp: data.Exp, Sig: data.Sig, Referer: data.Referer, Type: "hls"}, nil
}

func (c *Client) AnimejoyEpisodes(ctx context.Context, animeID, provider string) ([]int, error) {
	var data struct {
		Episodes []int `json:"episodes"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/%s/episodes", c.public, url.PathEscape(animeID), url.PathEscape(provider))
	if err := c.getJSON(ctx, u, metaTimeout, &data); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return data.Episodes, nil
}

func (c *Client) AnimejoyStream(ctx context.Context, animeID, provider string, episode int) (*Stream, error) {
	var data struct {
		URL     string `json:"url"`
		Referer string `json:"referer"`
		Exp     string `json:"exp"`
		Sig     string `json:"sig"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/%s/stream?episode=%d", c.public, url.PathEscape(animeID), url.PathEscape(provider), episode)
	if err := c.getJSON(ctx, u, streamTimeout, &data); err != nil {
		return nil, err
	}
	if data.URL == "" {
		return nil, ErrNotFound
	}
	return &Stream{URL: data.URL, Exp: data.Exp, Sig: data.Sig, Referer: data.Referer, Type: "mp4"}, nil
}
