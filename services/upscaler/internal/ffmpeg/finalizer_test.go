//go:build unix

package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/source"
)

// ---------------------------------------------------------------------------
// Fake ffmpeg scripts for Finalizer
// ---------------------------------------------------------------------------

// fakeFfmpegFinalizer:
//   - On concat call (-f concat present): write concat_argv.txt + create a fake
//     {tmp}/video.mkv in the directory of the output arg.
//   - On remux call (-hls_time present): write remux_argv.txt + create fake
//     playlist.m3u8 + segment_000.ts in the directory of the last arg (playlist).
//
// argv recording: the script writes to a file derived from the OUTPUT argument.
// Concat: last arg is {tmp}/video.mkv → {tmp}/concat_argv.txt.
// Remux: last arg is {out}/playlist.m3u8 → {out}/remux_argv.txt.
const fakeFfmpegFinalizerScript = `#!/bin/sh
# Determine which call this is by scanning argv for -f concat or -hls_time.
IS_CONCAT=0
IS_REMUX=0
for a in "$@"; do
  case "$a" in
    concat) IS_CONCAT=1;;
    -hls_time) IS_REMUX=1;;
  esac
done

# Get the last argument (output path).
LAST=""
for a in "$@"; do LAST="$a"; done
OUTDIR="$(dirname "$LAST")"
mkdir -p "$OUTDIR"

if [ "$IS_CONCAT" = "1" ]; then
  # Record concat argv.
  : > "$OUTDIR/concat_argv.txt"
  for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/concat_argv.txt"; done
  # Produce fake video.mkv (last arg).
  echo "fake mkv" > "$LAST"
  exit 0
fi

if [ "$IS_REMUX" = "1" ]; then
  # Record remux argv.
  : > "$OUTDIR/remux_argv.txt"
  for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/remux_argv.txt"; done
  # Produce fake HLS output.
  echo "#EXTM3U" > "$OUTDIR/playlist.m3u8"
  echo "fake ts" > "$OUTDIR/segment_000.ts"
  exit 0
fi

exit 0
`

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeFakeScript(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write script %s: %v", path, err)
	}
}

func writeFakeSegments(t *testing.T, dir string, names []string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("fake mkv data"), 0o644); err != nil {
			t.Fatalf("write segment %s: %v", n, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFinalizer_Concat_ArgvHasRequiredFlags(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	// Create fake upscaled segment files.
	segDir := filepath.Join(dir, "segs")
	if err := os.MkdirAll(segDir, 0o755); err != nil {
		t.Fatalf("mkdir segDir: %v", err)
	}
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv", "seg_00001.mkv", "seg_00002.mkv"})

	// Use outDir as work dir so concat_argv.txt survives after Concat returns.
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	sc := Sidecars{AudioPath: filepath.Join(dir, "audio.mka"), SubPaths: []string{filepath.Join(dir, "subs.mks")}}

	f := NewFinalizer(ffmpegBin).withWorkDir(outDir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := f.Concat(ctx, segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	// concat_argv.txt is in outDir (which is the work dir for this test).
	concatArgvPath := filepath.Join(outDir, "concat_argv.txt")
	data, err := os.ReadFile(concatArgvPath)
	if err != nil {
		t.Fatalf("concat_argv.txt not found in outDir: %v", err)
	}
	concatArgv := string(data)

	required := []string{"-f", "concat", "-safe", "0", "-c:v", "copy"}
	for _, tok := range required {
		if !strings.Contains(concatArgv, tok) {
			t.Errorf("concat argv missing %q\nfull:\n%s", tok, concatArgv)
		}
	}
}

func TestFinalizer_ConcatTxt_HasSortedFileLines(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	if err := os.MkdirAll(segDir, 0o755); err != nil {
		t.Fatalf("mkdir segDir: %v", err)
	}
	// Write out of lexical order to confirm the finalizer sorts them.
	writeFakeSegments(t, segDir, []string{"seg_00002.mkv", "seg_00000.mkv", "seg_00001.mkv"})

	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	sc := Sidecars{}

	// Use outDir as work dir so concat.txt survives after Concat returns.
	f := NewFinalizer(ffmpegBin).withWorkDir(outDir)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	concatTxtPath := filepath.Join(outDir, "concat.txt")
	data, err := os.ReadFile(concatTxtPath)
	if err != nil {
		t.Fatalf("read concat.txt: %v", err)
	}
	content := string(data)

	// Each line must be: file 'seg_NNNNN.mkv'
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 3 {
		t.Fatalf("concat.txt lines = %d, want 3:\n%s", len(lines), content)
	}
	// Lines must appear in sorted order.
	wantOrder := []string{"seg_00000.mkv", "seg_00001.mkv", "seg_00002.mkv"}
	for i, want := range wantOrder {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want to contain %q", i, lines[i], want)
		}
		if !strings.HasPrefix(lines[i], "file '") {
			t.Errorf("line %d = %q, want `file '...'` format", i, lines[i])
		}
	}
}

func TestFinalizer_Remux_ArgvHasLibx264AndHLS(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	if err := os.MkdirAll(segDir, 0o755); err != nil {
		t.Fatalf("mkdir segDir: %v", err)
	}
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv"})

	outDir := filepath.Join(dir, "out")
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	sc := Sidecars{
		AudioPath: filepath.Join(dir, "audio.mka"),
		SubPaths:  []string{filepath.Join(dir, "subs.mks")},
	}

	f := NewFinalizer(ffmpegBin)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	remuxArgvPath := filepath.Join(outDir, "remux_argv.txt")
	data, err := os.ReadFile(remuxArgvPath)
	if err != nil {
		t.Fatalf("remux_argv.txt not found in outDir: %v", err)
	}
	a := string(data)

	required := []string{
		"-c:v", "libx264",
		"-crf", "18",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"playlist.m3u8",
	}
	for _, tok := range required {
		if !strings.Contains(a, tok) {
			t.Errorf("remux argv missing %q\nfull:\n%s", tok, a)
		}
	}
}

func TestFinalizer_Remux_PixFmt_8bit(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	_ = os.MkdirAll(segDir, 0o755)
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv"})

	outDir := filepath.Join(dir, "out")
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	sc := Sidecars{}

	f := NewFinalizer(ffmpegBin)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "remux_argv.txt"))
	if err != nil {
		t.Fatalf("remux_argv.txt: %v", err)
	}
	a := string(data)
	if !strings.Contains(a, "yuv420p") {
		t.Errorf("8-bit source: expected -pix_fmt yuv420p; argv:\n%s", a)
	}
	if strings.Contains(a, "yuv420p10le") {
		t.Errorf("8-bit source: must NOT use yuv420p10le; argv:\n%s", a)
	}
}

func TestFinalizer_Remux_PixFmt_10bit(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	_ = os.MkdirAll(segDir, 0o755)
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv"})

	outDir := filepath.Join(dir, "out10")
	probe := source.ProbeResult{PixFmt: "yuv420p10le", FPS: "24000/1001"}
	sc := Sidecars{}

	f := NewFinalizer(ffmpegBin)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "remux_argv.txt"))
	if err != nil {
		t.Fatalf("remux_argv.txt: %v", err)
	}
	a := string(data)
	if !strings.Contains(a, "yuv420p10le") {
		t.Errorf("10-bit source: expected -pix_fmt yuv420p10le; argv:\n%s", a)
	}
}

