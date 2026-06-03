package eighteenanime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL   = "https://18anime.me"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126 Safari/537.36"
)

// Client is an HTTP client for the 18anime.me provider.
type Client struct {
	httpClient *http.Client
	searchBase string // base URL for search requests; defaults to baseURL
}

// NewClient returns a Client with a sensible default timeout.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 8 * time.Second},
		searchBase: baseURL,
	}
}

// SearchHit represents a single parsed result from the search results page.
type SearchHit struct {
	Slug string
	URL  string
}

// resultHrefRe matches hentai episode/series anchors on the search results page.
// Example: href="https://18anime.me/hentai/1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2.html"
var resultHrefRe = regexp.MustCompile(`href="(https?://18anime\.me/hentai/([0-9]+-[a-z0-9-]+)\.html)"`)

// parseSearchResults extracts unique SearchHit values from a search results HTML page.
// Each anchor typically appears twice (thumbnail + title link); duplicates are dropped.
func parseSearchResults(html string) []SearchHit {
	seen := map[string]bool{}
	var hits []SearchHit
	for _, m := range resultHrefRe.FindAllStringSubmatch(html, -1) {
		url, slug := m[1], m[2]
		if seen[slug] {
			continue
		}
		seen[slug] = true
		hits = append(hits, SearchHit{Slug: slug, URL: url})
	}
	return hits
}

// normalize strips all non-alphanumeric characters and lowercases s so that
// title strings can be compared against URL slugs.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bestMatch scores hits against the wanted title using normalized substring
// containment first, then per-token overlap as a fallback.
// Returns nil when no hit exceeds a zero score.
func bestMatch(title string, hits []SearchHit) *SearchHit {
	want := normalize(title)
	var best *SearchHit
	bestScore := -1
	for i := range hits {
		slugNorm := normalize(hits[i].Slug)
		score := 0
		if want != "" && (strings.Contains(slugNorm, want) || strings.Contains(want, slugNorm)) {
			score = len(want)
		} else {
			for _, tok := range strings.Fields(strings.ToLower(title)) {
				if len(tok) > 2 && strings.Contains(slugNorm, normalize(tok)) {
					score++
				}
			}
		}
		if score > bestScore {
			bestScore, best = score, &hits[i]
		}
	}
	if bestScore <= 0 {
		return nil
	}
	return best
}

// Mirror is a single playback mirror for an episode.
type Mirror struct {
	Link    string
	Quality string
}

// mirrorRe matches multi-line pretty-printed mirror JSON objects of the form:
//
//	"link": "https://...",
//	"quality": "FullHD"
//
// Go's regexp \s matches newlines, so \s* bridges the two fields across lines.
var mirrorRe = regexp.MustCompile(`"link"\s*:\s*"([^"]+)"\s*,?\s*"quality"\s*:\s*"([^"]*)"`)

// parseEpisodeMirrors extracts Mirror values from an episode page HTML.
// Duplicate links (each entry typically appears twice in the page) are dropped.
func parseEpisodeMirrors(html string) []Mirror {
	seen := map[string]bool{}
	var out []Mirror
	for _, m := range mirrorRe.FindAllStringSubmatch(html, -1) {
		link := strings.ReplaceAll(m[1], `\/`, `/`)
		if seen[link] {
			continue
		}
		seen[link] = true
		out = append(out, Mirror{Link: link, Quality: m[2]})
	}
	return out
}

// supportedMirrors filters all mirrors down to only those whose links contain a
// supported embed host, returned in failover order (mp4upload first, turbovid second).
func supportedMirrors(all []Mirror) []Mirror {
	order := []string{"mp4upload", "turbovid"}
	var out []Mirror
	for _, host := range order {
		for _, m := range all {
			if strings.Contains(m.Link, host) {
				out = append(out, m)
			}
		}
	}
	return out
}

// Episode represents a single episode of a series on 18anime.me.
type Episode struct {
	Slug   string `json:"slug"` // full slug including numeric id, e.g. "1167-...-episode-2"
	URL    string `json:"url"`
	Number int    `json:"number"` // 1-based episode number
}

