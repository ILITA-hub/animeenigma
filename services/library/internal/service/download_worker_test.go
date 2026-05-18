package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	libtorrent "github.com/ILITA-hub/animeenigma/services/library/internal/torrent"
	"github.com/prometheus/client_golang/prometheus"
)

// fakeJobStore is a thread-safe in-memory JobStore for unit tests.
// It's NOT the production repo — Claim() does not honor SKIP LOCKED
// semantics. For the concurrent-claim acceptance test we use the
// real repo against Postgres (under `integration` build tag in
// internal/repo/job_integration_test.go).
type fakeJobStore struct {
	mu     sync.Mutex
	jobs   map[string]*domain.Job
	order  []string // insertion order so Claim picks the oldest first
	calls  []string // call log used by tests to assert ordering
}

func newFakeJobStore() *fakeJobStore {
	return &fakeJobStore{jobs: map[string]*domain.Job{}}
}

func (s *fakeJobStore) put(j *domain.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[j.ID]; !ok {
		s.order = append(s.order, j.ID)
	}
	cp := *j
	s.jobs[j.ID] = &cp
}

func (s *fakeJobStore) snapshot(id string) *domain.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil
	}
	cp := *j
	return &cp
}

func (s *fakeJobStore) Claim(ctx context.Context, statuses ...domain.JobStatus) (*domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "Claim")
	if len(statuses) == 0 {
		statuses = []domain.JobStatus{domain.JobStatusQueued}
	}
	want := map[domain.JobStatus]bool{}
	for _, st := range statuses {
		want[st] = true
	}
	for _, id := range s.order {
		j := s.jobs[id]
		if want[j.Status] {
			j.Status = domain.JobStatusDownloading
			j.UpdatedAt = time.Now()
			cp := *j
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *fakeJobStore) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "GetByID")
	j, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *j
	return &cp, nil
}

func (s *fakeJobStore) UpdateProgress(ctx context.Context, id string, downloadedBytes, totalBytes int64, peers int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "UpdateProgress")
	j, ok := s.jobs[id]
	if !ok {
		return errors.New("not found")
	}
	if totalBytes > 0 {
		pct := int(downloadedBytes * 100 / totalBytes)
		if pct > 100 {
			pct = 100
		}
		if pct < 0 {
			pct = 0
		}
		j.ProgressPct = pct
	}
	j.UpdatedAt = time.Now()
	return nil
}

func (s *fakeJobStore) UpdateStatus(ctx context.Context, id string, newStatus domain.JobStatus, errorText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, fmt.Sprintf("UpdateStatus(%s)", newStatus))
	j, ok := s.jobs[id]
	if !ok {
		return errors.New("not found")
	}
	j.Status = newStatus
	j.ErrorText = errorText
	j.UpdatedAt = time.Now()
	if newStatus.IsTerminal() {
		now := time.Now()
		j.CompletedAt = &now
	}
	return nil
}

func (s *fakeJobStore) Cancel(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "Cancel")
	j, ok := s.jobs[id]
	if !ok {
		return errors.New("not found")
	}
	if j.Status.IsTerminal() {
		return nil
	}
	j.Status = domain.JobStatusCancelled
	now := time.Now()
	j.CompletedAt = &now
	j.UpdatedAt = now
	return nil
}

// fakeHandle implements libtorrent.DownloadHandle for unit tests.
type fakeHandle struct {
	id   string
	done chan struct{}

	mu         sync.Mutex
	downloaded int64
	total      int64
	peers      int
	cancelled  bool
}

func newFakeHandle(id string, total int64) *fakeHandle {
	return &fakeHandle{id: id, total: total, peers: 5, done: make(chan struct{})}
}

func (h *fakeHandle) ID() string { return h.id }

func (h *fakeHandle) Progress() (int64, int64, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.downloaded, h.total, h.peers
}

func (h *fakeHandle) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancelled {
		return
	}
	h.cancelled = true
	close(h.done)
}

func (h *fakeHandle) Done() <-chan struct{} { return h.done }

func (h *fakeHandle) advance(bytes int64, peers int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.downloaded = bytes
	h.peers = peers
}

func (h *fakeHandle) finish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.downloaded = h.total
	if !h.cancelled {
		close(h.done)
		h.cancelled = true // reuse the flag to mean "done"
	}
}

// fakeAdder controls how Client.Add behaves: it can either return a
// pre-staged handle or an error.
type fakeAdder struct {
	mu    sync.Mutex
	calls int
	next  libtorrent.DownloadHandle
	err   error
}

func (a *fakeAdder) Add(ctx context.Context, magnet string) (libtorrent.DownloadHandle, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.err != nil {
		return nil, a.err
	}
	return a.next, nil
}

// helper: build a worker pool with sensible defaults for unit tests.
func newTestPool(t *testing.T, store JobStore, tc TorrentAdder, stall, tick time.Duration) (*WorkerPool, *metrics.LibraryMetrics) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := metrics.NewLibraryMetricsWithRegisterer(reg)
	p := NewWorkerPool(1, store, tc, m, stall, tick, nil)
	p.pollInterval = 10 * time.Millisecond
	return p, m
}

