package service

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSmootherRisesFastDecaysSlow(t *testing.T) {
	s := NewSmoother(0.5, 0.05)
	// Step input 0 -> 1: alphaUp=0.5 halves the gap each tick.
	if v := s.Tick(1.0); !almost(v, 0.5) {
		t.Fatalf("tick1 = %v; want 0.5", v)
	}
	if v := s.Tick(1.0); !almost(v, 0.75) {
		t.Fatalf("tick2 = %v; want 0.75", v)
	}
	// Input drops to 0: alphaDown=0.05 decays 5% of the gap per tick.
	if v := s.Tick(0); !almost(v, 0.75*0.95) {
		t.Fatalf("decay tick = %v; want %v", v, 0.75*0.95)
	}
}

func TestSmootherSnapsToZeroAndReset(t *testing.T) {
	s := NewSmoother(0.5, 0.05)
	s.Tick(0.004) // tiny raw
	// Residue below 0.005 with raw 0 snaps to exact 0 so a recovered box
	// publishes a clean 0.00.
	if v := s.Tick(0); v != 0 {
		t.Fatalf("snap = %v; want exact 0", v)
	}
	s.Tick(1.0)
	s.Reset()
	if v := s.Tick(0); v != 0 {
		t.Fatalf("post-reset = %v; want 0", v)
	}
}
