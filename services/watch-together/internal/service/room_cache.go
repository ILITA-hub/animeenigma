// Package service — room_cache.go is a tiny in-process per-room cache for the
// canonical Room HASH (wt:room:{id}).
//
// Why it exists (audit finding watch-together L802): the drift engine
// (sync.go) reads the canonical room on EVERY 1Hz playback:time_tick per
// member to compute expected playback position. During an active co-watch
// that is ~N HGETALL/sec/room (N = members currently playing) — the dominant
// steady-state Redis load this service generates. Almost every one of those
// reads returns the identical room state, because the room only changes on a
// play/pause/seek/state-change event.
//
// This service is the SOLE writer of wt:room:{id} (every mutation flows
// through repo.UpdateRoomState, invoked only from the inbound router), so an
// in-process cache can be kept exactly consistent:
//
//   - Write-path invalidation: the inbound router calls Invalidate(roomID)
//     immediately after any UpdateRoomState, so a seek/pause/state-change
//     anchor is visible on the very NEXT tick — corrections never aim at a
//     stale anchor.
//   - TTL backstop: even absent an explicit invalidation, a cached entry is
//     re-fetched after roomCacheTTL. The TTL is held well under the 1.5s
//     DriftNone band so bounded staleness can never make the engine correct
//     toward a stale anchor.
//
// Scope: in-process state only (single-instance v1.0), same as DriftEngine,
// RateLimiter, and CatalogClient. Multi-instance scale-out is deferred to v2;
// the obvious upgrade is Redis keyspace-notification invalidation. Errors are
// never cached — a transient miss (repo.ErrNotFound after TTL expiry) recovers
// on the next tick.
package service

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
)

// roomGetter is the narrow read surface the cache wraps. *repo.RoomRepo
// satisfies it by signature, so production wires the real repo behind the
// cache; tests can pass a counting fake to assert read coalescing without
// live Redis. The DriftEngine's OnTimeTick takes this same interface so it
// reads through whatever the caller injects (cache in prod, repo in the
// existing pure drift-band tests).
type roomGetter interface {
	GetRoom(ctx context.Context, roomID string) (*domain.Room, error)
}

// roomCacheTTL bounds how long a cached Room may be served before a forced
// re-fetch. Chosen well under the 1.5s DriftNone band (sync.go
// softDriftLowerSeconds) so worst-case staleness (a write that somehow missed
// Invalidate) can never push a correction toward a stale anchor: a 1s-old
// anchor differs from the live one by at most ~1s of wall-clock advance, which
// stays inside the no-action band. In practice write-path invalidation keeps
// the cache fresher than this.
const roomCacheTTL = 1 * time.Second

// cachedRoom is a per-room cache entry. The Room pointer is treated as
// immutable once stored — callers must not mutate the returned value (the
// drift engine only reads it). fetchedAt + roomCacheTTL define expiry.
type cachedRoom struct {
	room      *domain.Room
	fetchedAt time.Time
}

// RoomCache is a TTL + write-invalidated cache over a roomGetter. Safe for
// concurrent use: mu guards the map, and the underlying GetRoom on a miss runs
// OUTSIDE the lock so a slow Redis read never serializes unrelated rooms'
// ticks.
type RoomCache struct {
	getter roomGetter

	mu      sync.Mutex
	entries map[string]cachedRoom
	now     func() time.Time // injectable for tests
}

// NewRoomCache builds a cache over getter (the real *repo.RoomRepo in
// production). Panics-free on nil getter only matters in tests; production
// always passes a live repo.
func NewRoomCache(getter roomGetter) *RoomCache {
	return &RoomCache{
		getter:  getter,
		entries: make(map[string]cachedRoom),
		now:     time.Now,
	}
}

// SetClockForTest swaps the wall-clock source for TTL arithmetic. Test-only —
// mirrors CatalogClient.SetClockForTest so the suite can advance time
// deterministically past the TTL.
func (c *RoomCache) SetClockForTest(fn func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = fn
}

// GetRoom returns the cached Room for roomID, fetching from the underlying
// getter on a miss or an expired entry. The returned pointer must be treated
// as read-only by the caller. Getter errors are propagated and never cached.
//
// Named GetRoom (not Get) so *RoomCache itself satisfies roomGetter — that's
// what lets the inbound router inject the cache wherever a roomGetter is
// expected (notably DriftEngine.OnTimeTick) as a drop-in for *repo.RoomRepo.
func (c *RoomCache) GetRoom(ctx context.Context, roomID string) (*domain.Room, error) {
	c.mu.Lock()
	entry, ok := c.entries[roomID]
	now := c.now()
	if ok && now.Sub(entry.fetchedAt) < roomCacheTTL {
		room := entry.room
		c.mu.Unlock()
		return room, nil
	}
	c.mu.Unlock()

	// Miss or expired — fetch outside the lock so a slow Redis read does not
	// block other rooms' ticks.
	room, err := c.getter.GetRoom(ctx, roomID)
	if err != nil {
		// Do not cache errors — a transient miss must recover next tick.
		return nil, err
	}

	c.mu.Lock()
	c.entries[roomID] = cachedRoom{room: room, fetchedAt: c.now()}
	c.mu.Unlock()
	return room, nil
}

// Invalidate drops the cached entry for roomID so the next Get re-fetches the
// canonical state. Called by the inbound router immediately after every
// UpdateRoomState write (play/pause/seek/state-change), making the new
// playback anchor visible on the very next time_tick. No-op if the room was
// never cached.
func (c *RoomCache) Invalidate(roomID string) {
	c.mu.Lock()
	delete(c.entries, roomID)
	c.mu.Unlock()
}
