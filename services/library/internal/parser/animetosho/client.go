// Package animetosho is the library service's parser for the AnimeTosho
// JSON feed at https://feed.animetosho.org/json.
//
// AnimeTosho is a Nyaa.si mirror that adds a normalized JSON feed plus a
// `show=mal&id=` filter. When the catalog has a MAL ID for the anime an
// admin is queuing, this feed gives more reliable hits than Nyaa's
// title-keyed search (Nyaa metadata quality varies per uploader). When
// no MAL ID is available we fall back to a free-text `q=` query.
//
// Both routes return a JSON array of releases; we normalize each entry
// into the shared domain.Release shape (see internal/domain/release.go).
// The merger in internal/service/search.go dedupes across providers by
// the lowercase-hex InfoHash field — when AnimeTosho omits info_hash
// (rare but possible), we derive it from the magnet URI via
// github.com/anacrolix/torrent/metainfo.ParseMagnetURI so the dedupe key
// stays consistent with what Nyaa emits.
package animetosho

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/anacrolix/torrent/metainfo"
)

// Config controls AnimeTosho client behavior. All fields are populated
// from env via services/library/internal/config/config.go so the
// operator can swap the upstream host (e.g. point at a local mock during
// integration tests) without a redeploy beyond an env-var change.
type Config struct {
	// BaseURL of the AnimeTosho JSON feed. Default
	// "https://feed.animetosho.org". The /json path is appended by Search.
	BaseURL string

	// HTTPTimeout per request. Default 15s — torrent indexers are slower
	// and more variable than the streaming APIs in services/catalog.
	HTTPTimeout time.Duration

	// UserAgent header. Default "AnimeEnigma/1.0 (library service)".
	UserAgent string
}

// Client is the AnimeTosho JSON-feed client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// SearchParams is the per-call input to Search. When MALID > 0 the
// MAL-feed path is used; otherwise the free-text query path is used.
// Limit is clamped to [1, 200] (default 50 when <= 0).
type SearchParams struct {
	MALID int
	Query string
	Limit int
}

const (
	defaultBaseURL  = "https://feed.animetosho.org"
	defaultTimeout  = 15 * time.Second
	defaultUA       = "AnimeEnigma/1.0 (library service)"
	defaultLimit    = 50
	maxLimit        = 200
	providerName    = "animetosho"
)

// qualityRegex matches the most common torrent resolution tokens. Other
// values (e.g. 360p, 4K-without-2160 designator) surface as empty Quality.
var qualityRegex = regexp.MustCompile(`(?i)\b(2160|1080|720|480)p\b`)

// uploaderRegex extracts the leading [Group] bracket from a release title.
// AnimeTosho does not surface uploader as a separate field, so we regex
// it off the title; empty when absent.
var uploaderRegex = regexp.MustCompile(`^\[([^\]]+)\]`)

// NewClient builds a Client from a Config. Empty/zero fields fall back
// to safe defaults. The constructor makes no network calls.
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

// rawEntry mirrors the relevant subset of an AnimeTosho JSON feed item.
// AnimeTosho returns many more fields (nyaa_subcat, seeders, etc.) that
// Phase 2 ignores. Phase 3+ may grow this shape.
type rawEntry struct {
	Title     string `json:"title"`
	Link      string `json:"link"`       // magnet:?xt=urn:btih:...
	InfoHash  string `json:"info_hash"`  // lowercase hex, sometimes empty
	TotalSize int64  `json:"total_size"` // bytes
	Timestamp int64  `json:"timestamp"`  // unix seconds
}

// Search hits the AnimeTosho JSON feed. Route selection:
//
//   - p.MALID > 0  → GET {BaseURL}/json?show=mal&id={p.MALID}
//   - otherwise    → GET {BaseURL}/json?q={url.QueryEscape(p.Query)}
//
// Returns a slice of normalized domain.Release entries with Source set
// to "animetosho" and MALID propagated from the caller (zero on the
// query path; non-zero on the MAL path). Non-2xx HTTP responses are
// wrapped via errors.ExternalAPI("animetosho", ...) so the merger can
// degrade soft.
func (c *Client) Search(ctx context.Context, p SearchParams) ([]domain.Release, error) {
	limit := clampLimit(p.Limit)

	endpoint, err := c.buildURL(p)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "build AnimeTosho URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "build AnimeTosho request")
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI(providerName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, errors.ExternalAPI(providerName, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	var entries []rawEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "decode AnimeTosho response")
	}

	releases := make([]domain.Release, 0, len(entries))
	for _, e := range entries {
		releases = append(releases, normalize(e, p.MALID))
	}

	if len(releases) > limit {
		releases = releases[:limit]
	}
	return releases, nil
}

// buildURL composes the JSON-feed URL for the requested route.
func (c *Client) buildURL(p SearchParams) (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	u, err := url.Parse(base + "/json")
	if err != nil {
		return "", err
	}
	q := u.Query()
	if p.MALID > 0 {
		q.Set("show", "mal")
		q.Set("id", fmt.Sprintf("%d", p.MALID))
	} else {
		q.Set("q", p.Query)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// normalize maps one rawEntry to a domain.Release. The caller passes the
// originating MAL ID so MAL-feed results carry it; query-path results
// pass 0.
func normalize(e rawEntry, malID int) domain.Release {
	infoHash := strings.ToLower(strings.TrimSpace(e.InfoHash))
	if infoHash == "" {
		// Derive from the magnet URI. Failure is tolerated — the merger
		// drops entries with empty InfoHash (they cannot be deduped).
		if m, err := metainfo.ParseMagnetUri(e.Link); err == nil {
			infoHash = strings.ToLower(m.InfoHash.HexString())
		}
	}

	r := domain.Release{
		Title:     e.Title,
		Magnet:    e.Link,
		InfoHash:  infoHash,
		SizeBytes: e.TotalSize,
		Source:    providerName,
		MALID:     malID,
		FoundAt:   time.Unix(e.Timestamp, 0).UTC(),
	}

	if m := qualityRegex.FindStringSubmatch(e.Title); len(m) > 1 {
		r.Quality = m[1] + "p"
	}
	if m := uploaderRegex.FindStringSubmatch(e.Title); len(m) > 1 {
		r.Uploader = m[1]
	}
	return r
}

// clampLimit normalizes a caller-provided Limit to [1, maxLimit] with
// defaultLimit applied when <= 0.
func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}
