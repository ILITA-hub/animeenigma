package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/ffmpeg"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/source"
)

// ── Handwritten fakes (orch-prefixed to avoid colliding with leaser_test.go) ───

// orchFakeJobRepo is an in-memory JobRepository. Jobs are keyed by ID; List
// filters by status. All status/meta/prefix mutations are recorded for asserts.
type orchFakeJobRepo struct {
	mu sync.Mutex

	jobs map[string]*domain.UpscaleJob

	statusCalls []orchStatusCall
	sourceMeta  map[string]orchSourceMeta
	outputs     map[string]string

	// updateStatusErrOn, when non-nil, returns an error for the matching status.
	updateStatusErrOn map[domain.JobStatus]error
}

type orchStatusCall struct {
	id      string
	status  domain.JobStatus
	errText string
}

type orchSourceMeta struct {
	codec, pixfmt, fps string
	height             int
	segCount           int
}

func newOrchFakeJobRepo(jobs ...*domain.UpscaleJob) *orchFakeJobRepo {
	m := make(map[string]*domain.UpscaleJob, len(jobs))
	for _, j := range jobs {
		m[j.ID] = j
	}
	return &orchFakeJobRepo{
		jobs:       m,
		sourceMeta: make(map[string]orchSourceMeta),
		outputs:    make(map[string]string),
	}
}

func (f *orchFakeJobRepo) List(_ context.Context, flt repo.JobFilter) ([]domain.UpscaleJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.UpscaleJob
	for _, j := range f.jobs {
		if flt.Status != "" && j.Status != flt.Status {
			continue
		}
		out = append(out, *j)
	}
	return out, nil
}

func (f *orchFakeJobRepo) UpdateStatus(_ context.Context, id string, status domain.JobStatus, errText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateStatusErrOn != nil {
		if err, ok := f.updateStatusErrOn[status]; ok && err != nil {
			return err
		}
	}
	f.statusCalls = append(f.statusCalls, orchStatusCall{id: id, status: status, errText: errText})
	if j, ok := f.jobs[id]; ok {
		j.Status = status
		j.ErrorText = errText
	}
	return nil
}

func (f *orchFakeJobRepo) SetSourceMeta(_ context.Context, id, codec, pixfmt, fps string, height, segCount int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sourceMeta[id] = orchSourceMeta{codec: codec, pixfmt: pixfmt, fps: fps, height: height, segCount: segCount}
	if j, ok := f.jobs[id]; ok {
		j.SourceCodec = codec
		j.SourcePixFmt = pixfmt
		j.SourceFPS = fps
		j.SourceHeight = height
		j.SegmentCount = segCount
	}
	return nil
}

func (f *orchFakeJobRepo) SetOutputPrefix(_ context.Context, id, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs[id] = prefix
	if j, ok := f.jobs[id]; ok {
		j.OutputPrefix = prefix
	}
	return nil
}

// statusCountFor returns how many times a given (id,status) pair was recorded.
func (f *orchFakeJobRepo) statusCountFor(id string, status domain.JobStatus) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.statusCalls {
		if c.id == id && c.status == status {
			n++
		}
	}
	return n
}

func (f *orchFakeJobRepo) lastErrText(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.statusCalls) - 1; i >= 0; i-- {
		if f.statusCalls[i].id == id {
			return f.statusCalls[i].errText
		}
	}
	return ""
}

// orchFakeSegmentRepo records BulkInsertPending and returns scripted Counts.
type orchFakeSegmentRepo struct {
	mu sync.Mutex

	bulkInserts map[string]int
	counts      map[string][3]int // jobID -> {pending, leased, done}
}

func newOrchFakeSegmentRepo() *orchFakeSegmentRepo {
	return &orchFakeSegmentRepo{
		bulkInserts: make(map[string]int),
		counts:      make(map[string][3]int),
	}
}

func (f *orchFakeSegmentRepo) BulkInsertPending(_ context.Context, jobID string, n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bulkInserts[jobID] += n
	return nil
}

func (f *orchFakeSegmentRepo) Counts(_ context.Context, jobID string) (pending, leased, done int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.counts[jobID]
	return c[0], c[1], c[2], nil
}

func (f *orchFakeSegmentRepo) setCounts(jobID string, pending, leased, done int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[jobID] = [3]int{pending, leased, done}
}

// orchFakeResolver returns a scripted source path or error.
type orchFakeResolver struct {
	path string
	err  error
}

