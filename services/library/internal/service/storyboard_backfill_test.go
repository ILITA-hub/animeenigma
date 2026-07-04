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

// sbFakeRepo is a FAITHFUL fake of the storyboard-less episode queue: it holds a
// working set of episodes and derives ListWithoutStoryboard's result from the
// live HasStoryboard flag state (NOT a pre-canned queue of batches). This is the
// whole point of the rewrite — a canned-batch fake decouples list results from
// flag state, so a wedged production worker (which re-selects the same broken
// oldest row forever) would still pass an error-continue test. Here:
//   - ListWithoutStoryboard returns, in insertion order (the tests seed
//     oldest-first, mirroring the real created_at ASC), up to `limit` episodes
//     whose flag is still false in this fake's own state.
//   - SetHasStoryboard flips the flag, so a processed row drops out of the next
//     list exactly like the real repo.
//
// onList(callNum) and onSet(id) fire AFTER the state mutation (outside the lock)
// so a test can advance a clock or cancel the loop ctx deterministically.
type sbFakeRepo struct {
	mu        sync.Mutex
	rec       *sbRecorder
	eps       []domain.Episode // working set; HasStoryboard is mutated in place
	listErr   error
	listCalls int
	setCalls  []string
	onList    func(call int)
	onSet     func(id string)
}

