//go:build unix

package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript creates an executable shell script at path.
func writeScript(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write script %s: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// Fake ffmpeg scripts
// ---------------------------------------------------------------------------

// segmentArgvPrelude captures all argv into <outDir>/seg_argv.txt.
// outDir is determined from the last argument (the segment pattern
// {outDir}/seg_%05d.mkv) by taking its directory.
const segmentArgvPrelude = `#!/bin/sh
# Walk argv to find the output pattern (last arg).
SEGPAT=""
for a in "$@"; do SEGPAT="$a"; done
OUTDIR="$(dirname "$SEGPAT")"
mkdir -p "$OUTDIR"
# Record argv for the test to inspect.
: > "$OUTDIR/seg_argv.txt"
for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/seg_argv.txt"; done
`

// fakeFfmpegSegmentScript: when -f segment is present, writes two fake
// .mkv segment files and exits 0.
const fakeFfmpegSegmentScript = segmentArgvPrelude + `
# Check if this is a segmenter call (-f segment present).
SEGMENT_CALL=0
for a in "$@"; do
    if [ "$a" = "segment" ]; then SEGMENT_CALL=1; fi
done
if [ "$SEGMENT_CALL" = "1" ]; then
    echo "fake" > "$OUTDIR/seg_00000.mkv"
    echo "fake" > "$OUTDIR/seg_00001.mkv"
fi
exit 0
`

// demuxArgvPrelude captures argv into <outDir>/demux_argv.txt.
// For the audio/subs/chapters calls the last arg is the output path;
// we take its directory as OUTDIR.
const demuxArgvPrelude = `#!/bin/sh
LAST=""
for a in "$@"; do LAST="$a"; done
OUTDIR="$(dirname "$LAST")"
mkdir -p "$OUTDIR"
: > "$OUTDIR/demux_argv.txt"
for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/demux_argv.txt"; done
`

// fakeFfmpegDemuxScript: handles the three demux calls:
//   1. audio (-c:a copy → writes audio.mka)
//   2. subs  (-c:s copy → writes subs.mks)
//   3. chapters (-f ffmetadata → writes chapters.ini)
//   4. fonts  (-dump_attachment → creates no files; fonts dir is cwd)
//
// Because DemuxSidecars runs 4 separate ffmpeg invocations,
// each with a distinct last-arg, we write to the OUTDIR-derived path.
const fakeFfmpegDemuxScript = `#!/bin/sh
LAST=""
for a in "$@"; do LAST="$a"; done
OUTDIR="$(dirname "$LAST")"
mkdir -p "$OUTDIR"
# Append this call's argv to demux_argv.txt (multiple calls accumulate).
for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/demux_argv.txt"; done

# Audio call: last arg ends with .mka
case "$LAST" in
  *.mka) echo "fake audio" > "$LAST"; exit 0;;
esac

# Subs call: last arg ends with .mks
case "$LAST" in
  *.mks) echo "fake subs" > "$LAST"; exit 0;;
esac

# Chapters call: last arg ends with .ini
case "$LAST" in
  *.ini) echo "[CHAPTER]" > "$LAST"; exit 0;;
esac

# Font dump: no output file needed — DemuxSidecars uses cwd.
# The last arg is the source file path; cwd is the fonts/ dir.
# Write a fake font to simulate an attachment dump.
FONTSDIR="$(pwd)"
echo "fake font" > "$FONTSDIR/fake_font.ttf"
exit 0
`

// ---------------------------------------------------------------------------
// Tests: Segment
// ---------------------------------------------------------------------------

func TestSegment_ReturnsSortedPaths(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeFfmpegSegmentScript)

	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	segs, err := s.Segment(ctx, filepath.Join(dir, "src.mkv"), outDir, 45)
	if err != nil {
		t.Fatalf("Segment: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %v", len(segs), segs)
	}
	// Sorted: seg_00000.mkv < seg_00001.mkv
	if !strings.HasSuffix(segs[0], "seg_00000.mkv") {
		t.Errorf("segs[0] = %q, want ...seg_00000.mkv", segs[0])
	}
	if !strings.HasSuffix(segs[1], "seg_00001.mkv") {
		t.Errorf("segs[1] = %q, want ...seg_00001.mkv", segs[1])
	}
	// Both must be absolute.
	if !filepath.IsAbs(segs[0]) || !filepath.IsAbs(segs[1]) {
		t.Error("segment paths must be absolute")
	}
}