func (f *orchFakeResolver) Resolve(_ context.Context, _ *domain.UpscaleJob) (string, error) {
	return f.path, f.err
}

// orchFakeProber returns a scripted probe result / error per path.
type orchFakeProber struct {
	result source.ProbeResult
	err    error
}

func (f *orchFakeProber) Probe(_ context.Context, _ string) (source.ProbeResult, error) {
	return f.result, f.err
}

// orchFakeSegmenter returns n synthetic segment paths and records calls.
type orchFakeSegmenter struct {
	mu sync.Mutex

	segPaths   []string
	segErr     error
	demuxErr   error
	segCalls   int
	demuxCalls int
}

func (f *orchFakeSegmenter) Segment(_ context.Context, _, _ string, _ int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.segCalls++
	if f.segErr != nil {
		return nil, f.segErr
	}
	return f.segPaths, nil
}

func (f *orchFakeSegmenter) DemuxSidecars(_ context.Context, _, _ string) (ffmpeg.Sidecars, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.demuxCalls++
	if f.demuxErr != nil {
		return ffmpeg.Sidecars{}, f.demuxErr
	}
	return ffmpeg.Sidecars{}, nil
}

// orchFakeFinalizer records Concat calls (count + args) and returns a scripted error.
type orchFakeFinalizer struct {
	mu sync.Mutex

	calls   int
	lastOut string
	lastSeg string
	err     error
}

func (f *orchFakeFinalizer) Concat(_ context.Context, upscaledSegDir string, _ ffmpeg.Sidecars, _ source.ProbeResult, out string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastSeg = upscaledSegDir
	f.lastOut = out
	return f.err
}

// orchFakeWriter records EnsureBucket + Upload calls.
type orchFakeWriter struct {
	mu sync.Mutex

	ensureCalls int
	uploadCalls int
	lastPrefix  string
	lastFiles   []string
	ensureErr   error
	uploadErr   error
}

func (f *orchFakeWriter) EnsureBucket(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureCalls++
	return f.ensureErr
}

func (f *orchFakeWriter) Upload(_ context.Context, prefix string, filePaths []string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploadCalls++
	f.lastPrefix = prefix
	f.lastFiles = append([]string(nil), filePaths...)
	if f.uploadErr != nil {
		return 0, f.uploadErr
	}
	var total int64
	for range filePaths {
		total += 100
	}
	return total, nil
}

// ── Helper: build an orchestrator over fakes ──────────────────────────────────

type orchHarness struct {
	o    *Orchestrator
	jobs *orchFakeJobRepo
	segs *orchFakeSegmentRepo
	res  *orchFakeResolver
	prb  *orchFakeProber
	seg  *orchFakeSegmenter
	fin  *orchFakeFinalizer
	wr   *orchFakeWriter
}

