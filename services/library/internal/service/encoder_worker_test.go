package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
)

// ---- Stubs ----

// stubJobStore is a minimal in-memory JobStore mirror.
type stubJobStore struct {
	mu             sync.Mutex
	rows           map[string]*domain.Job
	statusHistory  []statusEvent
	getByIDHook    func(id string) (*domain.Job, error)
	claimQueue     []*domain.Job
	claimErr       error
	updateErr      error
}

type statusEvent struct {
	id     string
	status domain.JobStatus
	errTxt string
}

func newStubJobStore() *stubJobStore {
	return &stubJobStore{rows: make(map[string]*domain.Job)}
}

func (s *stubJobStore) addPending(job *domain.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[job.ID] = job
	s.claimQueue = append(s.claimQueue, job)
}

func (s *stubJobStore) Claim(_ context.Context, _ ...domain.JobStatus) (*domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	if len(s.claimQueue) == 0 {
		return nil, nil
	}
	job := s.claimQueue[0]
	s.claimQueue = s.claimQueue[1:]
	// Mirror real Claim behavior: flip to 'downloading' transiently.
	if r, ok := s.rows[job.ID]; ok {
		r.Status = domain.JobStatusDownloading
	}
	out := *job
	out.Status = domain.JobStatusDownloading
	return &out, nil
}

func (s *stubJobStore) GetByID(_ context.Context, id string) (*domain.Job, error) {
	if s.getByIDHook != nil {
		return s.getByIDHook(id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok {
		return nil, errors.New("not found")
	}
	out := *r
	return &out, nil
}

func (s *stubJobStore) UpdateStatus(_ context.Context, id string, status domain.JobStatus, errText string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusHistory = append(s.statusHistory, statusEvent{id: id, status: status, errTxt: errText})
	if r, ok := s.rows[id]; ok {
		r.Status = status
		r.ErrorText = errText
	}
	return nil
}

// stubEpisodeStore records every Create call.
type stubEpisodeStore struct {
	mu       sync.Mutex
	created  []domain.Episode
	createErr error
}

func (s *stubEpisodeStore) Create(_ context.Context, ep *domain.Episode) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created = append(s.created, *ep)
	return nil
}

// stubTranscoder returns a canned Result or error.
type stubTranscoder struct {
	result *ffmpeg.Result
	err    error
	calls  int
}

func (s *stubTranscoder) Transcode(_ context.Context, source string) (*ffmpeg.Result, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

// stubUploader records every Upload call.
type stubUploader struct {
	mu       sync.Mutex
	uploads  []uploadCall
	err      error
	bytes    int64
}

type uploadCall struct {
	prefix string
	files  []string
}

func (s *stubUploader) Upload(_ context.Context, prefix string, filePaths []string) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads = append(s.uploads, uploadCall{prefix: prefix, files: append([]string(nil), filePaths...)})
	if s.bytes == 0 {
		return 1234, nil
	}
	return s.bytes, nil
}

func (s *stubUploader) URLFor(path string) string { return "http://stub/" + path }

// stubDetector returns canned (n, ok).
type stubDetector struct {
	ep int
	ok bool
}

func (s *stubDetector) DetectEpisode(_, _ string) (int, bool) {
	return s.ep, s.ok
}

// stubResolver returns canned path or error.
type stubResolver struct {
	path string
	err  error
}

func (s *stubResolver) Resolve(_ context.Context, _ *domain.Job, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.path, nil
}

// stubMetrics records every method call.
type stubMetrics struct {
	mu              sync.Mutex
	jobsTotal       []string
	encodeDur       []float64
	uploadBytes     int64
	encodeFailures  []string
}

func (s *stubMetrics) IncJobsTotal(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobsTotal = append(s.jobsTotal, status)
}

func (s *stubMetrics) ObserveEncodeDuration(sec float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.encodeDur = append(s.encodeDur, sec)
}

func (s *stubMetrics) AddUploadBytes(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploadBytes += n
}

func (s *stubMetrics) IncEncodeFailures(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.encodeFailures = append(s.encodeFailures, reason)
}

// stubInvalidator records every Invalidate call so Phase-06 tests
// can assert webhook fire behavior.
type stubInvalidator struct {
	mu    sync.Mutex
	calls []string // shikimoriID per call
}

func (s *stubInvalidator) Invalidate(_ context.Context, shikimoriID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, shikimoriID)
}

