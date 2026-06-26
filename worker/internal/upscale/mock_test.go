package upscale

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMockUpscale_FrameCount(t *testing.T) {
	framesDir := t.TempDir()
	outDir := t.TempDir()

	// Create 3 fake frame files.
	for _, name := range []string{"frame_000.png", "frame_001.png", "frame_002.png"} {
		path := filepath.Join(framesDir, name)
		if err := os.WriteFile(path, []byte("fake frame"), 0o644); err != nil {
			t.Fatalf("write frame %s: %v", name, err)
		}
	}

	m := mockModel{}
	if err := m.Upscale(context.Background(), framesDir, outDir, 2); err != nil {
		t.Fatalf("Upscale: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("ReadDir outDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 output frames, got %d", len(entries))
	}
}