func newOrchHarness(t *testing.T, jobs *orchFakeJobRepo) *orchHarness {
	t.Helper()
	segs := newOrchFakeSegmentRepo()
	res := &orchFakeResolver{path: "/staging/job/source.mkv"}
	prb := &orchFakeProber{result: source.ProbeResult{Codec: "hevc", PixFmt: "yuv420p10le", FPS: "24000/1001", Height: 540}}
	seg := &orchFakeSegmenter{}
	fin := &orchFakeFinalizer{}
	wr := &orchFakeWriter{}

	o := NewOrchestrator(OrchestratorDeps{
		Jobs:      jobs,
		Segments:  segs,
		Resolver:  res,
		Prober:    prb,
		Segmenter: seg,
		Finalizer: fin,
		Writer:    wr,
		ListHLS: func(_ string) ([]string, error) {
			return []string{"/staging/job/hls/segment_000.ts", "/staging/job/hls/playlist.m3u8"}, nil
		},
		StagingDir:     "/staging",
		SegmentSeconds: 45,
	})
	return &orchHarness{o: o, jobs: jobs, segs: segs, res: res, prb: prb, seg: seg, fin: fin, wr: wr}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestSegmentJobAdvancesQueuedToUpscaling: a queued job advances to upscaling
// with SegmentCount==n pending segments + source meta set.
func TestSegmentJobAdvancesQueuedToUpscaling(t *testing.T) {
	job := &domain.UpscaleJob{ID: "job-1", ShikimoriID: "555", Episode: 3, Scale: 2, Status: domain.JobQueued}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)
	h.seg.segPaths = []string{"a.mkv", "b.mkv", "c.mkv"} // n = 3

	h.o.tick(context.Background())

	// segmenting then upscaling, in that order.
	if got := jobs.statusCountFor("job-1", domain.JobSegmenting); got != 1 {
		t.Errorf("expected 1 segmenting flip, got %d", got)
	}
	if got := jobs.statusCountFor("job-1", domain.JobUpscaling); got != 1 {
		t.Errorf("expected 1 upscaling flip, got %d", got)
	}
	if got := jobs.statusCountFor("job-1", domain.JobFailed); got != 0 {
		t.Errorf("expected 0 failed flips, got %d", got)
	}
	// source meta persisted with the ACTUAL segment count.
	meta, ok := jobs.sourceMeta["job-1"]
	if !ok {
		t.Fatal("expected source meta to be set")
	}
	if meta.segCount != 3 {
		t.Errorf("expected segCount 3, got %d", meta.segCount)
	}
	if meta.codec != "hevc" || meta.pixfmt != "yuv420p10le" || meta.fps != "24000/1001" {
		t.Errorf("unexpected source meta: %+v", meta)
	}
	// source height (from the probe) is persisted for the finalizer's prefix.
	if meta.height != 540 {
		t.Errorf("expected persisted source height 540, got %d", meta.height)
	}
	// segments seeded.
	if got := h.segs.bulkInserts["job-1"]; got != 3 {
		t.Errorf("expected 3 segments seeded, got %d", got)
	}
	if h.seg.demuxCalls != 1 {
		t.Errorf("expected 1 demux call, got %d", h.seg.demuxCalls)
	}
}

// TestProcessSegmentingReclaimsStuckJob (I2): a job stranded in `segmenting` by
// an OOM/restart mid-segmentation is reclaimed by the processSegmenting sweep —
// segmentJob re-runs idempotently and the job advances to upscaling. Without the
// sweep no code path re-selects a `segmenting` job, so it would livelock forever.
func TestProcessSegmentingReclaimsStuckJob(t *testing.T) {
	// Job left in `segmenting` (the threat-model "restart mid-segmentation" case).
	job := &domain.UpscaleJob{ID: "stuck-1", ShikimoriID: "999", Episode: 7, Scale: 2, Status: domain.JobSegmenting}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)
	h.seg.segPaths = []string{"a.mkv", "b.mkv"} // n = 2

	// A normal tick (which fires processSegmenting first) reclaims it.
	h.o.tick(context.Background())

	// segmentJob re-drove the job: it re-flipped to segmenting then to upscaling.
	if got := jobs.statusCountFor("stuck-1", domain.JobUpscaling); got != 1 {
		t.Errorf("expected the stuck job to advance to upscaling once, got %d", got)
	}
	if got := jobs.statusCountFor("stuck-1", domain.JobFailed); got != 0 {
		t.Errorf("expected 0 failed flips, got %d", got)
	}
	// Idempotent re-segmentation: segments were (re)seeded with the real count.
	if got := h.segs.bulkInserts["stuck-1"]; got != 2 {
		t.Errorf("expected 2 segments seeded on recovery, got %d", got)
	}
	if h.seg.segCalls != 1 {
		t.Errorf("expected the segmenter to run once on recovery, got %d", h.seg.segCalls)
	}
	// Final state is upscaling.
	if j := jobs.jobs["stuck-1"]; j.Status != domain.JobUpscaling {
		t.Errorf("expected final status upscaling, got %q", j.Status)
	}
}

// TestProcessSegmentingNoOpWhenNoneStuck: the sweep is inert when no job is in
// `segmenting` — it must not touch queued/upscaling/done jobs.
func TestProcessSegmentingNoOpWhenNoneStuck(t *testing.T) {
	job := &domain.UpscaleJob{ID: "up-1", ShikimoriID: "1", Episode: 1, Scale: 2, Status: domain.JobUpscaling}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)

	h.o.processSegmenting(context.Background())

	if h.seg.segCalls != 0 {
		t.Errorf("expected segmenter NOT to run, got %d calls", h.seg.segCalls)
	}
	if got := jobs.statusCountFor("up-1", domain.JobSegmenting); got != 0 {
		t.Errorf("expected no segmenting flips, got %d", got)
	}
}

