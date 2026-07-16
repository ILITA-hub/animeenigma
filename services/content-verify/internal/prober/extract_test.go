package prober

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