// ---- Tests ----

// validMagnet is a magnet URI shape anacrolix's parser accepts.
const validMagnet = "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=t"

func newHappyPool(t *testing.T, job *domain.Job) (*EncoderPool, *stubJobStore, *stubEpisodeStore, *stubUploader, *stubMetrics) {
	t.Helper()
	dir := t.TempDir()
	plPath := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(plPath, []byte("#EXTM3U"), 0o644); err != nil {
		t.Fatal(err)
	}
	segPath := filepath.Join(dir, "segment_001.ts")
	if err := os.WriteFile(segPath, []byte("AAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	js := newStubJobStore()
	js.addPending(job)
	es := &stubEpisodeStore{}
	tr := &stubTranscoder{result: &ffmpeg.Result{
		PlaylistPath: plPath,
		SegmentPaths: []string{segPath},
		DurationSec:  1450,
		SizeBytes:    1024,
	}}
	up := &stubUploader{bytes: 9999}
	det := &stubDetector{ep: 1, ok: true}
	res := &stubResolver{path: filepath.Join(dir, "src.mp4")}
	mt := &stubMetrics{}

	pool := NewEncoderPool(1, js, es, tr, up, det, res, mt, nil, nil)
	return pool, js, es, up, mt
}

func TestEncoder_HappyPath_WithShikimoriID(t *testing.T) {
	job := &domain.Job{
		ID:          "job-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
		Status:      domain.JobStatusEncoding,
	}
	pool, js, es, up, mt := newHappyPool(t, job)

	pool.processJob(context.Background(), &domain.Job{
		ID:          "job-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
	})
	_ = pool

	// Status history must end with done.
	if len(js.statusHistory) == 0 || js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status history did not end at done: %+v", js.statusHistory)
	}
	// Episode row inserted exactly once.
	if len(es.created) != 1 {
		t.Fatalf("episodes created = %d, want 1", len(es.created))
	}
	if es.created[0].ShikimoriID != "123" || es.created[0].EpisodeNumber != 1 {
		t.Fatalf("episode = %+v", es.created[0])
	}
	if es.created[0].MinioPath != "aeProvider/123/RAW/1/" {
		t.Fatalf("MinioPath = %q, want aeProvider/123/RAW/1/", es.created[0].MinioPath)
	}
	// Upload called with the unified aeProvider/<mal>/RAW/<ep>/ prefix and 2 files.
	if len(up.uploads) != 1 || up.uploads[0].prefix != "aeProvider/123/RAW/1/" {
		t.Fatalf("upload calls = %+v", up.uploads)
	}
	if len(up.uploads[0].files) != 2 {
		t.Fatalf("upload files = %d, want 2", len(up.uploads[0].files))
	}
	// Metrics: encode duration observed; upload bytes recorded.
	if len(mt.encodeDur) != 1 {
		t.Fatalf("encodeDur calls = %d, want 1", len(mt.encodeDur))
	}
	if mt.uploadBytes != 9999 {
		t.Fatalf("uploadBytes = %d, want 9999", mt.uploadBytes)
	}
}

func TestEncoder_HappyPath_NoShikimoriID(t *testing.T) {
	job := &domain.Job{
		ID:       "job-2",
		Magnet:   validMagnet,
		Uploader: "Ohys-Raws",
		Status:   domain.JobStatusEncoding,
	}
	pool, js, es, up, _ := newHappyPool(t, job)
	pool.processJob(context.Background(), job)
	_ = pool

	// No episode row inserted.
	if len(es.created) != 0 {
		t.Fatalf("episodes created = %d, want 0 when shikimori_id empty", len(es.created))
	}
	// Status ends at done.
	if js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status did not reach done: %+v", js.statusHistory)
	}
	// Upload prefix is pending/{job_id}/{ep}/.
	want := fmt.Sprintf("pending/%s/1/", job.ID)
	if len(up.uploads) != 1 || up.uploads[0].prefix != want {
		t.Fatalf("upload prefix = %q, want %q", up.uploads[0].prefix, want)
	}
}

