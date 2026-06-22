// Package service — sync.go is the watch-together drift detection engine.
//
// Background: each connected member emits a `playback:time_tick {time}`
// envelope ~1Hz so the server can compare the member's local playback
// position against the canonical room state and nudge them back into
// sync. The decision table comes from
// docs/superpowers/specs/2026-05-25-watch-together-design.md §Sync engine
// (WT-FOUND-07):
//
//	drift = abs(reported_time - expected_time)
//	expected_time = room.playback_time + wall-clock-since-anchor  (state=playing)
//	              = room.playback_time                            (state=paused)
//
//	drift <= 1.5s              → no action
//	1.5s < drift <= 5s         → soft correction, reset counter
//	drift > 5s                 → hard correction, ++counter
//	counter == 5 hard in a row → PERSISTENT_DRIFT, suspend further corrections
//
// The engine is pure: no Redis, no goroutines, no time package side-effects.
// Tests inject the "now" value and the room state, then assert on the
// returned (*Correction, error). The InboundRouter (inbound.go) is the only
// production caller — it translates the *Correction into either a
// playback:correction envelope or a PERSISTENT_DRIFT error envelope and
// sends it via hub.SendTo (per-recipient, never broadcast).
package service

import (
	"context"
	"math"
	"sync"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
)

// DriftSeverity is the result band of a single drift evaluation.
type DriftSeverity string

const (
	// DriftNone is returned when reported_time is within 1.5s of expected.
	// The router emits nothing for this case — the member is in sync.
	DriftNone DriftSeverity = ""
	// DriftSoft is the 1.5s < drift <= 5s band. The router sends a
	// playback:correction envelope and the engine resets the consecutive-hard
	// counter for that member (drift recovered).
	DriftSoft DriftSeverity = "soft"
	// DriftHard is the drift > 5s band. The router sends a playback:correction
	// envelope and the engine increments the consecutive-hard counter; on the
	// 5th consecutive hit the severity escalates to DriftPersistent.
	DriftHard DriftSeverity = "hard"
	// DriftPersistent is emitted when consecutive hard drifts reach
	// PersistentDriftThreshold. The router sends an error envelope with code
	// PERSISTENT_DRIFT, the engine marks the member's state as suspended,
	// and all subsequent OnTimeTick calls return (nil, nil) until Reset is
	// invoked (typically from WS disconnect cleanup in 01.5's OnClose hook).
	DriftPersistent DriftSeverity = "persistent"
)

// PersistentDriftThreshold is the number of consecutive hard (>5s) drifts
// after which a member is declared persistently drifting. Exported so the
// 01.9 smoke / integration test can compile a probe that confirms the
// number is what the design doc claims (5).
const PersistentDriftThreshold = 5

// softDriftLowerSeconds is the bottom of the soft-drift band. Anything strictly
// less than or equal to this is DriftNone; anything strictly greater AND <= 5s
// is DriftSoft.
const softDriftLowerSeconds = 1.5

// hardDriftThresholdSeconds is the upper edge of the soft band. drift > this
// is DriftHard.
const hardDriftThresholdSeconds = 5.0

// Correction is the engine's per-tick decision carrier. Severity discriminates
// what the router does:
//
//	DriftSoft / DriftHard  → SendTo(playback:correction {time, server_ts})
//	DriftPersistent        → SendTo(error {code: PERSISTENT_DRIFT, hint: reload})
//	DriftNone              → no envelope (Correction is nil in this case;
//	                         the engine returns (nil, nil), the router no-ops)
//
// Time + ServerTS are populated only for soft/hard corrections; persistent
// payloads carry no time data (the client is told to reload).
type Correction struct {
	Time     float64
	ServerTS int64
	Severity DriftSeverity
}

