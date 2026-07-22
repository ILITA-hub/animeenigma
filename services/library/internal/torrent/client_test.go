package torrent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gometrics "github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestNewClient_CreatesDownloadDir verifies that NewClient mkdirs the
// download directory if it doesn't already exist. Anacrolix would
// otherwise fail on first piece write.
func TestNewClient_CreatesDownloadDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "torrents")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s should not exist yet", dir)
	}
	c, err := NewClient(Config{
		DownloadDir:    dir,
		MaxPeers:       10,
		UploadRateKBPS: 0,
		SeedDuration:   time.Minute,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("download dir was not created: stat=%v err=%v", info, err)
	}
}

// TestNewClient_EmptyDownloadDirRejected — defensive: an empty
// DownloadDir is a configuration bug, not "use cwd".
func TestNewClient_EmptyDownloadDirRejected(t *testing.T) {
	_, err := NewClient(Config{})
	if err == nil {
		t.Fatal("expected NewClient to fail on empty DownloadDir")
	}
}

// TestClient_Add_RejectsInvalidMagnet — a malformed magnet must be
// rejected with a wrapped error before anacrolix's AddMagnet is
// called. The handler surfaces this as 400.
func TestClient_Add_RejectsInvalidMagnet(t *testing.T) {
	c, err := NewClient(Config{
		DownloadDir:  t.TempDir(),
		MaxPeers:     10,
		SeedDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	h, err := c.Add(context.Background(), "definitely-not-a-magnet")
	if err == nil {
		t.Fatal("expected Add() to fail on invalid magnet")
	}
	if h != nil {
		t.Fatal("expected nil handle on invalid magnet")
	}
}

// TestDownloadHandle_CancelIsIdempotent — repeated Cancel() calls must
// not panic, and Done() must resolve exactly once. The worker may call
// Cancel during a graceful shutdown AND in a deferred cleanup; both
// paths must be safe.
func TestDownloadHandle_CancelIsIdempotent(t *testing.T) {
	c, err := NewClient(Config{
		DownloadDir:  t.TempDir(),
		MaxPeers:     5,
		SeedDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Use a syntactically-valid magnet that won't resolve to anything
	// (no trackers) so we don't reach out to the network.
	magnet := "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=offline-test"
	h, err := c.Add(context.Background(), magnet)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Three concurrent Cancels — sync.Once should serialize, and
	// none should panic.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Cancel()
		}()
	}
	wg.Wait()

	// Done() must resolve.
	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not resolve after Cancel()")
	}

	// And calling Cancel() again after Done() resolves must still
	// be safe.
	h.Cancel()
}

// TestClient_CloseResolvesOutstandingHandles — closing the client
// must tear down all outstanding torrents' Done() channels so the
// worker goroutines can exit cleanly on shutdown.
func TestClient_CloseResolvesOutstandingHandles(t *testing.T) {
	c, err := NewClient(Config{
		DownloadDir:  t.TempDir(),
		MaxPeers:     5,
		SeedDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	magnet := "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=offline"
	h, err := c.Add(context.Background(), magnet)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Cancel() after Close() must still be safe (the lifecycle
	// goroutine may already have exited).
	h.Cancel()

	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("handle Done() did not resolve after Client.Close()")
	}
}

// TestHandle_ProgressBeforeMetadata — before <-GotInfo fires, Info()
// returns nil. Progress must report total == -1 so the worker can tell
// "we don't know the size yet" from "0 bytes downloaded".
func TestHandle_ProgressBeforeMetadata(t *testing.T) {
	c, err := NewClient(Config{
		DownloadDir:  t.TempDir(),
		MaxPeers:     5,
		SeedDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	magnet := "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=offline"
	h, err := c.Add(context.Background(), magnet)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(h.Cancel)

	downloaded, total, peers := h.Progress()
	if downloaded != 0 {
		t.Fatalf("downloaded = %d, want 0", downloaded)
	}
	if total != -1 {
		t.Fatalf("total = %d, want -1 (metadata not yet available)", total)
	}
	if peers < 0 {
		t.Fatalf("peers = %d, want non-negative", peers)
	}

	// ID is the lowercase hex infohash from the magnet. We don't
	// pin the exact value (anacrolix may canonicalize) but it must
	// be non-empty.
	if id := h.ID(); id == "" {
		t.Fatal("ID() returned empty string after Add")
	}
}

// --- degradation-aware seeding shed ---

// fakeShedChecker is a settable ShedChecker for driving the seed gate's
// transition logic without a live governor / DegradationWatcher.
type fakeShedChecker struct{ level atomic.Int32 }

func (f *fakeShedChecker) set(n int)  { f.level.Store(int32(n)) }
func (f *fakeShedChecker) Level() int { return int(f.level.Load()) }

// TestShouldPauseSeed pins the pure pause decision: pause at Elevated+ (level>=1),
// seed at Normal (0) and fail open on a negative/nonsense level.
func TestShouldPauseSeed(t *testing.T) {
	cases := []struct {
		level int
		want  bool
	}{
		{-1, false}, // nonsense level fails open (no pause)
		{0, false},  // Normal: seed
		{1, true},   // Elevated: pause
		{2, true},   // Critical: pause
	}
	for _, tc := range cases {
		if got := shouldPauseSeed(tc.level); got != tc.want {
			t.Errorf("shouldPauseSeed(%d) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

// TestReconcileSeedGate_TransitionsOnly asserts the gate applies Disallow(true)
// when the level rises to Elevated+ and Allow(false) when it returns to Normal,
// flips the ae_degradation_shed{subsystem="library_seed"} gauge accordingly, and
// acts EXACTLY once per transition (never on a steady-state repeat tick). The
// apply step is injected so the decision + one-shot behavior is asserted without
// a live anacrolix client.
func TestReconcileSeedGate_TransitionsOnly(t *testing.T) {
	shed := &fakeShedChecker{}
	var applied []bool // records the pause arg of each apply invocation, in order
	c := &Client{
		shed:          shed,
		applySeedGate: func(pause bool) { applied = append(applied, pause) },
	}

	gauge := func() float64 {
		return testutil.ToFloat64(gometrics.DegradationShed.WithLabelValues("library_seed"))
	}

	// Boot at Normal: seedPaused zero-value already means "not paused", so there
	// is no transition and no apply.
	c.reconcileSeedGate()
	c.reconcileSeedGate()
	if len(applied) != 0 {
		t.Fatalf("level 0 boot: applied=%v, want no calls", applied)
	}

	// Escalate to Elevated: exactly one Disallow(true); a repeat tick at the same
	// level must NOT re-apply.
	shed.set(1)
	c.reconcileSeedGate()
	c.reconcileSeedGate()
	if want := []bool{true}; !equalBools(applied, want) {
		t.Fatalf("after escalate: applied=%v, want %v", applied, want)
	}
	if g := gauge(); g != 1 {
		t.Fatalf("shed gauge = %v, want 1 (paused)", g)
	}

	// Critical (2) is still "pause" -- no new transition, no new apply.
	shed.set(2)
	c.reconcileSeedGate()
	if want := []bool{true}; !equalBools(applied, want) {
		t.Fatalf("level 1->2 (still paused): applied=%v, want %v", applied, want)
	}

	// Recover to Normal: exactly one Allow(false); gauge back to 0.
	shed.set(0)
	c.reconcileSeedGate()
	c.reconcileSeedGate()
	if want := []bool{true, false}; !equalBools(applied, want) {
		t.Fatalf("after recover: applied=%v, want %v", applied, want)
	}
	if g := gauge(); g != 0 {
		t.Fatalf("shed gauge = %v, want 0 (normal)", g)
	}
}

// TestReconcileSeedGate_NilCheckerFailsOpen — a Client with no ShedChecker
// (governor down/undeployed) must never pause seeding.
func TestReconcileSeedGate_NilCheckerFailsOpen(t *testing.T) {
	var applied []bool
	c := &Client{applySeedGate: func(pause bool) { applied = append(applied, pause) }}
	c.reconcileSeedGate()
	c.reconcileSeedGate()
	if len(applied) != 0 {
		t.Fatalf("nil checker: applied=%v, want no calls (fail-open)", applied)
	}
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