func TestEncoder_TranscodeFailure(t *testing.T) {
	job := &domain.Job{
		ID: "j-tx", Magnet: validMagnet, Uploader: "Ohys-Raws", ShikimoriID: "1", Status: domain.JobStatusEncoding,
	}
	dir := t.TempDir()
	js := newStubJobStore()
	js.addPending(job)
	tr := &stubTranscoder{err: errors.New("simulated ffmpeg explosion")}
	up := &stubUploader{}
	mt := &stubMetrics{}
	det := &stubDetector{ep: 1, ok: true}
	res := &stubResolver{path: filepath.Join(dir, "src.mp4")}
	pool := NewEncoderPool(1, js, &stubEpisodeStore{}, tr, up, det, res, mt, nil, nil)
	pool.processJob(context.Background(), job)

	// Status = failed with error text containing the simulated message.
	last := js.statusHistory[len(js.statusHistory)-1]
	if last.status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed", last.status)
	}
	if last.errTxt == "" {
		t.Fatalf("error_text empty; want ffmpeg error text")
	}
	if len(up.uploads) != 0 {
		t.Fatalf("Upload must not be called on transcode failure")
	}
	foundReason := false
	for _, r := range mt.encodeFailures {
		if r == "ffmpeg_error" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("expected IncEncodeFailures(\"ffmpeg_error\"); got %v", mt.encodeFailures)
	}
}

func TestEncoder_UploadFailure(t *testing.T) {
	job := &domain.Job{
		ID: "j-up", Magnet: validMagnet, Uploader: "Ohys-Raws", ShikimoriID: "5", Status: domain.JobStatusEncoding,
	}
	dir := t.TempDir()
	plPath := filepath.Join(dir, "playlist.m3u8")
	_ = os.WriteFile(plPath, []byte("p"), 0o644)
	js := newStubJobStore()
	js.addPending(job)
	tr := &stubTranscoder{result: &ffmpeg.Result{PlaylistPath: plPath, SegmentPaths: nil}}
	up := &stubUploader{err: errors.New("minio is angry")}
	es := &stubEpisodeStore{}
	mt := &stubMetrics{}
	pool := NewEncoderPool(1, js, es, tr, up, &stubDetector{ep: 1, ok: true}, &stubResolver{path: filepath.Join(dir, "s.mp4")}, mt, nil, nil)
	pool.processJob(context.Background(), job)

	last := js.statusHistory[len(js.statusHistory)-1]
	if last.status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed", last.status)
	}
	if len(es.created) != 0 {
		t.Fatalf("episodes Created must be empty on upload failure; got %d", len(es.created))
	}
	foundReason := false
	for _, r := range mt.encodeFailures {
		if r == "upload_error" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("expected upload_error reason; got %v", mt.encodeFailures)
	}
}

func TestEncoder_EpisodeDetectFailure(t *testing.T) {
	job := &domain.Job{
		ID: "j-det", Magnet: validMagnet, Uploader: "Unknown", Status: domain.JobStatusEncoding,
	}
	dir := t.TempDir()
	js := newStubJobStore()
	js.addPending(job)
	tr := &stubTranscoder{result: &ffmpeg.Result{}}
	up := &stubUploader{}
	det := &stubDetector{ep: 0, ok: false}
	res := &stubResolver{path: filepath.Join(dir, "weird.mp4")}
	mt := &stubMetrics{}
	pool := NewEncoderPool(1, js, &stubEpisodeStore{}, tr, up, det, res, mt, nil, nil)
	pool.processJob(context.Background(), job)

	last := js.statusHistory[len(js.statusHistory)-1]
	if last.status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed", last.status)
	}
	if tr.calls != 0 {
		t.Fatalf("Transcoder must NOT be invoked when episode detect fails; got %d calls", tr.calls)
	}
	foundReason := false
	for _, r := range mt.encodeFailures {
		if r == "episode_detect_failed" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("expected episode_detect_failed reason; got %v", mt.encodeFailures)
	}
}

