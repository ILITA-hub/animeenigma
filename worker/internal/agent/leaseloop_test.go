package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

// fakeConn implements leaseConn for test injection.
type fakeConn struct {
	mu     sync.Mutex
	grants []wire.LeaseGrantPayload
	idx    int
}

func (f *fakeConn) ReadGrant(_ context.Context) (wire.LeaseGrantPayload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.grants) {
		// Block forever — test cancels ctx.
		// Return empty to signal no-work (triggers idle path).
		return wire.LeaseGrantPayload{}, nil
	}
	g := f.grants[f.idx]
	f.idx++
	return g, nil
}

// fakeProc records calls and writes a fixed output file.
type fakeProc struct {
	mu    sync.Mutex
	calls []string // inSeg paths
}

func (p *fakeProc) Process(_ context.Context, inSeg, outSeg string) (Stats, error) {
	p.mu.Lock()
	p.calls = append(p.calls, inSeg)
	p.mu.Unlock()
	// Write a small output so Upload has something to stream.
	return Stats{}, os.WriteFile(outSeg, []byte("processed"), 0600)
}

func (p *fakeProc) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// buildLeaseClient builds a Client wired to a fake upload/download server.
// The server accepts GET (returns segContent) and PUT (200 OK) for any path.
func buildLeaseClient(t *testing.T, segContent string) (*Client, *httptest.Server, *atomic.Int64) {
	t.Helper()
	var putCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(segContent)) //nolint:errcheck
		case http.MethodPut:
			putCount.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := Config{ServerURL: srv.URL, EnrollToken: "tok", Mode: "batch"}
	c := NewClient(cfg)
	c.backoff = BackoffConfig{Initial: 5 * time.Millisecond, Max: 20 * time.Millisecond}
	return c, srv, &putCount
}

// TestLeaseLoop_ProcessesTwoSegments verifies the core loop:
// two grants → both processed → both uploaded → local files deleted → idle on empty.
func TestLeaseLoop_ProcessesTwoSegments(t *testing.T) {
	t.Parallel()

	c, srv, putCount := buildLeaseClient(t, "input-data")
	_ = srv

	proc := &fakeProc{}
	conn := &fakeConn{
		grants: []wire.LeaseGrantPayload{
			{JobID: "job-1", Idx: 0, Handles: wire.LeaseHandles{
				GetHandle: "gh0", GetExp: "99", GetSig: "gs0",
				PutHandle: "ph0", PutExp: "99", PutSig: "ps0",
			}},
			{JobID: "job-1", Idx: 1, Handles: wire.LeaseHandles{
				GetHandle: "gh1", GetExp: "99", GetSig: "gs1",
				PutHandle: "ph1", PutExp: "99", PutSig: "ps1",
			}},
			// empty grant: no more work
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run the loop in a goroutine; cancel after idle is reached.
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.RunLeaseLoop(ctx, "test-worker", proc, conn) //nolint:errcheck
	}()

	// Wait until both segments are processed.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if proc.callCount() >= 2 && putCount.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if proc.callCount() < 2 {
		t.Errorf("expected proc called 2 times, got %d", proc.callCount())
	}
	if putCount.Load() < 2 {
		t.Errorf("expected 2 PUT uploads, got %d", putCount.Load())
	}

	// Cancel and wait for goroutine to finish.
	cancel()
	<-done
}

// TestLeaseLoop_DeletesLocalFiles verifies process-and-delete: the worker
// removes both the input and output temp files after a successful upload.
func TestLeaseLoop_DeletesLocalFiles(t *testing.T) {
	t.Parallel()

	c, _, _ := buildLeaseClient(t, "seg-data")

	// Override TempDir to a controllable location.
	tmp := t.TempDir()

	// We intercept Process to record the paths and verify they are deleted after.
	var (
		mu       sync.Mutex
		inPaths  []string
		outPaths []string
	)

	proc := &recordingProc{fn: func(in, out string) error {
		mu.Lock()
		inPaths = append(inPaths, in)
		outPaths = append(outPaths, out)
		mu.Unlock()
		return os.WriteFile(out, []byte("ok"), 0600)
	}}

	conn := &fakeConn{
		grants: []wire.LeaseGrantPayload{
			{JobID: "job-del", Idx: 5, Handles: wire.LeaseHandles{
				GetHandle: "gh", GetExp: "1", GetSig: "gs",
				PutHandle: "ph", PutExp: "1", PutSig: "ps",
			}},
		},
	}

	// Point processSegment to our tmp dir by patching os.TempDir via the
	// segment path. We can't easily override os.TempDir, so instead we
	// verify after that the paths referenced by the proc no longer exist.
	_ = tmp // unused directly; we just check the paths proc receives

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.RunLeaseLoop(ctx, "w", proc, conn) //nolint:errcheck
	}()

	// Wait for the processor to be called.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(inPaths)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Give the defer cleanup a moment to run.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(inPaths) == 0 {
		t.Fatal("processor was never called")
	}
	for _, p := range inPaths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("input segment file %q still exists after processing", p)
		}
	}
	for _, p := range outPaths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("output segment file %q still exists after processing", p)
		}
	}
}

