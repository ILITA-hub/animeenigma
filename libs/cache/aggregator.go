package cache

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// CacheAggregator counts cache hit/miss/error outcomes per
// (key_class, result, operation) and flushes ONE summed `cache` effect row per
// counter on a ~10s interval (D-05/D-06/D-10). It is a deliberate clone of the
// Phase-2 HLSSessions reaper (services/streaming/internal/service/hls_sessions.go):
// same lock-only-on-map discipline, same oldest-eviction bound, same injectable
// clock, same graceful flushAll-on-Stop.
//
// Concurrency contract (T-03-11 / Pitfall 3): the mutex guards ONLY the map
// increment. The coarse baggage operation is read from ctx OUTSIDE the lock, and
// the lock is NEVER held across any IO. The map is bounded (maxEntries) with
// oldest-eviction so a flood of distinct (class,result,op) keys cannot OOM the
// process (T-03-10).
//
// Cache rows carry NO user_id and NO trace_id by design (D-06): Observe takes no
// user_id, and the emitted Effect sets only operation + key_class + result.
// Per D-10/A5 the operation is the COARSE baggage op (a cheap ctx read), never a
// per-op runtime.Callers stack-walk (which would violate D-11 on every Get/Set).
type CacheAggregator struct {
	sink          tracing.EffectSink
	flushInterval time.Duration // emit summed rows on this cadence (~10s, D-05)
	maxEntries    int           // hard map-size cap; oldest evicted on overflow (T-03-10)

	mu       sync.Mutex
	counters map[counterKey]*tally

	// now is the clock; overridable in tests for deterministic flushing.
	now func() time.Time

	stop   chan struct{}
	doneWG sync.WaitGroup
	once   sync.Once
}

// counterKey identifies one aggregated cache counter. result is the hit/miss/
// error/success outcome; operation is the coarse baggage op read at Observe-time.
type counterKey struct {
	keyClass  string
	result    string
	operation string
}

// tally accumulates one counter's request count between flushes.
type tally struct {
	requests  uint32
	firstSeen time.Time
	lastSeen  time.Time
}

const (
	defaultFlushInterval = 10 * time.Second // ~10s summed-row cadence (A2/D-05)
	// defaultMaxEntries is shared with the bound used by the HLS reaper analog;
	// the cache class set is small, but ops × classes × results can grow, so the
	// same 10k bound applies.
	defaultCacheMaxEntries = 10000
)

// NewCacheAggregator constructs an aggregator. flushInterval<=0 and maxEntries<=0
// fall back to sane defaults. A nil sink makes Observe/flush a no-op (mirrors the
// HLSSessions sink==nil guard) so cache-less call paths and tests need no
// aggregator. Call Start() to launch the flusher and Stop() to drain + halt.
func NewCacheAggregator(sink tracing.EffectSink, flushInterval time.Duration, maxEntries int) *CacheAggregator {
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	if maxEntries <= 0 {
		maxEntries = defaultCacheMaxEntries
	}
	return &CacheAggregator{
		sink:          sink,
		flushInterval: flushInterval,
		maxEntries:    maxEntries,
		counters:      make(map[counterKey]*tally),
		now:           time.Now,
		stop:          make(chan struct{}),
	}
}

// Observe folds one cache outcome into its (key_class, result, operation)
// counter. The coarse operation is read from ctx baggage ONCE, OUTSIDE the lock
// (D-10/A5 — never a stack-walk here, T-03-12); the lock then brackets only the
// map increment (T-03-11 / Pitfall 3, cloned from hls_sessions.go Observe). A nil
// sink is a no-op.
func (a *CacheAggregator) Observe(ctx context.Context, keyClass, result string) {
	if a.sink == nil {
		return
	}
	// Cheap ctx read OUTSIDE the lock. When no operation is seeded we fall back to
	// the SAME origin-shaped label the effect resolver uses
	// (tracing.FallbackOperationName → "goroutine/<purpose>" /
	// "scheduled_job/<purpose>", defaulting to "goroutine/unknown"), so a
	// frame-less cache effect reads identically to a frame-less egress/db effect
	// instead of leaking a bare raw origin or the literal "unknown".
	_, op := tracing.ReadBaggage(ctx)
	if op == "" {
		op = tracing.FallbackOperationName(ctx)
	}
	now := a.now()
	key := counterKey{keyClass: keyClass, result: result, operation: op}

	a.mu.Lock()
	defer a.mu.Unlock()

	t := a.counters[key]
	if t == nil {
		a.evictIfFullLocked()
		t = &tally{firstSeen: now}
		a.counters[key] = t
	}
	t.requests++
	t.lastSeen = now
}

// evictIfFullLocked drops the oldest (by lastSeen) counter when the map is at
// capacity, flushing it so its data is not lost. Caller MUST hold a.mu.
// (Cloned from hls_sessions.go evictIfFullLocked — T-03-10 DoS guard.)
func (a *CacheAggregator) evictIfFullLocked() {
	if len(a.counters) < a.maxEntries {
		return
	}
	var oldestKey counterKey
	var oldest *tally
	for k, t := range a.counters {
		if oldest == nil || t.lastSeen.Before(oldest.lastSeen) {
			oldestKey, oldest = k, t
		}
	}
	if oldest != nil {
		a.recordLocked(oldestKey, oldest)
		delete(a.counters, oldestKey)
	}
}

// recordLocked emits one aggregated `cache` Effect for a counter. Caller MUST
// hold a.mu; Record is contractually non-blocking so it is safe under the lock.
// The Effect carries operation + key_class + result only — NO user_id, NO
// trace_id (D-06). result is folded into Operation as "<op> [<result>]" since the
// Effect wire contract has no dedicated result field; key_class rides Target with
// TargetKind="key_class".
func (a *CacheAggregator) recordLocked(key counterKey, t *tally) {
	if a.sink == nil || t.requests == 0 {
		return
	}
	a.sink.Record(tracing.Effect{
		Origin:     "api",
		Operation:  key.operation + " [" + key.result + "]",
		EffectKind: "cache",
		Target:     key.keyClass,
		TargetKind: "key_class",
		Requests:   int(t.requests),
	})
}

// flush emits+deletes every counter as of `now`. The cache aggregator flushes ALL
// counters each tick (cache outcomes are cheap to re-accumulate, and a ~10s
// summed cadence is the design — there is no per-counter idle window like the HLS
// reaper). Separated from the flusher loop so tests drive it deterministically.
func (a *CacheAggregator) flush() {
	a.flushAll()
}

// flushAll emits+deletes EVERY counter regardless of age. Used by the flusher
// tick and by Stop() for graceful shutdown (D-06).
func (a *CacheAggregator) flushAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for key, t := range a.counters {
		a.recordLocked(key, t)
		delete(a.counters, key)
	}
}

// len returns the current counter-map size (test/diagnostics helper).
func (a *CacheAggregator) len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.counters)
}

// Start launches the background flusher. Safe to call once; subsequent calls are
// no-ops. A nil sink skips the goroutine entirely.
func (a *CacheAggregator) Start() {
	if a.sink == nil {
		return
	}
	a.doneWG.Add(1)
	go func() {
		defer a.doneWG.Done()
		ticker := time.NewTicker(a.flushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-a.stop:
				return
			case <-ticker.C:
				a.flush()
			}
		}
	}()
}

// Stop halts the flusher and drains all outstanding counters (D-06). Idempotent.
func (a *CacheAggregator) Stop() {
	a.once.Do(func() {
		close(a.stop)
	})
	a.doneWG.Wait()
	a.flushAll()
}
