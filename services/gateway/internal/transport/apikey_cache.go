package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// apiKeyCacheTTL bounds how long a successfully-resolved API-key claim is
// reused without re-hitting the auth service (audit finding L473). It is kept
// deliberately short: a cached claim means a revoked or role-changed API key
// stays valid for up to this TTL. 60s also aligns with the daily-granularity
// deriveAPIKeySessionID (apikey_session.go buckets the SID per UTC-day), so a
// short cache never crosses a SID boundary in a way that matters.
const apiKeyCacheTTL = 60 * time.Second

// resolveFunc is the signature of the upstream API-key resolver. The real
// implementation is resolveApiKey (which POSTs to the auth service); tests
// inject a fake to count upstream hits deterministically.
type resolveFunc func(authServiceURL, apiKey string) (*authz.Claims, error)

// apiKeyCacheEntry is one cached, successfully-resolved API-key claim.
type apiKeyCacheEntry struct {
	claims    *authz.Claims
	expiresAt time.Time
}

// apiKeyCache is a small in-memory TTL cache for resolved API-key claims,
// keyed on sha256(rawKey). Hot ak_* keys skip the per-request blocking auth
// POST. Only successful resolutions are cached — a resolve that errors (e.g.
// the auth service returns 401 for a revoked key) is never stored, so a
// revoked key recovers within the TTL window rather than being pinned valid.
//
// The cache stores the upstream claims (UserID/Username/Role). The short-lived
// downstream JWT is still minted fresh per request from those claims by the
// middleware (local + cheap); the eliminated cost is the blocking auth round
// trip, which is the actual hot-path expense (the auth client pool is only
// MaxIdleConnsPerHost:2 — router.go).
type apiKeyCache struct {
	mu      sync.RWMutex
	entries map[string]apiKeyCacheEntry
	now     func() time.Time
	resolve resolveFunc
}

// newAPIKeyCache builds a cache backed by the given upstream resolver and clock.
// Pass nil for now to use time.Now (production); tests pass an injectable clock.
func newAPIKeyCache(resolve resolveFunc, now func() time.Time) *apiKeyCache {
	if now == nil {
		now = time.Now
	}
	return &apiKeyCache{
		entries: make(map[string]apiKeyCacheEntry),
		now:     now,
		resolve: resolve,
	}
}

// hashKey returns the cache key for a raw API key. The raw key never lives in
// the map (only its hash), matching the apikey_session.go derivation contract.
func hashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// get returns a non-expired cached claim for the key, or (nil, false).
func (c *apiKeyCache) get(rawKey string) (*authz.Claims, bool) {
	key := hashKey(rawKey)
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || !c.now().Before(entry.expiresAt) {
		return nil, false
	}
	return entry.claims, true
}

// set stores a successful resolution with the configured TTL.
func (c *apiKeyCache) set(rawKey string, claims *authz.Claims) {
	entry := apiKeyCacheEntry{
		claims:    claims,
		expiresAt: c.now().Add(apiKeyCacheTTL),
	}
	key := hashKey(rawKey)
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
}

// resolveCached returns the resolved claims for an API key, serving from the
// in-memory TTL cache on a hit and falling back to the upstream resolver on a
// miss/expiry. Successful upstream resolutions are cached; errors are not.
//
// On a hit it returns a COPY of the cached claims so a caller that mutates the
// returned struct (the middlewares set SessionID per request) never corrupts
// the shared cache entry.
func (c *apiKeyCache) resolveCached(authServiceURL, apiKey string) (*authz.Claims, error) {
	if claims, ok := c.get(apiKey); ok {
		cp := *claims
		return &cp, nil
	}
	resolved, err := c.resolve(authServiceURL, apiKey)
	if err != nil {
		return nil, err
	}
	c.set(apiKey, resolved)
	cp := *resolved
	return &cp, nil
}
