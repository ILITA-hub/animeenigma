// Package nyaa is the library service's parser for Nyaa.si's RSS feed.
//
// Nyaa.si is the larger of the two torrent indexers wired into the
// library service (the other is AnimeTosho). Its search is title-keyed
// and metadata quality varies per uploader, so the aggregator in
// internal/service/search.go ranks AnimeTosho-with-MAL-ID hits above
// Nyaa entries when both providers return results for the same query.
//
// Endpoint: GET {BaseURL}/?page=rss&q={query}&c=1_2&f=0
//   - c=1_2 → anime / English-translated subcategory (still surfaces
//     raw JP releases because uploaders cross-post here).
//   - f=0   → no quality filter; we extract resolution from the title
//     ourselves and let downstream UI decide what to show.
//
// The RSS feed exposes Nyaa-specific namespaced fields (nyaa:infoHash,
// nyaa:size) which we decode via encoding/xml's namespace-aware tags.
// We synthesize a magnet URI from the info hash + title because the
// <link> element points at the .torrent download page, not at a magnet.
package nyaa

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// Config controls the Nyaa client. Mirrors the AnimeTosho client shape
// for symmetry — both clients are wired the same way in main.go.
type Config struct {
	// BaseURL of the Nyaa.si site. Default "https://nyaa.si". The
	// `/?page=rss&q=...` path is appended by Search.
	BaseURL string

	// HTTPTimeout per request. Default 15s.
	HTTPTimeout time.Duration

	// UserAgent header. Default "AnimeEnigma/1.0 (library service)".
	UserAgent string
}

// Client is the Nyaa RSS client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

const (
	defaultBaseURL = "https://nyaa.si"
	defaultTimeout = 15 * time.Second
	defaultUA      = "AnimeEnigma/1.0 (library service)"
	defaultLimit   = 50
	maxLimit       = 200
	providerName   = "nyaa"
)

// qualityRegex matches the most common torrent resolution tokens.
var qualityRegex = regexp.MustCompile(`(?i)\b(2160|1080|720|480)p\b`)

// NewClient builds a Nyaa client. Empty/zero fields fall back to safe
// defaults. The constructor makes no network calls.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = defaultTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUA
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
	}
}

// rssFeed is the outer envelope of Nyaa's RSS response.
type rssFeed struct {
	XMLName xml.Name  `xml:"rss"`
	Channel rssChan   `xml:"channel"`
}

type rssChan struct {
	Items []rssItem `xml:"item"`
}

// rssItem mirrors one <item> in the feed. Nyaa publishes the info hash
// + size + category under its own namespace (xmlns:nyaa="...") and the
// uploader under Dublin Core (xmlns:dc="..."). encoding/xml uses the
// `space namespace-uri` form OR matches by local name when the doc has
// declared the namespace prefix in the rss root — we use the prefix
// form because it's the simplest and matches what Nyaa actually emits.
type rssItem struct {
	Title    string `xml:"title"`
	Link     string `xml:"link"`
	PubDate  string `xml:"pubDate"`
	Creator  string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	InfoHash string `xml:"https://nyaa.si/xmlns/nyaa infoHash"`
	NyaaSize string `xml:"https://nyaa.si/xmlns/nyaa size"`
}

// Search queries Nyaa for `query` and returns up to `limit` normalized
// Release entries. Limit is clamped to [1, 200] (default 50 when <= 0).
// Non-2xx responses are wrapped via errors.ExternalAPI so the merger
// can degrade soft when Nyaa is down.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]domain.Release, error) {
	limit = clampLimit(limit)

	endpoint, err := c.buildURL(query)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "build Nyaa URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "build Nyaa request")
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.5")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI(providerName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, errors.ExternalAPI(providerName, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "decode Nyaa RSS")
	}

	releases := make([]domain.Release, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		releases = append(releases, normalize(it))
	}
	if len(releases) > limit {
		releases = releases[:limit]
	}
	return releases, nil
}

// buildURL assembles the Nyaa RSS URL with the locked query parameters.
func (c *Client) buildURL(query string) (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	u, err := url.Parse(base + "/")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("page", "rss")
	q.Set("q", query)
	q.Set("c", "1_2")
	q.Set("f", "0")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// normalize maps one <item> to a domain.Release. The merger expects
// InfoHash to be lowercase hex; we normalize here so its dedupe key is
// case-stable.
func normalize(it rssItem) domain.Release {
	hash := strings.ToLower(strings.TrimSpace(it.InfoHash))

	// Synthesize a magnet URI. Nyaa's <link> is the .torrent download
	// page, not a magnet, so we build one from the info hash and dn=title.
	var magnet string
	if hash != "" {
		magnet = "magnet:?xt=urn:btih:" + hash + "&dn=" + url.QueryEscape(it.Title)
	}

	r := domain.Release{
		Title:     it.Title,
		Magnet:    magnet,
		InfoHash:  hash,
		Uploader:  it.Creator,
		SizeBytes: parseSize(it.NyaaSize),
		Source:    providerName,
		MALID:     0,
		FoundAt:   parsePubDate(it.PubDate),
	}
	if m := qualityRegex.FindStringSubmatch(it.Title); len(m) > 1 {
		r.Quality = m[1] + "p"
	}
	return r
}

// parsePubDate accepts RFC1123Z (Nyaa's documented format) with a
// fallback to RFC1123 for any oddly formatted entries. Returns zero
// time on parse failure — the merger treats zero FoundAt as "rank last".
func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// parseSize accepts Nyaa's human-readable size strings — "1.4 GiB",
// "700 MiB", "512 MB", "1024 B", etc. — and returns bytes. Unrecognized
// formats return 0 instead of erroring; SizeBytes is informational, the
// merger doesn't rank on it.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Walk the string, collecting the leading numeric prefix (digits +
	// at most one decimal point), then split off the trailing unit token.
	numEnd := 0
	sawDot := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			numEnd = i + 1
			continue
		}
		if ch == '.' && !sawDot && numEnd > 0 {
			sawDot = true
			numEnd = i + 1
			continue
		}
		break
	}
	if numEnd == 0 {
		return 0
	}
	numStr := s[:numEnd]
	unit := strings.TrimSpace(strings.ToLower(s[numEnd:]))

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	multiplier := float64(1)
	switch unit {
	case "", "b":
		multiplier = 1
	case "kb":
		multiplier = 1000
	case "kib":
		multiplier = 1024
	case "mb":
		multiplier = 1000 * 1000
	case "mib":
		multiplier = 1024 * 1024
	case "gb":
		multiplier = 1000 * 1000 * 1000
	case "gib":
		multiplier = 1024 * 1024 * 1024
	case "tb":
		multiplier = 1000 * 1000 * 1000 * 1000
	case "tib":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0
	}
	return int64(val * multiplier)
}

// clampLimit normalizes a caller-provided Limit to [1, maxLimit].
func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}