// fetch GETs a URL with UA + optional Referer; caps body at 2 MiB.
func (c *Client) fetch(ctx context.Context, u, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eighteenanime: GET %s -> %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(b), err
}

// searchHTML POSTs the DataLife Engine search form.
func (c *Client) searchHTML(ctx context.Context, query string) (string, error) {
	form := url.Values{
		"do":        {"search"},
		"subaction": {"search"},
		"story":     {query},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.searchBase+"/index.php?do=search", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eighteenanime: search -> %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

var idPrefixRe = regexp.MustCompile(`^[0-9]+-`)
var epSuffixRe = regexp.MustCompile(`-(?:episode-)?([0-9]+)$`)

// baseSlugAndEpisode splits a hit slug into (baseSlug, episodeNumber).
// The numeric id prefix (e.g. "1167-") and trailing episode suffix are stripped.
// If no episode suffix is found, number defaults to 1.
func baseSlugAndEpisode(slug string) (string, int) {
	s := idPrefixRe.ReplaceAllString(slug, "")
	ep := 1
	if m := epSuffixRe.FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			ep = n
		}
		s = strings.TrimSuffix(s, m[0])
	}
	return s, ep
}

// extractorFor picks an extractor function by embed host.
func extractorFor(link string) func(string) (*ExtractedSource, error) {
	switch {
	case strings.Contains(link, "mp4upload"):
		return extractMP4Upload
	case strings.Contains(link, "turbovid"):
		return extractTurbovid
	default:
		return nil
	}
}

// resolveStream tries supported mirrors in order; first success wins.
func (c *Client) resolveStream(ctx context.Context, mirrors []Mirror) (*ExtractedSource, error) {
	supported := supportedMirrors(mirrors)
	if len(supported) == 0 {
		return nil, fmt.Errorf("eighteenanime: no supported mirrors")
	}
	var lastErr error
	for _, m := range supported {
		ex := extractorFor(m.Link)
		if ex == nil {
			continue
		}
		page, err := c.fetch(ctx, m.Link, baseURL+"/")
		if err != nil {
			lastErr = err
			continue
		}
		src, err := ex(page)
		if err != nil {
			lastErr = err
			continue
		}
		return src, nil
	}
	return nil, fmt.Errorf("eighteenanime: all mirrors failed: %w", lastErr)
}

// Search returns the best-matching hit for a title query.
func (c *Client) Search(ctx context.Context, title string) (*SearchHit, error) {
	page, err := c.searchHTML(ctx, title)
	if err != nil {
		return nil, err
	}
	hit := bestMatch(title, parseSearchResults(page))
	if hit == nil {
		return nil, fmt.Errorf("eighteenanime: no match for %q", title)
	}
	return hit, nil
}

// ListEpisodes returns all episodes belonging to the matched series, sorted ascending.
func (c *Client) ListEpisodes(ctx context.Context, title string) ([]Episode, error) {
	page, err := c.searchHTML(ctx, title)
	if err != nil {
		return nil, err
	}
	hits := parseSearchResults(page)
	best := bestMatch(title, hits)
	if best == nil {
		return nil, fmt.Errorf("eighteenanime: no match for %q", title)
	}
	wantBase, _ := baseSlugAndEpisode(best.Slug)
	var eps []Episode
	for _, h := range hits {
		base, num := baseSlugAndEpisode(h.Slug)
		if base != wantBase {
			continue
		}
		eps = append(eps, Episode{Slug: h.Slug, URL: h.URL, Number: num})
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].Number < eps[j].Number })
	return eps, nil
}

// GetStream resolves a playable stream URL for an episode page URL.
func (c *Client) GetStream(ctx context.Context, episodeURL string) (*ExtractedSource, error) {
	page, err := c.fetch(ctx, episodeURL, baseURL+"/")
	if err != nil {
		return nil, err
	}
	return c.resolveStream(ctx, parseEpisodeMirrors(page))
}