func (r *sbFakeRepo) ListWithoutStoryboard(_ context.Context, limit int) ([]domain.Episode, error) {
	r.mu.Lock()
	r.listCalls++
	call := r.listCalls
	if r.listErr != nil {
		err := r.listErr
		r.mu.Unlock()
		return nil, err
	}
	var out []domain.Episode
	for _, ep := range r.eps {
		if ep.HasStoryboard {
			continue
		}
		out = append(out, ep)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	onList := r.onList
	r.mu.Unlock()
	if onList != nil {
		onList(call)
	}
	return out, nil
}

func (r *sbFakeRepo) SetHasStoryboard(_ context.Context, id string) error {
	r.mu.Lock()
	if r.rec != nil {
		r.rec.add("set")
	}
	r.setCalls = append(r.setCalls, id)
	for i := range r.eps {
		if r.eps[i].ID == id {
			r.eps[i].HasStoryboard = true
		}
	}
	onSet := r.onSet
	r.mu.Unlock()
	if onSet != nil {
		onSet(id)
	}
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

// sbFakeMaker records Storyboard sourcePaths + durations and can fail two ways:
//   - errOn[callNum] (1-indexed) — fail the Nth call, then recover (used to
//     model a transient failure that a cooldown-elapsed retry clears).
//   - failDurations[durationSec] — fail UNCONDITIONALLY whenever an episode with
//     that duration is processed. durationSec is the only per-episode signal that
//     reaches Storyboard (the temp source path is random), so it is how a test
//     pins a PERMANENT per-episode failure to one specific episode.
//
// Each success mints its own temp subdir so the worker's post-upload
// RemoveAll(filepath.Dir(VTTPath)) mirrors the real Storyboard contract without
// wiping a shared dir.
type sbFakeMaker struct {
	mu            sync.Mutex
	rec           *sbRecorder
	calls         int
	sourcePaths   []string
	durations     []int
	errOn         map[int]bool
	failDurations map[int]bool
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
	if m.errOn[m.calls] || m.failDurations[durationSec] {
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

// callCount returns the number of Storyboard invocations so far (thread-safe).
func (m *sbFakeMaker) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
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

// contains reports whether s appears in xs.
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

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
	repo := &sbFakeRepo{rec: rec, eps: []domain.Episode{ep}}
	// One episode, then it drops out via the flag. Cancel once it is flagged.
	repo.onSet = func(string) { cancel() }
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

	repo := &sbFakeRepo{eps: []domain.Episode{{ID: "ep-x", MinioPath: "aeProvider/1/RAW/1/"}}}
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

// TestBackfill_EpisodeErrorContinues — a starvation guard. ep-1 is the oldest
// storyboard-less row and its Storyboard fails PERMANENTLY (unconditional, keyed
// on its duration). The failing row must NOT wedge the queue: ep-2, which sits
// behind it, must still be processed to completion (upload + flag) within a
// bounded number of cycles. ep-1's flag stays false throughout.
//
// This MUST fail against the pre-fix worker: that worker lists limit=1, so it
// re-selects ep-1 (the oldest still-flagless row) every cycle forever and never
// reaches ep-2. The safety cap in onList turns that wedge into a clean assertion
// failure instead of a hang.
func TestBackfill_EpisodeErrorContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ep1 := domain.Episode{ID: "ep-1", MinioPath: "aeProvider/1/RAW/1/", DurationSec: intPtr(1000)}
	ep2 := domain.Episode{ID: "ep-2", MinioPath: "aeProvider/2/RAW/1/", DurationSec: intPtr(1200)}
	repo := &sbFakeRepo{eps: []domain.Episode{ep1, ep2}} // ep-1 oldest (front of slice)
	store := &sbFakeStore{}
	// ep-1 (duration 1000) fails on EVERY attempt; ep-2 (duration 1200) succeeds.
	maker := &sbFakeMaker{failDurations: map[int]bool{1000: true}}
	guard := &sbFakeGuard{allowed: true}

	repo.onSet = func(id string) {
		if id == "ep-2" {
			cancel() // ep-2 made it through despite ep-1 being broken → done
		}
	}
	// Safety net: a wedged worker never flags ep-2, so bound the run and let the
	// assertions below fail cleanly rather than looping forever.
	repo.onList = func(call int) {
		if call > 30 {
			cancel()
		}
	}

	b := NewStoryboardBackfill(repo, store, maker, guard, 20, time.Millisecond, "", nil)
	b.Run(ctx)

	// ep-2 must have been processed to completion.
	if !contains(repo.setCalls, "ep-2") {
		t.Fatalf("SetHasStoryboard ids = %v, want to include ep-2 (queue must advance past broken ep-1)", repo.setCalls)
	}
	if !contains(store.uploadPrefix, ep2.MinioPath) {
		t.Fatalf("UploadStoryboard prefixes = %v, want to include %s", store.uploadPrefix, ep2.MinioPath)
	}
	// ep-1 permanently failed before upload/flag — it must NEVER be flagged.
	if contains(repo.setCalls, "ep-1") {
		t.Fatalf("SetHasStoryboard ids = %v, ep-1 must never be flagged (it fails permanently)", repo.setCalls)
	}
}

// TestBackfill_FailedEpisodeRetriedAfterCooldown — a TRANSIENTLY failing episode
// (fails once, would succeed on retry) must NOT be retried while it is cooling
// down, and MUST become eligible again once the cooldown elapses. A fake clock
// proves both halves without waiting the real 6h window.
func TestBackfill_FailedEpisodeRetriedAfterCooldown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ep := domain.Episode{ID: "ep-1", MinioPath: "aeProvider/1/RAW/1/", DurationSec: intPtr(1000)}
	repo := &sbFakeRepo{eps: []domain.Episode{ep}}
	store := &sbFakeStore{}
	maker := &sbFakeMaker{errOn: map[int]bool{1: true}} // 1st attempt fails, retry succeeds
	guard := &sbFakeGuard{allowed: true}

	// Injectable clock — everything runs in this test goroutine (Run is called
	// synchronously and every hook fires inside a fake method), so a plain
	// captured variable needs no locking.
	now := time.Unix(1_700_000_000, 0)

	// attemptsWhileCoolingDown = number of Storyboard attempts made at the moment
	// we finally jump the clock past the cooldown. If the cooldown gate works,
	// ep-1 is skipped while cooling, so this is exactly 1 (the initial failure).
	attemptsWhileCoolingDown := -1
	repo.onList = func(call int) {
		// Let the loop take a few cooldown-gated cycles first, then jump past the
		// cooldown so the next eligibility check re-admits ep-1.
		if call == 4 {
			attemptsWhileCoolingDown = maker.callCount()
			now = now.Add(2 * time.Hour) // > cooldown (set to 1h below)
		}
		if call > 50 { // safety: never hang if eligibility regresses
			cancel()
		}
	}
	repo.onSet = func(string) { cancel() } // ep-1 finally succeeds → done

	b := NewStoryboardBackfill(repo, store, maker, guard, 20, time.Millisecond, "", nil)
	b.cooldown = time.Hour
	b.clock = func() time.Time { return now }
	b.Run(ctx)

	if attemptsWhileCoolingDown != 1 {
		t.Fatalf("Storyboard attempts during cooldown = %d, want 1 (cooldown must gate the retry)", attemptsWhileCoolingDown)
	}
	if got := maker.callCount(); got != 2 {
		t.Fatalf("total Storyboard attempts = %d, want 2 (fail, then post-cooldown success)", got)
	}
	if !reflect.DeepEqual(repo.setCalls, []string{"ep-1"}) {
		t.Fatalf("SetHasStoryboard ids = %v, want [ep-1] (retried + succeeded after cooldown)", repo.setCalls)
	}
}
