package service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// countingGetter is a roomGetter fake that records how many times GetRoom was
// invoked and returns a canned Room (or error). Real code over mocks: it
// satisfies the same roomGetter interface that *repo.RoomRepo does, so the
// cache + engine exercise their production read path without live Redis.
type countingGetter struct {
	mu    sync.Mutex
	calls int
	room  *domain.Room
	err   error
}

func (g *countingGetter) GetRoom(_ context.Context, roomID string) (*domain.Room, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.err != nil {
		return nil, g.err
	}
	// Return a copy so callers can't mutate the canonical fixture.
	r := *g.room
	r.ID = roomID
	return &r, nil
}

func (g *countingGetter) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func sampleRoom(roomID string) *domain.Room {
	return &domain.Room{
		ID:                      roomID,
		PlaybackState:           domain.StatePlaying,
		PlaybackTime:            100.0,
		PlaybackTimeUpdatedAtMs: 1_700_000_000_000,
		HostUserID:              "host",
	}
}

// ----------------------------------------------------------------------------
// RoomCache Test 1 — repeated Get within the TTL window hits the underlying
// getter exactly once (the cache coalesces the per-tick HGETALL).
// ----------------------------------------------------------------------------

func TestRoomCache_GetWithinTTL_FetchesOnce(t *testing.T) {
	g := &countingGetter{room: sampleRoom("room-1")}
	cache := NewRoomCache(g)

	const ticks = 10
	for i := 0; i < ticks; i++ {
		got, err := cache.GetRoom(context.Background(), "room-1")
		if err != nil {
			t.Fatalf("Get tick %d: %v", i, err)
		}
		if got == nil || got.ID != "room-1" {
			t.Fatalf("Get tick %d returned %+v, want room-1", i, got)
		}
	}

	if n := g.callCount(); n != 1 {
		t.Fatalf("getter called %d times across %d ticks, want 1 (cache coalesces)", n, ticks)
	}
}

// ----------------------------------------------------------------------------
// RoomCache Test 2 — advancing the clock past the TTL triggers a fresh fetch.
// ----------------------------------------------------------------------------