// TestIndependentAllDoneFlip: with NO lease_req, the orchestrator flips an
// upscaling job to finalizing when Counts shows all done (the I-1 liveness fix).
func TestIndependentAllDoneFlip(t *testing.T) {
	job := &domain.UpscaleJob{ID: "job-2", ShikimoriID: "555", Episode: 1, Scale: 2, Status: domain.JobUpscaling}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)

	// All segments done; nothing pending/leased. No worker ever called lease_req.
	h.segs.setCounts("job-2", 0, 0, 5)

	h.o.detectAllDone(context.Background())

	if got := jobs.statusCountFor("job-2", domain.JobFinalizing); got != 1 {
		t.Fatalf("expected independent flip to finalizing exactly once, got %d", got)
	}
}

// TestNoFlipWhileSegmentsOutstanding: a job with pending/leased segments is NOT
// flipped; a job with zero done segments (still segmenting) is NOT flipped.
func TestNoFlipWhileSegmentsOutstanding(t *testing.T) {
	cases := []struct {
		name                  string
		pending, leased, done int
		wantFlip              bool
	}{
		{"pending-remaining", 2, 0, 3, false},
		{"leased-remaining", 0, 1, 4, false},
		{"none-done-yet", 0, 0, 0, false},
		{"all-done", 0, 0, 6, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := &domain.UpscaleJob{ID: "j", ShikimoriID: "1", Episode: 1, Scale: 2, Status: domain.JobUpscaling}
			jobs := newOrchFakeJobRepo(job)
			h := newOrchHarness(t, jobs)
			h.segs.setCounts("j", tc.pending, tc.leased, tc.done)

			h.o.detectAllDone(context.Background())

			got := jobs.statusCountFor("j", domain.JobFinalizing)
			want := 0
			if tc.wantFlip {
				want = 1
			}
			if got != want {
				t.Errorf("flip count = %d, want %d (counts p=%d l=%d d=%d)", got, want, tc.pending, tc.leased, tc.done)
			}
		})
	}
}

// TestFinalizeJobUploadsAndCompletes: a finalizing job runs Concat once, uploads
// to the REAL height-based UpscaledPrefix derived from the persisted SourceHeight
// (540) × Scale (2) = 1080 — NOT the scale-only fallback — sets the output
// prefix, and flips to done. No filesystem re-probe is involved at finalize.
func TestFinalizeJobUploadsAndCompletes(t *testing.T) {
	// Source meta persisted at segment time: 540p source, scale 2 -> 1080p.
	job := &domain.UpscaleJob{
		ID: "job-3", ShikimoriID: "777", Episode: 12, Scale: 2,
		Status: domain.JobFinalizing, SourcePixFmt: "yuv420p10le", SourceHeight: 540,
	}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)

	h.o.processFinalizing(context.Background())

	if h.fin.calls != 1 {
		t.Fatalf("expected Concat called exactly once, got %d", h.fin.calls)
	}
	if h.wr.ensureCalls != 1 {
		t.Errorf("expected EnsureBucket once, got %d", h.wr.ensureCalls)
	}
	if h.wr.uploadCalls != 1 {
		t.Errorf("expected Upload once, got %d", h.wr.uploadCalls)
	}
	// scaleHeight = SourceHeight(540) × Scale(2) = 1080 → the REAL prefix.
	wantPrefix := "aeProvider/777/UPSCALED-1080p/12/"
	if h.wr.lastPrefix != wantPrefix {
		t.Errorf("upload prefix = %q, want %q", h.wr.lastPrefix, wantPrefix)
	}
	if got := jobs.outputs["job-3"]; got != wantPrefix {
		t.Errorf("output prefix recorded = %q, want %q", got, wantPrefix)
	}
	if got := jobs.statusCountFor("job-3", domain.JobDone); got != 1 {
		t.Errorf("expected done flip once, got %d", got)
	}
	if got := jobs.statusCountFor("job-3", domain.JobFailed); got != 0 {
		t.Errorf("expected no failed flip, got %d", got)
	}
}

