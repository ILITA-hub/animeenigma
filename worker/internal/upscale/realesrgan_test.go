//go:build unix

package upscale

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScript creates an executable shell script at path.
func writeScript(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write script %s: %v", path, err)
	}
}

// writeFakeRealesrgan writes a fake realesrgan-ncnn-vulkan binary to dir that:
//   - Parses the -o flag to find outDir.
//   - Creates outDir and writes argv.txt containing one argument per line.
//   - Exits 0.
func writeFakeRealesrgan(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "realesrgan-ncnn-vulkan")
	writeScript(t, path, `#!/bin/sh
# Parse -o to find outDir.
OUTDIR=""
PREV=""
for a in "$@"; do
    if [ "$PREV" = "-o" ]; then OUTDIR="$a"; fi
    PREV="$a"
done
mkdir -p "$OUTDIR"
: > "$OUTDIR/argv.txt"
for a in "$@"; do printf '%s\n' "$a" >> "$OUTDIR/argv.txt"; done
exit 0
`)
	return path
}

func TestRealesrgan_Realtime_Argv(t *testing.T) {
	binDir := t.TempDir()
	framesDir := t.TempDir()
	outDir := t.TempDir()

	binPath := writeFakeRealesrgan(t, binDir)

	m := newRealesrgan("realtime", "realesr-animevideov3", binPath)
	if err := m.Upscale(context.Background(), framesDir, outDir, 4); err != nil {
		t.Fatalf("Upscale: %v", err)
	}

	argv, err := os.ReadFile(filepath.Join(outDir, "argv.txt"))
	if err != nil {
		t.Fatalf("read argv.txt: %v", err)
	}
	a := string(argv)

	for _, want := range []string{"-s", "4", "-n", "realesr-animevideov3"} {
		if !strings.Contains(a, want) {
			t.Errorf("argv missing %q\nfull argv:\n%s", want, a)
		}
	}
}

func TestRealesrgan_BestQuality_Argv(t *testing.T) {
	binDir := t.TempDir()
	framesDir := t.TempDir()
	outDir := t.TempDir()

	binPath := writeFakeRealesrgan(t, binDir)

	m := newRealesrgan("best-quality", "realesrgan-x4plus-anime", binPath)
	if err := m.Upscale(context.Background(), framesDir, outDir, 4); err != nil {
		t.Fatalf("Upscale: %v", err)
	}

	argv, err := os.ReadFile(filepath.Join(outDir, "argv.txt"))
	if err != nil {
		t.Fatalf("read argv.txt: %v", err)
	}
	a := string(argv)

	for _, want := range []string{"-s", "4", "-n", "realesrgan-x4plus-anime"} {
		if !strings.Contains(a, want) {
			t.Errorf("argv missing %q\nfull argv:\n%s", want, a)
		}
	}
}

func TestGet_Realtime(t *testing.T) {
	m, err := Get("realtime")
	if err != nil {
		t.Fatalf("Get(realtime): %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil model for realtime")
	}
}

func TestGet_BestQuality(t *testing.T) {
	m, err := Get("best-quality")
	if err != nil {
		t.Fatalf("Get(best-quality): %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil model for best-quality")
	}
}
