package torrent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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