// ComputeDrift is the pure-function core of the engine. Pulled out as a
// package-level function (not a method) so unit tests can hammer it without
// constructing a DriftEngine.
//
// Inputs are all values, no pointers. roomUpdatedAtMs is the
// playback_time_updated_at field from the Redis Room HASH (unix
// milliseconds). nowMs is the server's wall-clock at evaluation time
// (also unix ms — caller passes time.Now().UnixMilli() in production).
//
// When state == StatePlaying, expected_time advances with wall-clock since
// the anchor. When state == StatePaused (or anything else — defensive
// fallback), expected_time is just the stored playback_time. The abs(...)
// is taken with math.Abs so negative drifts (member is ahead of room)
// produce the same correction signal as positive drifts (member is behind).
func ComputeDrift(state string, roomTime float64, roomUpdatedAtMs int64, reportedTime float64, nowMs int64) float64 {
	expected := roomTime
	if state == domain.StatePlaying {
		// (now - anchor) / 1000 → seconds elapsed since the anchor.
		// If nowMs < roomUpdatedAtMs (clock skew between writer + reader),
		// the delta is negative — that's fine, expected just rewinds, and
		// the abs in the drift calc will still produce a positive number.
		elapsedSeconds := float64(nowMs-roomUpdatedAtMs) / 1000.0
		expected = roomTime + elapsedSeconds
	}
	return math.Abs(reportedTime - expected)
}

// computeExpected mirrors ComputeDrift's expected_time calc so the engine
// can compute both drift AND the expected time (for the correction payload)
// in one pass without calling ComputeDrift twice.
func computeExpected(state string, roomTime float64, roomUpdatedAtMs int64, nowMs int64) float64 {
	if state == domain.StatePlaying {
		return roomTime + float64(nowMs-roomUpdatedAtMs)/1000.0
	}
	return roomTime
}

// memberDriftState is the per-(roomID, userID) book-keeping the engine needs
// to detect persistent drift. consecutiveHardDrifts counts the current run of
// >5s drifts since the last sub-5s drift; on reaching PersistentDriftThreshold
// the member is marked suspended and the engine ignores all subsequent ticks
// for that member until Reset is called.
type memberDriftState struct {
	consecutiveHardDrifts int
	suspended             bool
}

// DriftEngine maintains per-member drift counters and emits Correction values
// based on time_tick reports. Concurrency: state mutations are gated by mu;
// the engine itself is safe to call from many goroutines (one per WS
// connection's readPump in practice).
//
// The map is keyed by the composite "{roomID}|{userID}" string so two members
// in different rooms never collide. Removing a member's entry on disconnect
// is the caller's responsibility (call Reset from the WS OnClose hook).
type DriftEngine struct {
	mu      sync.Mutex
	members map[string]*memberDriftState
	log     *logger.Logger
}

// NewDriftEngine constructs an empty engine. Pass nil for log to fall back
// to logger.Default(). All state lives in-memory — restart of the
// watch-together service resets every member's counter, which is acceptable
// because the next time_tick (~1 second later) re-establishes a clean
// baseline.
func NewDriftEngine(log *logger.Logger) *DriftEngine {
	if log == nil {
		log = logger.Default()
	}
	return &DriftEngine{
		members: make(map[string]*memberDriftState),
		log:     log,
	}
}

// memberKey is the composite map key for the per-(room, user) state.
// Centralized so tests + Reset use the exact same format as OnTimeTick.
func memberKey(roomID, userID string) string {
	return roomID + "|" + userID
}

