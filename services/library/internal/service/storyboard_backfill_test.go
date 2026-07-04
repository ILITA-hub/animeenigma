package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
)

// ---- Shared ordered-event recorder ----

// sbRecorder captures the cross-fake call ordering so a test can assert the
// exact DownloadPrefix → Storyboard → UploadStoryboard → SetHasStoryboard
// sequence a single processOne must produce.
type sbRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *sbRecorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *sbRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// ---- Fakes ----

// sbFakeRepo drives ListWithoutStoryboard from a queue of batches and records
// every SetHasStoryboard id. onExhaust fires when the queue is drained (the
// call that returns an empty page) — a test uses it to cancel the loop ctx.
type sbFakeRepo struct {
	mu        sync.Mutex
	rec       *sbRecorder
	batches   [][]domain.Episode
	listErr   error
	listCalls int
	setCalls  []string
	onExhaust func()
}

func (r *sbFakeRepo) ListWithoutStoryboard(_ context.Context, _ int) ([]domain.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listCalls++
	if r.listErr != nil {
		return nil, r.listErr
	}
	if len(r.batches) == 0 {
		if r.onExhaust != nil {
			r.onExhaust()
		}
		return nil, nil
	}
	batch := r.batches[0]
	r.batches = r.batches[1:]
	return batch, nil
}

func (r *sbFakeRepo) SetHasStoryboard(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rec != nil {
		r.rec.add("set")
	}
	r.setCalls = append(r.setCalls, id)
	return nil
}

// sbDownloadCall records a single DownloadPrefix invocation.
type sbDownloadCall struct {
	prefix  string
	destDir string
}

// sbFakeStore records DownloadPrefix + UploadStoryboard calls.
type sbFakeStore struct {
	mu            sync.Mutex
	rec           *sbRecorder
	downloadErr   error
	downloadCalls []sbDownloadCall
	uploadErr     error
	uploadPrefix  []string
}

func (s *sbFakeStore) DownloadPrefix(_ context.Context, prefix, destDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil {
		s.rec.add("download")
	}
	s.downloadCalls = append(s.downloadCalls, sbDownloadCall{prefix: prefix, destDir: destDir})
	return s.downloadErr
}

func (s *sbFakeStore) UploadStoryboard(_ context.Context, prefix string, _ []string, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil {
		s.rec.add("upload")
	}
	s.uploadPrefix = append(s.uploadPrefix, prefix)
	return s.uploadErr
}

// sbFakeMaker records Storyboard sourcePaths + durations and errors on the
// call numbers named in errOn (1-indexed). Each success mints its own temp
// subdir so the worker's post-upload RemoveAll(filepath.Dir(VTTPath)) mirrors
// the real Storyboard contract without wiping a shared dir.
type sbFakeMaker struct {
	mu          sync.Mutex
	rec         *sbRecorder
	calls       int
	sourcePaths []string
	durations   []int
	errOn       map[int]bool
}

func (m *sbFakeMaker) Storyboard(_ context.Context, sourcePath string, durationSec int) (*ffmpeg.StoryboardResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.rec != nil {
		m.rec.add("storyboard")
	}
	m.sourcePaths = append(m.sourcePaths, sourcePath)
	m.durations = append(m.durations, durationSec)
	if m.errOn[m.calls] {
		return nil, errors.New("simulated storyboard failure")
	}
	dir, err := os.MkdirTemp("", "sb-out-")
	if err != nil {
		return nil, err
	}
	return &ffmpeg.StoryboardResult{
		SheetPaths: []string{filepath.Join(dir, "storyboard_001.jpg")},
		VTTPath:    filepath.Join(dir, "storyboard.vtt"),
	}, nil
}

// sbFakeGuard returns a canned Allow verdict; onCall fires each call so a test
// can cancel the loop ctx after the guard is consulted.
type sbFakeGuard struct {
	mu      sync.Mutex
	allowed bool
	err     error
	calls   int
	onCall  func()
}

func (g *sbFakeGuard) Allow(_ int) (bool, int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.onCall != nil {
		g.onCall()
	}
	return g.allowed, 0, g.err
}

// ---- Tests ----

func intPtr(v int) *int { return &v }

