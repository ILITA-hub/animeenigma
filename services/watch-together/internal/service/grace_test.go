package service

import (
	stderrors "errors"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// graceTestSlack is the small overrun we add when sleeping past a timer's
// expected fire time. AfterFunc + goroutine scheduling typically lands well
// under 10ms in the watch-together CI runners, but we use 50ms to keep the
// suite robust on contended hardware (and under -race where goroutine
// scheduling is noisier).
const graceTestSlack = 50 * time.Millisecond

// graceFakeHub satisfies HubFanout and records every Broadcast/SendTo call
// with a monotonic timestamp so tests can assert ordering vs. repo.DeleteRoom
// observations. Mirrors inbound_test.go's fakeHub but with order-tracking.
type graceFakeHub struct {
	mu    sync.Mutex
	calls []graceHubCall
}

type graceHubCall struct {
	method        string // "Broadcast" or "SendTo"
	roomID        string
	userID        string
	excludeUserID string
	env           domain.Envelope
	at            time.Time
}

func newGraceFakeHub() *graceFakeHub { return &graceFakeHub{} }

func (h *graceFakeHub) Broadcast(_ context.Context, roomID string, env domain.Envelope, excludeUserID string) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, graceHubCall{
		method:        "Broadcast",
		roomID:        roomID,
		excludeUserID: excludeUserID,
		env:           env,
		at:            time.Now(),
	})
	return 1, nil
}

func (h *graceFakeHub) SendTo(_ context.Context, roomID, userID string, env domain.Envelope) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, graceHubCall{
		method: "SendTo",
		roomID: roomID,
		userID: userID,
		env:    env,
		at:     time.Now(),
	})
	return 1, nil
}

func (h *graceFakeHub) snapshot() []graceHubCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]graceHubCall, len(h.calls))
	copy(out, h.calls)
	return out
}

func (h *graceFakeHub) countByType(msgType string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.calls {
		if c.env.Type == msgType {
			n++
		}
	}
	return n
}

// graceFixture wires a miniredis-backed repo + a fakeHub + a fresh
// GraceManager with the supplied period. Cleanup is registered via t.Cleanup
// so leaked goroutines (a real bug) become obvious failures.
type graceFixture struct {
	repo *repo.RoomRepo
	hub  *graceFakeHub
	mgr  *GraceManager
	mr   *miniredis.Miniredis
}

func newGraceFixture(t *testing.T, period time.Duration) *graceFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)
	h := newGraceFakeHub()
	mgr := NewGraceManager(r, h, period, log)
	t.Cleanup(mgr.Close)

	return &graceFixture{repo: r, hub: h, mgr: mgr, mr: mr}
}

// seedRoom plants a minimal room HASH so DeleteRoom can be observed by
// re-fetching and asserting ErrNotFound. Pinned values so tests don't need
// to construct full Room structs.
func (f *graceFixture) seedRoom(t *testing.T, roomID string) {
	t.Helper()
	room := &domain.Room{
		ID:                      roomID,
		CreatedAt:               1700000000,
		AnimeID:                 "anime-1",
		EpisodeID:               "ep-1",
		Player:                  domain.PlayerAnimeLib,
		TranslationID:           "trans-1",
		PlaybackState:           domain.StatePaused,
		PlaybackTime:            0,
		PlaybackTimeUpdatedAtMs: 1700000000_000,
		HostUserID:              "host",
	}
	if err := f.repo.CreateRoom(context.Background(), room); err != nil {
		t.Fatalf("seedRoom CreateRoom: %v", err)
	}
}

// roomExists reports whether the wt:room:{id} HASH is present in Redis.
func (f *graceFixture) roomExists(t *testing.T, roomID string) bool {
	t.Helper()
	_, err := f.repo.GetRoom(context.Background(), roomID)
	if err == nil {
		return true
	}
	if stderrors.Is(err, repo.ErrNotFound) {
		return false
	}
	t.Fatalf("roomExists GetRoom: %v", err)
	return false
}

// ----------------------------------------------------------------------------
// Test 1: Start fires DeleteRoom + room:closed broadcast after the period.
// ----------------------------------------------------------------------------

func TestGraceManager_Start_FiresDeleteAfterPeriod(t *testing.T) {
	period := 50 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "room-1")

	fx.mgr.Start("room-1")
	// Wait for the timer to fire + the fire goroutine to do its Broadcast +
	// DeleteRoom calls. Generous slack on -race builds.
	time.Sleep(period + graceTestSlack + 50*time.Millisecond)

	if fx.roomExists(t, "room-1") {
		t.Fatalf("expected room deleted after grace fire, but it still exists")
	}
	if n := fx.hub.countByType(domain.MsgRoomClosed); n != 1 {
		t.Fatalf("expected 1 room:closed broadcast, got %d", n)
	}
}

