package upscale

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// makeFakeFFmpeg writes a fake ffmpeg shell script to dir and returns its path.
//
// The script distinguishes decode from encode by checking for the "-f matroska"
// flag, which is only present in the encode invocation.
//
// Decode mode (no "-f matroska" flag):
//
//	Extracts the output pattern directory (the arg after all flags whose value
//	ends with %06d.ppm) and writes 3 synthetic PPM files there.
//
// Encode mode ("-f matroska" present):
//
//	Writes a small sentinel file to the last argument (outSegPath).
//
// NOTE: tests that use this helper must NOT call t.Parallel() because they
// mutate the package-level FFmpegBin variable.
func makeFakeFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	// Use "-f matroska" as the discriminator: decode never passes -f matroska.
	script := `#!/bin/sh
# Fake ffmpeg for pipeline tests.
# Discriminate decode vs encode by the -f matroska flag.
IS_ENCODE=0
for arg in "$@"; do
  case "$arg" in
    matroska) IS_ENCODE=1 ;;
  esac
done

if [ "$IS_ENCODE" = "1" ]; then
  # Encode mode: last argument is the output file.
  OUTPUT=""
  for arg in "$@"; do
    OUTPUT="$arg"
  done
  printf 'fake-matroska' > "$OUTPUT"
  exit 0
else
  # Decode mode: find the arg ending in %06d.ppm (the output pattern).
  for arg in "$@"; do
    case "$arg" in
      *%06d.ppm)
        DIR=$(dirname "$arg")
        mkdir -p "$DIR"
        printf 'P6\n1 1\n255\n\000\000\000' > "$DIR/000001.ppm"
        printf 'P6\n1 1\n255\n\000\000\000' > "$DIR/000002.ppm"
        printf 'P6\n1 1\n255\n\000\000\000' > "$DIR/000003.ppm"
        exit 0
        ;;
    esac
  done
  exit 1
fi
`
	bin := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return bin
}

// setFakeFFmpeg sets FFmpegBin to a fake binary and registers a cleanup to
// restore the original value. Must NOT be called from parallel tests.
func setFakeFFmpeg(t *testing.T) {
	t.Helper()
	orig := FFmpegBin
	FFmpegBin = makeFakeFFmpeg(t, t.TempDir())
	t.Cleanup(func() { FFmpegBin = orig })
}

// TestProcess_OutputSegmentCreated verifies that Process creates the outSegPath.
// Sequential (no t.Parallel) because it mutates the package-level FFmpegBin.
func TestProcess_OutputSegmentCreated(t *testing.T) {
	setFakeFFmpeg(t)

	tmpDir := t.TempDir()
	inSeg := filepath.Join(tmpDir, "input.mkv")
	outSeg := filepath.Join(tmpDir, "output.mkv")
	if err := os.WriteFile(inSeg, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Get("mock")
	if err != nil {
		t.Fatalf("Get(mock): %v", err)
	}

	_, err = Process(context.Background(), inSeg, outSeg, m, 2, tmpDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if _, err := os.Stat(outSeg); os.IsNotExist(err) {
		t.Error("expected outSegPath to exist after Process")
	}
}

// TestProcess_FrameCount verifies that Stats.Frames equals the number of frames
// the fake ffmpeg produces (3).
// Sequential (no t.Parallel) because it mutates the package-level FFmpegBin.
func TestProcess_FrameCount(t *testing.T) {
	setFakeFFmpeg(t)

	tmpDir := t.TempDir()
	inSeg := filepath.Join(tmpDir, "input.mkv")
	outSeg := filepath.Join(tmpDir, "output.mkv")
	if err := os.WriteFile(inSeg, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, _ := Get("mock")
	stats, err := Process(context.Background(), inSeg, outSeg, m, 2, tmpDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if stats.Frames != 3 {
		t.Errorf("Stats.Frames = %d, want 3", stats.Frames)
	}
}

// TestProcess_FPSNonNegative verifies that all FPS fields are >= 0.
// Sequential (no t.Parallel) because it mutates the package-level FFmpegBin.
func TestProcess_FPSNonNegative(t *testing.T) {
	setFakeFFmpeg(t)

	tmpDir := t.TempDir()
	inSeg := filepath.Join(tmpDir, "input.mkv")
	outSeg := filepath.Join(tmpDir, "output.mkv")
	if err := os.WriteFile(inSeg, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, _ := Get("mock")
	stats, err := Process(context.Background(), inSeg, outSeg, m, 2, tmpDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if stats.DecodeFPS < 0 {
		t.Errorf("DecodeFPS = %f, want >= 0", stats.DecodeFPS)
	}
	if stats.InferenceFPS < 0 {
		t.Errorf("InferenceFPS = %f, want >= 0", stats.InferenceFPS)
	}
	if stats.EncodeFPS < 0 {
		t.Errorf("EncodeFPS = %f, want >= 0", stats.EncodeFPS)
	}
}

// TestProcess_TempDirsCleanedUp verifies that the temp frames-in and frames-out
// directories are removed after Process returns.
// Sequential (no t.Parallel) because it mutates the package-level FFmpegBin.
func TestProcess_TempDirsCleanedUp(t *testing.T) {
	setFakeFFmpeg(t)

	// Use a dedicated workDir so we can enumerate its contents after Process.
	workDir := t.TempDir()
	inSeg := filepath.Join(workDir, "input.mkv")
	outSeg := filepath.Join(workDir, "output.mkv")
	if err := os.WriteFile(inSeg, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, _ := Get("mock")
	_, err := Process(context.Background(), inSeg, outSeg, m, 2, workDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatalf("ReadDir workDir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			// frames-in-* and frames-out-* dirs must be cleaned up.
			t.Errorf("unexpected temp dir remaining in workDir: %s", e.Name())
		}
	}
}

// TestProcess_ContextCancellation verifies that cancelling the context before
// Process starts returns an error (ffmpeg is killed via context).
// Sequential (no t.Parallel) because it mutates the package-level FFmpegBin.
func TestProcess_ContextCancellation(t *testing.T) {
	setFakeFFmpeg(t)

	tmpDir := t.TempDir()
	inSeg := filepath.Join(tmpDir, "input.mkv")
	outSeg := filepath.Join(tmpDir, "output.mkv")
	if err := os.WriteFile(inSeg, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before any ffmpeg call

	m, _ := Get("mock")
	_, err := Process(ctx, inSeg, outSeg, m, 2, tmpDir)
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}
}