// TestBackfill_ProcessesEpisodeAndSetsFlag — one pending episode then an empty
// page. The worker must download the prefix, storyboard the LOCAL playlist with
// the episode's duration, upload the sprites under the same prefix, flip the
// flag, and leave no temp dir behind — all in that order.
func TestBackfill_ProcessesEpisodeAndSetsFlag(t *testing.T) {
	rec := &sbRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	ep := domain.Episode{
		ID:            "ep-1",
		ShikimoriID:   "123",
		EpisodeNumber: 3,
		MinioPath:     "aeProvider/123/RAW/3/",
		DurationSec:   intPtr(1400),
		HasStoryboard: false,
	}
	repo := &sbFakeRepo{rec: rec, batches: [][]domain.Episode{{ep}}, onExhaust: cancel}
	store := &sbFakeStore{rec: rec}
	maker := &sbFakeMaker{rec: rec}
	guard := &sbFakeGuard{allowed: true}

	b := NewStoryboardBackfill(repo, store, maker, guard, 20, time.Millisecond, "", nil)
	b.Run(ctx)

	if got := rec.snapshot(); !reflect.DeepEqual(got, []string{"download", "storyboard", "upload", "set"}) {
		t.Fatalf("call order = %v, want [download storyboard upload set]", got)
	}
	if len(store.downloadCalls) != 1 {
		t.Fatalf("DownloadPrefix calls = %d, want 1", len(store.downloadCalls))
	}
	if store.downloadCalls[0].prefix != ep.MinioPath {
		t.Fatalf("DownloadPrefix prefix = %q, want %q", store.downloadCalls[0].prefix, ep.MinioPath)
	}
	destDir := store.downloadCalls[0].destDir
	wantSource := filepath.Join(destDir, "playlist.m3u8")
	if len(maker.sourcePaths) != 1 || maker.sourcePaths[0] != wantSource {
		t.Fatalf("Storyboard source = %v, want [%s]", maker.sourcePaths, wantSource)
	}
	if len(maker.durations) != 1 || maker.durations[0] != 1400 {
		t.Fatalf("Storyboard duration = %v, want [1400]", maker.durations)
	}
	if len(store.uploadPrefix) != 1 || store.uploadPrefix[0] != ep.MinioPath {
		t.Fatalf("UploadStoryboard prefix = %v, want [%s]", store.uploadPrefix, ep.MinioPath)
	}
	if !reflect.DeepEqual(repo.setCalls, []string{"ep-1"}) {
		t.Fatalf("SetHasStoryboard ids = %v, want [ep-1]", repo.setCalls)
	}
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Fatalf("temp dir %q not removed (stat err=%v)", destDir, err)
	}
}

// TestBackfill_DiskGuardDisallowedSkipsWork — when the guard says no, the worker
// touches neither the repo, the object store, nor the transcoder that cycle.
func TestBackfill_DiskGuardDisallowedSkipsWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	repo := &sbFakeRepo{batches: [][]domain.Episode{{{ID: "ep-x", MinioPath: "aeProvider/1/RAW/1/"}}}}
	store := &sbFakeStore{}
	maker := &sbFakeMaker{}
	// Guard disallows and cancels the loop after the first consultation.
	guard := &sbFakeGuard{allowed: false, onCall: cancel}

	b := NewStoryboardBackfill(repo, store, maker, guard, 20, time.Millisecond, "", nil)
	b.Run(ctx)

	if repo.listCalls != 0 {
		t.Fatalf("ListWithoutStoryboard calls = %d, want 0 when disk guard disallows", repo.listCalls)
	}
	if len(store.downloadCalls) != 0 {
		t.Fatalf("DownloadPrefix calls = %d, want 0 when disk guard disallows", len(store.downloadCalls))
	}
	if maker.calls != 0 {
		t.Fatalf("Storyboard calls = %d, want 0 when disk guard disallows", maker.calls)
	}
}

// TestBackfill_EpisodeErrorContinues — the first episode's Storyboard errors:
// its flag must NOT be set (retried next full pass), but the second episode is
// still processed to completion in the following cycle.
func TestBackfill_EpisodeErrorContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ep1 := domain.Episode{ID: "ep-1", MinioPath: "aeProvider/1/RAW/1/", DurationSec: intPtr(1000)}
	ep2 := domain.Episode{ID: "ep-2", MinioPath: "aeProvider/2/RAW/1/", DurationSec: intPtr(1200)}
	repo := &sbFakeRepo{
		batches:   [][]domain.Episode{{ep1}, {ep2}},
		onExhaust: cancel,
	}
	store := &sbFakeStore{}
	maker := &sbFakeMaker{errOn: map[int]bool{1: true}} // first Storyboard fails
	guard := &sbFakeGuard{allowed: true}

	b := NewStoryboardBackfill(repo, store, maker, guard, 20, time.Millisecond, "", nil)
	b.Run(ctx)

	if maker.calls != 2 {
		t.Fatalf("Storyboard calls = %d, want 2 (both episodes attempted)", maker.calls)
	}
	// ep1 failed before UploadStoryboard/SetHasStoryboard; only ep2 completes.
	if !reflect.DeepEqual(repo.setCalls, []string{"ep-2"}) {
		t.Fatalf("SetHasStoryboard ids = %v, want [ep-2] (ep-1 failed, no flag)", repo.setCalls)
	}
	if len(store.uploadPrefix) != 1 || store.uploadPrefix[0] != ep2.MinioPath {
		t.Fatalf("UploadStoryboard prefix = %v, want [%s] (only ep-2)", store.uploadPrefix, ep2.MinioPath)
	}
}
