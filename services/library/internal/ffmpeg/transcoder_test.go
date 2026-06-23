//go:build unix

package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript creates a shell script that emulates ffmpeg/ffprobe for
// testing the wrapper without depending on a real binary. The script
// writes its argv to argsFile (sidecar), optionally writes fake output
// segments + playlist into the directory specified by
// `-hls_segment_filename`, and exits with the configured code.
func writeScript(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write script %s: %v", path, err)
	}
}

// Scripts use POSIX sh idioms (last-arg via `eval`) so they work
// under /bin/sh whether dash or bash is the underlying interpreter.

// fakeFfprobeScript emits a static JSON blob with duration=1450.5 +
// bitrate=3200000 (3200 kbps after /1000). The transcoder must then
// choose bv = min(3200, MaxBitrateKbps).
const fakeFfprobeScript = `#!/bin/sh
cat <<'JSON'
{"format":{"duration":"1450.5","bit_rate":"3200000"}}
JSON
`

// lastArg in POSIX sh: shift down to the final argument via `eval`.
// We capture it into PLAYLIST.
const lastArgPrelude = `# Walk argv to its last element.
PLAYLIST=""
for a in "$@"; do PLAYLIST="$a"; done
OUTDIR="$(dirname "$PLAYLIST")"
mkdir -p "$OUTDIR"
# Record argv for the test to read.
: > "$OUTDIR/argv.txt"
for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/argv.txt"; done
`

// fakeFfmpegSucceedScript writes argv to a sidecar file in the dir
// containing playlist.m3u8, creates fake playlist + 6 segments, exits 0.
const fakeFfmpegSucceedScript = `#!/bin/sh
` + lastArgPrelude + `
echo "#EXTM3U" > "$PLAYLIST"
for i in 1 2 3 4 5 6; do
    NUM=$(printf "%03d" $i)
    echo "fake segment $NUM" > "$OUTDIR/segment_${NUM}.ts"
done
exit 0
`

// fakeFfmpegFailScript emits a long stderr line and exits 1.
const fakeFfmpegFailScript = `#!/bin/sh
` + lastArgPrelude + `
echo "Invalid data found when processing input" 1>&2
echo "ffmpeg failed at stream 0" 1>&2
exit 1
`

// fakeFfmpegHugeStderrScript emits many KB of stderr to exercise the
// ring buffer overflow path.
const fakeFfmpegHugeStderrScript = `#!/bin/sh
` + lastArgPrelude + `
i=0
while [ $i -lt 200 ]; do
    echo "stderr-line-$(printf '%05d' $i) padding padding padding padding padding padding" 1>&2
    i=$((i+1))
done
echo "FINAL_MARKER_END" 1>&2
exit 1
`

func TestTranscode_SuccessPath(t *testing.T) {
	dir := t.TempDir()
	ffprobeBin := filepath.Join(dir, "fake_ffprobe.sh")
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffprobeBin, fakeFfprobeScript)
	writeScript(t, ffmpegBin, fakeFfmpegSucceedScript)

	tmpdir := filepath.Join(dir, "tmp")
	tr := NewTranscoder(Config{
		BinaryPath:     ffmpegBin,
		FfprobePath:    ffprobeBin,
		Tmpdir:         tmpdir,
		MaxBitrateKbps: 5000,
	}, nil)

	source := filepath.Join(dir, "fake_source.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := tr.Transcode(ctx, source)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	if res.PlaylistPath == "" {
		t.Fatalf("PlaylistPath empty")
	}
	if len(res.SegmentPaths) != 6 {
		t.Fatalf("SegmentPaths len = %d, want 6", len(res.SegmentPaths))
	}
	if res.DurationSec != 1450 {
		t.Fatalf("DurationSec = %d, want 1450 (from ffprobe)", res.DurationSec)
	}
	if res.SizeBytes <= 0 {
		t.Fatalf("SizeBytes = %d, want > 0", res.SizeBytes)
	}
	// Verify argv captured by the fake binary contains every SPEC-locked flag,
	// including the chosen bv = min(3200, 5000) = 3200.
	argv, err := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	if err != nil {
		t.Fatalf("argv.txt: %v", err)
	}
	a := string(argv)
	wanted := []string{
		"-hide_banner", "-nostats", "-y",
		"-c:v", "libx264", "-preset", "veryfast",
		"-b:v", "3200k",
		"-maxrate", "3200k",
		"-bufsize", "6400k",
		"-c:a", "aac", "-b:a", "128k",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
	}
	for _, w := range wanted {
		if !strings.Contains(a, w) {
			t.Errorf("argv missing %q\nfull argv:\n%s", w, a)
		}
	}
	// Temp dir MUST NOT be cleaned by Transcode on success (caller does it).
	if _, err := os.Stat(res.PlaylistPath); err != nil {
		t.Errorf("playlist must exist post-Transcode: %v", err)
	}
}

