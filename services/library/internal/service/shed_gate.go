package service

import (
	"math"
	"sync/atomic"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	gometrics "github.com/ILITA-hub/animeenigma/libs/metrics"
)

// ShedChecker is the narrow degradation-consumer surface (satisfied by
// *cache.DegradationWatcher). Shared by WorkerPool, EncoderPool and the
// storyboard backfill loop.
type ShedChecker interface {
	Level() int
	Score() float64
}

const (
	storyboardPauseScore = 0.30
	encodeReduceScore    = 0.40
	encodePauseScore     = 0.80
	downloadReduceScore  = 0.55
	downloadPauseScore   = 0.90
)

// shedGate wraps a ShedChecker for one subsystem: it answers "should this
// loop skip claiming new work right now?", keeps the
// ae_degradation_shed{subsystem} gauge in sync, and logs exactly once per
// state change. Safe for concurrent use by multiple worker goroutines
// (both pools run >1 worker by default) — the paused flag is atomic, so the
// transition log/gauge fire once, not per worker.
type shedGate struct {
	checker     ShedChecker
	subsystem   string
	pauseAt     float64
	log         *logger.Logger
	paused      atomic.Bool
	initialized atomic.Bool
}

func newShedGate(subsystem string, pauseAt float64, log *logger.Logger) *shedGate {
	return &shedGate{subsystem: subsystem, pauseAt: pauseAt, log: log}
}

// set wires the degradation watcher (nil-safe; nil never sheds).
func (g *shedGate) set(c ShedChecker) { g.checker = c }

// shed reports whether new-work admission is currently shed, updating the
// gauge + logging on state changes only.
func (g *shedGate) shed() bool {
	score, level := 0.0, 0
	if g.checker != nil {
		score, level = g.checker.Score(), g.checker.Level()
	}
	shed := level >= 2 || score >= g.pauseAt
	changed := g.paused.Swap(shed) != shed
	first := !g.initialized.Swap(true)
	if first || changed {
		v := 0.0
		if shed {
			v = 1
			if changed && g.log != nil {
				g.log.Infow("pausing new work: platform degraded",
					"subsystem", g.subsystem, "score", score, "pause_at", g.pauseAt, "level", level)
			}
		} else if changed && g.log != nil {
			g.log.Infow("resuming work: degradation cleared", "subsystem", g.subsystem)
		}
		gometrics.DegradationShed.WithLabelValues(g.subsystem).Set(v)
	}
	return shed
}

// gradedLimiter is a score-driven concurrency limiter shared by the encoder
// and download pools. Between reduceAt and pauseAt it removes worker slots one
// at a time instead of stopping every worker on the same level transition.
// Level 2 remains a hard backstop. Already-running work is never interrupted.
//
// Surplus workers — those beyond the current cap — simply wait in their poll
// loop; the extra jobs stay queued rather than being shed or dropped. This
// preserves full throughput on an idle host while preventing concurrent heavy
// work from stacking when pressure rises.
//
// Safe for concurrent use by every pool goroutine: the active count is atomic,
// and the shed gauge + one-shot transition log fire only when the cap changes.
type gradedLimiter struct {
	checker    ShedChecker
	subsystem  string
	maxWorkers int
	reduceAt   float64
	pauseAt    float64
	log        *logger.Logger
	// setActive mirrors the live active-worker count to
	// library_encode_active_workers. Injected (rather than imported) so the
	// single-emitter gauge stays in the library metrics package — a plain gauge
	// in libs/metrics would auto-register as an impostor 0-series in every
	// importing binary (the auto-registration trap). nil-safe.
	setActive func(int)

	active      atomic.Int32
	lastCap     atomic.Int32
	initialized atomic.Bool
}

// newEncodeLimiter builds a limiter capping concurrent transcodes at maxWorkers
// (forced >=1). setActive may be nil (no gauge). The checker is wired later via
// set(); a nil checker never sheds, pinning the cap at maxWorkers.
func newGradedLimiter(subsystem string, maxWorkers int, reduceAt, pauseAt float64, setActive func(int), log *logger.Logger) *gradedLimiter {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	l := &gradedLimiter{
		subsystem: subsystem, maxWorkers: maxWorkers,
		reduceAt: reduceAt, pauseAt: pauseAt,
		setActive: setActive, log: log,
	}
	// Seed lastCap at the level-0 cap so a normal boot logs no transition; a
	// boot into an already-degraded level logs exactly one.
	l.lastCap.Store(int32(maxWorkers))
	return l
}

func newEncodeLimiter(maxWorkers int, setActive func(int), log *logger.Logger) *gradedLimiter {
	return newGradedLimiter("library_encode", maxWorkers, encodeReduceScore, encodePauseScore, setActive, log)
}

func newDownloadLimiter(maxWorkers int, log *logger.Logger) *gradedLimiter {
	return newGradedLimiter("library_download", maxWorkers, downloadReduceScore, downloadPauseScore, nil, log)
}

// set wires the degradation watcher (nil-safe; nil pins the cap at maxWorkers).
func (l *gradedLimiter) set(c ShedChecker) { l.checker = c }

// capFor maps the smoothed score onto [maxWorkers..0]. floor rounding removes
// one slot as each score band is crossed; the epsilon only protects exact
// integer boundaries from floating-point representation error.
func (l *gradedLimiter) capFor(score float64, level int) int {
	switch {
	case level >= 2 || score >= l.pauseAt:
		return 0
	case score <= l.reduceAt:
		return l.maxWorkers
	case l.pauseAt <= l.reduceAt:
		return 0
	}
	cap := int(math.Floor(float64(l.maxWorkers)*(l.pauseAt-score)/(l.pauseAt-l.reduceAt) + 1e-9))
	// pauseAt is the only score that fully pauses admission. Keep one worker
	// alive throughout the graduated band even when floor rounding reaches 0.
	if cap < 1 {
		return 1
	}
	if cap > l.maxWorkers {
		return l.maxWorkers
	}
	return cap
}

// currentCap reads the live score/level, computes the cap, and — on a cap
// change — updates ae_degradation_shed{subsystem} and logs once. Shed intensity
// honestly reflects the pool: full=0, reduced=1, paused=2.
func (l *gradedLimiter) currentCap() int {
	level, score := 0, 0.0
	if l.checker != nil {
		level = l.checker.Level()
		score = l.checker.Score()
	}
	c := l.capFor(score, level)
	prev := l.lastCap.Swap(int32(c))
	changed := prev != int32(c)
	first := !l.initialized.Swap(true)
	if first || changed {
		shed := 0.0
		switch {
		case c == 0:
			shed = 2
		case c < l.maxWorkers:
			shed = 1
		}
		gometrics.DegradationShed.WithLabelValues(l.subsystem).Set(shed)
		if changed && l.log != nil {
			l.log.Infow("worker concurrency cap changed (platform degradation)",
				"subsystem", l.subsystem, "cap", c, "max", l.maxWorkers,
				"score", score, "reduce_at", l.reduceAt, "pause_at", l.pauseAt, "level", level)
		}
	}
	return c
}

// tryAcquire reserves one worker slot when the live active count is below the
// current cap. Returns false when paused or saturated; the caller waits and
// leaves the job queued. On success the optional active-workers gauge is bumped.
func (l *gradedLimiter) tryAcquire() bool {
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
func (l *gradedLimiter) release() {
	n := l.active.Add(-1)
	if n < 0 { // defensive: never let a stray double-release drive the gauge negative
		n = 0
		l.active.Store(0)
	}
	l.publish(n)
}

func (l *gradedLimiter) publish(n int32) {
	if l.setActive != nil {
		l.setActive(int(n))
	}
}
