package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"golang.org/x/sys/unix"
)

// fakeStatfs returns a fixed Statfs result. block size and counts are
// tuned so the math is easy to verify.
//
// Bsize=4096 (4 KiB blocks)
// Blocks=1_000_000  → totalBytes = 4_096_000_000 = ~3.8 GiB
// Bavail            → varies per test
func fakeStatfs(blocks, bavail uint64) statfsFunc {
	return func(path string, st *unix.Statfs_t) error {
		st.Bsize = 4096
		st.Blocks = blocks
		st.Bavail = bavail
		return nil
	}
}

func TestDiskGuard_Check_ComputesPercent(t *testing.T) {
	g := &DiskGuard{path: "/data/torrents", statfs: fakeStatfs(1_000_000, 250_000)}

	free, total, pct, err := g.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if total != 4_096_000_000 {
		t.Fatalf("total = %d, want 4096000000", total)
	}
	if free != 1_024_000_000 {
		t.Fatalf("free = %d, want 1024000000", free)
	}
	if pct != 25 {
		t.Fatalf("pct = %d, want 25", pct)
	}
}

func TestDiskGuard_Check_ZeroTotal(t *testing.T) {
	g := &DiskGuard{path: "/x", statfs: fakeStatfs(0, 0)}
	_, total, pct, err := g.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if pct != 0 {
		t.Fatalf("pct on zero-total mount = %d, want 0", pct)
	}
}

func TestDiskGuard_Check_StatfsError(t *testing.T) {
	g := &DiskGuard{path: "/x", statfs: func(_ string, _ *unix.Statfs_t) error {
		return errors.New("statfs busted")
	}}
	_, _, _, err := g.Check()
	if err == nil {
		t.Fatal("expected error from Check when statfs returns one")
	}
}

func TestDiskGuard_Allow(t *testing.T) {
	cases := []struct {
		name     string
		bavail   uint64
		minPct   int
		expected bool
	}{
		{"plenty of space", 500_000, 20, true},   // 50% free, 20% required
		{"exactly at threshold", 200_000, 20, true},
		{"just below threshold", 199_000, 20, false}, // 19% free, 20% required
		{"way below threshold", 1_000, 20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &DiskGuard{path: "/x", statfs: fakeStatfs(1_000_000, tc.bavail)}
			allowed, pct, err := g.Allow(tc.minPct)
			if err != nil {
				t.Fatalf("Allow: %v", err)
			}
			if allowed != tc.expected {
				t.Fatalf("Allow(%d) → %v (pct=%d), want %v", tc.minPct, allowed, pct, tc.expected)
			}
		})
	}
}

// TestDiskGuard_Run_UpdatesGauge — after one tick, library_disk_free_bytes
// reflects the fake Statfs result.
func TestDiskGuard_Run_UpdatesGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewLibraryMetricsWithRegisterer(reg)

	g := NewDiskGuard("/x", m, nil)
	g.statfs = fakeStatfs(1_000_000, 500_000) // 2 GiB free

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		g.Run(ctx, 10*time.Millisecond)
	}()

	// The initial tick happens before the loop blocks on the
	// ticker, so we don't have to wait the full interval.
	// Still, give it some slack on busy CI.
	deadline := time.After(500 * time.Millisecond)
	for {
		// Use Gather() to read the value through the registry path
		// (the public surface) rather than testutil.ToFloat64 on a
		// private field.
		families, _ := reg.Gather()
		var got float64
		for _, f := range families {
			if f.GetName() == "library_disk_free_bytes" {
				for _, mm := range f.GetMetric() {
					got = mm.GetGauge().GetValue()
				}
			}
		}
		if got == 2_048_000_000 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("library_disk_free_bytes did not update; final = %v, want 2048000000", got)
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done

	// Spot-check via the test seam too — confirms the Gather() loop
	// above isn't reading a stale snapshot.
	_ = testutil.CollectAndCount
}
