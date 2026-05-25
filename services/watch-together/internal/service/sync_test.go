package service

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// driftFixture wires up a miniredis-backed RoomRepo and a Room with the given
// playback state. The DriftEngine itself is constructed fresh per test so
// no state leaks across test functions.
type driftFixture struct {
	repo    *repo.RoomRepo
	engine  *DriftEngine
	roomID  string
	nowMs   int64
	cleanup func()
}

func newDriftFixture(t *testing.T, state string, playbackTime float64, updatedAtMs int64) *driftFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)

	roomID := "room-drift-test"
	room := &domain.Room{
		ID:                      roomID,
		CreatedAt:               1700000000,
		AnimeID:                 "anime-1",
		EpisodeID:               "ep-1",
		Player:                  domain.PlayerAnimeLib,
		TranslationID:           "trans-1",
		PlaybackState:           state,
		PlaybackTime:            playbackTime,
		PlaybackTimeUpdatedAtMs: updatedAtMs,
		HostUserID:              "host",
	}
	if err := r.CreateRoom(context.Background(), room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	return &driftFixture{
		repo:    r,
		engine:  NewDriftEngine(log),
		roomID:  roomID,
		cleanup: func() {},
	}
}

// ----------------------------------------------------------------------------
// Test 1 — Pure ComputeDrift, playing state: room_time=100, updated 2s ago,
// reported 99.5 → expected = 100 + 2.0 = 102; drift = abs(99.5 - 102) = 2.5.
// ----------------------------------------------------------------------------

func TestDrift_ComputeDrift_Playing_AdvancesByWallClock(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	updatedAtMs := nowMs - 2000 // 2 seconds ago

	got := ComputeDrift(domain.StatePlaying, 100.0, updatedAtMs, 99.5, nowMs)
	want := 2.5
	if abs(got-want) > 1e-9 {
		t.Fatalf("ComputeDrift = %v, want %v", got, want)
	}
}

// ----------------------------------------------------------------------------
// Test 2 — Paused state: drift is just |reported - playback_time|; no
// wall-clock advance regardless of how long ago updated_at was.
// ----------------------------------------------------------------------------

func TestDrift_ComputeDrift_Paused_NoWallClockAdvance(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	updatedAtMs := nowMs - 60_000 // 60 seconds ago — irrelevant when paused

	got := ComputeDrift(domain.StatePaused, 100.0, updatedAtMs, 99.5, nowMs)
	want := 0.5
	if abs(got-want) > 1e-9 {
		t.Fatalf("ComputeDrift (paused) = %v, want %v", got, want)
	}
}

// ----------------------------------------------------------------------------
// Test 3 — OnTimeTick with drift = 1.0s → no correction.
// Room state: playing, time=100, updated_at = now-0ms; reported 101 → drift 1.0.
// ----------------------------------------------------------------------------

func TestDrift_OnTimeTick_InSync_ReturnsNil(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 101.0, nowMs)
	if err != nil {
		t.Fatalf("OnTimeTick: %v", err)
	}
	if got != nil {
		t.Fatalf("Correction = %+v, want nil", got)
	}
}

// ----------------------------------------------------------------------------
// Test 4 — Drift = 2.5s → soft correction with time≈expected (102), severity=soft.
// ----------------------------------------------------------------------------

func TestDrift_OnTimeTick_SoftBand_ReturnsSoftCorrection(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	// Updated 2s ago, room_time=100 → expected at now = 102.
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs-2000)

	// reported 99.5 → drift = |99.5 - 102| = 2.5 (soft band).
	got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 99.5, nowMs)
	if err != nil {
		t.Fatalf("OnTimeTick: %v", err)
	}
	if got == nil {
		t.Fatal("Correction = nil, want soft correction")
	}
	if got.Severity != DriftSoft {
		t.Errorf("Severity = %q, want %q", got.Severity, DriftSoft)
	}
	if abs(got.Time-102.0) > 1e-6 {
		t.Errorf("Correction.Time = %v, want 102", got.Time)
	}
	if got.ServerTS != nowMs {
		t.Errorf("Correction.ServerTS = %d, want %d", got.ServerTS, nowMs)
	}
}

// ----------------------------------------------------------------------------
// Test 5 — Drift = 6s → hard correction; counter goes to 1.
// ----------------------------------------------------------------------------

func TestDrift_OnTimeTick_HardBand_IncrementsCounter(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	// Updated 0ms ago, room_time=100 → expected=100; reported 106 → drift=6.
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	if err != nil {
		t.Fatalf("OnTimeTick: %v", err)
	}
	if got == nil || got.Severity != DriftHard {
		t.Fatalf("Severity = %v, want hard", got)
	}
	if abs(got.Time-100.0) > 1e-6 {
		t.Errorf("Correction.Time = %v, want 100", got.Time)
	}
}