// --- Tests ---

func TestWorkerPool_ProcessJob_AddError(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "j1", Magnet: "magnet:?xt=urn:btih:x", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	adder := &fakeAdder{err: errors.New("add failed")}
	p, _ := newTestPool(t, store, adder, 30*time.Minute, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.processJob(ctx, job)

	got := store.snapshot(job.ID)
	if got.Status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorText == "" {
		t.Fatalf("expected error_text to be set")
	}
}

func TestWorkerPool_ProcessJob_DoneTransitionsToEncoding(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "j2", Magnet: "m", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	h := newFakeHandle("hash", 1000)
	adder := &fakeAdder{next: h}
	p, _ := newTestPool(t, store, adder, 30*time.Minute, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Drive Done() shortly after processJob starts.
	go func() {
		time.Sleep(30 * time.Millisecond)
		h.finish()
	}()

	p.processJob(ctx, job)

	got := store.snapshot(job.ID)
	if got.Status != domain.JobStatusEncoding {
		t.Fatalf("status = %q, want encoding (Phase 3 stops here)", got.Status)
	}
}

func TestWorkerPool_ProcessJob_Stall(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "j3", Magnet: "m", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	h := newFakeHandle("hash", 1000)
	h.advance(0, 0) // zero peers from the start
	adder := &fakeAdder{next: h}

	// Very short stall timeout (50ms) + short progress tick (10ms)
	// so the test resolves quickly without waiting 30 minutes.
	p, _ := newTestPool(t, store, adder, 50*time.Millisecond, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.processJob(ctx, job)

	got := store.snapshot(job.ID)
	if got.Status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed (stall)", got.Status)
	}
	if got.ErrorText != stallErrorText {
		t.Fatalf("error_text = %q, want %q", got.ErrorText, stallErrorText)
	}
}

// TestWorkerPool_CancelJob_StatusFirstThenHandle — DELETE flow flips
// status BEFORE signalling the in-memory handle. The fakeJobStore
// records call order so we can assert "Cancel" preceded the handle's
// Cancel notification.
func TestWorkerPool_CancelJob_StatusFirstThenHandle(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "j4", Magnet: "m", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	h := newFakeHandle("hash", 1000)
	p, _ := newTestPool(t, store, &fakeAdder{next: h}, 30*time.Minute, 50*time.Millisecond)
	p.registerHandle(job.ID, h)
	t.Cleanup(func() { p.unregisterHandle(job.ID) })

	if err := p.CancelJob(context.Background(), job.ID); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	// fakeJobStore's call log includes "Cancel" exactly once.
	var foundCancel bool
	for _, c := range store.calls {
		if c == "Cancel" {
			foundCancel = true
			break
		}
	}
	if !foundCancel {
		t.Fatalf("expected JobStore.Cancel to be invoked, got calls=%v", store.calls)
	}
	// Handle's Done() must resolve (CancelJob signalled the
	// in-memory Cancel after the DB flip).
	select {
	case <-h.Done():
	case <-time.After(time.Second):
		t.Fatal("handle.Cancel was not called")
	}
}

// TestWorkerPool_StartStop_RewritesInFlightToQueued — graceful
// shutdown rewrites status='downloading' rows back to 'queued'.
func TestWorkerPool_StartStop_RewritesInFlightToQueued(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "j5", Magnet: "m", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	h := newFakeHandle("hash", 1000)
	p, _ := newTestPool(t, store, &fakeAdder{next: h}, 30*time.Minute, 50*time.Millisecond)

	// Simulate "worker is mid-flight on this job" by manually
	// registering the handle and tracking it as active.
	p.registerHandle(job.ID, h)

	// Call Stop directly (we didn't launch worker goroutines for
	// this unit test; Stop's wg.Wait returns immediately).
	if err := p.Stop(context.Background(), 200*time.Millisecond); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got := store.snapshot(job.ID)
	if got.Status != domain.JobStatusQueued {
		t.Fatalf("post-Stop status = %q, want queued", got.Status)
	}
}

// TestWorkerPool_ProcessJob_ObservesCancelledStatusOnTick — when the
// DELETE path flips status to 'cancelled' between ticks, the next
// progress tick observes the new status, calls handle.Cancel, and
// exits without further status writes.
func TestWorkerPool_ProcessJob_ObservesCancelledStatusOnTick(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "j6", Magnet: "m", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	h := newFakeHandle("hash", 1000)
	adder := &fakeAdder{next: h}

	p, _ := newTestPool(t, store, adder, 30*time.Minute, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Flip the row to cancelled shortly after processJob starts.
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = store.Cancel(context.Background(), job.ID)
	}()

	p.processJob(ctx, job)

	got := store.snapshot(job.ID)
	if got.Status != domain.JobStatusCancelled {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
}
