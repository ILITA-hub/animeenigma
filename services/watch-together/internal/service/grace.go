// Package service — grace.go is the post-last-disconnect reconnect window
// manager (Plan 05.1, WT-POLISH-02).
//
// Background: when the last WebSocket connection in a room drops, we DO
// NOT immediately tear the room down. Instead a per-room timer is started
// for cfg.GracePeriod (default 5m). If a connection arrives for the room
// during that window, the timer is cancelled and the room state survives
// intact. If the window elapses with no reconnect, the timer fires,
// broadcasts a room:closed envelope, and deletes the 3 persistent Redis
// keys explicitly (so the next GET /rooms/{id} returns 410 Gone even
// before the natural TTL would have expired).
//
// Crucially: during the grace window we do NOT call repo.RefreshTTL. The
// existing sliding TTL (default 900s) keeps the keys alive long enough
// for the 5m grace to elapse, and we want them to expire naturally if
// someone fails to start the grace timer (defense in depth — even if a
// bug in the WS handler skipped Start, the room still wouldn't leak).
//
// Concurrency model:
//   - sync.Map keyed by roomID. Each value is a *graceEntry holding the
//     *time.Timer and startedAt timestamp.
//   - Double-Start replaces the prior timer atomically via Stop +
//     LoadOrStore.
//   - Cancel uses sync.Map.LoadAndDelete to win the race against a
//     concurrently firing timer: whichever code path deletes the entry
//     first "wins"; the other returns early without touching Redis.
//   - Close flips a `closed` atomic flag so any further Start invocations
//     (which DO occur during hub.Close → OnClose → graceMgr.Start cascade
//     on SIGTERM) are no-ops. This keeps SIGTERM teardown quiet.
//
// Metrics:
//   - wt_grace_started_total — incremented on every Start (including
//     replacement Starts) when not closed.
//   - wt_grace_recoveries_total — incremented when Cancel successfully
//     stops a pending timer (the "user reconnected in time" signal).
//
// Plan 05.2 may add additional grace-related counters but MUST NOT
// re-register these two — they are owned by this file.
package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// graceFireTimeout is the bounded background context budget for the
// fire() codepath. fire() does one hub.Broadcast + one repo.DeleteRoom.
// 5s is comfortably larger than the typical Redis RTT (sub-ms locally,
// single-digit ms in prod) but short enough to be obvious in logs if
// something stalls.
const graceFireTimeout = 5 * time.Second

// graceStartedTotal counts every Start invocation that successfully
// schedules a timer (i.e. the manager was not Closed at the time). Bumped
// even when Start replaces an existing timer — the new timer is a fresh
// schedule and deserves the count.
var graceStartedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "wt_grace_started_total",
	Help: "Total grace-period timers started after a room's last connection dropped",
})

// graceRecoveriesTotal counts the "user reconnected in time" events.
// Incremented ONLY when Cancel observes a pending timer and successfully
// stops it. Cancels of already-fired or never-existed timers do not bump.
var graceRecoveriesTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "wt_grace_recoveries_total",
	Help: "Total grace timers cancelled by a returning connection (recovery within window)",
})

// graceMetricsOnce gates the prometheus.MustRegister so multiple GraceManagers
// in the same process (only happens in tests) don't panic on re-registration.
var graceMetricsOnce sync.Once

func registerGraceMetrics() {
	graceMetricsOnce.Do(func() {
		prometheus.MustRegister(graceStartedTotal, graceRecoveriesTotal)
	})
}

// graceEntry is the per-room book-keeping for a pending grace timer. The
// timer is from time.AfterFunc; startedAt is captured at Start() time so
// fire() can log the elapsed duration for forensics.
type graceEntry struct {
	timer     *time.Timer
	startedAt time.Time
}