// ----------------------------------------------------------------------------
// Test 2: Cancel before fire stops the timer; no broadcast, no delete.
// ----------------------------------------------------------------------------

func TestGraceManager_Cancel_BeforeFire_StopsTimer(t *testing.T) {
	period := 200 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "room-1")

	fx.mgr.Start("room-1")
	time.Sleep(20 * time.Millisecond) // well before fire

	if !fx.mgr.Cancel("room-1") {
		t.Fatalf("Cancel returned false; expected true for a pending timer")
	}

	// Wait past when the timer WOULD have fired.
	time.Sleep(period + graceTestSlack)

	if !fx.roomExists(t, "room-1") {
		t.Fatalf("room was deleted despite cancellation")
	}
	if n := fx.hub.countByType(domain.MsgRoomClosed); n != 0 {
		t.Fatalf("expected 0 room:closed broadcasts after cancel, got %d", n)
	}
}

// ----------------------------------------------------------------------------
// Test 3: Double-Start replaces the timer (clock resets each Start).
// ----------------------------------------------------------------------------

func TestGraceManager_DoubleStart_ReplacesTimer(t *testing.T) {
	period := 100 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "room-1")

	fx.mgr.Start("room-1")
	time.Sleep(30 * time.Millisecond)
	// Second Start resets the clock — total wait from second Start is what
	// matters for whether the timer fires.
	fx.mgr.Start("room-1")

	// 80ms after second Start = total 110ms from FIRST start. If the timer
	// were still anchored to the first Start, it would have fired by now.
	time.Sleep(80 * time.Millisecond)
	if !fx.roomExists(t, "room-1") {
		t.Fatalf("room deleted too early; second Start should have reset the clock")
	}
	if n := fx.hub.countByType(domain.MsgRoomClosed); n != 0 {
		t.Fatalf("expected 0 broadcasts before second-Start fire, got %d", n)
	}

	// Now sleep enough so the SECOND timer has definitely fired
	// (130ms more = 210ms from second Start; period is 100ms).
	time.Sleep(130 * time.Millisecond)
	if fx.roomExists(t, "room-1") {
		t.Fatalf("expected room deleted after second-Start fire")
	}
	if n := fx.hub.countByType(domain.MsgRoomClosed); n != 1 {
		t.Fatalf("expected exactly 1 room:closed broadcast after second-Start fire, got %d", n)
	}
}

// ----------------------------------------------------------------------------
// Test 4: Cancel after fire returns false (no pending timer to stop).
// ----------------------------------------------------------------------------

func TestGraceManager_Cancel_AfterFire_ReturnsFalse(t *testing.T) {
	period := 30 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "room-1")

	fx.mgr.Start("room-1")
	time.Sleep(period + graceTestSlack + 30*time.Millisecond) // let it fire

	if fx.mgr.Cancel("room-1") {
		t.Fatalf("Cancel returned true; expected false (timer already fired)")
	}
}

// ----------------------------------------------------------------------------
// Test 5: Cancel on unknown room is a no-op (no panic, returns false).
// ----------------------------------------------------------------------------

func TestGraceManager_Cancel_UnknownRoom_NoOp(t *testing.T) {
	fx := newGraceFixture(t, 100*time.Millisecond)

	if fx.mgr.Cancel("never-started") {
		t.Fatalf("Cancel returned true; expected false for unknown room")
	}
}

// ----------------------------------------------------------------------------
// Test 6: Close stops every timer; no fires happen after Close.
// ----------------------------------------------------------------------------

func TestGraceManager_Close_StopsAllTimers(t *testing.T) {
	period := 200 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "a")
	fx.seedRoom(t, "b")
	fx.seedRoom(t, "c")

	fx.mgr.Start("a")
	fx.mgr.Start("b")
	fx.mgr.Start("c")

	fx.mgr.Close()

	time.Sleep(period + graceTestSlack + 100*time.Millisecond)

	if n := fx.hub.countByType(domain.MsgRoomClosed); n != 0 {
		t.Fatalf("expected 0 room:closed broadcasts after Close, got %d", n)
	}
	// All three rooms still in Redis.
	for _, id := range []string{"a", "b", "c"} {
		if !fx.roomExists(t, id) {
			t.Errorf("room %q was deleted despite Close", id)
		}
	}
}

// ----------------------------------------------------------------------------
// Test 7: Active reports pending state correctly.
// ----------------------------------------------------------------------------