func TestSegment_ArgvContainsRequiredFlags(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeFfmpegSegmentScript)

	outDir := filepath.Join(dir, "out2")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.Segment(ctx, filepath.Join(dir, "src.mkv"), outDir, 45)
	if err != nil {
		t.Fatalf("Segment: %v", err)
	}

	argv, err := os.ReadFile(filepath.Join(outDir, "seg_argv.txt"))
	if err != nil {
		t.Fatalf("read seg_argv.txt: %v", err)
	}
	a := string(argv)

	required := []string{
		"-hide_banner",
		"-nostats",
		"-y",
		"-map", "0:v:0",
		"-c:v", "copy",
		"-an",
		"-sn",
		"-f", "segment",
		"-segment_time", "45",
		"-reset_timestamps", "1",
		"-segment_format", "matroska",
	}
	for _, tok := range required {
		if !strings.Contains(a, tok) {
			t.Errorf("argv missing %q\nfull argv:\n%s", tok, a)
		}
	}
}

func TestSegment_OutputPatternIs5DigitPadded(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeFfmpegSegmentScript)

	outDir := filepath.Join(dir, "out3")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	_, err := s.Segment(context.Background(), filepath.Join(dir, "src.mkv"), outDir, 30)
	if err != nil {
		t.Fatalf("Segment: %v", err)
	}

	argv, _ := os.ReadFile(filepath.Join(outDir, "seg_argv.txt"))
	if !strings.Contains(string(argv), "seg_%05d.mkv") {
		t.Errorf("output pattern must use %%05d; argv:\n%s", string(argv))
	}
}

