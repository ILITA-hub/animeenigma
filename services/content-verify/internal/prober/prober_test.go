package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// fakeRunner returns canned LID/Hardsub results regardless of input, so
// tests can drive Probe's control flow without real analyzers.
type fakeRunner struct {
	lid     *LIDResult
	lidErr  error
	hardsub *HardsubResult
	hsErr   error
}

func (f *fakeRunner) LID(ctx context.Context, wavs []string) (*LIDResult, error) {
	if f.lidErr != nil {
		return nil, f.lidErr
	}
	return f.lid, nil
}

func (f *fakeRunner) Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error) {
	if f.hsErr != nil {
		return nil, f.hsErr
	}
	return f.hardsub, nil
}

func threeAgreeingEnFragments() *LIDResult {
	return &LIDResult{Fragments: []LIDFragment{
		{Lang: "en", Prob: 0.99, Speech: true, SpeechSeconds: 24},
		{Lang: "en", Prob: 0.97, Speech: true, SpeechSeconds: 22},
		{Lang: "en", Prob: 0.96, Speech: true, SpeechSeconds: 25},
	}}
}

// writeFakeFFmpeg writes an executable shell-script ffmpeg stub that
// creates the requested output wav (the arg right after the first "-y")
// and exits 0. PATH-independent — the caller passes its path directly as
// ffmpegPath.
func writeFakeFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\n" +
		"prev=\"\"\n" +
		"out=\"\"\n" +
		"found=0\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$found\" = \"0\" ] && [ \"$prev\" = \"-y\" ]; then\n" +
		"    out=\"$arg\"\n" +
		"    found=1\n" +
		"  fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"touch \"$out\"\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

// writeFailingFFmpeg always exits non-zero (simulates a dead stream: even
// the first fragment cannot be pulled).
func writeFailingFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write failing ffmpeg: %v", err)
	}
	return path
}

// writeFlakyFFmpeg fails only for the fragment whose output wav is
// frag_1.wav (the 2nd of 3 base fragments) so tests can exercise partial
// extraction while still writing the other requested wavs.
func writeFlakyFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\n" +
		"prev=\"\"\n" +
		"out=\"\"\n" +
		"found=0\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$found\" = \"0\" ] && [ \"$prev\" = \"-y\" ]; then\n" +
		"    out=\"$arg\"\n" +
		"    found=1\n" +
		"  fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"case \"$out\" in\n" +
		"  *frag_1.wav) exit 1 ;;\n" +
		"esac\n" +
		"touch \"$out\"\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write flaky ffmpeg: %v", err)
	}
	return path
}

// buildCatalog serves an ANIMEJOY-style stream (Type "mp4", so LocalizeHLS
// is skipped) for anime "a1", a 404 for anime "missing", and a
// scraper-leg mp4 stream (with soft-subtitle tracks) for anime "a1" /
// episode "ep-1".
func buildCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/animejoy/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"url":"https://cdn.example/video.mp4","referer":"https://animejoy.example/","exp":"1","sig":"s"}}`))
	})
	mux.HandleFunc("/api/anime/missing/animejoy/stream", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/api/anime/a1/scraper/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"stream":{"headers":{"Referer":"https://x/"},` +
			`"sources":[{"url":"https://cdn.example/x.mp4","exp":"1","sig":"s","type":"mp4"}],` +
			`"tracks":[{"file":"en.vtt","label":"English","kind":"captions"}]}}}`))
	})
	return httptest.NewServer(mux)
}

func TestProbeVerified(t *testing.T) {
	srv := buildCatalog(t)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{lid: threeAgreeingEnFragments(), hardsub: &HardsubResult{Frames: 0}}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "a1", Provider: "animejoy", Key: domain.UnitKey{Server: "animejoy"}, Episode: 1}
	v := p.Probe(context.Background(), u, 0)

	if v.Status != domain.StatusVerified {
		t.Fatalf("status: got %q want verified: %+v", v.Status, v)
	}
	if v.Audio == nil || !v.Audio.Verified || v.Audio.Lang != "en" {
		t.Fatalf("audio verdict: %+v", v.Audio)
	}
	if v.Fails != 0 {
		t.Fatalf("fails should stay 0 on success: %d", v.Fails)
	}
	if v.Sample.Fragments != 3 {
		t.Fatalf("sample fragments: got %d want 3", v.Sample.Fragments)
	}
}

func TestProbeUnreachable404(t *testing.T) {
	srv := buildCatalog(t)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{lid: threeAgreeingEnFragments()}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "missing", Provider: "animejoy", Key: domain.UnitKey{Server: "animejoy"}, Episode: 1}
	v := p.Probe(context.Background(), u, 2)

	if v.Status != domain.StatusUnreachable {
		t.Fatalf("status: got %q want unreachable: %+v", v.Status, v)
	}
	if v.Fails != 3 {
		t.Fatalf("fails: got %d want prevFails+1=3", v.Fails)
	}
}

