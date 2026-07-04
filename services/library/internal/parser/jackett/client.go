// Package jackett is the library service's parser for a self-hosted
// Jackett instance — the multi-indexer torrent aggregator that fronts
// the library search's PRIMARY tier (Nyaa + AnimeTosho remain the
// fallback, see internal/service/jackett_tier.go).
//
// Jackett exposes a Newznab/Torznab API plus an internal JSON results
// endpoint that its own dashboard uses for manual search. We consume the
// JSON endpoint because it mirrors the AnimeTosho client's shape and hands
// us Seeders, InfoHash, MagnetUri and Tracker as first-class fields —
// Seeders being the swarm-health signal the two legacy indexers never
// surfaced (a peerless donghua pack is exactly what stalled the first
// MinIO preload).
//
// Endpoint: GET {BaseURL}/api/v2.0/indexers/{filter}/results?apikey={key}&Query={q}
//   - {filter} defaults to `!status:failing` (Config.IndexerFilter): fan out
//     only across configured indexers Jackett does NOT currently mark
//     failing. Jackett's per-indexer fail-soft covers indexers that error
//     QUICKLY — ones that hang (Cloudflare challenges, dead domains) stall
//     the whole aggregate until Jackett's internal ~100s cap, far past
//     HTTPTimeout, which killed the tier on every query when 14 of 41
//     configured indexers rotted (2026-07-04).
//   - optional repeated &Category[]={cat} narrows to e.g. 5070 (TV/Anime).
//
// The filtered aggregate is still the slow path (seconds across dozens of
// indexers) so the operator wires a generous HTTPTimeout (default 30s).
// Results are normalized into the shared domain.Release shape and ranked
// Seeders DESC before the limit cap, so the healthiest swarms surface first.
package jackett

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/anacrolix/torrent/metainfo"
)

// Config controls the Jackett client. All fields are populated from env
// via services/library/internal/config/config.go.
type Config struct {
	// BaseURL of the Jackett instance. Default "http://jackett:9117"
	// (the docker-network DNS name — NOT the host-bound 127.0.0.1:9117,
	// which only the operator's browser can reach). The
	// /api/v2.0/indexers/{filter}/results path is appended by Search.
	BaseURL string

	// IndexerFilter is the aggregate-endpoint indexer filter segment
	// (Jackett "filtered aggregate" syntax). Default "!status:failing":
	// skip indexers Jackett currently marks failing — permanently broken
	// ones (Cloudflare-challenged, dead domains) otherwise stall the whole
	// aggregate response until Jackett's internal ~100s cap, far past
	// HTTPTimeout, killing the tier. "all" restores the unfiltered fan-out.
	IndexerFilter string

	// APIKey is Jackett's server API key (Dashboard → top-right). Required:
	// an empty key disables the whole primary tier upstream in main.go, so
	// the client is never constructed with one.
	APIKey string

	// Categories optionally narrows the search to Torznab category IDs
	// (e.g. ["5070"] for TV/Anime). Empty ⇒ all categories (max recall).
	Categories []string

	// HTTPTimeout per request. Default 30s — the filtered aggregate query
	// is far slower than a single indexer.
	HTTPTimeout time.Duration

	// UserAgent header. Default "AnimeEnigma/1.0 (library service)".
	UserAgent string
}

// Client is the Jackett JSON-results client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

const (
	defaultBaseURL       = "http://jackett:9117"
	defaultIndexerFilter = "!status:failing"
	defaultTimeout       = 30 * time.Second
	defaultUA            = "AnimeEnigma/1.0 (library service)"
	defaultLimit         = 50
	maxLimit             = 200
	providerName         = "jackett"
)

// qualityRegex matches the most common torrent resolution tokens.
var qualityRegex = regexp.MustCompile(`(?i)\b(2160|1080|720|480)p\b`)

// uploaderRegex extracts the leading [Group] bracket from a release title.
var uploaderRegex = regexp.MustCompile(`^\[([^\]]+)\]`)

// NewClient builds a Client from a Config. Empty/zero fields fall back to
// safe defaults. The constructor makes no network calls.
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
	if cfg.IndexerFilter == "" {
		cfg.IndexerFilter = defaultIndexerFilter
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
	}
}

// resultsEnvelope is the outer shape of Jackett's JSON results response.
// We only decode Results; the Indexers[] array (per-indexer error/elapsed
// telemetry) is ignored — partial results are the fail-soft contract.
type resultsEnvelope struct {
	Results []rawResult `json:"Results"`
}