// TestFinalizeLegacyHeightFallback: a legacy job with SourceHeight==0 (e.g.
// queued before height was persisted) falls back to Scale alone for the prefix
// so the upload stays well-formed rather than crashing.
func TestFinalizeLegacyHeightFallback(t *testing.T) {
	job := &domain.UpscaleJob{
		ID: "legacy", ShikimoriID: "888", Episode: 4, Scale: 2,
		Status: domain.JobFinalizing, SourceHeight: 0,
	}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)

	h.o.processFinalizing(context.Background())

	// SourceHeight==0 → fallback to Scale (2) → UPSCALED-2p.
	wantPrefix := "aeProvider/888/UPSCALED-2p/4/"
	if h.wr.lastPrefix != wantPrefix {
		t.Errorf("legacy fallback prefix = %q, want %q", h.wr.lastPrefix, wantPrefix)
	}
	if got := jobs.statusCountFor("legacy", domain.JobDone); got != 1 {
		t.Errorf("expected done flip once, got %d", got)
	}
}

// TestFinalizeRunsExactlyOnce: after a finalizing job completes (→done), a
// subsequent tick does NOT re-run Concat/Upload (idempotent — driven by status).
func TestFinalizeRunsExactlyOnce(t *testing.T) {
	job := &domain.UpscaleJob{ID: "job-4", ShikimoriID: "9", Episode: 1, Scale: 2, Status: domain.JobFinalizing}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)

	h.o.tick(context.Background()) // finalizes -> done
	h.o.tick(context.Background()) // job is now done; must NOT finalize again

	if h.fin.calls != 1 {
		t.Errorf("expected Concat exactly once across two ticks, got %d", h.fin.calls)
	}
	if h.wr.uploadCalls != 1 {
		t.Errorf("expected Upload exactly once across two ticks, got %d", h.wr.uploadCalls)
	}
}

// TestResolveFailureMarksFailedAndKeepsPolling: a resolve error flips the job to
// failed (with errText) and the poller continues processing other jobs.
func TestResolveFailureMarksFailedAndKeepsPolling(t *testing.T) {
	bad := &domain.UpscaleJob{ID: "bad", ShikimoriID: "1", Episode: 1, Scale: 2, Status: domain.JobQueued}
	jobs := newOrchFakeJobRepo(bad)
	h := newOrchHarness(t, jobs)
	h.res.err = source.ErrSourceGone

	h.o.tick(context.Background())

	if got := jobs.statusCountFor("bad", domain.JobFailed); got != 1 {
		t.Fatalf("expected job flipped to failed once, got %d", got)
	}
	if et := jobs.lastErrText("bad"); et == "" {
		t.Error("expected non-empty error text on failed job")
	}
	if got := jobs.statusCountFor("bad", domain.JobUpscaling); got != 0 {
		t.Errorf("job must not reach upscaling on resolve failure, got %d", got)
	}

	// Poller still runs: a second tick over a fresh good job advances it.
	good := &domain.UpscaleJob{ID: "good", ShikimoriID: "2", Episode: 1, Scale: 2, Status: domain.JobQueued}
	jobs.mu.Lock()
	jobs.jobs["good"] = good
	jobs.mu.Unlock()
	h.res.err = nil
	h.seg.segPaths = []string{"x.mkv"}

	h.o.tick(context.Background())
	if got := jobs.statusCountFor("good", domain.JobUpscaling); got != 1 {
		t.Errorf("poller did not keep running: good job not advanced (got %d)", got)
	}
}

// TestProbeFailureMarksFailed: a probe error flips the job to failed and never
// segments.
func TestProbeFailureMarksFailed(t *testing.T) {
	job := &domain.UpscaleJob{ID: "p", ShikimoriID: "1", Episode: 1, Scale: 2, Status: domain.JobQueued}
	jobs := newOrchFakeJobRepo(job)
	h := newOrchHarness(t, jobs)
	h.prb.err = errors.New("ffprobe: no video stream")

	h.o.tick(context.Background())

	if got := jobs.statusCountFor("p", domain.JobFailed); got != 1 {
		t.Fatalf("expected failed once, got %d", got)
	}
	if h.seg.segCalls != 0 {
		t.Errorf("segmenter must not run after probe failure, got %d calls", h.seg.segCalls)
	}
}

// TestStopHaltsRun verifies Stop() returns Run() promptly (no ctx needed).
func TestStopHaltsRun(t *testing.T) {
	jobs := newOrchFakeJobRepo()
	h := newOrchHarness(t, jobs)
	done := make(chan struct{})
	go func() {
		h.o.Run(context.Background())
		close(done)
	}()
	h.o.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}

// TestCtxCancelHaltsRun verifies ctx cancellation returns Run() promptly.
func TestCtxCancelHaltsRun(t *testing.T) {
	jobs := newOrchFakeJobRepo()
	h := newOrchHarness(t, jobs)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.o.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}
