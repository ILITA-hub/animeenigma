package service

import (
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// HLSSessions aggregates HLS-proxy egress into ONE effect row per
// (sess-token, upstream-host) — never one row per ~6s segment (AR-EGRESS-04,
// D-03/D-04/D-05). A manifest rewrite mints one ?sess= token (libs/videoutils
// rewriteHLSURL); every segment GET under that token lands here via Observe and
// is tallied into a per-session sessionTally. An idle-timeout reaper emits a
// single aggregated tracing.Effect when a session goes quiet, then deletes it.
//
// Concurrency contract (T-02-LOCK / Pitfall 5): the mutex guards ONLY the map
// mutation. It is NEVER held across io.Copy — Observe takes per-segment byte
// counts that the caller already measured, so the lock window is a few map
// operations. The map is bounded (maxSessions) with oldest-eviction on
// overflow so a flood of distinct tokens cannot OOM the process (T-02-DOS).
type HLSSessions struct {
	sink       tracing.EffectSink
	idleWindow time.Duration // flush a session after this much inactivity (30–60s)
	maxEntries int           // hard map-size cap; oldest evicted on overflow

	mu       sync.Mutex
	sessions map[sessKey]*sessionTally

	// now is the clock; overridable in tests for deterministic flushing.
	now func() time.Time

	stop   chan struct{}
	doneWG sync.WaitGroup
	once   sync.Once
}

// sessKey identifies a session by its per-manifest token + upstream host. The
// host is part of the key so a watch whose segments rotate across hosts (rare)
// produces one row per (token, host) pair, matching AR-EGRESS-04 semantics.
type sessKey struct {
	sess string
	host string
}

// sessionTally accumulates one session's egress between flushes.
type sessionTally struct {
	bytesIn   uint64
	bytesOut  uint64
	segments  uint32
	firstSeen time.Time
	lastSeen  time.Time
	host      string
	provider  string
	operation string
	userID    string
}

const (
	defaultIdleWindow = 45 * time.Second // tunable 30–60s (D-03)
	defaultMaxEntries = 10000            // bound the map (T-02-DOS)
	reaperInterval    = 10 * time.Second // reaper scan cadence
)

// NewHLSSessions constructs an aggregator. idleWindow<=0 and maxEntries<=0 fall
// back to sane defaults. Call Start() to launch the reaper and Stop() to flush
// + halt on graceful shutdown.
func NewHLSSessions(sink tracing.EffectSink, idleWindow time.Duration, maxEntries int) *HLSSessions {
	if idleWindow <= 0 {
		idleWindow = defaultIdleWindow
	}
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}
	return &HLSSessions{
		sink:       sink,
		idleWindow: idleWindow,
		maxEntries: maxEntries,
		sessions:   make(map[sessKey]*sessionTally),
		now:        time.Now,
		stop:       make(chan struct{}),
	}
}

// Mint captures the manifest-fetch baggage (provider/operation/user_id) for a
// session token at the moment the manifest is proxied. Segment GETs are fresh
// browser requests that carry only ?sess=, so this is the only point where the
// inbound attribution is available. Mint is idempotent-ish: if the session
// already exists (a segment arrived first) it backfills any empty attribution
// fields without resetting the byte/segment tallies.
func (s *HLSSessions) Mint(sess, host, provider, operation, userID string) {
	if sess == "" || s.sink == nil {
		return
	}
	now := s.now()
	key := sessKey{sess: sess, host: host}

	s.mu.Lock()
	defer s.mu.Unlock()

	t := s.sessions[key]
	if t == nil {
		s.evictIfFullLocked()
		t = &sessionTally{firstSeen: now, lastSeen: now, host: host}
		s.sessions[key] = t
	}
	if t.provider == "" {
		t.provider = provider
	}
	if t.operation == "" {
		t.operation = operation
	}
	if t.userID == "" {
		t.userID = userID
	}
}

// Observe folds one segment GET's byte counts into its session tally. The byte
// counts are measured by the caller (the proxy's countReader for bytes_in and
// the client-sink CountingResponseWriter for bytes_out), so NO copy happens
// under the lock (T-02-LOCK). A sess=="" (rand-failed manifest) is ignored —
// those segments fall back to no aggregation rather than colliding on one key.
func (s *HLSSessions) Observe(sess, host string, bytesIn, bytesOut uint64) {
	if sess == "" || s.sink == nil {
		return
	}
	now := s.now()
	key := sessKey{sess: sess, host: host}

	s.mu.Lock()
	defer s.mu.Unlock()

	t := s.sessions[key]
	if t == nil {
		s.evictIfFullLocked()
		t = &sessionTally{firstSeen: now, host: host}
		s.sessions[key] = t
	}
	t.bytesIn += bytesIn
	t.bytesOut += bytesOut
	t.segments++
	t.lastSeen = now
}

// evictIfFullLocked drops the oldest (by lastSeen) session when the map is at
// capacity, flushing it so its data is not lost. Caller MUST hold s.mu.
func (s *HLSSessions) evictIfFullLocked() {
	if len(s.sessions) < s.maxEntries {
		return
	}
	var oldestKey sessKey
	var oldest *sessionTally
	for k, t := range s.sessions {
		if oldest == nil || t.lastSeen.Before(oldest.lastSeen) {
			oldestKey, oldest = k, t
		}
	}
	if oldest != nil {
		s.recordLocked(oldestKey, oldest)
		delete(s.sessions, oldestKey)
	}
}

// recordLocked emits one aggregated Effect for a tally. Caller MUST hold s.mu;
// the sink Record is required to be non-blocking (the Producer is). Kept under
// the lock only because Record is contractually fast — no io here.
func (s *HLSSessions) recordLocked(key sessKey, t *sessionTally) {
	if s.sink == nil || t.segments == 0 {
		return
	}
	duration := t.lastSeen.Sub(t.firstSeen)
	s.sink.Record(tracing.Effect{
		Origin:     "api",
		Operation:  t.operation,
		UserID:     t.userID,
		EffectKind: "egress",
		Host:       t.host,
		Provider:   t.provider,
		Target:     t.host,
		Status:     200,
		BytesIn:    int(t.bytesIn),
		BytesOut:   int(t.bytesOut),
		DurationMS: int(duration.Milliseconds()),
		Requests:   int(t.segments),
	})
}

// flushIdle scans the map and emits+deletes every session idle for longer than
// idleWindow as of `now`. Separated from the reaper loop so tests can drive
// flushing deterministically with an injected clock.
func (s *HLSSessions) flushIdle(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, t := range s.sessions {
		if now.Sub(t.lastSeen) >= s.idleWindow {
			s.recordLocked(key, t)
			delete(s.sessions, key)
		}
	}
}

// flushAll emits+deletes EVERY open session regardless of idle window. Used by
// Stop() for graceful shutdown (D-06).
func (s *HLSSessions) flushAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, t := range s.sessions {
		s.recordLocked(key, t)
		delete(s.sessions, key)
	}
}

// len returns the current map size (test/diagnostics helper).
func (s *HLSSessions) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// Start launches the background idle reaper. Safe to call once; subsequent
// calls are no-ops.
func (s *HLSSessions) Start() {
	if s.sink == nil {
		return
	}
	s.doneWG.Add(1)
	go func() {
		defer s.doneWG.Done()
		ticker := time.NewTicker(reaperInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.flushIdle(s.now())
			}
		}
	}()
}

// Stop halts the reaper and flushes all open sessions (D-06). Idempotent.
func (s *HLSSessions) Stop() {
	s.once.Do(func() {
		close(s.stop)
	})
	s.doneWG.Wait()
	s.flushAll()
}
