package animepahe

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

// MalSync provider slug we extract from the malsync response. Locked by
// CONTEXT.md decision: cache key shape is `malsync:{mal_id}:animepahe`.
const malSyncProviderSlug = "animepahe"

// malSyncCacheTTL is the positive-cache duration for MAL → AnimePahe ID
// resolutions. RESEARCH.md says the MAL-side mapping is stable; 24h matches
// the CONTEXT.md decision.
const malSyncCacheTTL = 24 * time.Hour

// malSyncMissTTL is the negative-cache duration for malsync 404s. Same TTL
// as positive cache — we don't want to re-hit a dead mapping on every page
// load. Transient (5xx) failures are NOT cached as misses.
const malSyncMissTTL = 24 * time.Hour

// malSyncMaxBody caps the response body read at 256 KiB. Real malsync
// responses are < 4 KiB; this is purely a DoS guard.
const malSyncMaxBody = 256 << 10

// MalSyncClient resolves a MAL ID to a provider-specific identifier via
// the api.malsync.moe service. Results are cached for 24h (positive) or
// 24h (negative). Wire failures are NOT cached so transient outages don't
// poison the cache.
type MalSyncClient struct {
	http    *http.Client
	cache   cache.Cache
	baseURL string
}

// MalSyncOption configures a MalSyncClient. See WithMalSyncHTTPClient,
// WithMalSyncBaseURL.
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
// on a confirmed miss (upstream 404 or no entry for the requested provider);
// returns ("", false, err) only on transport / decoding errors that should
// NOT be cached as misses.
//
// Cache flow:
//
//  1. Check `malsync:{malID}:{provider}` — positive cache.
//  2. Check `malsync:{malID}:{provider}:miss` — negative cache.
//  3. GET https://api.malsync.moe/mal/anime/{malID}; map response.Sites[provider].
//  4. On hit: Set positive key for 24h.
//  5. On 404 or no-entry: Set miss key for 24h.
//  6. On 5xx / transport error: return error without writing miss cache.
func (m *MalSyncClient) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	if malID == "" {
		return "", false, nil
	}
	hitKey := fmt.Sprintf("malsync:%s:%s", malID, provider)
	missKey := hitKey + ":miss"

	// 1. Positive cache hit?
	var cached string
	if err := m.cache.Get(ctx, hitKey, &cached); err == nil && cached != "" {
		return cached, true, nil
	} else if err != nil && !errors.Is(err, cache.ErrNotFound) {
		// Unexpected cache backend failure — treat as a miss and fall through
		// to the upstream. We do NOT propagate the cache error because the
		// authoritative source is upstream, and a redis blip shouldn't break
		// the whole lookup path.
		_ = err
	}

	// 2. Negative cache hit?
	var missed bool
	if err := m.cache.Get(ctx, missKey, &missed); err == nil && missed {
		return "", false, nil
	}

	// 3. Upstream.
	url := fmt.Sprintf("%s/mal/anime/%s", m.baseURL, malID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	site, ok := msr.Sites[provider]
	if !ok || len(site) == 0 {
		// Confirmed: malsync knows the anime but has no entry for our provider.
		_ = m.cache.Set(ctx, missKey, true, malSyncMissTTL)
		return "", false, nil
	}
	// Pick the entry with the lexicographically-smallest key. Real malsync
	// data has one or two entries per provider (sub/dub variants); for
	// AnimePahe specifically the identifier is the AnimePahe anime ID and
	// any entry yields the same value. WR-07: Go map iteration order is
	// randomized, so iterating `site` directly produced a non-deterministic
	// cached value across cold starts. Sort the keys first so the cache is
	// stable per (mal_id, provider).
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
			// Phase 27 A9: also write the persistent reverse-mapping key
			// so /release 404 invalidation can find the malID without an
			// in-memory map (works across process restarts). Best-effort
			// — log-level error not fatal; the forward cache is still
			// useful even if the reverse write fails.
			_ = m.cache.Set(ctx, reverseKey(provider, id), malID, malSyncCacheTTL)
			return id, true, nil
		}
	}
	// Map present but all entries empty — treat as a miss.
	_ = m.cache.Set(ctx, missKey, true, malSyncMissTTL)
	return "", false, nil
}

// reverseKey builds the persistent reverse-mapping cache key used by
// LookupMalID + Invalidate. Format: `malsync_reverse:<provider>:<providerID>`.
//
// Phase 27 A9 (SCRAPER-HEAL-30): replaces an earlier in-memory reverse map.
// Persistent storage means /release 404 invalidation works across process
// restarts and across scraper processes.
func reverseKey(provider, providerID string) string {
	return fmt.Sprintf("malsync_reverse:%s:%s", provider, providerID)
}

// LookupMalID is the inverse of Lookup: given a (provider, providerID),
// returns the malID that was written by a prior Lookup positive-cache
// hit. Returns ("", nil) cleanly when no reverse mapping exists — the
// caller (ListEpisodes /release 404 path) treats that as an acceptable
// no-op (nothing to invalidate; FindID was never called for this
// providerID in the past 24h, so the forward MalSync entry is also
// absent).
//
// Non-cache errors are propagated verbatim; the call site logs them but
// does not fail the user-visible request.
func (m *MalSyncClient) LookupMalID(ctx context.Context, providerID, provider string) (string, error) {
	if providerID == "" || provider == "" {
		return "", nil
	}
	var malID string
	err := m.cache.Get(ctx, reverseKey(provider, providerID), &malID)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return malID, nil
}

// Invalidate evicts BOTH the forward and reverse MalSync cache entries
// for a (malID, provider, providerID) triple in one variadic Delete.
//
// Phase 27 A9: single-strike — one /release 404 evicts both directions
// so the next FindID call re-runs the resolver's /search. The error from
// cache.Delete is propagated verbatim but is best-effort at the call site
// (failing to evict a stale mapping is strictly less bad than failing
// the user-visible 404).
func (m *MalSyncClient) Invalidate(ctx context.Context, malID, provider, providerID string) error {
	if malID == "" || provider == "" {
		return nil
	}
	forwardKey := fmt.Sprintf("malsync:%s:%s", malID, provider)
	revKey := reverseKey(provider, providerID)
	return m.cache.Delete(ctx, forwardKey, revKey)
}
