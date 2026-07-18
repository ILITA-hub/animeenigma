// Package kage is the Kage Project (fansubs.ru) client — the long-running
// Russian anime fansub archive. It replaces anime365 (smotret-anime.org),
// which went fully paywalled in July 2026 and was retired.
//
// Kage has no API. Three server-rendered, windows-1251-encoded PHP surfaces:
//
//	search:   POST /search.php  body query=<cp1251 title>  → base.php?id=N links
//	series:   GET  /base.php?id=N                          → release rows (srt ids)
//	download: POST /base.php    body srt=N                 → RAR/ZIP archive or bare file
//
// The markup is rigidly machine-generated, so regex parsing is stable in
// practice; every parse is fail-soft (unparseable rows are skipped).
package kage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

const defaultBaseURL = "http://www.fansubs.ru" // plain HTTP: the site refuses TLS

// maxResponseBytes caps any single upstream body read. The largest legitimate
// payloads are subtitle archives for long series (hundreds of KB, fonts can
// push a few MB); 32MB is far above that while still bounding a hostile body.
const maxResponseBytes = 32 << 20

// Config configures the Kage client. Empty fields fall back to defaults.
type Config struct {
	BaseURL   string
	Enabled   bool
	UserAgent string
	Timeout   time.Duration
	Transport http.RoundTripper // egress-recording transport (AR-EGRESS-03)
}

// Client is the Kage read-only HTTP client.
type Client struct {
	baseURL    string
	enabled    bool
	userAgent  string
	httpClient *http.Client
}

// SeriesRef is one search result: a Kage title page.
type SeriesRef struct {
	ID    int
	Title string
}

// Release is one downloadable subtitle archive on a series page.
type Release struct {
	SrtID  int    // hidden form id for POST base.php downloads
	Label  string // e.g. "ТВ 1-28", "Фильм"
	Format string // e.g. "ass", "srt" (lowercased first token of the format cell)
	Date   string // upload date as shown, e.g. "13.12.25"
	Author string // fansubber name (may be empty)
	Team   string // fansub team (may be empty)
	EpFrom int    // parsed episode range; 0,0 = unbounded (movie/unparsed label)
	EpTo   int
}

// ContainsEpisode reports whether the release's parsed label range covers ep.
// An unbounded range (no digits in the label) matches everything.
func (r Release) ContainsEpisode(ep int) bool {
	if r.EpFrom == 0 && r.EpTo == 0 {
		return true
	}
	return r.EpFrom <= ep && ep <= r.EpTo
}

// NewClient builds a Client, applying safe defaults.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 12 * time.Second // archive downloads are slower than API calls
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "AnimeEnigma/1.0 (+https://animeenigma.org)"
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		enabled:    cfg.Enabled,
		userAgent:  cfg.UserAgent,
		httpClient: &http.Client{Timeout: cfg.Timeout, Transport: cfg.Transport},
	}
}

// IsConfigured reports whether the provider is enabled. Kage needs no key,
// so this is just the enable flag; the aggregator skips it when false.
func (c *Client) IsConfigured() bool { return c != nil && c.enabled }

// reSearchResult matches search-result title links: base.php?id=N + title text.
var reSearchResult = regexp.MustCompile(`<a href="base\.php\?id=(\d+)">([^<]+)`)

// SearchSeries POSTs the title to /search.php and returns the matching title
// pages. The query is cp1251-encoded (Kage's forms are windows-1251); pure
// ASCII romaji passes through unchanged.
func (c *Client) SearchSeries(ctx context.Context, title string) ([]SeriesRef, error) {
	body, err := c.postForm(ctx, "/search.php", url.Values{"query": {encodeCP1251(title)}})
	if err != nil {
		return nil, err
	}
	page := decodeCP1251(body)
	var out []SeriesRef
	for _, m := range reSearchResult.FindAllStringSubmatch(page, -1) {
		id, err := strconv.Atoi(m[1])
		if err != nil || id <= 0 {
			continue
		}
		out = append(out, SeriesRef{ID: id, Title: strings.TrimSpace(m[2])})
	}
	return out, nil
}

// Series-page parsing. The page is a sequence of blocks in document order:
// release blocks (a <form> with hidden srt id, bold label, format link) each
// eventually followed by an author block (<table class="row1"> with an
// au= profile link and an optional [team] link). One author block closes over
// all release blocks since the previous author block.
var (
	reReleaseForm = regexp.MustCompile(`(?s)<form method="post" action="base\.php">\s*<input type="hidden" name="srt" value="(\d+)">(.*?)</form>`)
	reAuthorBlock = regexp.MustCompile(`(?s)<table class="row1">(.*?)</table>`)
	reBold        = regexp.MustCompile(`<b>([^<]+)</b>`)
	reFormatCell  = regexp.MustCompile(`cntr=\d+">\s*<font[^>]*>([^<]+)</font>`)
	reDateCell    = regexp.MustCompile(`>(\d{2}\.\d{2}\.\d{2})<`)
	reAuthorLink  = regexp.MustCompile(`\?au=\d+">\s*<b>([^<]+)</b>`)
	reTeamLink    = regexp.MustCompile(`target=web>([^<]+)</a>\]`)
	reEpRange     = regexp.MustCompile(`(\d+)\s*-\s*(\d+)`)
	reEpSingle    = regexp.MustCompile(`\b(\d+)\b`)
)

