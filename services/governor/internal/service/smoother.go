package service

import "math"

// Smoother is the asymmetric EWMA over the raw pressure score and the single
// smoothed state of the governor: rise fast so a genuine ramp is tracked in
// ~4 ticks (~60s at the 15s tick), decay slow (~5min) so the
// probes→pressure→fewer-probes feedback loop steps down and STAYS down instead
// of oscillating. The Quantizer turns this value into the discrete level, so
// the enter-fast/exit-slow alphas here and its enter/exit thresholds together
// shape the level's timing. Pure — no clock, no IO.
type Smoother struct {
	alphaUp, alphaDown float64
	value              float64
}

// NewSmoother builds a Smoother. Alphas outside (0,1] are clamped to sane
// defaults (0.5 up, 0.05 down).
func NewSmoother(alphaUp, alphaDown float64) *Smoother {
	if alphaUp <= 0 || alphaUp > 1 {
		alphaUp = 0.5
	}
	if alphaDown <= 0 || alphaDown > 1 {
		alphaDown = 0.05
	}
	return &Smoother{alphaUp: alphaUp, alphaDown: alphaDown}
}

// Tick feeds one raw score sample and returns the smoothed value. The input
// is bounded defensively before smoothing: a NaN sample (e.g. a bad
// Prometheus scrape) is treated as 0, and anything outside [0,1] is clamped —
// a caller bug or upstream glitch must never propagate NaN/out-of-range
// values into the published score.
func (s *Smoother) Tick(raw float64) float64 {
	if math.IsNaN(raw) {
		raw = 0
	}
	raw = math.Max(0, math.Min(1, raw))

	a := s.alphaDown
	if raw > s.value {
		a = s.alphaUp
	}
	s.value += a * (raw - s.value)
	// Snap the asymptotic tail to a clean 0.00 once recovered. Uses the
	// clamped raw so a NaN sample (clamped to 0 above) still allows the snap.
	if raw == 0 && s.value < 0.005 {
		s.value = 0
	}
	return s.value
}

// Value returns the current smoothed value without advancing it — used to hold
// the score across a stale/grace tick where feeding a sample would be wrong.
func (s *Smoother) Value() float64 { return s.value }

// Reset zeroes the state (fail-open after sustained Prometheus loss).
func (s *Smoother) Reset() { s.value = 0 }
