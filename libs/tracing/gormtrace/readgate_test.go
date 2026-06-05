package gormtrace

import (
	"sync"
	"testing"
)

// TestReadGate exercises the in-memory P95 threshold gate: slow reads above the
// per-(op,table) P95 record, fast reads below it do not, missing keys fall back
// to the static cold-start default, and concurrent SetSnapshot/ShouldRecord is
// race-free.
func TestReadGate(t *testing.T) {
	t.Run("slow read above p95 records", func(t *testing.T) {
		g := NewReadGate(50)
		g.SetSnapshot(map[string]float64{"catalog.X|animes": 80})
		if !g.ShouldRecord("catalog.X", "animes", 120) {
			t.Fatalf("durMS=120 > p95=80 should record")
		}
	})

	t.Run("fast read below p95 does not record", func(t *testing.T) {
		g := NewReadGate(50)
		g.SetSnapshot(map[string]float64{"catalog.X|animes": 80})
		if g.ShouldRecord("catalog.X", "animes", 10) {
			t.Fatalf("durMS=10 < p95=80 should NOT record")
		}
	})

	t.Run("missing key uses static cold-start default", func(t *testing.T) {
		g := NewReadGate(50) // static default 50ms
		// No snapshot entries at all.
		if !g.ShouldRecord("unknown.Op", "unknown_table", 60) {
			t.Fatalf("durMS=60 > default=50 should record on missing key")
		}
		if g.ShouldRecord("unknown.Op", "unknown_table", 40) {
			t.Fatalf("durMS=40 < default=50 should NOT record on missing key")
		}
	})

	t.Run("static default tunable via constructor", func(t *testing.T) {
		g := NewReadGate(100)
		if g.ShouldRecord("a", "b", 90) {
			t.Fatalf("durMS=90 < default=100 should NOT record")
		}
		if !g.ShouldRecord("a", "b", 110) {
			t.Fatalf("durMS=110 > default=100 should record")
		}
	})

	t.Run("nil snapshot before first SetSnapshot uses default", func(t *testing.T) {
		g := NewReadGate(50)
		// ShouldRecord before any SetSnapshot must not panic and uses the default.
		if !g.ShouldRecord("a", "b", 60) {
			t.Fatalf("durMS=60 > default=50 should record with no snapshot set")
		}
	})

	t.Run("SetSnapshot atomically swaps; concurrent reads are race-free", func(t *testing.T) {
		g := NewReadGate(50)
		g.SetSnapshot(map[string]float64{"op|t": 80})

		var wg sync.WaitGroup
		// Concurrent writers swapping the snapshot.
		for w := 0; w < 4; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < 500; i++ {
					g.SetSnapshot(map[string]float64{"op|t": float64(50 + i%100)})
				}
			}(w)
		}
		// Concurrent readers.
		for r := 0; r < 8; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 1000; i++ {
					_ = g.ShouldRecord("op", "t", 60)
				}
			}()
		}
		wg.Wait()
	})
}