func TestProbeUnreachableFirstFragmentFailure(t *testing.T) {
	srv := buildCatalog(t)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFailingFFmpeg(t, t.TempDir()) // every ffmpeg invocation fails
	runner := &fakeRunner{lid: threeAgreeingEnFragments()}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "a1", Provider: "animejoy", Key: domain.UnitKey{Server: "animejoy"}, Episode: 1}
	v := p.Probe(context.Background(), u, 1)

	if v.Status != domain.StatusUnreachable {
		t.Fatalf("status: got %q want unreachable (first fragment dead = stream dead): %+v", v.Status, v)
	}
	if v.Fails != 2 {
		t.Fatalf("fails: got %d want prevFails+1=2", v.Fails)
	}
}

func TestProbePartialExtractionTolerated(t *testing.T) {
	srv := buildCatalog(t)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFlakyFFmpeg(t, t.TempDir()) // fragment idx 1 always fails
	runner := &fakeRunner{lid: threeAgreeingEnFragments(), hardsub: &HardsubResult{Frames: 0}}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "a1", Provider: "animejoy", Key: domain.UnitKey{Server: "animejoy"}, Episode: 1}
	v := p.Probe(context.Background(), u, 0)

	if v.Status != domain.StatusVerified {
		t.Fatalf("partial extraction should still verify with 2/3 fragments: %+v", v)
	}
	if v.Sample.Fragments != 2 {
		t.Fatalf("sample fragments: got %d want 2 (one dropped)", v.Sample.Fragments)
	}
}

func TestProbeSoftsubsFromTracks(t *testing.T) {
	srv := buildCatalog(t)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{lid: threeAgreeingEnFragments(), hardsub: &HardsubResult{Frames: 0}}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "a1", Provider: "gogoanime", Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 1, EpisodeID: "ep-1"}
	v := p.Probe(context.Background(), u, 0)

	if v.Status != domain.StatusVerified {
		t.Fatalf("status: got %q want verified: %+v", v.Status, v)
	}
	if len(v.Softsubs) != 1 || v.Softsubs[0].Lang != "English" || v.Softsubs[0].Kind != "captions" {
		t.Fatalf("softsubs not copied from stream tracks: %+v", v.Softsubs)
	}
}

// TestProbeEpisodeFallbackToOne verifies the "ближайший доступный" fallback:
// a non-scraper unit (no EpisodeID) whose latest episode 404s retries at
// episode 1 and, on success, reports the fallen-back episode number.
func TestProbeEpisodeFallbackToOne(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/animejoy/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("episode") != "1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"success":true,"data":{"url":"https://cdn.example/video.mp4","referer":"https://animejoy.example/","exp":"1","sig":"s"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{lid: threeAgreeingEnFragments(), hardsub: &HardsubResult{Frames: 0}}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "a1", Provider: "animejoy", Key: domain.UnitKey{Server: "animejoy"}, Episode: 5}
	v := p.Probe(context.Background(), u, 0)

	if v.Status != domain.StatusVerified {
		t.Fatalf("status: got %q want verified after ep-1 fallback: %+v", v.Status, v)
	}
	if v.Episode != 1 {
		t.Fatalf("episode: got %d want 1 (fallback)", v.Episode)
	}
}

// TestProbeScraperNoEpisodeFallback verifies scraper-leg units (EpisodeID
// set — the episode id is fixed at enumeration time, there's no "episode
// 1" to retry) do NOT get the ep-fallback: a single failed lookup goes
// straight to unreachable.
func TestProbeScraperNoEpisodeFallback(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/scraper/stream", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{lid: threeAgreeingEnFragments()}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)

	u := queue.Unit{AnimeID: "a1", Provider: "gogoanime", Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 5, EpisodeID: "ep-5"}
	v := p.Probe(context.Background(), u, 0)

	if v.Status != domain.StatusUnreachable {
		t.Fatalf("status: got %q want unreachable (no ep-fallback for scraper units): %+v", v.Status, v)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("scraper/stream calls: got %d want 1 (no retry)", got)
	}
}

func TestSampleOffsets(t *testing.T) {
	// Unknown/tiny duration falls back to fixed seeks.
	got := sampleOffsets(0, nil, nil)
	want := []float64{60, 240, 480, 300, 600, 720}
	if !floatsEqual(got, want) {
		t.Fatalf("unknown duration: got %v want %v", got, want)
	}
	if got := sampleOffsets(90, nil, nil); !floatsEqual(got, want) {
		t.Fatalf("tiny duration (<120): got %v want %v", got, want)
	}

	// A fraction offset landing inside the intro window is pushed past it.
	got = sampleOffsets(400, &catalogclient.TimeRange{Start: 80, End: 110}, nil)
	if got[0] != 120 { // 0.25*400=100 falls in [80,110] -> shifted to End+10
		t.Fatalf("intro-skip: got[0]=%v want 120 (offsets=%v)", got[0], got)
	}

	// A fraction offset landing at/after the outro start is pulled back
	// before it.
	got = sampleOffsets(400, nil, &catalogclient.TimeRange{Start: 90, End: 400})
	// every frac*400 >= 90 except 0.25*400=100 which is also >=90 -> all
	// clamped to outro.Start - fragmentSeconds - 10 = 90-30-10=50.
	for i, s := range got {
		if s != 50 {
			t.Fatalf("outro-skip: offset[%d]=%v want 50 (offsets=%v)", i, s, got)
		}
	}
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
