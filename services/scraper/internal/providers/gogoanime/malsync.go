package gogoanime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// MalSync default endpoint. Override with WithMalSyncBaseURL for tests.
const defaultMalSyncBaseURL = "https://api.malsync.moe"

// malSyncProviderSlug is the literal Sites-key string that api.malsync.moe
// would use for Gogoanime/Anitaku mappings.
//
// Note capitalization: malsync.moe's Sites map is case-sensitive and uses
// the upstream brand's TitleCase variant (e.g. "KickAssAnime", "Crunchyroll",
// "AnimeKAI"). Per RESEARCH.md Sources / Open Question 4, "Gogoanime" is
// the expected key shape — but as of 2026-05-12 malsync ships NO Gogoanime
// key at all. The probe stays in code as forward-compat: the moment malsync
// adds the key, every MAL ID that has a mapping gets the fast path for free
// (no provider-code change required).
//
// The internal Redis cache key shape stays lowercase ("malsync:<mal>:gogoanime")
// to match the rest of the scraper's key convention — only the wire-key into
// malsync.moe is upper-case.
const malSyncProviderSlug = "Gogoanime"

// malSyncCacheTTL is the positive-cache duration for MAL → Gogoanime ID
// resolutions. 24h matches the CONTEXT.md decision.
const malSyncCacheTTL = 24 * time.Hour

// malSyncMissTTL is the negative-cache duration for malsync 404s and
// no-key-for-Gogoanime responses. Same 24h — we don't want to re-hit a dead
// mapping on every page load. Transient (5xx) failures are NOT cached as
// misses so a brief malsync outage doesn't poison the cache for a day.
//
// On 2026-05-12 every Gogoanime Lookup is expected to miss because malsync
// ships no Gogoanime key; this means malsync:<mal_id>:gogoanime:miss is the
// steady-state cache entry and Lookup is effectively free on the hot path.
const malSyncMissTTL = 24 * time.Hour

// malSyncMaxBody caps the response body read at 256 KiB. Real malsync
// responses are < 4 KiB; this is purely a DoS guard.
const malSyncMaxBody = 256 << 10

// MalSyncClient resolves a MAL ID to a Gogoanime slug via the api.malsync.moe
// service. Results are cached for 24h (positive) or 24h (negative). Wire
// failures are NOT cached so transient outages don't poison the cache.
//
// EXPORTED type — Plan 18-04's main.go wires this constructor identically to
// the animepahe one.
type MalSyncClient struct {
	http    *http.Client
	cache   cache.Cache
	baseURL string
}

// MalSyncOption configures a MalSyncClient. See WithMalSyncHTTPClient,
// WithMalSyncBaseURL.
//
// EXPORTED type — kept symmetric with animepahe.MalSyncOption so the same
// option-pattern test rigs work for both providers.
type MalSyncOption func(*MalSyncClient)

// WithMalSyncHTTPClient overrides the http.Client used to call malsync.moe.
// Tests use this to inject an httptest.Server's Client().
func WithMalSyncHTTPClient(c *http.Client) MalSyncOption {
	return func(m *MalSyncClient) {
		if c != nil {
			m.http = c
		}
	}
}

// WithMalSyncBaseURL overrides the malsync base URL. Tests use this to point
// at an httptest.Server.
func WithMalSyncBaseURL(u string) MalSyncOption {
	return func(m *MalSyncClient) {
		if u != "" {
			m.baseURL = u
		}
	}
}

