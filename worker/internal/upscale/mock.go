package upscale

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func init() {
	Register(mockModel{})
}

// mockModel is the GPU-free upscale model used in tests and CI environments
// where realesrgan-ncnn-vulkan is not available. It copies each input frame
// to outDir verbatim, preserving the original filename.
type mockModel struct{}

func (mockModel) Name() string { return "mock" }

// Upscale copies every file in framesDir into outDir with the same filename.
// The scale parameter is accepted but unused (no actual upscaling is done).
func (mockModel) Upscale(_ context.Context, framesDir, outDir string, _ int) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mock upscale: mkdir outDir: %w", err)
	}

	entries, err := os.ReadDir(framesDir)
	if err != nil {
		return fmt.Errorf("mock upscale: read framesDir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(
			filepath.Join(framesDir, e.Name()),
			filepath.Join(outDir, e.Name()),
		); err != nil {
			return fmt.Errorf("mock upscale: copy %s: %w", e.Name(), err)
		}
	}
	return nil
}

// copyFile copies src to dst using io.Copy.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
