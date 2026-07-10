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
