// Package client contains the HTTP client(s) the spotlight resolvers use
// to talk to other services on the docker network. Phase 1 ships only
// WebClient — a thin wrapper around `http.Client` that fetches the
// nginx-served `/changelog.json` from the `web` container and decodes it
// into the wire format spotlight.LatestNewsData expects.
//
// The client is constructed with an injectable `*http.Client` so tests
// can substitute an `httptest.Server`-backed transport without touching
// the network. The 500ms default timeout sits snug under the 800ms
// per-card budget (HSB-BE-03) so the HTTP transport cuts off the request
// before the resolver's context deadline trips.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// maxChangelogEntries is the cap on returned entries. The Phase 2
// frontend renders at most 3 changelog rows in the spotlight card; we
// trim server-side so we never blow the JSON envelope with a long
// changelog history. Flattening preserves outer-array order (newest
// first), so "first 3 after flatten" === "3 newest entries".
const maxChangelogEntries = 3

// defaultBaseURL is the docker-network DNS name + nginx port for the
// frontend's static asset server. The `web` container serves
// `/changelog.json` at this URL across the project's compose network.
const defaultBaseURL = "http://web:80"

// defaultTimeout is the http.Client timeout floor — set below the 800ms
// per-card budget so the client gives up before the resolver's ctx
// deadline expires (HSB-BE-03 + Pitfall 8 in 01-RESEARCH.md).
const defaultTimeout = 500 * time.Millisecond

// changelogGroup is the wire-format outer-array element from
// frontend/web/public/changelog.json. UNEXPORTED helper type — callers
// receive the flattened []spotlight.ChangelogEntry shape.
type changelogGroup struct {
	Date    string `json:"date"`
	Entries []struct {
		Type    string `json:"type,omitempty"`
		Message string `json:"message"`
	} `json:"entries"`
}

// WebClient fetches changelog.json (and, in future phases, other
// JSON resources from the `web` container).
type WebClient struct {
	baseURL string
	http    *http.Client
}

// NewWebClient constructs a WebClient. Empty baseURL → "http://web:80".
// Nil hc → an http.Client with the 500ms default Timeout. Pass a non-nil
// hc from tests to inject an httptest.Server-backed transport.
func NewWebClient(baseURL string, hc *http.Client) *WebClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &WebClient{baseURL: baseURL, http: hc}
}

// BaseURL returns the configured base URL. Exported solely for tests
// (TestNewWebClient_Defaults asserts the empty-string default behavior).
func (c *WebClient) BaseURL() string {
	return c.baseURL
}

// GetChangelog fetches `/changelog.json` from the web container,
// flattens the outer per-date groups into individual ChangelogEntry
// rows, and returns at most maxChangelogEntries (3) entries — the
// newest first per the source file's ordering convention.
//
// Errors:
//   - request-build failure (bad URL) → wrapped "web client: build request"
//   - transport failure (DNS, refused, timeout, ctx cancel) → wrapped
//     "web client: fetch changelog" — `errors.Is(err, context.DeadlineExceeded)`
//     remains true through the wrap chain.
//   - non-200 status → wrapped "web client: unexpected status %d: ..."
//   - JSON decode failure → wrapped "web client: decode"
func (c *WebClient) GetChangelog(ctx context.Context) ([]spotlight.ChangelogEntry, error) {
	url := c.baseURL + "/changelog.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("web client: build request: %w", err)
	}
	req.Header.Set("User-Agent", "AnimeEnigma/1.0 (spotlight)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web client: fetch changelog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Cap body read at 512 bytes — we only need the error string,
		// no need to slurp a multi-megabyte HTML error page.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("web client: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var groups []changelogGroup
	if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil {
		return nil, fmt.Errorf("web client: decode: %w", err)
	}

	// Flatten — preserve order. Source file is sorted newest-first per
	// the LastUpdates.vue convention (see frontend/web/src/views/LastUpdates.vue).
	out := make([]spotlight.ChangelogEntry, 0, maxChangelogEntries)
	for _, g := range groups {
		for _, e := range g.Entries {
			out = append(out, spotlight.ChangelogEntry{
				Date:    g.Date,
				Type:    e.Type,
				Message: e.Message,
			})
			if len(out) >= maxChangelogEntries {
				return out, nil
			}
		}
	}
	return out, nil
}
