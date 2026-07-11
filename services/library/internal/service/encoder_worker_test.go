package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
)

// ---- Stubs ----

// stubJobStore is a minimal in-memory JobStore mirror.
type stubJobStore struct {
	mu            sync.Mutex
	rows          map[string]*domain.Job
	statusHistory []statusEvent
	getByIDHook   func(id string) (*domain.Job, error)
	claimQueue    []*domain.Job
	claimErr      error
	updateErr     error
	storageWrites map[string]string // job id → resolved storage written back
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

func (s *stubJobStore) ClaimForEncoding(_ context.Context) (*domain.Job, error) {
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
	// Mirror real ClaimForEncoding: flip 'encoding' → 'transcoding' (the
	// non-claimable in-progress state) atomically on claim.
	if r, ok := s.rows[job.ID]; ok {
		r.Status = domain.JobStatusTranscoding
	}
	out := *job
	out.Status = domain.JobStatusTranscoding
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

func (s *stubJobStore) UpdateStorage(_ context.Context, id, storage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storageWrites == nil {
		s.storageWrites = map[string]string{}
	}
	s.storageWrites[id] = storage
	if r, ok := s.rows[id]; ok {
		r.Storage = storage
	}
	return nil
}

// stubEpisodeStore records every Create call.
type stubEpisodeStore struct {
	mu        sync.Mutex
	created   []domain.Episode
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

	storyboardErr   error
	storyboardCalls []string // sourcePath per call
}

func (s *stubTranscoder) Transcode(_ context.Context, source string) (*ffmpeg.Result, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func (s *stubTranscoder) Storyboard(_ context.Context, sourcePath string, _ int) (*ffmpeg.StoryboardResult, error) {
	s.storyboardCalls = append(s.storyboardCalls, sourcePath)
	if s.storyboardErr != nil {
		return nil, s.storyboardErr
	}
	// A dedicated per-call subdirectory, NOT the bare os.TempDir() — the
	// worker's cleanup does os.RemoveAll(filepath.Dir(VTTPath)), mirroring
	// the real Storyboard()'s contract (its own MkdirTemp subdir). Returning
	// os.TempDir() itself here would make that RemoveAll wipe the entire
	// system temp directory out from under the test process.
	dir, err := os.MkdirTemp("", "storyboard-test-")
	if err != nil {
		return nil, err
	}
	return &ffmpeg.StoryboardResult{
		SheetPaths: []string{filepath.Join(dir, "storyboard_001.jpg")},
		VTTPath:    filepath.Join(dir, "storyboard.vtt"),
	}, nil
}

// stubUploader records every Upload call, including the class/override the
// encoder routes the storage service by.
type stubUploader struct {
	mu      sync.Mutex
	uploads []uploadCall
	err     error
	// storage is the resolved backend id Upload returns (default "minio").
	storage string

	uploadStoryboardErr     error
	uploadStoryboardPrefix  string
	uploadStoryboardStorage string
}

type uploadCall struct {
	class    string
	override string
	prefix   string
	files    []string
}

func (s *stubUploader) Upload(_ context.Context, class, override, prefix string, filePaths []string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads = append(s.uploads, uploadCall{
		class:    class,
		override: override,
		prefix:   prefix,
		files:    append([]string(nil), filePaths...),
	})
	if s.storage == "" {
		return "minio", nil
	}
	return s.storage, nil
}

func (s *stubUploader) UploadStoryboard(_ context.Context, storage, prefix string, _ []string, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploadStoryboardPrefix = prefix
	s.uploadStoryboardStorage = storage
	return s.uploadStoryboardErr
}

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
	mu             sync.Mutex
	jobsTotal      []string
	encodeDur      []float64
	uploadBytes    int64
	encodeFailures []string
	activeWorkers  int // last SetEncodeActiveWorkers value
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

func (s *stubMetrics) SetEncodeActiveWorkers(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeWorkers = n
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
	up := &stubUploader{storage: "minio"}
	det := &stubDetector{ep: 1, ok: true}
	res := &stubResolver{path: filepath.Join(dir, "src.mp4")}
	mt := &stubMetrics{}

	pool := NewEncoderPool(1, js, es, tr, up, det, res, mt, nil, nil)
	return pool, js, es, up, mt
}

// TestEncoder_PrefersKnownJobEpisode — autocache (and admin-with-known-episode)
// jobs carry the INTENDED episode from enqueue time (migration 009). The encoder
// must trust it and NOT depend on filename detection, which fails for many real
// release names (e.g. "...S04E10 VOSTFR ...-Tsundere-Raws (CR).mkv" — the generic
// detector only matches "- NN (" / "- NN ["). With a detector that returns
// ok=false, a job whose Episode is set must still encode to that episode number.
func TestEncoder_PrefersKnownJobEpisode(t *testing.T) {
	ep := 10
	job := &domain.Job{
		ID:          "job-ep",
		Magnet:      validMagnet,
		Uploader:    "Tsundere-Raws",
		ShikimoriID: "61316",
		Episode:     &ep,
		Source:      domain.JobSourceAutocache,
		Status:      domain.JobStatusEncoding,
	}
	pool, js, es, up, _ := newHappyPool(t, job)
	// Force filename detection to FAIL — the known job.Episode must be used.
	pool.detector = &stubDetector{ep: 0, ok: false}
	// The storage service routes an autocache (library-auto) job to s3 in prod;
	// have the stub resolve to s3 so we can prove the resolved backend flows through.
	up.storage = "s3"

	pool.processJob(context.Background(), &domain.Job{
		ID:          "job-ep",
		Magnet:      validMagnet,
		Uploader:    "Tsundere-Raws",
		ShikimoriID: "61316",
		Episode:     &ep,
		Source:      domain.JobSourceAutocache,
	})

	if len(js.statusHistory) == 0 || js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status history did not end at done: %+v", js.statusHistory)
	}
	if len(es.created) != 1 || es.created[0].EpisodeNumber != 10 {
		t.Fatalf("episode = %+v, want EpisodeNumber=10 (from job.Episode)", es.created)
	}
	// Autocache jobs must tag the episode as autocache (storage class), with the
	// freshness basis set, else the Accountant/Evictor mislabel it as admin.
	if es.created[0].Source != domain.EpisodeSourceAutocache {
		t.Fatalf("episode Source = %q, want autocache", es.created[0].Source)
	}
	if es.created[0].Track != domain.EpisodeTrackRaw {
		t.Fatalf("episode Track = %q, want raw", es.created[0].Track)
	}
	if es.created[0].DownloadedAt == nil {
		t.Fatalf("episode DownloadedAt is nil; want set (freshness rule-1 basis)")
	}
	// Autocache jobs route the storage service by ClassLibraryAuto (→ s3 in prod).
	if len(up.uploads) != 1 || up.uploads[0].class != domain.ClassLibraryAuto {
		t.Fatalf("upload class = %+v, want %q (autocache job)", up.uploads, domain.ClassLibraryAuto)
	}
	// The resolved backend (s3 here) is written back to the job + episode rows.
	if js.storageWrites["job-ep"] != "s3" {
		t.Fatalf("job storage write-back = %q, want s3", js.storageWrites["job-ep"])
	}
	if es.created[0].Storage != "s3" {
		t.Fatalf("episode Storage = %q, want s3 (resolved backend)", es.created[0].Storage)
	}
}

// TestEncoder_FallsBackToDetectorWhenEpisodeUnknown — when job.Episode is nil
// (e.g. a manual folder ingest with no pre-resolved episode), the encoder still
// uses filename detection.
func TestEncoder_FallsBackToDetectorWhenEpisodeUnknown(t *testing.T) {
	job := &domain.Job{
		ID:          "job-nodet",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
		Status:      domain.JobStatusEncoding,
	}
	pool, _, es, _, _ := newHappyPool(t, job) // default detector: ep=1, ok=true
	pool.processJob(context.Background(), &domain.Job{
		ID:          "job-nodet",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
	})
	if len(es.created) != 1 || es.created[0].EpisodeNumber != 1 {
		t.Fatalf("episode = %+v, want EpisodeNumber=1 (detector fallback)", es.created)
	}
	// A non-autocache (manual/admin ingest) job stays storage-class admin.
	if es.created[0].Source != domain.EpisodeSourceAdmin {
		t.Fatalf("episode Source = %q, want admin", es.created[0].Source)
	}
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
	// A manual/admin job routes the storage service by ClassLibraryManual with the
	// job's (empty) storage override.
	if up.uploads[0].class != domain.ClassLibraryManual {
		t.Fatalf("upload class = %q, want %q (non-autocache job)", up.uploads[0].class, domain.ClassLibraryManual)
	}
	if up.uploads[0].override != "" {
		t.Fatalf("upload override = %q, want empty (job.Storage unset)", up.uploads[0].override)
	}
	// The resolved backend id is written back onto both the job row and the episode row.
	if js.storageWrites["job-1"] != "minio" {
		t.Fatalf("job storage write-back = %q, want minio", js.storageWrites["job-1"])
	}
	if es.created[0].Storage != "minio" {
		t.Fatalf("episode Storage = %q, want minio (resolved backend)", es.created[0].Storage)
	}
	// Metrics: encode duration observed; upload bytes recorded (playlist 7B + seg 4B).
	if len(mt.encodeDur) != 1 {
		t.Fatalf("encodeDur calls = %d, want 1", len(mt.encodeDur))
	}
	if mt.uploadBytes != 11 {
		t.Fatalf("uploadBytes = %d, want 11 (#EXTM3U=7 + AAAA=4)", mt.uploadBytes)
	}
}

// TestEncoder_StoryboardFailureDoesNotFailJob — the storyboard pass is
// strictly best-effort: a Storyboard() error must not fail the job, and the
// created episode ships with HasStoryboard=false.
func TestEncoder_StoryboardFailureDoesNotFailJob(t *testing.T) {
	job := &domain.Job{
		ID:          "job-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
		Status:      domain.JobStatusEncoding,
	}
	pool, js, es, _, _ := newHappyPool(t, job)
	// Replace the happy-path transcoder with one whose Storyboard() fails; the
	// Transcode() result still needs real on-disk files for the worker's
	// RemoveAll cleanup + upload steps.
	dir := t.TempDir()
	plPath := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(plPath, []byte("#EXTM3U"), 0o644); err != nil {
		t.Fatal(err)
	}
	segPath := filepath.Join(dir, "segment_001.ts")
	if err := os.WriteFile(segPath, []byte("AAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	pool.transcoder = &stubTranscoder{
		result: &ffmpeg.Result{
			PlaylistPath: plPath,
			SegmentPaths: []string{segPath},
			DurationSec:  1450,
			SizeBytes:    1024,
		},
		storyboardErr: errors.New("boom"),
	}

	pool.processJob(context.Background(), &domain.Job{
		ID:          "job-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
	})

	// Status history must end with done — storyboard failure never fails the job.
	if len(js.statusHistory) == 0 || js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status history did not end at done: %+v", js.statusHistory)
	}
	if len(es.created) != 1 {
		t.Fatalf("episodes created = %d, want 1", len(es.created))
	}
	if es.created[0].HasStoryboard {
		t.Fatalf("HasStoryboard = true, want false on storyboard failure")
	}
}

// TestEncoder_StoryboardSuccessSetsFlagAndUploads — a successful Storyboard +
// UploadStoryboard sets HasStoryboard=true and uploads to the same prefix as
// the HLS upload.
func TestEncoder_StoryboardSuccessSetsFlagAndUploads(t *testing.T) {
	job := &domain.Job{
		ID:          "job-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
		Status:      domain.JobStatusEncoding,
	}
	pool, js, es, up, _ := newHappyPool(t, job)

	pool.processJob(context.Background(), &domain.Job{
		ID:          "job-1",
		Magnet:      validMagnet,
		Uploader:    "Ohys-Raws",
		ShikimoriID: "123",
	})

	if len(js.statusHistory) == 0 || js.statusHistory[len(js.statusHistory)-1].status != domain.JobStatusDone {
		t.Fatalf("status history did not end at done: %+v", js.statusHistory)
	}
	if len(es.created) != 1 {
		t.Fatalf("episodes created = %d, want 1", len(es.created))
	}
	if !es.created[0].HasStoryboard {
		t.Fatalf("HasStoryboard = false, want true on storyboard success")
	}
	if len(up.uploads) != 1 {
		t.Fatalf("upload calls = %+v, want 1 HLS upload recorded", up.uploads)
	}
	if up.uploadStoryboardPrefix != up.uploads[0].prefix {
		t.Fatalf("uploadStoryboardPrefix = %q, want it to match the HLS upload prefix %q",
			up.uploadStoryboardPrefix, up.uploads[0].prefix)
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
	up := &stubUploader{storage: "minio"}
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

// ---- AUTO-575: degradation-aware graded concurrency limiter ----

// fakeLevel is a mutable ShedChecker for encodeLimiter tests.
type fakeLevel struct{ lvl atomic.Int32 }

func (f *fakeLevel) set(n int)             { f.lvl.Store(int32(n)) }
func (f *fakeLevel) Level() int            { return int(f.lvl.Load()) }
func (f *fakeLevel) ShouldShed(m int) bool { return f.Level() >= m }

// TestEncodeLimiter_GradedCapByLevel proves the cap scales with the degradation
// level: maxWorkers at level 0, 1 at level 1, 0 at level 2 — and that the
// injected active-workers gauge tracks acquisitions/releases.
func TestEncodeLimiter_GradedCapByLevel(t *testing.T) {
	fl := &fakeLevel{}
	var active int32
	lim := newEncodeLimiter(3, func(n int) { atomic.StoreInt32(&active, int32(n)) }, nil)
	lim.set(fl)

	// Level 0 → full throughput (cap == maxWorkers == 3).
	fl.set(0)
	got := 0
	for lim.tryAcquire() {
		got++
	}
	if got != 3 {
		t.Fatalf("level 0: acquired %d slots, want 3 (maxWorkers)", got)
	}
	if n := atomic.LoadInt32(&active); n != 3 {
		t.Fatalf("active gauge = %d, want 3 at full cap", n)
	}
	for i := 0; i < got; i++ {
		lim.release()
	}
	if n := atomic.LoadInt32(&active); n != 0 {
		t.Fatalf("active gauge after drain = %d, want 0", n)
	}

	// Level 1 → serialize (cap == 1).
	fl.set(1)
	got = 0
	for lim.tryAcquire() {
		got++
	}
	if got != 1 {
		t.Fatalf("level 1: acquired %d slots, want 1 (serialized)", got)
	}
	for i := 0; i < got; i++ {
		lim.release()
	}

	// Level 2 → pause (cap == 0).
	fl.set(2)
	if lim.tryAcquire() {
		t.Fatalf("level 2: acquire succeeded, want 0 (paused)")
	}
	if n := atomic.LoadInt32(&active); n != 0 {
		t.Fatalf("active gauge at level 2 = %d, want 0", n)
	}
}

// TestEncodeLimiter_ReleaseFreesSlot proves a released slot is reusable — the
// job "waiting" at a saturated cap is admitted once the running one completes,
// rather than being dropped.
func TestEncodeLimiter_ReleaseFreesSlot(t *testing.T) {
	fl := &fakeLevel{}
	fl.set(1) // cap 1
	lim := newEncodeLimiter(2, nil, nil)
	lim.set(fl)

	if !lim.tryAcquire() {
		t.Fatal("first acquire at cap 1 should succeed")
	}
	if lim.tryAcquire() {
		t.Fatal("second acquire at cap 1 should fail (saturated)")
	}
	lim.release()
	if !lim.tryAcquire() {
		t.Fatal("acquire after release should succeed (job un-queues once slot frees)")
	}
}

// TestEncodeLimiter_NilCheckerRunsFull proves the fail-open contract: with no
// degradation watcher wired, the limiter never sheds and admits maxWorkers.
func TestEncodeLimiter_NilCheckerRunsFull(t *testing.T) {
	lim := newEncodeLimiter(2, nil, nil) // no checker wired
	got := 0
	for lim.tryAcquire() {
		got++
	}
	if got != 2 {
		t.Fatalf("nil checker: acquired %d, want 2 (maxWorkers — fail-open, never sheds)", got)
	}
}

// TestEncodeLimiter_ReleaseFloorsAtZero proves a stray double-release can't
// drive the active count (and thus the gauge) negative.
func TestEncodeLimiter_ReleaseFloorsAtZero(t *testing.T) {
	var active int32
	lim := newEncodeLimiter(2, func(n int) { atomic.StoreInt32(&active, int32(n)) }, nil)
	lim.release() // no prior acquire
	if n := atomic.LoadInt32(&active); n != 0 {
		t.Fatalf("active gauge after stray release = %d, want 0 (floored)", n)
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
