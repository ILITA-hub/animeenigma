package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// fakeFPStore is an in-memory FingerprintStore double: canned Fingerprints()
// results, and AddFingerprint() records what it was given for assertions.
type fakeFPStore struct {
	fps    []domain.SkipFingerprint
	fpsErr error
	added  []domain.SkipFingerprint
	addErr error
}

func (f *fakeFPStore) Fingerprints(ctx context.Context, animeID string) ([]domain.SkipFingerprint, error) {
	if f.fpsErr != nil {
		return nil, f.fpsErr
	}
	return f.fps, nil
}

func (f *fakeFPStore) AddFingerprint(ctx context.Context, fp domain.SkipFingerprint) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, fp)
	return nil
}

func testSkipConfig() SkipConfig {
	return SkipConfig{
		HeadWindow:   480 * time.Second,
		TailWindow:   480 * time.Second,
		MinMatch:     50 * time.Second,
		MaxMatch:     150 * time.Second,
		SimThreshold: 0.75,
	}
}

// TestSkipProbeLocateFound covers the locate path's absolute-time math for
// the tail (ed) side: a 1440s HLS episode, 480s TailWindow => the tail
// window starts at seek 960; a locate hit at relative start=100 must land
// at absolute EdStart=1060. The op side has no stored fingerprint at all,
// which must come back pending_fp rather than no_match.
func TestSkipProbeLocateFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/kodik/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"stream_url":"https://cdn.example/media.m3u8","referer":"","exp":"1","sig":"s"}}`))
	})
	mux.HandleFunc("/api/streaming/hls-proxy", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXTINF:1440.0,\nseg1.ts\n#EXT-X-ENDLIST\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{
		opLocate: &OpskipLocate{Found: true, Start: 100, End: 190, Similarity: 0.9, FpIndex: 0},
	}
	fps := &fakeFPStore{fps: []domain.SkipFingerprint{
		{ID: "fp-ed-1", AnimeID: "a1", Kind: domain.SkipKindEd, Fp: domain.FpInts{1, 2, 3}},
	}}
	p := NewSkipProber(cat, srv.URL, ffmpeg, t.TempDir(), runner, fps, testSkipConfig(), nil)
	p.retryWait = 0

	unit := queue.SkipUnit{AnimeID: "a1", Provider: "kodik", Team: "Trans1", TeamID: 7, Episode: 3}
	rows := p.Probe(context.Background(), queue.SkipTask{Unit: unit}, 0)

	if len(rows) != 1 {
		t.Fatalf("locate mode: want 1 row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.AnimeID != "a1" || row.Provider != "kodik" || row.Team != "Trans1" || row.Episode != 3 {
		t.Fatalf("row identity not copied from unit: %+v", row)
	}
	if row.OpStatus != domain.SkipPendingFP {
		t.Fatalf("op status: got %q want pending_fp (no op fingerprint stored): %+v", row.OpStatus, row)
	}
	if row.EdStatus != domain.SkipDetected {
		t.Fatalf("ed status: got %q want detected: %+v", row.EdStatus, row)
	}
	wantStart, wantEnd := 1440.0-480.0+100.0, 1440.0-480.0+190.0 // 1060, 1150
	if row.EdStart != wantStart || row.EdEnd != wantEnd {
		t.Fatalf("ed window: got [%v,%v] want [%v,%v]", row.EdStart, row.EdEnd, wantStart, wantEnd)
	}
	if row.Confidence != 0.9 {
		t.Fatalf("confidence: got %v want 0.9", row.Confidence)
	}
}

// TestSkipProbePairFound covers pair-bootstrap: two kodik episodes (HLS,
// same family) whose head AND tail windows both match under the fake
// runner's canned OpskipPair result. Both rows must come back detected on
// both sides with per-episode absolute times, and exactly 2 fingerprints
// (op + ed) must be persisted.
func TestSkipProbePairFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/kodik/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"stream_url":"https://cdn.example/media.m3u8","referer":"","exp":"1","sig":"s"}}`))
	})
	mux.HandleFunc("/api/streaming/hls-proxy", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXTINF:1440.0,\nseg1.ts\n#EXT-X-ENDLIST\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{
		opPair: &OpskipPair{Found: true, AStart: 10, AEnd: 100, BStart: 15, BEnd: 105, Similarity: 0.92, Fp: []uint32{1, 2, 3}},
	}
	fps := &fakeFPStore{}
	p := NewSkipProber(cat, srv.URL, ffmpeg, t.TempDir(), runner, fps, testSkipConfig(), nil)
	p.retryWait = 0

	unitA := queue.SkipUnit{AnimeID: "a1", Provider: "kodik", Team: "Trans1", TeamID: 7, Episode: 1}
	unitB := queue.SkipUnit{AnimeID: "a1", Provider: "kodik", Team: "Trans1", TeamID: 7, Episode: 2}
	rows := p.Probe(context.Background(), queue.SkipTask{Unit: unitA, Pair: &unitB}, 0)

	if len(rows) != 2 {
		t.Fatalf("pair mode: want 2 rows, got %d: %+v", len(rows), rows)
	}
	a, b := rows[0], rows[1]
	if a.Episode != 1 || b.Episode != 2 {
		t.Fatalf("row order/episode not preserved: a=%+v b=%+v", a, b)
	}
	if a.OpStatus != domain.SkipDetected || b.OpStatus != domain.SkipDetected {
		t.Fatalf("op status: want both detected: a=%+v b=%+v", a, b)
	}
	if a.OpStart != 10 || a.OpEnd != 100 || b.OpStart != 15 || b.OpEnd != 105 {
		t.Fatalf("op windows: a=[%v,%v] b=[%v,%v]", a.OpStart, a.OpEnd, b.OpStart, b.OpEnd)
	}
	if a.EdStatus != domain.SkipDetected || b.EdStatus != domain.SkipDetected {
		t.Fatalf("ed status: want both detected (HLS pair): a=%+v b=%+v", a, b)
	}
	wantTailSeek := 1440.0 - 480.0 // 960
	if a.EdStart != wantTailSeek+10 || a.EdEnd != wantTailSeek+100 {
		t.Fatalf("ed window a: got [%v,%v] want [%v,%v]", a.EdStart, a.EdEnd, wantTailSeek+10, wantTailSeek+100)
	}
	if b.EdStart != wantTailSeek+15 || b.EdEnd != wantTailSeek+105 {
		t.Fatalf("ed window b: got [%v,%v] want [%v,%v]", b.EdStart, b.EdEnd, wantTailSeek+15, wantTailSeek+105)
	}
	if a.Confidence != 0.92 || b.Confidence != 0.92 {
		t.Fatalf("confidence: a=%v b=%v want 0.92", a.Confidence, b.Confidence)
	}
	if len(fps.added) != 2 {
		t.Fatalf("fingerprints added: got %d want 2 (op+ed): %+v", len(fps.added), fps.added)
	}
	var sawOp, sawEd bool
	for _, f := range fps.added {
		if f.AnimeID != "a1" {
			t.Fatalf("fingerprint anime_id: got %q want a1", f.AnimeID)
		}
		if !strings.Contains(f.SourceNote, "kodik ep1+ep2") {
			t.Fatalf("fingerprint source note: got %q want to contain %q", f.SourceNote, "kodik ep1+ep2")
		}
		switch f.Kind {
		case domain.SkipKindOp:
			sawOp = true
		case domain.SkipKindEd:
			sawEd = true
		}
	}
	if !sawOp || !sawEd {
		t.Fatalf("expected one op and one ed fingerprint: %+v", fps.added)
	}
}