// GraceManager is the per-room timer registry. Safe for concurrent use;
// all mutating operations are sync.Map atomic primitives so there is no
// shared mutex contention across rooms.
type GraceManager struct {
	repo   *repo.RoomRepo
	hub    HubFanout
	period time.Duration
	log    *logger.Logger

	// entries: roomID → *graceEntry. sync.Map is the right primitive here
	// because the dominant access pattern is independent roomIDs — no
	// cross-room contention, and Cancel needs LoadAndDelete (the atomic
	// race winner) to suppress double-fire.
	entries sync.Map

	// closed atomic flag — flipped in Close() and read by Start() to skip
	// further scheduling. Rationale: hub.Close → connection OnClose →
	// graceMgr.Start would otherwise produce a flurry of timers immediately
	// before graceMgr.Close stops them all. Skipping Start during shutdown
	// keeps the teardown logs quiet and avoids racing the timer goroutines
	// against the Redis client Close.
	closed atomic.Bool
}

// NewGraceManager constructs a manager. `period` is the grace window
// (cfg.GracePeriod in production; short values in tests). Pass nil for
// `log` to fall back to logger.Default().
//
// The supplied HubFanout is used to broadcast room:closed when the timer
// fires; the supplied *RoomRepo is used to delete the 3 persistent keys.
// Both dependencies must be non-nil — passing nil is a programming error
// that would crash in fire(), so we don't add a defensive nil check.
func NewGraceManager(r *repo.RoomRepo, hub HubFanout, period time.Duration, log *logger.Logger) *GraceManager {
	if log == nil {
		log = logger.Default()
	}
	registerGraceMetrics()
	return &GraceManager{
		repo:   r,
		hub:    hub,
		period: period,
		log:    log,
	}
}

// Period returns the configured grace duration. Exposed for log context
// in the WS handler (so we don't have to thread cfg.GracePeriod through
// to every makeOnClose call site).
func (g *GraceManager) Period() time.Duration {
	return g.period
}

// Active reports whether a grace timer is currently pending for roomID.
// Used by tests and diagnostics; production code should not branch on
// this — race-free behavior comes from Start/Cancel's atomic primitives.
func (g *GraceManager) Active(roomID string) bool {
	_, ok := g.entries.Load(roomID)
	return ok
}

// Start schedules a grace timer for roomID. If a timer already exists,
// it is Stopped and replaced atomically (last-write-wins on double-Start;
// the second caller's clock is the one that counts).
//
// No-op if the manager has been Closed (suppresses the SIGTERM-driven
// OnClose storm).
func (g *GraceManager) Start(roomID string) {
	if g.closed.Load() {
		return
	}

	startedAt := time.Now()
	// AfterFunc fires the callback in its own goroutine. Capturing roomID
	// in the closure is safe — strings are immutable in Go.
	entry := &graceEntry{startedAt: startedAt}
	entry.timer = time.AfterFunc(g.period, func() {
		g.fire(roomID)
	})

	// Replace any existing entry. LoadAndDelete + Stop the old timer so
	// it can't fire after we've put the new one in place.
	if prev, ok := g.entries.LoadAndDelete(roomID); ok {
		if prevEntry, ok := prev.(*graceEntry); ok && prevEntry.timer != nil {
			prevEntry.timer.Stop()
		}
	}
	g.entries.Store(roomID, entry)

	graceStartedTotal.Inc()
	g.log.Infow("watch_together grace started",
		"room_id", roomID,
		"period", g.period,
	)
}