// OnTimeTick is the engine's main entry point. The router calls this once per
// inbound playback:time_tick envelope. It:
//
//  1. Looks up (or creates) the per-member state. If already suspended,
//     returns (nil, nil) immediately — no further corrections for this
//     member until Reset.
//  2. Fetches the canonical Room via the supplied roomGetter. In production
//     the router injects a *RoomCache (TTL + write-invalidated) so the per-
//     tick read collapses to near-zero Redis HGETALLs during active co-watch
//     (audit L802); *repo.RoomRepo also satisfies roomGetter so the existing
//     pure drift-band tests read straight through Redis unchanged. If the room
//     has vanished (TTL expired mid-session, or someone Deleted it), returns
//     (nil, repo.ErrNotFound) so the router can decide whether to close the
//     connection.
//  3. Computes drift via ComputeDrift.
//  4. Applies the decision table from the package comment.
//  5. Updates the member's counter + suspended flag accordingly.
//  6. Returns the Correction (or nil for DriftNone).
//
// The router translates the Correction into an envelope: soft/hard →
// playback:correction with Time+ServerTS; persistent → error envelope with
// PERSISTENT_DRIFT code (Time+ServerTS ignored).
func (e *DriftEngine) OnTimeTick(
	ctx context.Context,
	r roomGetter,
	roomID, userID string,
	reportedTime float64,
	nowMs int64,
) (*Correction, error) {
	key := memberKey(roomID, userID)

	e.mu.Lock()
	state, ok := e.members[key]
	if !ok {
		state = &memberDriftState{}
		e.members[key] = state
	}
	if state.suspended {
		e.mu.Unlock()
		// Suspended member — engine has already declared persistent drift
		// and emitted the error envelope. Stay silent until Reset is
		// called from the WS OnClose hook (member reconnect = fresh state).
		return nil, nil
	}
	e.mu.Unlock()

	room, err := r.GetRoom(ctx, roomID)
	if err != nil {
		// Includes repo.ErrNotFound — propagate so the router can decide.
		return nil, err
	}

	expected := computeExpected(room.PlaybackState, room.PlaybackTime, room.PlaybackTimeUpdatedAtMs, nowMs)
	drift := math.Abs(reportedTime - expected)

	switch {
	case drift <= softDriftLowerSeconds:
		// In sync. Reset the consecutive-hard counter — a single in-sync
		// tick "recovers" the member, per <drift_contract> Test 7.
		e.mu.Lock()
		state.consecutiveHardDrifts = 0
		e.mu.Unlock()
		return nil, nil

	case drift <= hardDriftThresholdSeconds:
		// Soft band: send a correction; reset the consecutive-hard counter
		// because this tick recovered the member back into the soft band.
		e.mu.Lock()
		state.consecutiveHardDrifts = 0
		e.mu.Unlock()
		DriftCorrectionsTotal.WithLabelValues("soft").Inc()
		return &Correction{
			Time:     expected,
			ServerTS: nowMs,
			Severity: DriftSoft,
		}, nil

	default:
		// Hard band (drift > 5s). Increment the counter; on the 5th
		// consecutive hard hit, escalate to DriftPersistent and suspend
		// further corrections for this member.
		e.mu.Lock()
		state.consecutiveHardDrifts++
		count := state.consecutiveHardDrifts
		if count >= PersistentDriftThreshold {
			state.suspended = true
		}
		e.mu.Unlock()

		if count >= PersistentDriftThreshold {
			DriftCorrectionsTotal.WithLabelValues("persistent").Inc()
			e.log.Warnw("watch_together drift persistent",
				"room_id", roomID,
				"user_id", userID,
				"drift_seconds", drift,
				"consecutive_hard", count,
			)
			return &Correction{Severity: DriftPersistent}, nil
		}

		DriftCorrectionsTotal.WithLabelValues("hard").Inc()
		return &Correction{
			Time:     expected,
			ServerTS: nowMs,
			Severity: DriftHard,
		}, nil
	}
}

// Reset clears the per-(roomID, userID) drift state. Called from the WS
// handler's OnClose hook in 01.5/01.6 wiring so a member who reconnects
// after a persistent-drift suspension gets a fresh counter. Also useful
// in tests for behavior 8.
//
// No-op if the member has no state yet (lookup miss). Safe for concurrent
// use with OnTimeTick.
func (e *DriftEngine) Reset(roomID, userID string) {
	key := memberKey(roomID, userID)
	e.mu.Lock()
	delete(e.members, key)
	e.mu.Unlock()
}