// TestSkipProbeResolveFailureUnreachable: a 404 on the only leg of a locate
// task must come back as a single unreachable row with Fails bumped.
func TestSkipProbeResolveFailureUnreachable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/missing/animejoy/stream", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{}
	fps := &fakeFPStore{}
	p := NewSkipProber(cat, srv.URL, ffmpeg, t.TempDir(), runner, fps, testSkipConfig(), nil)
	p.retryWait = 0

	unit := queue.SkipUnit{AnimeID: "missing", Provider: "animejoy", Episode: 1}
	rows := p.Probe(context.Background(), queue.SkipTask{Unit: unit}, 2)

	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.OpStatus != domain.SkipUnreachable || row.EdStatus != domain.SkipUnreachable {
		t.Fatalf("statuses: want both unreachable: %+v", row)
	}
	if row.Fails != 3 {
		t.Fatalf("fails: got %d want prevFails+1=3", row.Fails)
	}
}

// TestSkipProbeBudgetExpiredPendingFP mirrors prober.go's budget-ctx rule:
// a ctx that expires mid-resolve must NOT count as a failure — pending_fp,
// Fails left untouched (0), not unreachable+bumped.
func TestSkipProbeBudgetExpiredPendingFP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/animejoy/stream", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{}
	fps := &fakeFPStore{}
	p := NewSkipProber(cat, srv.URL, ffmpeg, t.TempDir(), runner, fps, testSkipConfig(), nil)
	p.retryWait = 0

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	unit := queue.SkipUnit{AnimeID: "a1", Provider: "animejoy", Episode: 1}
	rows := p.Probe(ctx, queue.SkipTask{Unit: unit}, 3) // prevFails=3: proves it did NOT become 4

	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.OpStatus != domain.SkipPendingFP || row.EdStatus != domain.SkipPendingFP {
		t.Fatalf("statuses: want both pending_fp (budget overrun, not a dead stream): %+v", row)
	}
	if row.Fails != 0 {
		t.Fatalf("fails: got %d want 0/untouched (no backoff for a budget overrun)", row.Fails)
	}
}