// GetReleases fetches a series page and returns its subtitle releases with
// author/team attribution.
func (c *Client) GetReleases(ctx context.Context, seriesID int) ([]Release, error) {
	body, err := c.get(ctx, "/base.php?id="+strconv.Itoa(seriesID))
	if err != nil {
		return nil, err
	}
	page := decodeCP1251(body)

	type span struct {
		start   int
		release *Release
		author  string
		team    string
	}
	var spans []span

	for _, idx := range reReleaseForm.FindAllStringSubmatchIndex(page, -1) {
		srtID, err := strconv.Atoi(page[idx[2]:idx[3]])
		if err != nil || srtID <= 0 {
			continue
		}
		inner := page[idx[4]:idx[5]]
		r := Release{SrtID: srtID}
		if m := reBold.FindStringSubmatch(inner); m != nil {
			r.Label = strings.TrimSpace(m[1])
		}
		if m := reFormatCell.FindStringSubmatch(inner); m != nil {
			if f := strings.Fields(m[1]); len(f) > 0 {
				r.Format = strings.ToLower(f[0])
			}
		}
		if m := reDateCell.FindStringSubmatch(inner); m != nil {
			r.Date = m[1]
		}
		r.EpFrom, r.EpTo = parseEpisodeRange(r.Label)
		spans = append(spans, span{start: idx[0], release: &r})
	}
	for _, idx := range reAuthorBlock.FindAllStringSubmatchIndex(page, -1) {
		inner := page[idx[2]:idx[3]]
		m := reAuthorLink.FindStringSubmatch(inner)
		if m == nil {
			continue // nav header table is also class="row1" — no author link, skip
		}
		s := span{start: idx[0], author: strings.TrimSpace(m[1])}
		if tm := reTeamLink.FindStringSubmatch(inner); tm != nil {
			s.team = strings.TrimSpace(tm[1])
		}
		spans = append(spans, s)
	}

	// Document order: each author block attributes every release block above it
	// back to the previous author block.
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out []Release
	var pending []*Release
	for _, s := range spans {
		if s.release != nil {
			pending = append(pending, s.release)
			continue
		}
		for _, r := range pending {
			r.Author, r.Team = s.author, s.team
			out = append(out, *r)
		}
		pending = nil
	}
	for _, r := range pending { // trailing releases with no author block
		out = append(out, *r)
	}
	return out, nil
}

// DownloadArchive POSTs the srt id to /base.php and returns the raw archive
// (or bare subtitle) bytes plus the upstream filename from Content-Disposition.
func (c *Client) DownloadArchive(ctx context.Context, srtID int) ([]byte, string, error) {
	body, header, err := c.fetch(ctx, "/base.php", url.Values{"srt": {strconv.Itoa(srtID)}})
	if err != nil {
		return nil, "", err
	}
	filename := ""
	if cd := header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			filename = params["filename"]
		}
	}
	return body, filename, nil
}

// Ping checks Kage reachability via the homepage. Returns latency.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	body, err := c.get(ctx, "/")
	if err != nil {
		return time.Since(start), err
	}
	if !strings.Contains(string(body), "Kage") {
		return time.Since(start), fmt.Errorf("kage: homepage did not look like Kage Project")
	}
	return time.Since(start), nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	body, _, err := c.fetch(ctx, path, nil)
	return body, err
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	body, _, err := c.fetch(ctx, path, form)
	return body, err
}

// fetch performs one Kage request and returns the size-capped body plus the
// response headers. form == nil ⇒ GET; otherwise a POST with an urlencoded
// body.
func (c *Client) fetch(ctx context.Context, path string, form url.Values) ([]byte, http.Header, error) {
	method, reqBody := http.MethodGet, io.Reader(nil)
	if form != nil {
		method, reqBody = http.MethodPost, strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("kage: %s %s: status %d", method, path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header, nil
}

// parseEpisodeRange extracts the episode span from a release label:
// "ТВ 1-28" → (1,28); "ТВ 5" → (5,5); "Фильм" → (0,0) = unbounded.
func parseEpisodeRange(label string) (int, int) {
	if m := reEpRange.FindStringSubmatch(label); m != nil {
		from, _ := strconv.Atoi(m[1])
		to, _ := strconv.Atoi(m[2])
		if from > 0 && to >= from {
			return from, to
		}
	}
	if m := reEpSingle.FindStringSubmatch(label); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 0 {
			return n, n
		}
	}
	return 0, 0
}

// encodeCP1251 converts UTF-8 text to windows-1251 for form submission.
// Unmappable runes are dropped by the encoder; ASCII passes through.
func encodeCP1251(s string) string {
	out, err := charmap.Windows1251.NewEncoder().String(s)
	if err != nil {
		return s // best effort: send as-is (pure-ASCII queries never hit this)
	}
	return out
}

// decodeCP1251 converts a windows-1251 page body to UTF-8.
func decodeCP1251(b []byte) string {
	return string(cp1251ToUTF8(b))
}

// cp1251ToUTF8 decodes windows-1251 bytes to UTF-8. Input that is already
// valid UTF-8 (pure-ASCII pages, UTF-8 archive entries, test fixtures)
// passes through — real cp1251 Cyrillic is never valid UTF-8.
func cp1251ToUTF8(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	out, err := charmap.Windows1251.NewDecoder().Bytes(b)
	if err != nil {
		return b
	}
	return out
}