func TestGraceManager_Active_ReportsPendingState(t *testing.T) {
	fx := newGraceFixture(t, 200*time.Millisecond)
	fx.seedRoom(t, "room-1")

	if fx.mgr.Active("room-1") {
		t.Fatalf("Active returned true before Start")
	}
	fx.mgr.Start("room-1")
	if !fx.mgr.Active("room-1") {
		t.Fatalf("Active returned false after Start; expected true")
	}
	fx.mgr.Cancel("room-1")
	if fx.mgr.Active("room-1") {
		t.Fatalf("Active returned true after Cancel; expected false")
	}
}

// ----------------------------------------------------------------------------
// Test 8: fire() broadcasts room:closed BEFORE repo.DeleteRoom.
// Asserts ordering via call timestamps + a Redis-existence probe taken at
// broadcast time (deterministic because the fakeHub captures the timestamp
// at the start of Broadcast — if the order were reversed the room would
// already be gone).
// ----------------------------------------------------------------------------

func TestGraceManager_Fire_BroadcastsRoomClosedBeforeDelete(t *testing.T) {
	period := 50 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "room-1")

	// Install a probing hub that records whether the room is STILL present
	// at the moment Broadcast is invoked. We achieve this with a custom hub
	// wrapping the fake.
	probingHub := &probingHubImpl{
		inner:    fx.hub,
		repo:     fx.repo,
		seenLive: false,
	}
	// Re-create the manager with the probing hub so the fire path goes
	// through it. Stop the original manager first to avoid double timers.
	fx.mgr.Close()
	fx.mgr = NewGraceManager(fx.repo, probingHub, period, logger.Default())
	t.Cleanup(fx.mgr.Close)

	fx.mgr.Start("room-1")
	time.Sleep(period + graceTestSlack + 80*time.Millisecond)

	if !probingHub.seenLive {
		t.Fatalf("Broadcast happened AFTER DeleteRoom (room was already gone at broadcast time)")
	}
	if fx.roomExists(t, "room-1") {
		t.Fatalf("expected room deleted after fire")
	}
}

// probingHubImpl wraps the fake hub and probes Redis on every Broadcast,
// recording whether the room HASH was still present at broadcast time.
type probingHubImpl struct {
	inner    *graceFakeHub
	repo     *repo.RoomRepo
	mu       sync.Mutex
	seenLive bool
}

func (p *probingHubImpl) Broadcast(ctx context.Context, roomID string, env domain.Envelope, excludeUserID string) (int, error) {
	if env.Type == domain.MsgRoomClosed {
		_, err := p.repo.GetRoom(ctx, roomID)
		p.mu.Lock()
		// Room was live (no ErrNotFound) at the instant Broadcast was called.
		if err == nil {
			p.seenLive = true
		}
		p.mu.Unlock()
	}
	return p.inner.Broadcast(ctx, roomID, env, excludeUserID)
}

func (p *probingHubImpl) SendTo(ctx context.Context, roomID, userID string, env domain.Envelope) (int, error) {
	return p.inner.SendTo(ctx, roomID, userID, env)
}

// ----------------------------------------------------------------------------
// Test 9: Start after Close is a no-op (SIGTERM-driven OnClose storm
// suppression — see Plan 05.1 Task 2D).
// ----------------------------------------------------------------------------

func TestGraceManager_StartAfterClose_NoOp(t *testing.T) {
	period := 50 * time.Millisecond
	fx := newGraceFixture(t, period)
	fx.seedRoom(t, "room-1")

	fx.mgr.Close()
	fx.mgr.Start("room-1")
	time.Sleep(period + graceTestSlack + 50*time.Millisecond)

	if !fx.roomExists(t, "room-1") {
		t.Fatalf("room was deleted; Start after Close should be no-op")
	}
	if n := fx.hub.countByType(domain.MsgRoomClosed); n != 0 {
		t.Fatalf("expected 0 broadcasts after Start-post-Close, got %d", n)
	}
	if fx.mgr.Active("room-1") {
		t.Fatalf("Active returned true after Start-post-Close; expected false")
	}
}

// ----------------------------------------------------------------------------
// Test 10: Period() exposes the configured grace period for logging context.
// ----------------------------------------------------------------------------

func TestGraceManager_Period_ReportsConfigured(t *testing.T) {
	fx := newGraceFixture(t, 123*time.Millisecond)
	if got := fx.mgr.Period(); got != 123*time.Millisecond {
		t.Fatalf("Period() = %v, want %v", got, 123*time.Millisecond)
	}
}