// TestSkipProbeRePairNotFoundSetsPairTried: a RePair task whose op/ed sides
// both come back not-found must still set PairTried on BOTH rows — that's
// what stops the re-pair scan from picking the same pair forever.
func TestSkipProbeRePairNotFoundSetsPairTried(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/animejoy/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"url":"https://cdn.example/video.mp4","referer":"","exp":"1","sig":"s"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{opPair: &OpskipPair{Found: false}}
	fps := &fakeFPStore{}
	p := NewSkipProber(cat, srv.URL, ffmpeg, t.TempDir(), runner, fps, testSkipConfig(), nil)
	p.retryWait = 0

	unitA := queue.SkipUnit{AnimeID: "a1", Provider: "animejoy", Episode: 3}
	unitB := queue.SkipUnit{AnimeID: "a1", Provider: "animejoy", Episode: 4}
	rows := p.Probe(context.Background(), queue.SkipTask{Unit: unitA, Pair: &unitB, RePair: true}, 0)

	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	if !rows[0].PairTried || !rows[1].PairTried {
		t.Fatalf("PairTried: want true on both rows regardless of outcome: %+v / %+v", rows[0], rows[1])
	}
	if rows[0].OpStatus != domain.SkipNoMatch || rows[1].OpStatus != domain.SkipNoMatch {
		t.Fatalf("op status: want no_match on both: %+v / %+v", rows[0], rows[1])
	}
	if len(fps.added) != 0 {
		t.Fatalf("no fingerprint should be added on a not-found pair: got %d", len(fps.added))
	}
}

// TestOpskipPySelftest runs the real analyzer's built-in self-check
// (synthetic fingerprints, no fpcalc/ffmpeg dependency) end-to-end. Skipped
// when python3 isn't on PATH — this is an environment smoke test, not a
// substitute for the fake-runner unit tests above.
func TestOpskipPySelftest(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	script := filepath.Join(wd, "..", "..", "analyzers", "opskip.py")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("opskip.py not found at %s: %v", script, err)
	}
	out, err := exec.Command(python3, script, "--selftest").CombinedOutput()
	if err != nil {
		t.Fatalf("opskip.py --selftest failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "selftest OK") {
		t.Fatalf("opskip.py --selftest did not report OK:\n%s", out)
	}
}