func TestTranscode_FailurePath_StderrTailIncluded(t *testing.T) {
	dir := t.TempDir()
	ffprobeBin := filepath.Join(dir, "fake_ffprobe.sh")
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg_fail.sh")
	writeScript(t, ffprobeBin, fakeFfprobeScript)
	writeScript(t, ffmpegBin, fakeFfmpegFailScript)

	tr := NewTranscoder(Config{
		BinaryPath:     ffmpegBin,
		FfprobePath:    ffprobeBin,
		Tmpdir:         dir,
		MaxBitrateKbps: 5000,
	}, nil)

	source := filepath.Join(dir, "fake_source.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := tr.Transcode(context.Background(), source)
	if err == nil {
		t.Fatalf("expected non-nil error on ffmpeg failure")
	}
	if !strings.Contains(err.Error(), "Invalid data found when processing input") {
		t.Errorf("error must contain stderr tail; got: %v", err)
	}

	// Temp dir must NOT be cleaned on failure — list dir contents.
	entries, _ := os.ReadDir(dir)
	foundEncodeDir := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "encode-") {
			foundEncodeDir = true
			break
		}
	}
	if !foundEncodeDir {
		t.Errorf("encode-* temp dir must be retained on failure for admin debugging")
	}
}

func TestTranscode_StderrRingBufferOverflow(t *testing.T) {
	dir := t.TempDir()
	ffprobeBin := filepath.Join(dir, "fake_ffprobe.sh")
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg_huge.sh")
	writeScript(t, ffprobeBin, fakeFfprobeScript)
	writeScript(t, ffmpegBin, fakeFfmpegHugeStderrScript)

	tr := NewTranscoder(Config{
		BinaryPath:     ffmpegBin,
		FfprobePath:    ffprobeBin,
		Tmpdir:         dir,
		MaxBitrateKbps: 5000,
	}, nil)

	source := filepath.Join(dir, "src.mp4")
	if err := os.WriteFile(source, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := tr.Transcode(context.Background(), source)
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	// FINAL_MARKER_END must be in the tail (within the last 2 KB).
	if !strings.Contains(msg, "FINAL_MARKER_END") {
		t.Errorf("ring buffer should preserve final stderr line; got tail:\n%s", msg)
	}
	// Error message length is bounded — stderr capture must NOT include
	// all 200 lines. Each line is ~80 bytes so 200 lines = ~16 KB. We
	// expect at most ~2 KB of tail + framing.
	if len(msg) > 5000 {
		t.Errorf("error message too large (%d bytes) — ring buffer not bounding capture", len(msg))
	}
}

func TestRingBuffer_WritesUnderCap(t *testing.T) {
	r := newRingBuffer(100)
	_, _ = r.Write([]byte("hello"))
	_, _ = r.Write([]byte(" world"))
	if got := r.String(); got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestRingBuffer_WritesOverflow(t *testing.T) {
	r := newRingBuffer(10)
	_, _ = r.Write([]byte("0123456789ABCDEFGHIJ"))
	got := r.String()
	if len(got) != 10 {
		t.Fatalf("len = %d, want 10", len(got))
	}
	if got != "ABCDEFGHIJ" {
		t.Fatalf("got %q, want last 10 bytes %q", got, "ABCDEFGHIJ")
	}
}

func TestRingBuffer_MultiWriteOverflow(t *testing.T) {
	r := newRingBuffer(5)
	for i := 0; i < 10; i++ {
		_, _ = r.Write([]byte(fmt.Sprintf("%d", i)))
	}
	got := r.String()
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if got != "56789" {
		t.Fatalf("got %q, want last 5 digits %q", got, "56789")
	}
}

func TestTranscode_ProbeFallback_DefaultBitrateUsed(t *testing.T) {
	dir := t.TempDir()
	// ffprobe script that returns an empty (parsable) JSON → durationSec=0, sourceKbps=0.
	emptyProbe := `#!/bin/sh
echo '{}'
`
	writeScript(t, filepath.Join(dir, "ep.sh"), emptyProbe)
	writeScript(t, filepath.Join(dir, "fm.sh"), fakeFfmpegSucceedScript)
	tr := NewTranscoder(Config{
		BinaryPath:     filepath.Join(dir, "fm.sh"),
		FfprobePath:    filepath.Join(dir, "ep.sh"),
		Tmpdir:         dir,
		MaxBitrateKbps: 5000,
	}, nil)
	src := filepath.Join(dir, "s.mp4")
	_ = os.WriteFile(src, []byte("s"), 0o644)
	res, err := tr.Transcode(context.Background(), src)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	// With no source bitrate, bv falls back to MaxBitrateKbps = 5000.
	if !strings.Contains(string(argv), "5000k") {
		t.Errorf("expected bv=5000k when ffprobe returns empty; argv:\n%s", string(argv))
	}
}

func TestTranscode_BitrateFloor(t *testing.T) {
	dir := t.TempDir()
	// ffprobe with tiny bitrate (100 kbps). Floor should bump to 500.
	tinyProbe := `#!/bin/sh
echo '{"format":{"duration":"60","bit_rate":"100000"}}'
`
	writeScript(t, filepath.Join(dir, "ep.sh"), tinyProbe)
	writeScript(t, filepath.Join(dir, "fm.sh"), fakeFfmpegSucceedScript)
	tr := NewTranscoder(Config{
		BinaryPath:     filepath.Join(dir, "fm.sh"),
		FfprobePath:    filepath.Join(dir, "ep.sh"),
		Tmpdir:         dir,
		MaxBitrateKbps: 5000,
	}, nil)
	src := filepath.Join(dir, "s.mp4")
	_ = os.WriteFile(src, []byte("s"), 0o644)
	res, err := tr.Transcode(context.Background(), src)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	if !strings.Contains(string(argv), "500k") {
		t.Errorf("expected bv floored to 500k; argv:\n%s", string(argv))
	}
}

func TestTranscode_ThreadsFlagEmittedWhenSet(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg.sh")
	ffprobe := filepath.Join(dir, "ffprobe.sh")
	writeScript(t, ffmpeg, fakeFfmpegSucceedScript)
	writeScript(t, ffprobe, fakeFfprobeScript)
	source := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewTranscoder(Config{
		BinaryPath: ffmpeg, FfprobePath: ffprobe, Tmpdir: dir,
		MaxBitrateKbps: 5000, Threads: 3,
	}, nil)
	res, err := tr.Transcode(context.Background(), source)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	argv, err := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(argv)), "\n")
	if !hasAdjacent(lines, "-threads", "3") {
		t.Fatalf("argv missing `-threads 3`: %v", lines)
	}
}

func TestTranscode_ThreadsFlagOmittedWhenZero(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg.sh")
	ffprobe := filepath.Join(dir, "ffprobe.sh")
	writeScript(t, ffmpeg, fakeFfmpegSucceedScript)
	writeScript(t, ffprobe, fakeFfprobeScript)
	source := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewTranscoder(Config{
		BinaryPath: ffmpeg, FfprobePath: ffprobe, Tmpdir: dir,
		MaxBitrateKbps: 5000, Threads: 0,
	}, nil)
	res, err := tr.Transcode(context.Background(), source)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	argv, err := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	for _, l := range strings.Split(string(argv), "\n") {
		if l == "-threads" {
			t.Fatalf("argv should not contain -threads when Threads=0: %s", string(argv))
		}
	}
}

// hasAdjacent reports whether lines contains a, immediately followed by b.
func hasAdjacent(lines []string, a, b string) bool {
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == a && lines[i+1] == b {
			return true
		}
	}
	return false
}