// NewMalSyncClient builds a MalSyncClient with the given cache and options.
// The Cache MUST be non-nil — there's no in-memory fallback because the
// 24h TTL needs durable storage to be worth anything.
//
// EXPORTED constructor — signature `func NewMalSyncClient(c cache.Cache,
// opts ...MalSyncOption) *MalSyncClient` matches animepahe.NewMalSyncClient
// exactly so main.go (Plan 18-04 Task 1) can wire both providers with the
// same boilerplate.
func NewMalSyncClient(c cache.Cache, opts ...MalSyncOption) *MalSyncClient {
	m := &MalSyncClient{
		http:    &http.Client{Timeout: 10 * time.Second},
		cache:   c,
		baseURL: defaultMalSyncBaseURL,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Lookup resolves (malID, provider) → providerID. Returns ("", false, nil)
// on a confirmed miss (upstream 404, or no entry for the requested provider);
// returns ("", false, err) only on transport / decoding errors that should
// NOT be cached as misses.
//
// Cache flow:
//
//  1. Check `malsync:{malID}:gogoanime` — positive cache (24h).
//  2. Check `malsync:{malID}:gogoanime:miss` — negative cache (24h).
//  3. GET https://api.malsync.moe/mal/anime/{malID}; map response.Sites[provider].
//  4. On hit: Set positive key for 24h.
//  5. On 404 or no-entry: Set miss key for 24h.
//  6. On 5xx / transport error: return error without writing miss cache.
//
// NOTE: the `provider` argument is accepted for forward-compat / symmetry with
// the animepahe contract — the Redis key shape is hardcoded to "gogoanime"
// (lowercase) regardless, because the Gogoanime provider only ever asks
// malsync for its own slug. Callers from inside this package pass
// malSyncProviderSlug = "Gogoanime" (TitleCase wire-key); the Redis cache
// key uses lowercase "gogoanime" per CONTEXT.md key-shape convention.
func (m *MalSyncClient) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	if malID == "" {
		return "", false, nil
	}
	hitKey := fmt.Sprintf("malsync:%s:gogoanime", malID)
	missKey := hitKey + ":miss"

	// 1. Positive cache hit?
	var cached string
	if err := m.cache.Get(ctx, hitKey, &cached); err == nil && cached != "" {
		return cached, true, nil
	} else if err != nil && !errors.Is(err, cache.ErrNotFound) {
		// Unexpected cache backend failure — treat as a miss and fall through
		// to the upstream. Don't propagate redis blips into the lookup path.
		_ = err
	}

	// 2. Negative cache hit?
	var missed bool
	if err := m.cache.Get(ctx, missKey, &missed); err == nil && missed {
		return "", false, nil
	}

	// 3. Upstream.
	reqURL := fmt.Sprintf("%s/mal/anime/%s", m.baseURL, malID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", false, fmt.Errorf("malsync: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("malsync: fetch: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		// Cache the miss for 24h.
		_ = m.cache.Set(ctx, missKey, true, malSyncMissTTL)
		return "", false, nil
	case resp.StatusCode >= 500:
		// Transient — do NOT cache as miss.
		return "", false, fmt.Errorf("malsync: upstream 5xx: %d", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return "", false, fmt.Errorf("malsync: unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, malSyncMaxBody))
	if err != nil {
		return "", false, fmt.Errorf("malsync: read body: %w", err)
	}
	var msr malSyncResponse
	if err := json.Unmarshal(body, &msr); err != nil {
		return "", false, fmt.Errorf("malsync: decode: %w", err)
	}

	// Look up the provider in the Sites map. The wire-key is "Gogoanime"
	// (TitleCase, per malsync's convention — see malSyncProviderSlug).
	site, ok := msr.Sites[provider]
	if !ok || len(site) == 0 {
		// Confirmed: malsync knows the anime but has no entry for our provider.
		// THIS IS THE STEADY STATE as of 2026-05-12 (RESEARCH.md Open Q4).
		_ = m.cache.Set(ctx, missKey, true, malSyncMissTTL)
		return "", false, nil
	}
	// Pick the entry with the lexicographically-smallest key for determinism.
	// Go map iteration order is randomized — without sorting, the cached
	// value would differ across cold starts. See animepahe WR-07 anchor.
	keys := make([]string, 0, len(site))
	for k := range site {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		entry := site[k]
		id := fmt.Sprintf("%v", entry.Identifier)
		if id != "" && id != "<nil>" {
			_ = m.cache.Set(ctx, hitKey, id, malSyncCacheTTL)
			return id, true, nil
		}
	}
	// Map present but all entries empty — treat as a miss.
	_ = m.cache.Set(ctx, missKey, true, malSyncMissTTL)
	return "", false, nil
}
