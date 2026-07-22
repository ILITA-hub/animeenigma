package service

import "github.com/ILITA-hub/animeenigma/services/governor/internal/domain"

// Quantizer turns the single smoothed pressure score into the discrete
// published level using per-boundary hysteresis (a Schmitt trigger). It
// REPLACES the old streak-based Machine: there is now ONE smoothed state (the
// EWMA score), and the level is a pure function of it, so level and score can
// never disagree — and a jittery-but-rising signal integrates through the
// EWMA instead of resetting a streak.
//
// Enter/exit asymmetry gives the "enter-fast / exit-slow" behavior together
// with the score EWMA's asymmetric alphas: the score rises fast toward a
// breach and decays slowly, and the exit thresholds sit well below the enter
// thresholds so a recovered box holds its level for the decay window before
// stepping down (anti-flap + lets the probes→pressure→fewer-probes loop settle).
//
// Score calibration (from the recording rules): 0.5 == the elevated breach
// point, 1.0 == the critical breach point. Defaults therefore enter Elevated a
// hair below 0.5 (so a sustained just-at-elevated signal still promotes) and
// enter Critical near 1.0; the exit thresholds are tuned for a multi-minute
// hold at the default alphas. Pure: no clock, no IO.
type Quantizer struct {
	enterElevated float64
	exitElevated  float64
	enterCritical float64
	exitCritical  float64
	level         domain.Level
}

// NewQuantizer builds a Quantizer starting at LevelNormal. Thresholds are
// sanitized: any non-monotone or out-of-(0,1) value falls back to the vetted
// defaults (0.45/0.20 elevated enter/exit, 0.90/0.55 critical enter/exit), and
// each exit is forced strictly below its enter so the Schmitt gap can never
// collapse into flapping.
func NewQuantizer(enterElevated, exitElevated, enterCritical, exitCritical float64) *Quantizer {
	q := &Quantizer{
		enterElevated: enterElevated,
		exitElevated:  exitElevated,
		enterCritical: enterCritical,
		exitCritical:  exitCritical,
	}
	if !validThresholds(q) {
		q.enterElevated, q.exitElevated = 0.45, 0.20
		q.enterCritical, q.exitCritical = 0.90, 0.55
	}
	return q
}

// validThresholds enforces 0 < exit < enter for each tier and
// elevated-enter < critical-enter (nested bands).
func validThresholds(q *Quantizer) bool {
	inRange := func(v float64) bool { return v > 0 && v < 1 }
	return inRange(q.exitElevated) && inRange(q.enterElevated) &&
		inRange(q.exitCritical) && inRange(q.enterCritical) &&
		q.exitElevated < q.enterElevated &&
		q.exitCritical < q.enterCritical &&
		q.enterElevated < q.enterCritical &&
		q.exitElevated < q.exitCritical
}

// Level returns the current quantized level.
func (q *Quantizer) Level() domain.Level { return q.level }

// Tick feeds one smoothed score and returns the (possibly unchanged) level.
// Transitions honor per-boundary hysteresis relative to the CURRENT level:
// raising uses the enter thresholds, lowering uses the (lower) exit thresholds.
func (q *Quantizer) Tick(score float64) domain.Level {
	switch q.level {
	case domain.LevelNormal:
		switch {
		case score >= q.enterCritical:
			q.level = domain.LevelCritical
		case score >= q.enterElevated:
			q.level = domain.LevelElevated
		}
	case domain.LevelElevated:
		switch {
		case score >= q.enterCritical:
			q.level = domain.LevelCritical
		case score <= q.exitElevated:
			q.level = domain.LevelNormal
		}
	case domain.LevelCritical:
		switch {
		case score <= q.exitElevated:
			// Score collapsed straight through the elevated band (rare given the
			// slow decay) — drop all the way rather than pin at Elevated.
			q.level = domain.LevelNormal
		case score <= q.exitCritical:
			q.level = domain.LevelElevated
		}
	}
	return q.level
}

// Reset forces the level back to Normal (fail-open after sustained Prometheus
// loss, mirroring Smoother.Reset).
func (q *Quantizer) Reset() { q.level = domain.LevelNormal }
