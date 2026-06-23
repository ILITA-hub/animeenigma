package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

// ── Handwritten fakes ─────────────────────────────────────────────────────────

type fakeJobRepo struct {
	mu      sync.Mutex
	jobs    []*domain.UpscaleJob
	updates []struct {
		id     string
		status domain.JobStatus
		errTxt string
	}
}

func (f *fakeJobRepo) NextEligible(_ context.Context) (*domain.UpscaleJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.jobs) == 0 {
		return nil, nil
	}
	return f.jobs[0], nil
}

func (f *fakeJobRepo) UpdateStatus(_ context.Context, id string, status domain.JobStatus, errText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, struct {
		id     string
		status domain.JobStatus
		errTxt string
	}{id, status, errText})
	// Also update the in-memory job status.
	for _, j := range f.jobs {
		if j.ID == id {
			j.Status = status
		}
	}
	return nil
}

func (f *fakeJobRepo) lastUpdate() (id string, status domain.JobStatus, found bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) == 0 {
		return "", "", false
	}
	u := f.updates[len(f.updates)-1]
	return u.id, u.status, true
}

// fakeSegmentRepo dispenses segments round-robin from a pre-seeded list.
// When all segments have been leased it returns (nil, nil), simulating
// SegmentRepository.LeaseNext when the job is exhausted.
type fakeSegmentRepo struct {
	mu   sync.Mutex
	segs []*domain.UpscaleSegment
	next int

	counts map[string]struct{ pending, leased, done int }
}

func (f *fakeSegmentRepo) LeaseNext(_ context.Context, jobID, workerID string, ttl time.Duration) (*domain.UpscaleSegment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next >= len(f.segs) {
		return nil, nil
	}
	seg := f.segs[f.next]
	f.next++
	return seg, nil
}

func (f *fakeSegmentRepo) Counts(_ context.Context, jobID string) (pending, leased, done int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.counts[jobID]
	return c.pending, c.leased, c.done, nil
}

type fakeWorkerRepo struct {
	mu   sync.Mutex
	hbs  []struct{ workerID, jobID string; seg int; at time.Time }
}

func (f *fakeWorkerRepo) Heartbeat(_ context.Context, workerID, jobID string, seg int, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hbs = append(f.hbs, struct {
		workerID, jobID string
		seg             int
		at              time.Time
	}{workerID, jobID, seg, now})
	return nil
}

func (f *fakeWorkerRepo) lastHeartbeat() (workerID, jobID string, seg int, found bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.hbs) == 0 {
		return "", "", 0, false
	}
	h := f.hbs[len(f.hbs)-1]
	return h.workerID, h.jobID, h.seg, true
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestLeaser_FirstSegment verifies that the first OnLeaseReq returns idx=0
// with valid get+put capability handles.
func TestLeaser_FirstSegment(t *testing.T) {
	jobID := "job-lease-001"
	job := &domain.UpscaleJob{ID: jobID, Status: domain.JobUpscaling}

	jobs := &fakeJobRepo{jobs: []*domain.UpscaleJob{job}}
	segs := &fakeSegmentRepo{
		segs: []*domain.UpscaleSegment{
			{JobID: jobID, Idx: 0, Status: domain.SegLeased},
			{JobID: jobID, Idx: 1, Status: domain.SegLeased},
		},
		counts: map[string]struct{ pending, leased, done int }{
			jobID: {pending: 1, leased: 1, done: 0},
		},
	}
	workers := &fakeWorkerRepo{}

	leaser := NewLeaser(jobs, segs, workers)
	ctx := context.Background()

	seg, handles, err := leaser.OnLeaseReq(ctx, "worker-A")
	if err != nil {
		t.Fatalf("OnLeaseReq: %v", err)
	}
	if seg == nil {
		t.Fatal("OnLeaseReq: expected a segment, got nil")
	}
	if seg.Idx != 0 {
		t.Errorf("seg.Idx = %d, want 0", seg.Idx)
	}

	// Verify the GET handle for this segment.
	if !capability.VerifyJobHandle(jobID, "segment-get", 0, handles.GetExp, handles.GetSig, time.Now()) {
		t.Error("GET handle verify failed for idx=0")
	}
	// Verify the PUT handle for this segment.
	if !capability.VerifyJobHandle(jobID, "segment-put", 0, handles.PutExp, handles.PutSig, time.Now()) {
		t.Error("PUT handle verify failed for idx=0")
	}

	// Handles for a different idx must NOT verify.
	if capability.VerifyJobHandle(jobID, "segment-get", 1, handles.GetExp, handles.GetSig, time.Now()) {
		t.Error("GET handle for idx=1 unexpectedly verified with idx=0 handles")
	}

	// Worker heartbeat was recorded.
	wID, jID, segIdx, found := workers.lastHeartbeat()
	if !found {
		t.Fatal("no heartbeat recorded after OnLeaseReq")
	}
	if wID != "worker-A" || jID != jobID || segIdx != 0 {
		t.Errorf("heartbeat = (worker=%q job=%q seg=%d), want (worker-A, %q, 0)", wID, jID, segIdx, jobID)
	}
}

