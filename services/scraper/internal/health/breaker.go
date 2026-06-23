package health

import (
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// breakerWindow is the trailing window over which wedged errors are counted.
const breakerWindow = 60 * time.Second

// breakerThreshold is the wedged-error count within breakerWindow that trips
// the breaker (forces the provider's health-cache entry DOWN).
const breakerThreshold = 3

// breakerHalfOpen is how long a tripped breaker stays closed before it lets one
// trial request through (half-open). It mirrors the spec's "half-open after
// 120s". Must be > breakerWindow so a single storm cannot immediately re-trip
// through the half-open path.
const breakerHalfOpen = 120 * time.Second

// providerBreaker is the per-provider breaker state.
type providerBreaker struct {
	fails     []time.Time // wedged-error timestamps within the trailing window
	trippedAt time.Time   // zero == not tripped
}

// Breaker is a per-provider circuit breaker that drives the InMemoryHealthCache.
// It counts sidecar "wedged" errors (provider pool wedged / over budget) and,
// at breakerThreshold within breakerWindow, forces the provider's health-cache
// entry DOWN so the orchestrator skips it per-request (orchestrator.go:317,536).
// It half-opens after breakerHalfOpen and clears on a single success.
//
// The breaker holds NO durable state beyond the in-memory cache it drives — it
// is safe across restarts (the durable signal of record is the catalog
// stream_providers.status row, written by the Phase 5 probe).
//
// Locking: a single mutex guards the per-provider map; no I/O under the lock.
type Breaker struct {
	mu    sync.Mutex
	cache *InMemoryHealthCache
	state map[string]*providerBreaker
	now   func() time.Time
}

// NewBreaker wires a breaker to an InMemoryHealthCache using the wall clock.
func NewBreaker(cache *InMemoryHealthCache) *Breaker {
	return NewBreakerWithNow(cache, time.Now)
}

// NewBreakerWithNow is the test constructor (injectable clock).
func NewBreakerWithNow(cache *InMemoryHealthCache, now func() time.Time) *Breaker {
	return &Breaker{
		cache: cache,
		state: make(map[string]*providerBreaker),
		now:   now,
	}
}

// Record feeds one sidecar outcome for `provider` into the breaker. wedged=true
// means the sidecar returned a wedged-kind error (sidecar.IsWedged); wedged=false
// means a success OR a non-wedged failure (challenge/empty) — both are treated
// as evidence the pool is NOT wedged and clear a tripped breaker.
//
// A nil receiver is a no-op so callers can run unconditionally even when the
// breaker is not configured.
func (b *Breaker) Record(provider string, wedged bool) {
	if b == nil || provider == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	pb := b.state[provider]
	if pb == nil {
		pb = &providerBreaker{}
		b.state[provider] = pb
	}
	now := b.now()

	if !wedged {
		// Success / non-wedged: clear the window; if tripped, rejoin immediately.
		pb.fails = pb.fails[:0]
		if !pb.trippedAt.IsZero() {
			pb.trippedAt = time.Time{}
			b.cache.Update(provider, upEntry(now))
		}
		return
	}

	// Tripped + within the closed window: keep it DOWN (re-stamp so the skip
	// never goes stale during a sustained storm) and do NOT re-trip.
	if !pb.trippedAt.IsZero() && now.Sub(pb.trippedAt) < breakerHalfOpen {
		b.cache.Update(provider, downEntry(now))
		return
	}

	// Tripped + past half-open: this wedged error is the trial. Reset to a fresh
	// window (counting this error) and rejoin (UP) so one request gets through;
	// if it is still wedged, the normal threshold logic below re-trips it.
	if !pb.trippedAt.IsZero() {
		pb.trippedAt = time.Time{}
		pb.fails = pb.fails[:0]
		b.cache.Update(provider, upEntry(now))
	}

	// Append + prune to the trailing window.
	pb.fails = append(pb.fails, now)
	cutoff := now.Add(-breakerWindow)
	kept := pb.fails[:0]
	for _, t := range pb.fails {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	pb.fails = kept

	if len(pb.fails) >= breakerThreshold {
		pb.trippedAt = now
		b.cache.Update(provider, downEntry(now))
		metrics.ProviderBreakerTripsTotal.WithLabelValues(provider).Inc()
	}
}

// downEntry / upEntry build the single-stage cache entry IsHealthy reads
// (StageStreamSegment, LastUpdated=now). Mirrors the orchestrator test helpers.
func downEntry(now time.Time) ProviderHealth {
	return ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: false, LastErr: "circuit breaker: provider wedged"}},
		LastUpdated: now,
	}
}

func upEntry(now time.Time) ProviderHealth {
	return ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: true, LastOK: now}},
		LastUpdated: now,
	}
}
