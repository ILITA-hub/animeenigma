package health

import (
	"sync"
	"time"
)

// cacheStaleTTL is the read-side staleness threshold (RESEARCH D3 + P-08).
// An entry older than this is treated as "unknown" and the cache fails open
// — IsHealthy returns true so the orchestrator keeps dispatching. This
// guarantees a probe outage cannot blank the service.
const cacheStaleTTL = 60 * time.Second

// MaxLastErrChars is the upper bound on LastErr length BEFORE storage. The
// probe (Plan 17-02) is the writer; it MUST truncate before calling Update.
// The admin handler (Plan 17-03) may also re-truncate as defense-in-depth.
// See RESEARCH P-05 + T-17-01-02 (information-disclosure threat).
const MaxLastErrChars = 256

// StageStatus is the cached state for ONE (provider, stage) pair.
// LastErr is truncated to MaxLastErrChars BEFORE storage (per RESEARCH P-05);
// downstream readers (admin handler) should not need to re-truncate but MAY
// do so as defense-in-depth.
type StageStatus struct {
	Up      bool      `json:"up"`
	LastOK  time.Time `json:"last_ok"`
	LastErr string    `json:"last_err,omitempty"`
}

// ProviderHealth is the cached state for ONE provider across all stages.
// LastUpdated is the timestamp of the last probe tick that wrote this entry —
// used by IsHealthy to detect stale (>cacheStaleTTL) entries and fail open.
type ProviderHealth struct {
	Stages      map[string]StageStatus `json:"stages"`
	LastUpdated time.Time              `json:"last_updated"`
}

// InMemoryHealthCache is the orchestrator-owned health cache. RWMutex-protected
// map. Written by the probe (15-min cadence), read by every request via the
// failover loop. Fail-open semantics (RESEARCH P-08): missing entries OR stale
// entries (>60s) return IsHealthy=true so a probe outage does NOT blank the
// service.
//
// Locking discipline (REVIEW.md CR-02): only the in-memory map is touched
// under the lock — no I/O. Snapshot then release if iteration is needed.
type InMemoryHealthCache struct {
	mu    sync.RWMutex
	state map[string]ProviderHealth
	now   func() time.Time
}

// NewInMemoryHealthCache constructs a production-ready cache wired to
// time.Now. Use NewInMemoryHealthCacheWithNow in tests that need to drive
// the stale-TTL branch deterministically.
func NewInMemoryHealthCache() *InMemoryHealthCache {
	return &InMemoryHealthCache{
		state: make(map[string]ProviderHealth),
		now:   time.Now,
	}
}

// NewInMemoryHealthCacheWithNow is the test constructor; production callers
// MUST use NewInMemoryHealthCache (which threads time.Now).
func NewInMemoryHealthCacheWithNow(now func() time.Time) *InMemoryHealthCache {
	return &InMemoryHealthCache{
		state: make(map[string]ProviderHealth),
		now:   now,
	}
}

// IsHealthy returns true if the provider's `stream_segment` stage was UP
// within the last cacheStaleTTL. The four fail-open branches are:
//
//  1. No entry for this provider             → true (unknown)
//  2. Entry older than cacheStaleTTL         → true (stale)
//  3. No stream_segment key in the Stages map → true (no oracle data)
//  4. Stages[stream_segment].Up == true       → true
//
// Only branch 4-inverse (fresh entry, has stream_segment, Up=false) returns
// false — the single condition that causes the orchestrator to skip.
//
// IMPORTANT — divergence with alerting (REVIEW.md WR-03):
// Branch 3 (missing stream_segment key) is hit whenever the probe SHORT-
// CIRCUITED on an earlier pipeline stage (search/episodes/servers/stream)
// and never reached stream_segment. In that case:
//
//   - IsHealthy returns TRUE (orchestrator keeps dispatching). Rationale:
//     the probe's search/FindID may fail for golden-pool reasons (MAL ID
//     drift, anime removed) without user lookups also failing. Fail-open
//     here matches the broader "probe outage MUST NOT blank the service"
//     posture from RESEARCH P-08.
//
//   - The alert rule `provider-health-stream-segment-down` only fires on
//     a fresh stream_segment Up=false event — NOT on missing-key. So a
//     provider whose first stage is broken will keep getting traffic AND
//     will not page anyone.
//
// This is by design — the alert rule
// `provider-health-search-down` (or equivalent for whichever earlier
// stage flipped) covers the page-the-operator side. The orchestrator
// gate is intentionally less aggressive than the alert: a single broken
// upstream stage is not a reason to immediately stop dispatching, because
// real users may still get good streams via the failover chain or via
// the partial pipeline that does work.
//
// If a future Phase decides the gate should be more aggressive (e.g.
// "any DOWN stage = skip"), update both IsHealthy AND the alert rules
// in one PR — the two MUST agree on the gate semantics.
func (c *InMemoryHealthCache) IsHealthy(provider string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h, ok := c.state[provider]
	if !ok {
		return true // unknown = fail-open
	}
	if c.now().Sub(h.LastUpdated) > cacheStaleTTL {
		return true // stale = fail-open
	}
	seg, ok := h.Stages[StageStreamSegment]
	if !ok {
		return true // no oracle = fail-open
	}
	return seg.Up
}

// Update overwrites the cached entry for `provider`. LastErr fields inside
// `h.Stages` MUST already be truncated by the caller (probe) to MaxLastErrChars.
func (c *InMemoryHealthCache) Update(provider string, h ProviderHealth) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state[provider] = h
}

// AdminSnapshot returns a deep copy of the cache state for the admin endpoint.
// Mutations to the returned map (and its nested Stages maps) do NOT affect
// the cache.
//
// Plan 17-03 will JSON-marshal this directly; deep-copy semantics mean any
// downstream redaction (e.g. truncating LastErr again) won't write back into
// the live cache by accident.
func (c *InMemoryHealthCache) AdminSnapshot() map[string]ProviderHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]ProviderHealth, len(c.state))
	for k, v := range c.state {
		stages := make(map[string]StageStatus, len(v.Stages))
		for sk, sv := range v.Stages {
			stages[sk] = sv
		}
		out[k] = ProviderHealth{Stages: stages, LastUpdated: v.LastUpdated}
	}
	return out
}