// TestLeaser_SecondSegment verifies the second OnLeaseReq returns idx=1.
func TestLeaser_SecondSegment(t *testing.T) {
	jobID := "job-lease-002"
	job := &domain.UpscaleJob{ID: jobID, Status: domain.JobUpscaling}

	jobs := &fakeJobRepo{jobs: []*domain.UpscaleJob{job}}
	segs := &fakeSegmentRepo{
		segs: []*domain.UpscaleSegment{
			{JobID: jobID, Idx: 0, Status: domain.SegLeased},
			{JobID: jobID, Idx: 1, Status: domain.SegLeased},
		},
		counts: map[string]struct{ pending, leased, done int }{
			jobID: {pending: 1, leased: 1, done: 0},
		},
	}
	workers := &fakeWorkerRepo{}

	leaser := NewLeaser(jobs, segs, workers)
	ctx := context.Background()

	// First call → idx=0.
	if _, _, err := leaser.OnLeaseReq(ctx, "worker-A"); err != nil {
		t.Fatalf("first OnLeaseReq: %v", err)
	}
	// Second call → idx=1.
	seg, handles, err := leaser.OnLeaseReq(ctx, "worker-B")
	if err != nil {
		t.Fatalf("second OnLeaseReq: %v", err)
	}
	if seg == nil || seg.Idx != 1 {
		t.Fatalf("second OnLeaseReq: seg.Idx = %v, want 1", seg)
	}
	if !capability.VerifyJobHandle(jobID, "segment-get", 1, handles.GetExp, handles.GetSig, time.Now()) {
		t.Error("GET handle verify failed for idx=1")
	}
}

// TestLeaser_AllDoneFlipsJobToFinalizing verifies that when LeaseNext returns
// nil (no available segment) AND Counts shows pending=0,leased=0,done>0, the
// job status is flipped to JobFinalizing.
func TestLeaser_AllDoneFlipsJobToFinalizing(t *testing.T) {
	jobID := "job-lease-003"
	job := &domain.UpscaleJob{ID: jobID, Status: domain.JobUpscaling}

	jobs := &fakeJobRepo{jobs: []*domain.UpscaleJob{job}}
	segs := &fakeSegmentRepo{
		segs: []*domain.UpscaleSegment{}, // no segments to lease
		counts: map[string]struct{ pending, leased, done int }{
			jobID: {pending: 0, leased: 0, done: 3},
		},
	}
	workers := &fakeWorkerRepo{}

	leaser := NewLeaser(jobs, segs, workers)
	ctx := context.Background()

	seg, _, err := leaser.OnLeaseReq(ctx, "worker-C")
	if err != nil {
		t.Fatalf("OnLeaseReq: %v", err)
	}
	if seg != nil {
		t.Errorf("expected nil segment (all done), got %+v", seg)
	}

	// Job must have been flipped to finalizing.
	jID, status, found := jobs.lastUpdate()
	if !found {
		t.Fatal("expected UpdateStatus call for all-done job, got none")
	}
	if jID != jobID {
		t.Errorf("UpdateStatus job = %q, want %q", jID, jobID)
	}
	if status != domain.JobFinalizing {
		t.Errorf("UpdateStatus status = %q, want %q", status, domain.JobFinalizing)
	}
}

// TestLeaser_NoJobReturnsNil verifies that when NextEligible returns nil,
// OnLeaseReq returns (nil, zero, nil).
func TestLeaser_NoJobReturnsNil(t *testing.T) {
	jobs := &fakeJobRepo{} // no jobs
	segs := &fakeSegmentRepo{}
	workers := &fakeWorkerRepo{}

	leaser := NewLeaser(jobs, segs, workers)
	ctx := context.Background()

	seg, handles, err := leaser.OnLeaseReq(ctx, "worker-D")
	if err != nil {
		t.Fatalf("OnLeaseReq: %v", err)
	}
	if seg != nil {
		t.Errorf("expected nil segment when no jobs, got %+v", seg)
	}
	if handles != (LeasedHandles{}) {
		t.Errorf("expected zero handles when no jobs, got %+v", handles)
	}
}
