package prober

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestExtractFragmentRealFFmpeg drives the REAL ffmpeg binary end-to-end
// (not a fake stub). A fake stub that just touches the output wav can't
// catch an argv-ordering regression like the -t-after--i bug this test
// guards: it never reads its own argv to decide how much work to
// simulate, so it can't tell "bounded to durSec" from "ran to EOF".
//
// Asserts: the wav exists, and the frames dir has ~5 PNGs (fps=1/6 over
// durSec=30s), NOT ~13+ — which is what an unbounded (-t scoped only to
// the wav output, decoding the png stream from seek to end-of-input)
// extraction over the 90s synthetic clip would produce.
func TestExtractFragmentRealFFmpeg(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found on PATH — skipping real-ffmpeg integration test")
	}

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "test.mp4")
	genCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	gen := exec.CommandContext(genCtx, ffmpegPath,
		"-f", "lavfi", "-i", "testsrc=duration=90:size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=duration=90",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		"-y", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate synthetic input: %v\n%s", err, out)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		t.Fatalf("mkdir frames: %v", err)
	}

	extractCtx, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	wav, err := ExtractFragment(extractCtx, ffmpegPath, src, 10, 30, 0, dir)
	if err != nil {
		t.Fatalf("ExtractFragment: %v", err)
	}
	if _, err := os.Stat(wav); err != nil {
		t.Fatalf("wav not written: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "frames"))
	if err != nil {
		t.Fatalf("read frames dir: %v", err)
	}
	n := len(entries)
	if n < 4 || n > 7 {
		t.Fatalf("frame count: got %d, want ~5 (4-7 for fps=1/6 over 30s) — an unbounded extraction over the 90s clip would produce ~13+", n)
	}
}

// TestExtractFragmentRealFFmpegHLS covers the OTHER leg of the
// -allowed_extensions conditional added alongside the -t fix above: a
// local .m3u8 input must still open correctly (allowed_extensions ALL
// applied) and must still be duration-bounded by -t like the mp4 leg.
func TestExtractFragmentRealFFmpegHLS(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found on PATH — skipping real-ffmpeg integration test")
	}

	hlsDir := t.TempDir()
	playlist := filepath.Join(hlsDir, "index.m3u8")
	genCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	gen := exec.CommandContext(genCtx, ffmpegPath,
		"-f", "lavfi", "-i", "testsrc=duration=90:size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=duration=90",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		"-f", "hls", "-hls_time", "10", "-hls_list_size", "0",
		"-y", playlist)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate synthetic HLS input: %v\n%s", err, out)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		t.Fatalf("mkdir frames: %v", err)
	}

	extractCtx, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	wav, err := ExtractFragment(extractCtx, ffmpegPath, playlist, 10, 30, 0, dir)
	if err != nil {
		t.Fatalf("ExtractFragment (m3u8 leg): %v", err)
	}
	if _, err := os.Stat(wav); err != nil {
		t.Fatalf("wav not written: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "frames"))
	if err != nil {
		t.Fatalf("read frames dir: %v", err)
	}
	n := len(entries)
	if n < 4 || n > 7 {
		t.Fatalf("frame count: got %d, want ~5 (4-7 for fps=1/6 over 30s)", n)
	}
}

// probeWavDurationChannels shells out to ffprobe for the wav's duration and
// channel count, mirroring the assertions in the other real-ffmpeg tests
// here (they drive real ffmpeg rather than a fake stub for the same reason:
// a stub can't catch an argv-ordering regression).
func probeWavDurationChannels(t *testing.T, ffprobePath, wav string) (dur float64, channels int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-show_entries", "stream=duration,channels",
		"-of", "default=noprint_wrappers=1",
		wav).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe %s: %v\n%s", wav, err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "duration="):
			d, perr := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "duration=")), 64)
			if perr == nil {
				dur = d
			}
		case strings.HasPrefix(line, "channels="):
			c, perr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "channels=")))
			if perr == nil {
				channels = c
			}
		}
	}
	return dur, channels
}

// TestExtractWindowRealFFmpeg drives real ffmpeg: a 20s sine-wav input,
// ExtractWindow(ctx, ffmpeg, input, 5, 10, "head", dir) must produce
// <dir>/head.wav, mono, ~10s long (the -ss/-t-as-input-options bound, same
// discipline as ExtractFragment — no frames output here, audio-only).
func TestExtractWindowRealFFmpeg(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found on PATH — skipping real-ffmpeg integration test")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found on PATH — skipping real-ffmpeg integration test")
	}

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "sine.wav")
	genCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	gen := exec.CommandContext(genCtx, ffmpegPath,
		"-f", "lavfi", "-i", "sine=duration=20",
		"-y", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate synthetic sine input: %v\n%s", err, out)
	}

	dir := t.TempDir()
	extractCtx, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	wav, err := ExtractWindow(extractCtx, ffmpegPath, src, 5, 10, "head", dir)
	if err != nil {
		t.Fatalf("ExtractWindow: %v", err)
	}
	if want := filepath.Join(dir, "head.wav"); wav != want {
		t.Fatalf("wav path: got %s want %s", wav, want)
	}
	if _, err := os.Stat(wav); err != nil {
		t.Fatalf("wav not written: %v", err)
	}

	dur, channels := probeWavDurationChannels(t, ffprobePath, wav)
	if channels != 1 {
		t.Fatalf("channels: got %d want 1 (mono)", channels)
	}
	if dur < 9.0 || dur > 11.0 {
		t.Fatalf("duration: got %f want ~10s (9-11)", dur)
	}
}

// TestExtractWindowNegativeSeekSSEOF covers the -sseof leg: a negative seek
// means "seek from end of file" (used for mp4 tails where duration is
// unknown) and must be emitted as -sseof instead of -ss. A 20s input with
// seek=-5, durSec=3 should yield a ~3s wav pulled from the last 5s of the
// clip.
func TestExtractWindowNegativeSeekSSEOF(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found on PATH — skipping real-ffmpeg integration test")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found on PATH — skipping real-ffmpeg integration test")
	}

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "sine.wav")
	genCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	gen := exec.CommandContext(genCtx, ffmpegPath,
		"-f", "lavfi", "-i", "sine=duration=20",
		"-y", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate synthetic sine input: %v\n%s", err, out)
	}

	dir := t.TempDir()
	extractCtx, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	wav, err := ExtractWindow(extractCtx, ffmpegPath, src, -5, 3, "tail", dir)
	if err != nil {
		t.Fatalf("ExtractWindow (sseof): %v", err)
	}
	if _, err := os.Stat(wav); err != nil {
		t.Fatalf("wav not written: %v", err)
	}

	dur, channels := probeWavDurationChannels(t, ffprobePath, wav)
	if channels != 1 {
		t.Fatalf("channels: got %d want 1 (mono)", channels)
	}
	if dur < 2.0 || dur > 4.0 {
		t.Fatalf("duration: got %f want ~3s (2-4)", dur)
	}
}
