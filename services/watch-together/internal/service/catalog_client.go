// Package service — catalog_client.go is the watch-together → catalog HTTP
// back-channel used to validate state-change requests.
//
// Backs WT-STATE-02 (Plan 04.2 / 04.3): when a member sends
// state:change_episode / state:change_player / state:change_translation, the
// inbound router calls ValidateEpisode here BEFORE mutating Redis and
// broadcasting room:state_changed. If the catalog reports the combination is
// unavailable, the router instead sends a sender-only error envelope with
// ErrCodeEpisodeUnavailable / ErrCodePlayerUnavailable /
// ErrCodeTranslationUnavailable. See:
//
//	.planning/workstreams/watch-together/phases/04-state-switching/04-CONTEXT.md
//	(§Implementation Decisions — Inbound message handlers)
//
// Endpoint contract (delivered by Plan 04.1 in services/catalog/):
//
//	GET {CatalogURL}/internal/anime/{shikimoriID}/episodes/validate
//	    ?player=...&episode_id=...&translation_id=...&watch_type=...
//	→ 200  {"success":true,"data":{"valid":bool,"reason":string}}
//	  400  {"success":false,"error":{"code":"INVALID_INPUT","message":"..."}}
//	  500  {"success":false,"error":{"code":"INTERNAL","message":"..."}}
//
// The error channel of ValidateEpisode is reserved for transport / protocol
// failures only — logical Valid=false comes back as part of ValidateResult
// so the router can produce the right ErrCode without inspecting an error
// chain. This mirrors the EpisodeChecker pattern in
// services/notifications/internal/service/catalog_client.go.
//
// Cache strategy (CONTEXT §Claude's Discretion):
//
//	Positive results (Valid=true) cached for catalogPositiveCacheTTL (5s)
//	keyed on the full (shikimoriID, player, episode_id, translation_id,
//	watch_type) tuple. Absorbs rapid switcher clicks (next/prev/skip-ten)
//	without hammering catalog.
//
//	Negative results NEVER cached — if the catalog state changes (admin
//	enables a new episode, parser refreshes the player→translation map),
//	the next switch attempt must see the new state immediately. The 5s
//	stale window on positives is acceptable because positives are the
//	stable-by-design case.
//
// In-process state only (single-instance v1.0). Same scope as RateLimiter and
// DriftEngine — multi-instance scale-out is deferred to v2 and the obvious
// upgrade is a Redis-backed cache.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// ValidateResult is the unwrapped catalog answer. Valid==true means the
// (anime, player, episode, translation, watch_type) tuple resolves to a
// playable stream; Valid==false carries a Reason matching one of the
// ErrCode*Unavailable constants from package domain (or any catalog-specific
// reason string the router can map to ErrCode*).
type ValidateResult struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`
}

// CatalogClient performs WT-STATE-02 validation against the catalog service.
//
// Concurrency: mu protects the cache map. The HTTP call itself runs OUTSIDE
// the lock so a slow catalog response never serializes unrelated lookups.
// The http.Client is safe for concurrent use.
type CatalogClient struct {
	baseURL string
	http    *http.Client
	log     *logger.Logger

	mu    sync.Mutex
	cache map[string]cachedValidation // positive (Valid=true) results only
	now   func() time.Time            // injectable for tests
}

// cachedValidation is the per-key positive-cache entry.
type cachedValidation struct {
	result   ValidateResult
	expireAt time.Time
}

// catalogClientTimeout is the per-call HTTP deadline. State-change validation
// runs in the WS inbound critical path — the user is waiting for the next
// frame to render — so we cap upstream latency aggressively. A slow catalog
// is treated as transport failure (the router will surface the error to the
// sender and refuse the state change rather than block the WS goroutine).
const catalogClientTimeout = 3 * time.Second

// catalogPositiveCacheTTL is the in-process cache window for Valid=true
// results. Picked from CONTEXT §Claude's Discretion: short enough that a
// fresh-from-admin state propagates within ~one click of latency, long
// enough to absorb the rapid switcher click pattern (next-episode mash) that
// would otherwise hit catalog 5+ times per second.
const catalogPositiveCacheTTL = 5 * time.Second

// catalogEnvelope is the wire shape catalog wraps every response in
// (libs/httputil.JSON convention — see services/notifications client docblock).
type catalogEnvelope struct {
	Success bool             `json:"success"`
	Data    ValidateResult   `json:"data"`
	Error   *catalogErrorObj `json:"error,omitempty"`
}

type catalogErrorObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewCatalogClient constructs a client targeting the catalog service at
// baseURL (typically cfg.CatalogURL, default http://catalog:8081). A trailing
// slash on baseURL is stripped defensively — config.Load() already trims one,
// but a future caller passing the URL through a different path shouldn't be
// silently bitten by "//internal/..." double-slashes.
//
// Pass nil for log to fall back to logger.Default() (matches NewRoomService /
// NewDriftEngine convention in this package).
func NewCatalogClient(baseURL string, log *logger.Logger) *CatalogClient {
	if log == nil {
		log = logger.Default()
	}
	return &CatalogClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: catalogClientTimeout},
		log:     log,
		cache:   make(map[string]cachedValidation),
		now:     time.Now,
	}
}

// SetClockForTest swaps the wall-clock source for cache-TTL arithmetic. Only
// callable from inside this package (and _test.go files compiled with it) so
// production code can never accidentally inject a fake clock.
func (c *CatalogClient) SetClockForTest(fn func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = fn
}

// cacheKey builds the lookup key from the validation tuple. Pipe separator is
// safe because none of the inputs is allowed to contain '|' (player is a
// fixed enum, IDs are UUIDs or short opaque tokens, watch_type ∈ sub|dub).
func cacheKey(shikimoriID, player, episodeID, translationID, watchType string) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s",
		shikimoriID, player, episodeID, translationID, watchType)
}

// ValidateEpisode asks the catalog service whether the given combination is
// playable. Returns:
//
//	(ValidateResult{Valid:true},               nil)  — combo resolves cleanly
//	(ValidateResult{Valid:false, Reason:"…"}, nil)  — catalog says no (NOT an error)
//	(ValidateResult{},                         err)  — transport / protocol failure
//
// The Reason field on a logical no-answer is what the router maps onto an
// ErrCode constant when crafting the sender-only error envelope.
func (c *CatalogClient) ValidateEpisode(
	ctx context.Context,
	shikimoriID, player, episodeID, translationID, watchType string,
) (ValidateResult, error) {
	key := cacheKey(shikimoriID, player, episodeID, translationID, watchType)

	// Cache check — release the lock BEFORE the HTTP call so concurrent
	// lookups for unrelated keys aren't blocked by a slow catalog.
	c.mu.Lock()
	entry, ok := c.cache[key]
	now := c.now()
	c.mu.Unlock()
	if ok && now.Before(entry.expireAt) {
		return entry.result, nil
	}

	// Build the request URL. url.PathEscape on shikimoriID guards against a
	// future ID format with reserved characters; today shikimori IDs are
	// numeric so this is defensive.
	q := url.Values{}
	q.Set("player", player)
	q.Set("episode_id", episodeID)
	q.Set("translation_id", translationID)
	q.Set("watch_type", watchType)
	endpoint := fmt.Sprintf("%s/internal/anime/%s/episodes/validate?%s",
		c.baseURL, url.PathEscape(shikimoriID), q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("build catalog validate request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Includes context.DeadlineExceeded (3s client timeout), context
		// cancellation, DNS failures, refused connections.
		return ValidateResult{}, fmt.Errorf("catalog validate transport error: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return ValidateResult{}, fmt.Errorf("read catalog validate response: %w", readErr)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var env catalogEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return ValidateResult{}, fmt.Errorf("decode catalog validate response: %w", err)
		}
		if !env.Success {
			// 200 + success:false is a protocol violation by catalog, but
			// handle it gracefully rather than panic.
			msg := "unknown catalog error"
			if env.Error != nil && env.Error.Message != "" {
				msg = env.Error.Message
			}
			return ValidateResult{}, fmt.Errorf("catalog validate envelope success=false: %s", msg)
		}
		result := env.Data
		// Cache positive results only. Negative results stay un-cached so a
		// fixed catalog state propagates on the next switcher click.
		if result.Valid {
			c.mu.Lock()
			c.cache[key] = cachedValidation{
				result:   result,
				expireAt: c.now().Add(catalogPositiveCacheTTL),
			}
			c.mu.Unlock()
		}
		return result, nil

	case http.StatusBadRequest:
		return ValidateResult{}, fmt.Errorf("catalog rejected request (400): %s", string(body))

	default:
		c.log.Warnw("catalog validate non-2xx",
			"status", resp.StatusCode,
			"shikimori_id", shikimoriID,
			"player", player,
		)
		return ValidateResult{}, fmt.Errorf("catalog internal error (status %d): %s",
			resp.StatusCode, string(body))
	}
}