func TestSegment_ErrorOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	failScript := `#!/bin/sh
echo "ffmpeg error" >&2
exit 1
`
	ffmpegBin := filepath.Join(dir, "fail_ffmpeg.sh")
	writeScript(t, ffmpegBin, failScript)

	s := NewSegmenter(ffmpegBin)
	outDir := filepath.Join(dir, "out_fail")
	_ = os.MkdirAll(outDir, 0o755)

	_, err := s.Segment(context.Background(), filepath.Join(dir, "src.mkv"), outDir, 45)
	if err == nil {
		t.Fatal("expected error from failing ffmpeg, got nil")
	}
	if !strings.Contains(err.Error(), "ffmpeg error") {
		t.Errorf("error should contain stderr tail; got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: DemuxSidecars
// ---------------------------------------------------------------------------

func TestDemuxSidecars_ArgvContainsAudioFlags(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeFfmpegDemuxScript)

	outDir := filepath.Join(dir, "demux_out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sidecars, err := s.DemuxSidecars(ctx, filepath.Join(dir, "src.mkv"), outDir)
	if err != nil {
		t.Fatalf("DemuxSidecars: %v", err)
	}

	// Audio file should exist.
	if sidecars.AudioPath == "" {
		t.Error("AudioPath should be set when audio.mka written")
	}
	if _, err := os.Stat(sidecars.AudioPath); err != nil {
		t.Errorf("AudioPath %q not found: %v", sidecars.AudioPath, err)
	}

	// Read accumulated argv from the audio call.
	argv, err := os.ReadFile(filepath.Join(outDir, "demux_argv.txt"))
	if err != nil {
		t.Fatalf("read demux_argv.txt: %v", err)
	}
	a := string(argv)

	// Audio-specific flags.
	audioRequired := []string{
		"-map", "0:a?",
		"-c:a", "copy",
	}
	for _, tok := range audioRequired {
		if !strings.Contains(a, tok) {
			t.Errorf("demux argv missing audio flag %q\nfull argv:\n%s", tok, a)
		}
	}

	// Sub-specific flags.
	subRequired := []string{
		"-map", "0:s?",
		"-c:s", "copy",
	}
	for _, tok := range subRequired {
		if !strings.Contains(a, tok) {
			t.Errorf("demux argv missing subs flag %q\nfull argv:\n%s", tok, a)
		}
	}

	// Chapters flag.
	if !strings.Contains(a, "-f") || !strings.Contains(a, "ffmetadata") {
		t.Errorf("demux argv missing chapters ffmetadata flag\nfull argv:\n%s", a)
	}
}

func TestDemuxSidecars_SubsAndChaptersSet(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeFfmpegDemuxScript)

	outDir := filepath.Join(dir, "demux_out2")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	sidecars, err := s.DemuxSidecars(context.Background(), filepath.Join(dir, "src.mkv"), outDir)
	if err != nil {
		t.Fatalf("DemuxSidecars: %v", err)
	}

	if sidecars.ChaptersPath == "" {
		t.Error("ChaptersPath should be set when chapters.ini written")
	}
	// SubPaths may be empty (depends on fake script producing subs.mks).
	// The subs file should exist if produced.
	subsPath := filepath.Join(outDir, "subs.mks")
	if _, err := os.Stat(subsPath); err == nil {
		// Subs file exists — SubPaths must contain it.
		found := false
		for _, sp := range sidecars.SubPaths {
			if sp == subsPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subs.mks exists but not in SubPaths: %v", sidecars.SubPaths)
		}
	}
}

func TestDemuxSidecars_FontsCollected(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeFfmpegDemuxScript)

	outDir := filepath.Join(dir, "demux_out3")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	sidecars, err := s.DemuxSidecars(context.Background(), filepath.Join(dir, "src.mkv"), outDir)
	if err != nil {
		t.Fatalf("DemuxSidecars: %v", err)
	}

	// Fake script writes fake_font.ttf into the fonts/ dir.
	fontsDir := filepath.Join(outDir, "fonts")
	if _, err := os.Stat(fontsDir); err != nil {
		t.Fatalf("fonts dir not created: %v", err)
	}
	if len(sidecars.FontPaths) == 0 {
		t.Error("FontPaths should be non-empty when font files are present")
	}
	for _, fp := range sidecars.FontPaths {
		if _, err := os.Stat(fp); err != nil {
			t.Errorf("FontPath %q not found: %v", fp, err)
		}
	}
}

func TestDemuxSidecars_EmptyOutputsNoError(t *testing.T) {
	dir := t.TempDir()
	// Script that succeeds but writes nothing.
	emptyScript := `#!/bin/sh
exit 0
`
	ffmpegBin := filepath.Join(dir, "fake_empty.sh")
	writeScript(t, ffmpegBin, emptyScript)

	outDir := filepath.Join(dir, "demux_empty")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	s := NewSegmenter(ffmpegBin)
	sidecars, err := s.DemuxSidecars(context.Background(), filepath.Join(dir, "src.mkv"), outDir)
	if err != nil {
		t.Fatalf("DemuxSidecars with empty outputs: %v", err)
	}
	// All fields should be empty/nil — not an error.
	if sidecars.AudioPath != "" {
		t.Errorf("expected empty AudioPath, got %q", sidecars.AudioPath)
	}
	if len(sidecars.SubPaths) != 0 {
		t.Errorf("expected empty SubPaths, got %v", sidecars.SubPaths)
	}
	if len(sidecars.FontPaths) != 0 {
		t.Errorf("expected empty FontPaths, got %v", sidecars.FontPaths)
	}
	if sidecars.ChaptersPath != "" {
		t.Errorf("expected empty ChaptersPath, got %q", sidecars.ChaptersPath)
	}
}
