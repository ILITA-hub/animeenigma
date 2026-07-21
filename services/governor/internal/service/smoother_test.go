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

func TestSmootherClampsOutOfRangeRaw(t *testing.T) {
	s := NewSmoother(0.5, 0.05)
	// raw 1.5 must clamp to 1.0 before smoothing: alphaUp=0.5 halves the 0->1
	// gap, same as a legitimate raw of exactly 1.0 would.
	if v := s.Tick(1.5); !almost(v, 0.5) {
		t.Fatalf("Tick(1.5) = %v; want 0.5 (clamped to 1.0)", v)
	}
}

func TestSmootherTreatsNaNAsZeroAndAllowsSnap(t *testing.T) {
	s := NewSmoother(0.5, 0.05)
	s.Tick(0.004) // tiny raw, residue lands below the 0.005 snap threshold
	// A NaN sample (e.g. a bad scrape) must be treated as raw 0 — never
	// propagated as NaN — and the raw==0 snap check must use the clamped
	// value so a NaN input still snaps the tail to a clean 0.
	if v := s.Tick(math.NaN()); v != 0 {
		t.Fatalf("Tick(NaN) = %v; want exact 0 (NaN clamped to raw 0, snap applies)", v)
	}
}