// Tightened version of Test 5 — verifies counter goes from 0 → 1 after one
// hard drift by checking that a 4th hard drift doesn't yet trigger
// persistent (it would if the counter started at 1+ instead of 1).
func TestDrift_OnTimeTick_HardBand_CounterRunsUpTo4(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	// 4 hard drifts in a row.
	for i := 0; i < 4; i++ {
		got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
		if err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		if got == nil || got.Severity != DriftHard {
			t.Fatalf("tick %d severity = %v, want hard", i, got)
		}
	}
}

// ----------------------------------------------------------------------------
// Test 6 — 5 consecutive hard drifts → 5th returns DriftPersistent;
// 6th call returns (nil, nil) because the member is suspended.
// ----------------------------------------------------------------------------

func TestDrift_OnTimeTick_PersistentDrift_SuspendsMember(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	for i := 0; i < 4; i++ {
		got, _ := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
		if got == nil || got.Severity != DriftHard {
			t.Fatalf("tick %d: severity = %v, want hard", i, got)
		}
	}

	// 5th — persistent.
	got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	if err != nil {
		t.Fatalf("5th tick err: %v", err)
	}
	if got == nil || got.Severity != DriftPersistent {
		t.Fatalf("5th tick severity = %v, want persistent", got)
	}

	// 6th — suspended, returns (nil, nil) regardless of reported.
	got, err = fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	if err != nil {
		t.Fatalf("6th tick err: %v", err)
	}
	if got != nil {
		t.Fatalf("6th tick (suspended) = %+v, want nil", got)
	}
}

// ----------------------------------------------------------------------------
// Test 7 — counter resets on a sub-5s drift. 4 hards, then 1 in-sync,
// then 1 hard → severity hard (counter started at 1 again, not 5).
// ----------------------------------------------------------------------------

func TestDrift_OnTimeTick_InSyncResetsCounter(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	// 4 hard drifts.
	for i := 0; i < 4; i++ {
		got, _ := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
		if got == nil || got.Severity != DriftHard {
			t.Fatalf("tick %d severity = %v, want hard", i, got)
		}
	}

	// In-sync tick (reported = 100 → drift = 0).
	got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 100.0, nowMs)
	if err != nil {
		t.Fatalf("recovery tick err: %v", err)
	}
	if got != nil {
		t.Fatalf("recovery tick = %+v, want nil", got)
	}

	// Now a hard drift — should be hard (not persistent), since counter reset.
	got, err = fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	if err != nil {
		t.Fatalf("post-recovery hard tick err: %v", err)
	}
	if got == nil || got.Severity != DriftHard {
		t.Fatalf("post-recovery severity = %v, want hard (counter should have reset)", got)
	}
}

// ----------------------------------------------------------------------------
// Test 8 — Reset(roomID, userID) removes the per-member state; subsequent
// OnTimeTick after a previously-suspended member is reset starts fresh.
// ----------------------------------------------------------------------------

func TestDrift_Reset_ClearsSuspendedState(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	// Trigger persistent drift.
	for i := 0; i < 5; i++ {
		_, _ = fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	}

	// Confirm suspended (no correction on the 6th call).
	got, _ := fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	if got != nil {
		t.Fatalf("pre-reset suspended call = %+v, want nil", got)
	}

	// Reset — member's state goes away.
	fx.engine.Reset(fx.roomID, "alice")

	// Subsequent in-sync call returns nil (drift 0) — but more importantly,
	// the engine should treat alice as a fresh member with a 0 counter.
	got, _ = fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 100.0, nowMs)
	if got != nil {
		t.Fatalf("post-reset in-sync call = %+v, want nil", got)
	}

	// And a single hard drift is again severity=hard (not persistent).
	got, _ = fx.engine.OnTimeTick(context.Background(), fx.repo, fx.roomID, "alice", 106.0, nowMs)
	if got == nil || got.Severity != DriftHard {
		t.Fatalf("post-reset severity = %v, want hard", got)
	}
}

// ----------------------------------------------------------------------------
// Bonus — OnTimeTick on a missing room returns repo.ErrNotFound (so the
// router can decide whether to close the connection).
// ----------------------------------------------------------------------------

func TestDrift_OnTimeTick_MissingRoom_ReturnsNotFound(t *testing.T) {
	nowMs := int64(1_700_000_000_000)
	fx := newDriftFixture(t, domain.StatePlaying, 100.0, nowMs)

	got, err := fx.engine.OnTimeTick(context.Background(), fx.repo, "no-such-room", "alice", 100.0, nowMs)
	if !stderrors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want repo.ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("Correction = %+v, want nil on missing room", got)
	}
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
