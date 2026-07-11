package service

import (
	"sync/atomic"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	gometrics "github.com/ILITA-hub/animeenigma/libs/metrics"
)

// ShedChecker is the narrow degradation-consumer surface (satisfied by
// *cache.DegradationWatcher). Shared by WorkerPool, EncoderPool and the
// storyboard backfill loop.
type ShedChecker interface {
	ShouldShed(min int) bool
	Level() int
}

// shedGate wraps a ShedChecker for one subsystem: it answers "should this
// loop skip claiming new work right now?", keeps the
// ae_degradation_shed{subsystem} gauge in sync, and logs exactly once per
// state change. Safe for concurrent use by multiple worker goroutines
// (both pools run >1 worker by default) — the paused flag is atomic, so the
// transition log/gauge fire once, not per worker.
type shedGate struct {
	checker   ShedChecker
	subsystem string
	log       *logger.Logger
	paused    atomic.Bool
}

func newShedGate(subsystem string, log *logger.Logger) *shedGate {
	return &shedGate{subsystem: subsystem, log: log}
}

// set wires the degradation watcher (nil-safe; nil never sheds).
func (g *shedGate) set(c ShedChecker) { g.checker = c }

// shed reports whether new-work admission is currently shed, updating the
// gauge + logging on state changes only.
func (g *shedGate) shed() bool {
	shed := g.checker != nil && g.checker.ShouldShed(1)
	if g.paused.Swap(shed) != shed {
		v := 0.0
		if shed {
			v = 1
			if g.log != nil {
				g.log.Infow("pausing new work: platform degraded",
					"subsystem", g.subsystem, "level", g.checker.Level())
			}
		} else if g.log != nil {
			g.log.Infow("resuming work: degradation cleared", "subsystem", g.subsystem)
		}
		gometrics.DegradationShed.WithLabelValues(g.subsystem).Set(v)
	}
	return shed
}

// encodeLimiter is a degradation-aware, GRADED concurrency limiter for the
// encoder pool (AUTO-575). Where shedGate is a binary admit/pause gate (still
// used unchanged by the download + storyboard loops), the limiter derives a
// graded active-worker CAP from the live degradation level and admits at most
// that many concurrent transcodes at once:
//
//	level 0  → maxWorkers   (full throughput when the host is idle)
//	level 1  → 1            (serialize transcodes — no CPU stacking)
//	level 2+ → 0            (pause admission; already-running transcodes finish)
//
// Surplus workers — those beyond the current cap — simply wait in their poll
// loop; the extra jobs stay queued in the DB (status='encoding', unclaimed)
// rather than being shed or dropped. This preserves full throughput on an idle
// host while preventing the CPU-stacking that tripped the host-pressure
// governor — the blunt alternative (a flat LIBRARY_ENCODE_WORKERS=1) would
// halve throughput even when the box is idle.
//
// Safe for concurrent use by every encoder worker goroutine: the active count
// is an atomic, and the shed gauge + one-shot transition log fire only when the
// cap actually changes.
type encodeLimiter struct {
	checker    ShedChecker
	maxWorkers int
	log        *logger.Logger
	// setActive mirrors the live active-worker count to
	// library_encode_active_workers. Injected (rather than imported) so the
	// single-emitter gauge stays in the library metrics package — a plain gauge
	// in libs/metrics would auto-register as an impostor 0-series in every
	// importing binary (the auto-registration trap). nil-safe.
	setActive func(int)

	active  atomic.Int32
	lastCap atomic.Int32
}

// newEncodeLimiter builds a limiter capping concurrent transcodes at maxWorkers
// (forced >=1). setActive may be nil (no gauge). The checker is wired later via
// set(); a nil checker never sheds, pinning the cap at maxWorkers.
func newEncodeLimiter(maxWorkers int, setActive func(int), log *logger.Logger) *encodeLimiter {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	l := &encodeLimiter{maxWorkers: maxWorkers, setActive: setActive, log: log}
	// Seed lastCap at the level-0 cap so a normal boot logs no transition; a
	// boot into an already-degraded level logs exactly one.
	l.lastCap.Store(int32(maxWorkers))
	return l
}

// set wires the degradation watcher (nil-safe; nil pins the cap at maxWorkers).
func (l *encodeLimiter) set(c ShedChecker) { l.checker = c }

// capFor maps a degradation level to the concurrent-transcode cap. maxWorkers
// is always >=1, so level 1 → 1 is a reduction (or a no-op when maxWorkers==1).
func (l *encodeLimiter) capFor(level int) int {
	switch {
	case level <= 0:
		return l.maxWorkers
	case level == 1:
		return 1
	default: // level >= 2
		return 0
	}
}

// currentCap reads the live level, computes the cap, and — on a cap change —
// updates ae_degradation_shed{subsystem="library_encode"} and logs once. Shed
// intensity honestly reflects what the encoder actually does: cap==maxWorkers →
// 0 (normal), 0<cap<maxWorkers → 1 (reduced), cap==0 → 2 (paused).
func (l *encodeLimiter) currentCap() int {
	level := 0
	if l.checker != nil {
		level = l.checker.Level()
	}
	c := l.capFor(level)
	if prev := l.lastCap.Swap(int32(c)); prev != int32(c) {
		shed := 0.0
		switch {
		case c == 0:
			shed = 2
		case c < l.maxWorkers:
			shed = 1
		}
		gometrics.DegradationShed.WithLabelValues("library_encode").Set(shed)
		if l.log != nil {
			l.log.Infow("encoder concurrency cap changed (platform degradation)",
				"cap", c, "max", l.maxWorkers, "level", level)
		}
	}
	return c
}

// tryAcquire reserves one transcode slot when the live active count is below
// the current cap. Returns false when the cap is 0 (Critical) or already
// saturated (this is a surplus worker) — the caller then waits and the job
// stays queued. On success the injected active-workers gauge is bumped.
func (l *encodeLimiter) tryAcquire() bool {
	c := l.currentCap()
	for {
		cur := l.active.Load()
		if int(cur) >= c {
			return false
		}
		if l.active.CompareAndSwap(cur, cur+1) {
			l.publish(cur + 1)
			return true
		}
	}
}

// release returns a previously-acquired slot and lowers the gauge.
func (l *encodeLimiter) release() {
	n := l.active.Add(-1)
	if n < 0 { // defensive: never let a stray double-release drive the gauge negative
		n = 0
		l.active.Store(0)
	}
	l.publish(n)
}

func (l *encodeLimiter) publish(n int32) {
	if l.setActive != nil {
		l.setActive(int(n))
	}
}
