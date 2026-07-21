package service

// Smoother is the asymmetric EWMA over the raw pressure score: rise fast so a
// genuine ramp is tracked in ~4 ticks (~60s at the 15s tick, mirroring the
// level machine's enterTicks), decay slow (~5min, mirroring exitTicks) so the
// probes→pressure→fewer-probes feedback loop steps down and STAYS down
// instead of oscillating. Pure — no clock, no IO.
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

// Tick feeds one raw score sample and returns the smoothed value.
func (s *Smoother) Tick(raw float64) float64 {
	a := s.alphaDown
	if raw > s.value {
		a = s.alphaUp
	}
	s.value += a * (raw - s.value)
	// Snap the asymptotic tail to a clean 0.00 once recovered.
	if raw == 0 && s.value < 0.005 {
		s.value = 0
	}
	return s.value
}

// Reset zeroes the state (fail-open after sustained Prometheus loss).
func (s *Smoother) Reset() { s.value = 0 }