func TestRoomCache_GetAfterTTL_Refetches(t *testing.T) {
	g := &countingGetter{room: sampleRoom("room-1")}
	cache := NewRoomCache(g)

	base := time.Unix(1_700_000_000, 0)
	now := base
	cache.SetClockForTest(func() time.Time { return now })

	if _, err := cache.GetRoom(context.Background(), "room-1"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if n := g.callCount(); n != 1 {
		t.Fatalf("after first Get getter calls = %d, want 1", n)
	}

	// Still within TTL — no refetch.
	now = base.Add(roomCacheTTL / 2)
	if _, err := cache.GetRoom(context.Background(), "room-1"); err != nil {
		t.Fatalf("within-TTL Get: %v", err)
	}
	if n := g.callCount(); n != 1 {
		t.Fatalf("within-TTL getter calls = %d, want 1", n)
	}

	// Past the TTL — must refetch.
	now = base.Add(roomCacheTTL + time.Millisecond)
	if _, err := cache.GetRoom(context.Background(), "room-1"); err != nil {
		t.Fatalf("post-TTL Get: %v", err)
	}
	if n := g.callCount(); n != 2 {
		t.Fatalf("post-TTL getter calls = %d, want 2 (TTL expired)", n)
	}
}

// ----------------------------------------------------------------------------
// RoomCache Test 3 — Invalidate forces the next Get to refetch (write-path
// invalidation: a seek/pause anchor must be visible on the very next tick,
// not after the TTL elapses).
// ----------------------------------------------------------------------------

func TestRoomCache_Invalidate_ForcesRefetch(t *testing.T) {
	g := &countingGetter{room: sampleRoom("room-1")}
	cache := NewRoomCache(g)

	if _, err := cache.GetRoom(context.Background(), "room-1"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if n := g.callCount(); n != 1 {
		t.Fatalf("after first Get getter calls = %d, want 1", n)
	}

	// A write happened — invalidate.
	cache.Invalidate("room-1")

	// Next Get must hit the getter again even though the TTL has not elapsed.
	if _, err := cache.GetRoom(context.Background(), "room-1"); err != nil {
		t.Fatalf("post-invalidate Get: %v", err)
	}
	if n := g.callCount(); n != 2 {
		t.Fatalf("post-invalidate getter calls = %d, want 2", n)
	}
}

// ----------------------------------------------------------------------------
// RoomCache Test 4 — getter errors (e.g. repo.ErrNotFound) are propagated and
// NOT cached, so a transient miss can recover on the next tick.
// ----------------------------------------------------------------------------

func TestRoomCache_GetterError_NotCached(t *testing.T) {
	g := &countingGetter{err: repo.ErrNotFound}
	cache := NewRoomCache(g)

	_, err := cache.GetRoom(context.Background(), "gone")
	if !stderrors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want repo.ErrNotFound", err)
	}

	// A second Get must still hit the getter (errors are not cached).
	_, _ = cache.GetRoom(context.Background(), "gone")
	if n := g.callCount(); n != 2 {
		t.Fatalf("getter calls after two error Gets = %d, want 2 (errors not cached)", n)
	}
}

// ----------------------------------------------------------------------------
// RoomCache Test 5 — distinct rooms cache independently.
// ----------------------------------------------------------------------------

func TestRoomCache_DistinctRooms(t *testing.T) {
	g := &countingGetter{room: sampleRoom("ignored")}
	cache := NewRoomCache(g)

	if _, err := cache.GetRoom(context.Background(), "room-A"); err != nil {
		t.Fatalf("room-A Get: %v", err)
	}
	if _, err := cache.GetRoom(context.Background(), "room-B"); err != nil {
		t.Fatalf("room-B Get: %v", err)
	}
	// Re-Get room-A — should be cached.
	if _, err := cache.GetRoom(context.Background(), "room-A"); err != nil {
		t.Fatalf("room-A re-Get: %v", err)
	}
	if n := g.callCount(); n != 2 {
		t.Fatalf("getter calls = %d, want 2 (one per distinct room)", n)
	}
}

// ----------------------------------------------------------------------------
// Engine Test — OnTimeTick driven N times within the TTL window reads the
// canonical room from the cache once, not N times. This is the hot-path read
// the finding (L802) is about: ~1 HGETALL/sec/member during active co-watch
// collapses to near-zero.
// ----------------------------------------------------------------------------

func TestDriftEngine_OnTimeTick_UsesRoomCache(t *testing.T) {
	g := &countingGetter{room: sampleRoom("room-1")}
	cache := NewRoomCache(g)
	engine := NewDriftEngine(nil)

	nowMs := int64(1_700_000_000_000)
	const ticks = 8
	for i := 0; i < ticks; i++ {
		// reported == room time so every tick is in-sync (DriftNone) — keeps
		// the focus on the read count, not the correction logic.
		if _, err := engine.OnTimeTick(context.Background(), cache, "room-1", "alice", 100.0, nowMs); err != nil {
			t.Fatalf("OnTimeTick tick %d: %v", i, err)
		}
	}

	if n := g.callCount(); n != 1 {
		t.Fatalf("getter called %d times across %d ticks, want 1 (engine reads via cache)", n, ticks)
	}
}

// ----------------------------------------------------------------------------
// Router integration — a write (playback:seek) must invalidate the room cache
// so the very next time_tick computes drift against the NEW anchor, not the
// stale one the previous tick cached. Without write-path invalidation this
// test fails: the second tick would still see the pre-seek anchor and report
// in-sync (no correction).
//
// Uses the real miniredis-backed router fixture from inbound_test.go so the
// Redis write + cache read + invalidation all run production code.
// ----------------------------------------------------------------------------

func TestRouter_TimeTick_AfterSeek_CorrectsAgainstNewAnchor(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-cache-invalidate"

	// Paused room at playback_time=0. Paused means expected == playback_time
	// (no wall-clock advance), so drift is purely |reported - playback_time|
	// — deterministic regardless of the pinned clock.
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePaused
	r.PlaybackTime = 0
	fx.seedRoom(t, r)

	// Tick 1: reported 0 → in-sync (drift 0). Populates the cache with the
	// playback_time=0 anchor. No correction expected.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackTimeTick, map[string]interface{}{"time": 0.0})
	if _, ok := fx.hub.findFirst("SendTo", domain.MsgPlaybackCorrection); ok {
		t.Fatalf("tick 1 unexpectedly produced a correction; calls=%v", fx.hub.snapshot())
	}

	// Seek to 200 → repo.UpdateRoomState writes playback_time=200 AND the
	// router invalidates the room cache.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 200.0})

	// Sanity: Redis actually holds the new anchor.
	got, err := fx.repo.GetRoom(context.Background(), roomID)
	if err != nil {
		t.Fatalf("GetRoom after seek: %v", err)
	}
	if got.PlaybackTime != 200.0 {
		t.Fatalf("post-seek playback_time = %v, want 200", got.PlaybackTime)
	}

	// Tick 2: reported 0 again. Expected is now 200 (paused) → drift = 200 →
	// hard correction toward ~200. If the cache had NOT been invalidated the
	// engine would still see playback_time=0 → drift 0 → no correction.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackTimeTick, map[string]interface{}{"time": 0.0})

	call, ok := fx.hub.findFirst("SendTo", domain.MsgPlaybackCorrection)
	if !ok {
		t.Fatalf("tick 2 produced no correction — cache was not invalidated after seek; calls=%v", fx.hub.snapshot())
	}
	var c domain.PlaybackCorrectionData
	if err := json.Unmarshal(call.env.Data, &c); err != nil {
		t.Fatalf("unmarshal correction: %v", err)
	}
	if c.Time < 199.9 || c.Time > 200.1 {
		t.Errorf("correction.Time = %v, want ~200 (corrected toward the post-seek anchor)", c.Time)
	}
}
