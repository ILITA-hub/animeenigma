// Package handler — request-level negative-result cache (owner directive
// 2026-07-20).
//
// Problem: the per-provider caches (episodes 6h, servers 15m, streams ≤5m)
// only remember SUCCESS. A failing chain — a Cloudflare-walled provider, a
// missing episode — re-ran the full resolve (FindID included, which for
// browser-engine providers means a Camoufox sidecar session + challenge
// solve) on EVERY read. During the 07-18→07-20 miruro/animepahe outage that
// added up to ~2000 pointless challenge attempts against our own IP
// reputation, driven mostly by content-verify's enumeration loop.
//
// Fix: cache the NEGATIVE outcome of a whole request identity for 1h at the
// single choke point every driver shares (aePlayer, content-verify, probes —
// all arrive through these handlers via the catalog forwarder). While an
// entry is live, the identical request is answered instantly from Redis with
// the stored error (503/404) + `retry_after_seconds`, and no provider — and
// no sidecar — is touched. Entries simply expire; a provider recovering
// mid-window becomes visible to a given request key at most 1h later (the
// scraper's own health probe bypasses HTTP, so health/policy recovery is NOT
// delayed by this cache).
package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// NegTTL is how long a negative result is served from cache before the chain
// is allowed to run again. 1h per the owner directive; consumers learn the
// remaining window via `error.retry_after_seconds` in the 503/404 body.
const NegTTL = time.Hour

// negOpTimeout bounds the Redis round-trips so a cache outage can never
// stall a request — every miss/error falls through to the live chain.
const negOpTimeout = 2 * time.Second

// negCacheTotal counts negative-cache activity per (operation, event).
// event: "hit" = request served from a cached negative (zero provider work);
// "store" = a fresh chain failure was recorded.
var negCacheTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "scraper_negcache_total",
		Help: "Scraper negative-result cache events (hit = served from cache, store = failure recorded)",
	},
	[]string{"operation", "event"},
)

// negEntry is the stored shape of one negative result. Status/Code/Message
// mirror the writeError envelope so a cache hit replays the original error;
// Until lets the handler compute an honest retry_after_seconds.
type negEntry struct {
	Status  int       `json:"status"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Tried   []string  `json:"tried"`
	Until   time.Time `json:"until"`
}

// NegCache is a namespaced 1h negative-result cache over libs/cache. The EN
// and adult handler chains share one Redis, so each gets its own namespace
// ("en"/"adult") — identical mal_id + op across chains must not collide.
// All methods are nil-receiver-safe and fail-open: Get treats any cache
// error as a miss (a plain miss IS an error from libs/cache, so it stays
// unlogged); Store failures are logged at debug level.
type NegCache struct {
	c   cache.Cache
	ns  string
	log *logger.Logger
	now func() time.Time
}

// NewNegCache builds a NegCache. c must be non-nil (production passes the
// service-wide Redis cache); ns distinguishes handler chains sharing Redis.
func NewNegCache(c cache.Cache, ns string, log *logger.Logger) *NegCache {
	return &NegCache{c: c, ns: ns, log: log, now: time.Now}
}

// key builds the request-identity cache key. The identity is everything that
// changes the chain's outcome: operation, mal_id, prefer/exclusive (they
// pick the provider order), and the per-op params. Title/alt-titles are
// derived from mal_id upstream and userKey is quota-only — both excluded.
func (n *NegCache) key(op string, parts ...string) string {
	return "scraper:neg:" + n.ns + ":" + op + ":" + strings.Join(parts, ":")
}

// Get returns the live negative entry for the key, or (nil, false) on miss,
// expiry, or any cache error (fail-open).
func (n *NegCache) Get(ctx context.Context, key string) (*negEntry, bool) {
	if n == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, negOpTimeout)
	defer cancel()
	var e negEntry
	if err := n.c.Get(ctx, key, &e); err != nil {
		return nil, false
	}
	if e.Status == 0 || !n.now().Before(e.Until) {
		return nil, false
	}
	return &e, true
}

// Store records a negative result for NegTTL. Detached from the request
// context so a canceled/expired request can still persist the outcome it
// paid for.
func (n *NegCache) Store(key string, status int, code, msg string, tried []string) {
	if n == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), negOpTimeout)
	defer cancel()
	e := negEntry{Status: status, Code: code, Message: msg, Tried: tried, Until: n.now().Add(NegTTL)}
	if err := n.c.Set(ctx, key, e, NegTTL); err != nil && n.log != nil {
		n.log.Debugw("negcache store failed", "key", key, "error", err)
	}
}

// retryAfterSeconds is the remaining lifetime of a stored entry, floored at
// 1s so a just-expiring entry never advertises "retry in 0".
func (e *negEntry) retryAfterSeconds(now time.Time) int {
	s := int(e.Until.Sub(now).Seconds())
	if s < 1 {
		s = 1
	}
	return s
}

// negKeyFor derives the cache key for one handler request. Empty when the
// handler has no cache attached (tests, cache-less deployments).
func (h *ScraperHandler) negKeyFor(op string, qp queryParams) string {
	if h.neg == nil {
		return ""
	}
	return h.neg.key(op, qp.malID, qp.prefer, strconv.FormatBool(qp.exclusive),
		qp.episode, qp.server, qp.category)
}