func TestEncoder_SourceMissing(t *testing.T) {
	job := &domain.Job{
		ID: "j-src", Magnet: validMagnet, Uploader: "Ohys-Raws", ShikimoriID: "1", Status: domain.JobStatusEncoding,
	}
	js := newStubJobStore()
	js.addPending(job)
	res := &stubResolver{err: errors.New("no video file found")}
	mt := &stubMetrics{}
	pool := NewEncoderPool(1, js, &stubEpisodeStore{}, &stubTranscoder{}, &stubUploader{}, &stubDetector{}, res, mt, nil, nil)
	pool.processJob(context.Background(), job)

	last := js.statusHistory[len(js.statusHistory)-1]
	if last.status != domain.JobStatusFailed {
		t.Fatalf("status = %q, want failed", last.status)
	}
	foundReason := false
	for _, r := range mt.encodeFailures {
		if r == "source_missing" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("expected source_missing reason; got %v", mt.encodeFailures)
	}
}

func TestEncoder_CancelledMidFlight_DoesNotWriteDone(t *testing.T) {
	job := &domain.Job{
		ID: "j-cx", Magnet: validMagnet, Uploader: "Ohys-Raws", ShikimoriID: "9", Status: domain.JobStatusEncoding,
	}
	dir := t.TempDir()
	plPath := filepath.Join(dir, "playlist.m3u8")
	_ = os.WriteFile(plPath, []byte("p"), 0o644)
	js := newStubJobStore()
	js.addPending(job)
	// After the post-Transcode check, return the row with status=cancelled.
	js.getByIDHook = func(id string) (*domain.Job, error) {
		return &domain.Job{ID: id, Status: domain.JobStatusCancelled}, nil
	}
	tr := &stubTranscoder{result: &ffmpeg.Result{PlaylistPath: plPath}}
	up := &stubUploader{}
	pool := NewEncoderPool(1, js, &stubEpisodeStore{}, tr, up, &stubDetector{ep: 1, ok: true}, &stubResolver{path: filepath.Join(dir, "s.mp4")}, &stubMetrics{}, nil, nil)
	pool.processJob(context.Background(), job)

	// Status history should NOT contain JobStatusDone.
	for _, s := range js.statusHistory {
		if s.status == domain.JobStatusDone {
			t.Fatalf("worker reached done despite cancellation: %+v", js.statusHistory)
		}
	}
	// Upload should not have been called either.
	if len(up.uploads) != 0 {
		t.Fatalf("upload must not run after cancellation observed")
	}
}

// ---- Phase 06 (workstream raw-jp / v0.2): invalidator fire ----