func TestFinalizer_Remux_NoAudio_OmitsAudioInput(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	_ = os.MkdirAll(segDir, 0o755)
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv"})

	outDir := filepath.Join(dir, "out_noaudio")
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	// Empty AudioPath: no audio sidecar.
	sc := Sidecars{AudioPath: ""}

	f := NewFinalizer(ffmpegBin)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "remux_argv.txt"))
	if err != nil {
		t.Fatalf("remux_argv.txt: %v", err)
	}
	a := string(data)

	// With no audio sidecar there must be no second -i and no -map 1:a?
	lines := strings.Split(a, "\n")
	inputCount := 0
	for _, l := range lines {
		if l == "-i" {
			inputCount++
		}
	}
	// Only one -i: the video.mkv from concat step.
	if inputCount != 1 {
		t.Errorf("no-audio case: expected 1 -i, got %d; argv:\n%s", inputCount, a)
	}
	if strings.Contains(a, "1:a?") {
		t.Errorf("no-audio case: argv must NOT contain -map 1:a?; argv:\n%s", a)
	}
}

func TestFinalizer_Remux_NoSubs_OmitsSubsInput(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	_ = os.MkdirAll(segDir, 0o755)
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv"})

	outDir := filepath.Join(dir, "out_nosubs")
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	// Audio present, no subs.
	sc := Sidecars{AudioPath: filepath.Join(dir, "audio.mka"), SubPaths: nil}

	f := NewFinalizer(ffmpegBin)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "remux_argv.txt"))
	if err != nil {
		t.Fatalf("remux_argv.txt: %v", err)
	}
	a := string(data)

	lines := strings.Split(a, "\n")
	inputCount := 0
	for _, l := range lines {
		if l == "-i" {
			inputCount++
		}
	}
	// video.mkv + audio.mka = 2 inputs; no subs.
	if inputCount != 2 {
		t.Errorf("no-subs case: expected 2 -i, got %d; argv:\n%s", inputCount, a)
	}
	if strings.Contains(a, "2:s?") {
		t.Errorf("no-subs case: argv must NOT contain -map 2:s?; argv:\n%s", a)
	}
}

func TestFinalizer_Remux_ProducesHLSOutputFiles(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeFakeScript(t, ffmpegBin, fakeFfmpegFinalizerScript)

	segDir := filepath.Join(dir, "segs")
	_ = os.MkdirAll(segDir, 0o755)
	writeFakeSegments(t, segDir, []string{"seg_00000.mkv"})

	outDir := filepath.Join(dir, "hls_out")
	probe := source.ProbeResult{PixFmt: "yuv420p", FPS: "24/1"}
	sc := Sidecars{}

	f := NewFinalizer(ffmpegBin)
	if err := f.Concat(context.Background(), segDir, sc, probe, outDir); err != nil {
		t.Fatalf("Concat: %v", err)
	}

	// Fake script writes playlist.m3u8 + segment_000.ts in outDir.
	for _, want := range []string{"playlist.m3u8", "segment_000.ts"} {
		p := filepath.Join(outDir, want)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected output file %s not found: %v", want, err)
		}
	}
}
