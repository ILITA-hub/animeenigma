//go:build unix

package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fake ffmpeg: records argv, writes 2 sheet JPEGs next to the output pattern (last arg).
const fakeStoryboardFfmpeg = `#!/bin/sh
` + lastArgPrelude + `
N=$(printf "%03d" 1); echo "jpg" > "$OUTDIR/storyboard_${N}.jpg"
N=$(printf "%03d" 2); echo "jpg" > "$OUTDIR/storyboard_${N}.jpg"
exit 0
`

func TestBuildStoryboardVTT_TwelveSeconds(t *testing.T) {
	got := BuildStoryboardVTT(12)
	if !strings.HasPrefix(got, "WEBVTT\n") {
		t.Fatalf("missing WEBVTT header:\n%s", got)
	}
	// ceil(12/5) = 3 cues; last cue clamps to 12s.
	for _, want := range []string{
		"00:00:00.000 --> 00:00:05.000\nstoryboard_001.jpg#xywh=0,0,160,90",
		"00:00:05.000 --> 00:00:10.000\nstoryboard_001.jpg#xywh=160,0,160,90",
		"00:00:10.000 --> 00:00:12.000\nstoryboard_001.jpg#xywh=320,0,160,90",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing cue %q in:\n%s", want, got)
		}
	}
}

func TestBuildStoryboardVTT_SecondSheetAndRowWrap(t *testing.T) {
	// frame index 100 (t=500s) is the first cell of sheet 2; index 10 (t=50s) wraps to row 2.
	got := BuildStoryboardVTT(520)
	if !strings.Contains(got, "00:08:20.000 --> 00:08:25.000\nstoryboard_002.jpg#xywh=0,0,160,90") {
		t.Errorf("frame 100 must land on sheet 2 cell (0,0):\n%s", got)
	}
	if !strings.Contains(got, "00:00:50.000 --> 00:00:55.000\nstoryboard_001.jpg#xywh=0,90,160,90") {
		t.Errorf("frame 10 must wrap to row 2 (y=90):\n%s", got)
	}
}

func TestBuildStoryboardVTT_NonPositiveDuration(t *testing.T) {
	if got := BuildStoryboardVTT(0); got != "" {
		t.Fatalf("want empty VTT for 0 duration, got %q", got)
	}
}

func TestStoryboard_SuccessPath(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeStoryboardFfmpeg)
	tr := NewTranscoder(Config{BinaryPath: ffmpegBin, FfprobePath: "/bin/true", Tmpdir: dir}, nil)
	src := filepath.Join(dir, "src.mp4")
	if err := os.WriteFile(src, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tr.Storyboard(context.Background(), src, 900)
	if err != nil {
		t.Fatalf("Storyboard: %v", err)
	}
	if len(res.SheetPaths) != 2 {
		t.Fatalf("SheetPaths len = %d, want 2", len(res.SheetPaths))
	}
	if filepath.Base(res.VTTPath) != StoryboardVTTName {
		t.Fatalf("VTTPath = %q", res.VTTPath)
	}
	vtt, err := os.ReadFile(res.VTTPath)
	if err != nil || !strings.HasPrefix(string(vtt), "WEBVTT") {
		t.Fatalf("VTT file must exist with header: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(filepath.Dir(res.VTTPath), "argv.txt"))
	for _, w := range []string{"fps=1/5", "tile=10x10", "-q:v", "storyboard_%03d.jpg"} {
		if !strings.Contains(string(argv), w) {
			t.Errorf("argv missing %q:\n%s", w, string(argv))
		}
	}
}

func TestStoryboard_FfmpegFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fail.sh")
	writeScript(t, ffmpegBin, fakeFfmpegFailScript)
	tr := NewTranscoder(Config{BinaryPath: ffmpegBin, FfprobePath: "/bin/true", Tmpdir: dir}, nil)
	src := filepath.Join(dir, "src.mp4")
	_ = os.WriteFile(src, []byte("s"), 0o644)
	if _, err := tr.Storyboard(context.Background(), src, 900); err == nil {
		t.Fatal("expected error on ffmpeg failure")
	}

	// Unlike Transcode (which RETAINS its per-call dir on failure so admins can
	// inspect a failed encode job), the storyboard pass is best-effort +
	// high-frequency: it MUST clean its temp dir on every error path so junk
	// dirs don't accumulate. Assert no storyboard-* dir survives.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read tmpdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "storyboard-") {
			t.Errorf("storyboard-* temp dir must be cleaned on ffmpeg failure, found %q", e.Name())
		}
	}
}

// fakeFfprobeWithDuration emits a JSON blob with the specified duration (in seconds).
func fakeFfprobeScriptWithDuration(durationSec string) string {
	return `#!/bin/sh
cat <<'JSON'
{"format":{"duration":"` + durationSec + `","bit_rate":"1000000"}}
JSON
`
}

// fakeFfprobeEmptyScript returns an empty JSON object (no duration).
const fakeFfprobeEmptyScript = `#!/bin/sh
cat <<'JSON'
{}
JSON
`

func TestStoryboard_SelfProbesWhenDurationUnknown(t *testing.T) {
	dir := t.TempDir()
	ffprobeBin := filepath.Join(dir, "fake_ffprobe.sh")
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffprobeBin, fakeFfprobeScriptWithDuration("12.0"))
	writeScript(t, ffmpegBin, fakeStoryboardFfmpeg)

	tr := NewTranscoder(Config{
		BinaryPath:  ffmpegBin,
		FfprobePath: ffprobeBin,
		Tmpdir:      dir,
	}, nil)

	src := filepath.Join(dir, "src.mp4")
	if err := os.WriteFile(src, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Call with durationSec=0 to force self-probe
	res, err := tr.Storyboard(context.Background(), src, 0)
	if err != nil {
		t.Fatalf("Storyboard: %v", err)
	}

	// Verify the VTT was generated for a 12-second video (ceil(12/5) = 3 cues)
	vtt, err := os.ReadFile(res.VTTPath)
	if err != nil {
		t.Fatalf("read VTT: %v", err)
	}
	vttStr := string(vtt)
	if !strings.Contains(vttStr, "00:00:10.000 --> 00:00:12.000") {
		t.Errorf("missing final cue for 12s duration in:\n%s", vttStr)
	}
	// Ensure we don't have a 4th cue (which would start at 00:00:15)
	if strings.Contains(vttStr, "00:00:15.000") {
		t.Errorf("should not have cue at 15s for 12s duration:\n%s", vttStr)
	}
}

func TestStoryboard_ErrorWhenDurationStillUnknown(t *testing.T) {
	dir := t.TempDir()
	ffprobeBin := filepath.Join(dir, "fake_ffprobe.sh")
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffprobeBin, fakeFfprobeEmptyScript)
	writeScript(t, ffmpegBin, fakeStoryboardFfmpeg)

	tr := NewTranscoder(Config{
		BinaryPath:  ffmpegBin,
		FfprobePath: ffprobeBin,
		Tmpdir:      dir,
	}, nil)

	src := filepath.Join(dir, "src.mp4")
	if err := os.WriteFile(src, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Call with durationSec=0; ffprobe returns empty, should fail
	_, err := tr.Storyboard(context.Background(), src, 0)
	if err == nil {
		t.Fatal("expected error when duration is unknown")
	}
	if !strings.Contains(err.Error(), "unknown duration") {
		t.Errorf("error should mention 'unknown duration', got: %v", err)
	}
}