// newHappyPoolWithInvalidator mirrors newHappyPool but lets the caller
// inject a CatalogInvalidator (typically a *stubInvalidator).
func newHappyPoolWithInvalidator(t *testing.T, job *domain.Job, inv CatalogInvalidator) (*EncoderPool, *stubJobStore, *stubEpisodeStore, *stubUploader, *stubMetrics) {
	t.Helper()
	dir := t.TempDir()
	plPath := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(plPath, []byte("#EXTM3U"), 0o644); err != nil {
		t.Fatal(err)
	}
	segPath := filepath.Join(dir, "segment_001.ts")
	if err := os.WriteFile(segPath, []byte("AAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	js := newStubJobStore()
	js.addPending(job)
	es := &stubEpisodeStore{}
	tr := &stubTranscoder{result: &ffmpeg.Result{
		PlaylistPath: plPath,
		SegmentPaths: []string{segPath},
		DurationSec:  1450,
		SizeBytes:    1024,
	}}
	up := &stubUploader{bytes: 9999}
	det := &stubDetector{ep: 1, ok: true}
	res := &stubResolver{path: filepath.Join(dir, "src.mp4")}
	mt := &stubMetrics{}

	pool := NewEncoderPool(1, js, es, tr, up, det, res, mt, nil, inv)
	return pool, js, es, up, mt
}

func TestEncoder_InvalidatorFires_OnDoneWithShikimori(t *testing.T) {
	inv := &stubInvalidator{}
	job := &domain.Job{
		ID:          "job-inv-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "57466",
		Status:      domain.JobStatusEncoding,
	}
	pool, js, _, _, _ := newHappyPoolWithInvalidator(t, job, inv)
	pool.processJob(context.Background(), job)

	if js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status did not reach done: %+v", js.statusHistory)
	}
	if len(inv.calls) != 1 {
		t.Fatalf("invalidator calls = %d, want exactly 1", len(inv.calls))
	}
	if inv.calls[0] != "57466" {
		t.Errorf("invalidator called with %q, want 57466", inv.calls[0])
	}
}

func TestEncoder_InvalidatorSkipped_OnEmptyShikimori(t *testing.T) {
	inv := &stubInvalidator{}
	job := &domain.Job{
		ID:       "job-inv-2",
		Magnet:   validMagnet,
		Uploader: "Ohys-Raws",
		Status:   domain.JobStatusEncoding,
		// ShikimoriID intentionally empty
	}
	pool, _, _, _, _ := newHappyPoolWithInvalidator(t, job, inv)
	pool.processJob(context.Background(), job)

	if len(inv.calls) != 0 {
		t.Errorf("invalidator called %d times for empty shikimori_id; want 0", len(inv.calls))
	}
}

func TestEncoder_InvalidatorSkipped_OnFailure(t *testing.T) {
	inv := &stubInvalidator{}
	job := &domain.Job{
		ID:          "job-inv-3",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "57466",
		Status:      domain.JobStatusEncoding,
	}
	dir := t.TempDir()
	js := newStubJobStore()
	js.addPending(job)
	tr := &stubTranscoder{err: errors.New("ffmpeg boom")}
	pool := NewEncoderPool(1, js, &stubEpisodeStore{}, tr, &stubUploader{}, &stubDetector{ep: 1, ok: true},
		&stubResolver{path: filepath.Join(dir, "s.mp4")}, &stubMetrics{}, nil, inv)
	pool.processJob(context.Background(), job)

	if len(inv.calls) != 0 {
		t.Errorf("invalidator called %d times on transcode failure; want 0", len(inv.calls))
	}
}

func TestEncoder_NilInvalidator_Safe(t *testing.T) {
	job := &domain.Job{
		ID:          "job-inv-4",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "57466",
		Status:      domain.JobStatusEncoding,
	}
	pool, js, _, _, _ := newHappyPoolWithInvalidator(t, job, nil) // nil invalidator
	pool.processJob(context.Background(), job)
	if js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status did not reach done with nil invalidator: %+v", js.statusHistory)
	}
}

// ---- DefaultSourceResolver tests ----

func TestDefaultSourceResolver_ReturnsLargestVideo(t *testing.T) {
	dir := t.TempDir()
	hash := "deadbeef"
	root := filepath.Join(dir, hash)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	smallMP4 := filepath.Join(root, "small.mp4")
	if err := os.WriteFile(smallMP4, []byte("AAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	largeMKV := filepath.Join(root, "sub", "large.MKV") // case-insensitive
	if err := os.WriteFile(largeMKV, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(root, "notes.txt")
	_ = os.WriteFile(junk, make([]byte, 1000), 0o644)

	r := NewDefaultSourceResolver(dir)
	got, err := r.Resolve(context.Background(), &domain.Job{}, hash)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != largeMKV {
		t.Fatalf("got %s, want %s", got, largeMKV)
	}
}

func TestDefaultSourceResolver_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	hash := "emptyhash"
	if err := os.MkdirAll(filepath.Join(dir, hash), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewDefaultSourceResolver(dir)
	_, err := r.Resolve(context.Background(), &domain.Job{}, hash)
	if err == nil {
		t.Fatalf("expected error on empty dir, got nil")
	}
}

func TestDefaultSourceResolver_MissingDir(t *testing.T) {
	dir := t.TempDir()
	r := NewDefaultSourceResolver(dir)
	_, err := r.Resolve(context.Background(), &domain.Job{}, "doesnotexist")
	if err == nil {
		t.Fatalf("expected error on missing dir")
	}
}

func TestDefaultSourceResolver_EmptyInfohash(t *testing.T) {
	r := NewDefaultSourceResolver(t.TempDir())
	_, err := r.Resolve(context.Background(), &domain.Job{}, "")
	if err == nil {
		t.Fatalf("expected error on empty infohash")
	}
}