// Cancel attempts to stop the pending grace timer for roomID. Returns
// true if a pending timer was successfully stopped (the "user reconnected
// in time" case), false if no pending timer existed or it had already
// fired.
//
// Concurrency: Cancel and fire race for the same entry via
// sync.Map.LoadAndDelete. Whoever deletes the entry first wins; the
// loser returns early without touching Redis. timer.Stop() is itself
// safe to call after the timer has fired (returns false), but we use
// LoadAndDelete's atomic delete as the primary race winner — checking
// Stop's return after the entry was already removed by fire() would be
// unreliable since fire() runs in a separate goroutine.
func (g *GraceManager) Cancel(roomID string) bool {
	prev, ok := g.entries.LoadAndDelete(roomID)
	if !ok {
		return false
	}
	entry, ok := prev.(*graceEntry)
	if !ok || entry.timer == nil {
		return false
	}
	// Stop returns true if the timer was still pending. If the timer has
	// already fired (Stop=false) the fire goroutine raced us — but we
	// already won the LoadAndDelete, so the fire goroutine will observe
	// the missing entry and return early. We report based on Stop's
	// return so the metric reflects actual recoveries, not just deletion
	// races.
	stopped := entry.timer.Stop()
	if stopped {
		graceRecoveriesTotal.Inc()
		g.log.Infow("watch_together grace cancelled",
			"room_id", roomID,
			"elapsed", time.Since(entry.startedAt),
		)
	}
	return stopped
}

// Close stops every pending timer and prevents future Starts from
// scheduling new ones. Called from main.go on SIGTERM AFTER hub.Close
// — see the docstring on the closed flag for rationale.
//
// Idempotent: calling Close twice is a no-op (the second call's
// iteration finds an empty map).
func (g *GraceManager) Close() {
	g.closed.Store(true)
	g.entries.Range(func(key, value interface{}) bool {
		if entry, ok := value.(*graceEntry); ok && entry.timer != nil {
			entry.timer.Stop()
		}
		g.entries.Delete(key)
		return true
	})
}

// fire is invoked from the AfterFunc goroutine when the grace timer
// elapses without a Cancel. It:
//
//  1. LoadAndDelete's the entry. If the entry is no longer present, a
//     concurrent Cancel won the race — return early without doing
//     anything visible.
//  2. Broadcasts room:closed to every connected member (any reconnects
//     between Cancel-check and now will still see the close event,
//     which is the correct UX — they joined just as the timer fired).
//  3. DeleteRoom removes the 3 persistent keys. Failures are logged but
//     not retried — the natural TTL will reap them anyway.
//
// Broadcast intentionally runs BEFORE DeleteRoom so any race-y
// in-flight GetRoom from the room:closed handler still finds a valid
// room (rare; the broadcast is to local in-memory connection sets, not
// a Redis trip, so the window is microseconds).
func (g *GraceManager) fire(roomID string) {
	prev, ok := g.entries.LoadAndDelete(roomID)
	if !ok {
		// Concurrent Cancel beat us. Return quietly.
		return
	}
	entry, _ := prev.(*graceEntry)

	ctx, cancel := context.WithTimeout(context.Background(), graceFireTimeout)
	defer cancel()

	// 1. Broadcast room:closed. Empty Data per the design — the receiver
	// only cares that the room is gone, not why.
	env := domain.Envelope{
		Type: domain.MsgRoomClosed,
		Data: []byte(`{}`),
	}
	if _, err := g.hub.Broadcast(ctx, roomID, env, ""); err != nil {
		g.log.Warnw("watch_together grace fire broadcast",
			"room_id", roomID,
			"err", err,
		)
	}

	// 2. Telemetry — Plan 05.2 helper. Decrements RoomsActive gauge and
	// observes ChatMessagesPerRoom + SessionDurationSeconds histograms.
	// Must run BEFORE DeleteRoom (the room snapshot is still readable).
	if room, err := g.repo.GetRoom(ctx, roomID); err == nil && room != nil {
		observeRoomTeardown(ctx, g.repo, g.log, room)
	}

	// 3. Delete the 3 persistent keys.
	if err := g.repo.DeleteRoom(ctx, roomID); err != nil {
		g.log.Warnw("watch_together grace fire delete",
			"room_id", roomID,
			"err", err,
		)
	}

	elapsed := time.Duration(0)
	if entry != nil {
		elapsed = time.Since(entry.startedAt)
	}
	g.log.Infow("watch_together grace fired",
		"room_id", roomID,
		"elapsed", elapsed,
	)
}
