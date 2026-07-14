// Package roster is the analytics-side client for the catalog stream_providers
// roster (the DB single source of truth for provider EXISTENCE — AUTO-608).
// It replaces the compile-time knownProviders map: player-telemetry whitelisting,
// the playability roster filter, and probe-target membership all key off this.
package roster

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Row is the minimal projection of a stream_providers row analytics needs.
type Row struct {
	Name            string `json:"name"`
	Group           string `json:"group"`
	Status          string `json:"status"`
	ScraperOperated bool   `json:"scraper_operated"`
}

// fallbackSnapshot is the embedded cold-start roster, used ONLY until the
// first successful catalog fetch (e.g. analytics boots before catalog).
// Mirrors the seed roster; new providers do NOT need to be added here — they
// arrive via the live fetch. Keep tombstones so legacy events keep recording.
var fallbackSnapshot = []Row{
	{Name: "gogoanime", Group: "en", ScraperOperated: true},
	{Name: "animepahe", Group: "en", ScraperOperated: true},
	{Name: "allanime", Group: "en", ScraperOperated: true},
	{Name: "allanime-okru", Group: "en", ScraperOperated: true},
	{Name: "animefever", Group: "en", ScraperOperated: true},
	{Name: "miruro", Group: "en", ScraperOperated: true},
	{Name: "nineanime", Group: "en", ScraperOperated: true},
	{Name: "animekai", Group: "en", ScraperOperated: true},
	{Name: "18anime", Group: "adult", ScraperOperated: true},
	{Name: "ae", Group: "firstparty"},
	{Name: "kodik-noads", Group: "ru"},
	{Name: "kodik-iframe", Group: "ru"},
	{Name: "animelib", Group: "ru"},
	{Name: "hanime", Group: "adult"},
	{Name: "animejoy-sibnet", Group: "ru"},
	{Name: "animejoy-allvideo", Group: "ru"},
}

// Client fetches + TTL-caches the roster with last-good fallback.
type Client struct {
	url  string
	ttl  time.Duration
	http *http.Client

	mu        sync.Mutex
	rows      []Row
	names     map[string]struct{}
	fetchedAt time.Time
	everGood  bool
}

// New builds a client over CATALOG_URL. ttl 60s matches the scraper's
// remote-config refresh cadence.
func New(catalogURL string, ttl time.Duration) *Client {
	c := &Client{
		url:  strings.TrimRight(catalogURL, "/") + "/internal/scraper/providers",
		ttl:  ttl,
		http: &http.Client{Timeout: 10 * time.Second},
	}
	c.install(fallbackSnapshot, false)
	return c
}

func (c *Client) install(rows []Row, good bool) {
	names := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		names[strings.ToLower(r.Name)] = struct{}{}
	}
	c.rows, c.names = rows, names
	if good {
		c.everGood = true
		c.fetchedAt = time.Now()
	}
}

// refresh fetches when stale; on error the last-good (or fallback) set stays.
func (c *Client) refresh(ctx context.Context) {
	if c.everGood && time.Since(c.fetchedAt) < c.ttl {
		return
	}
	if !c.everGood && time.Since(c.fetchedAt) < time.Second {
		return // don't hammer an unreachable catalog on the cold path
	}
	c.fetchedAt = time.Now() // stamp attempt time even on failure (retry backoff)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	// Envelope: {"success":true,"data":{"providers":[...]}} — decode data.providers
	// (ISS-032: decoding the root silently yields an EMPTY roster).
	var body struct {
		Data struct {
			Providers []Row `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || len(body.Data.Providers) == 0 {
		return // empty/undecodable ⇒ keep last-good
	}
	c.install(body.Data.Providers, true)
}

// Known reports (case-insensitively) whether name is a roster row.
func (c *Client) Known(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh(context.Background())
	_, ok := c.names[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// Rows returns the last-good roster rows.
func (c *Client) Rows(ctx context.Context) []Row {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh(ctx)
	out := make([]Row, len(c.rows))
	copy(out, c.rows)
	return out
}