// rawResult mirrors the relevant subset of one Jackett result row.
// Jackett emits many more fields (Guid, Details, Category, Peers, Grabs,
// Tracker, TrackerId, …); we decode the ones the library needs.
type rawResult struct {
	Title       string `json:"Title"`
	MagnetUri   string `json:"MagnetUri"` // often null/empty per-indexer
	InfoHash    string `json:"InfoHash"`  // often null/empty per-indexer
	Size        int64  `json:"Size"`
	Seeders     int    `json:"Seeders"`
	PublishDate string `json:"PublishDate"` // RFC3339, e.g. 2023-09-29T00:00:00
}

// Search queries Jackett's filtered indexer aggregate for `query` and
// returns up to `limit` normalized Release entries, ranked Seeders DESC.
// Limit is clamped to [1, 200] (default 50 when <= 0). Non-2xx responses
// are wrapped via errors.ExternalAPI("jackett", …) so the tier can degrade
// soft to the Nyaa+AnimeTosho fallback.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]domain.Release, error) {
	limit = clampLimit(limit)

	endpoint, err := c.buildURL(query)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "build Jackett URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "build Jackett request")
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI(providerName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, errors.ExternalAPI(providerName, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	var env resultsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "decode Jackett response")
	}

	releases := make([]domain.Release, 0, len(env.Results))
	for _, e := range env.Results {
		r, ok := normalize(e)
		if !ok {
			// No usable magnet/InfoHash (Link-only .torrent proxy entries) —
			// the merger + the downloader both require an InfoHash, so drop.
			continue
		}
		releases = append(releases, r)
	}

	// Rank healthiest swarms first, then cap. Stable so equal-seeder
	// entries keep Jackett's own (relevance) ordering.
	sort.SliceStable(releases, func(i, j int) bool {
		return releases[i].Seeders > releases[j].Seeders
	})
	if len(releases) > limit {
		releases = releases[:limit]
	}
	return releases, nil
}

// buildURL assembles the aggregated JSON-results URL with the api key,
// the query, and any configured categories.
func (c *Client) buildURL(query string) (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	u, err := url.Parse(base + "/api/v2.0/indexers/" + url.PathEscape(c.cfg.IndexerFilter) + "/results")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("apikey", c.cfg.APIKey)
	q.Set("Query", query)
	for _, cat := range c.cfg.Categories {
		cat = strings.TrimSpace(cat)
		if cat != "" {
			q.Add("Category[]", cat)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// normalize maps one rawResult to a domain.Release. Magnet recovery:
// prefer MagnetUri; else synthesize one from InfoHash; the InfoHash itself
// is taken from the field or derived from MagnetUri. Returns ok=false when
// neither a magnet nor an info hash is available (entry is undedupeable +
// undownloadable, so the caller drops it).
func normalize(e rawResult) (domain.Release, bool) {
	infoHash := strings.ToLower(strings.TrimSpace(e.InfoHash))

	// Derive InfoHash from the magnet when the field is empty.
	if infoHash == "" && e.MagnetUri != "" {
		if m, err := metainfo.ParseMagnetUri(e.MagnetUri); err == nil {
			infoHash = strings.ToLower(m.InfoHash.HexString())
		}
	}
	if infoHash == "" {
		return domain.Release{}, false
	}

	magnet := strings.TrimSpace(e.MagnetUri)
	if magnet == "" {
		// Synthesize from the info hash, same as nyaa.normalize.
		magnet = "magnet:?xt=urn:btih:" + infoHash + "&dn=" + url.QueryEscape(e.Title)
	}

	r := domain.Release{
		Title:     e.Title,
		Magnet:    magnet,
		InfoHash:  infoHash,
		SizeBytes: e.Size,
		Source:    providerName,
		Seeders:   e.Seeders,
		FoundAt:   parsePublishDate(e.PublishDate),
	}
	if m := qualityRegex.FindStringSubmatch(e.Title); len(m) > 1 {
		r.Quality = m[1] + "p"
	}
	if m := uploaderRegex.FindStringSubmatch(e.Title); len(m) > 1 {
		r.Uploader = m[1]
	}
	return r, true
}

// parsePublishDate accepts Jackett's RFC3339 PublishDate, tolerating both
// the zoned and the naive (no-offset) forms it emits per indexer. Returns
// zero time on parse failure — the tier treats zero FoundAt as "rank last"
// only within the legacy merger, which Jackett results bypass.
func parsePublishDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
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
