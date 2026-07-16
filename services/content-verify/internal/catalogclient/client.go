// Package catalogclient talks to catalog: queue membership, capability
// structure, and per-provider stream resolution (scraper / kodik / animejoy
// legs). All scraper calls pass prefer=<provider>&exclusive=true so the
// probe result is attributable to exactly one provider.
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

type Client struct {
	base string
	hc   *http.Client
}

func New(catalogURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{base: strings.TrimRight(catalogURL, "/"), hc: hc}
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
// (dst receives the "data" value). 404 → ErrNotFound.
func (c *Client) getJSON(ctx context.Context, u string, dst any) error {
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
	if err := c.getJSON(ctx, c.base+"/internal/verify/membership", &m); err != nil {
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
	if err := c.getJSON(ctx, c.base+"/api/anime/"+url.PathEscape(animeID)+"/capabilities", &data); err != nil {
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
	if err := c.getJSON(ctx, c.base+"/api/anime/"+url.PathEscape(animeID)+"/kodik/translations", &tr); err != nil {
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
	u := fmt.Sprintf("%s/api/anime/%s/scraper/episodes?prefer=%s&exclusive=true", c.base, url.PathEscape(animeID), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
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
		c.base, url.PathEscape(animeID), url.QueryEscape(episodeID), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
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
		c.base, url.PathEscape(animeID), url.QueryEscape(episodeID), url.QueryEscape(serverID), url.QueryEscape(category), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
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
	u := fmt.Sprintf("%s/api/anime/%s/kodik/stream?episode=%d&translation=%d", c.base, url.PathEscape(animeID), episode, translation)
	if err := c.getJSON(ctx, u, &data); err != nil {
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
	u := fmt.Sprintf("%s/api/anime/%s/%s/episodes", c.base, url.PathEscape(animeID), url.PathEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
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
	u := fmt.Sprintf("%s/api/anime/%s/%s/stream?episode=%d", c.base, url.PathEscape(animeID), url.PathEscape(provider), episode)
	if err := c.getJSON(ctx, u, &data); err != nil {
		return nil, err
	}
	if data.URL == "" {
		return nil, ErrNotFound
	}
	return &Stream{URL: data.URL, Exp: data.Exp, Sig: data.Sig, Referer: data.Referer, Type: "mp4"}, nil
}