// recordingProc is a Processor that calls fn(inSeg, outSeg).
type recordingProc struct {
	fn func(in, out string) error
}

func (p *recordingProc) Process(_ context.Context, inSeg, outSeg string) (Stats, error) {
	return Stats{}, p.fn(inSeg, outSeg)
}

// TestLeaseLoop_IdlesOnEmptyGrant verifies that an empty grant (no work)
// causes the loop to wait and then retry (not exit).
func TestLeaseLoop_IdlesOnEmptyGrant(t *testing.T) {
	t.Parallel()

	c, _, _ := buildLeaseClient(t, "")

	var grantCalls atomic.Int64
	conn := &countingConn{fn: func() wire.LeaseGrantPayload {
		n := grantCalls.Add(1)
		if n <= 3 {
			return wire.LeaseGrantPayload{} // empty = no work
		}
		// block by returning empty forever; test cancels ctx
		return wire.LeaseGrantPayload{}
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.RunLeaseLoop(ctx, "w", CopyProcessor{}, conn) //nolint:errcheck
	}()

	// We expect multiple grant calls (idle retry loop), not just one.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if grantCalls.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done

	if grantCalls.Load() < 2 {
		t.Errorf("expected >=2 grant calls (idle+retry), got %d", grantCalls.Load())
	}
}

// countingConn implements leaseConn via a function.
type countingConn struct {
	fn func() wire.LeaseGrantPayload
}

func (cc *countingConn) ReadGrant(ctx context.Context) (wire.LeaseGrantPayload, error) {
	if ctx.Err() != nil {
		return wire.LeaseGrantPayload{}, ctx.Err()
	}
	return cc.fn(), nil
}

// TestLeaseLoop_WorkerIDInRequests verifies that X-Worker-Id is sent on GET/PUT.
func TestLeaseLoop_WorkerIDInRequests(t *testing.T) {
	t.Parallel()

	const wantWorkerID = "wid-check"
	var (
		mu             sync.Mutex
		gotWorkerIDs   []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotWorkerIDs = append(gotWorkerIDs, r.Header.Get("X-Worker-Id"))
		mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("data")) //nolint:errcheck
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := Config{ServerURL: srv.URL}
	c := NewClient(cfg)

	proc := &fakeProc{}
	conn := &fakeConn{
		grants: []wire.LeaseGrantPayload{
			{JobID: "job-wid", Idx: 0, Handles: wire.LeaseHandles{
				GetHandle: "gh", GetExp: "1", GetSig: "gs",
				PutHandle: "ph", PutExp: "1", PutSig: "ps",
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.RunLeaseLoop(ctx, wantWorkerID, proc, conn) //nolint:errcheck
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(gotWorkerIDs)
		mu.Unlock()
		if n >= 2 { // at least GET + PUT
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	for _, id := range gotWorkerIDs {
		if id != wantWorkerID {
			t.Errorf("X-Worker-Id = %q, want %q", id, wantWorkerID)
		}
	}
}

// TestCopyProcessor_StubCopiesInToOut verifies the Task-15 stub behaviour.
func TestCopyProcessor_StubCopiesInToOut(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.seg")
	outPath := filepath.Join(dir, "out.seg")

	content := []byte("stub-segment-content")
	if err := os.WriteFile(inPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	p := CopyProcessor{}
	stats, err := p.Process(context.Background(), inPath, outPath)
	if err != nil {
		t.Fatalf("CopyProcessor.Process: %v", err)
	}

	got, _ := os.ReadFile(outPath)
	if string(got) != string(content) {
		t.Errorf("output = %q, want %q", got, content)
	}
	if stats.BytesRead != int64(len(content)) {
		t.Errorf("BytesRead = %d, want %d", stats.BytesRead, len(content))
	}
	if stats.BytesWritten != int64(len(content)) {
		t.Errorf("BytesWritten = %d, want %d", stats.BytesWritten, len(content))
	}
}

// TestLeaseLoop_ContextCancellation verifies the loop exits cleanly when ctx
// is cancelled while idle.
func TestLeaseLoop_ContextCancellation(t *testing.T) {
	t.Parallel()

	c, _, _ := buildLeaseClient(t, "")

	conn := &fakeConn{grants: []wire.LeaseGrantPayload{}} // always empty

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- c.RunLeaseLoop(ctx, "w", CopyProcessor{}, conn)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("RunLeaseLoop returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLeaseLoop did not exit after context cancellation")
	}
}

// TestLeaseLoop_SendChannelFull verifies that a full send channel results in
// an error (not a hang).
func TestLeaseLoop_SendChannelFull(t *testing.T) {
	t.Parallel()

	cfg := Config{ServerURL: "http://unused"}
	c := NewClient(cfg)
	// Drain all capacity of the send channel to simulate full.
	for i := 0; i < sendBuf; i++ {
		c.send <- []byte(fmt.Sprintf("filler-%d", i))
	}

	conn := &fakeConn{grants: []wire.LeaseGrantPayload{
		{JobID: "j", Idx: 0},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.RunLeaseLoop(ctx, "w", CopyProcessor{}, conn)
	if err == nil {
		t.Error("expected error when send channel is full")
	}
}
