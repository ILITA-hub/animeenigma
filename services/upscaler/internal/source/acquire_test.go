package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

// makeFile creates a file at path with the given content length (bytes of 'x').
func makeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	content := make([]byte, size)
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func TestResolve_CopiesLargestVideoFile(t *testing.T) {
	torrentsDir := t.TempDir()
	stagingDir := t.TempDir()

	const infohash = "abc123infohash"
	jobDir := filepath.Join(torrentsDir, infohash)

	// Create two video files (different sizes) and a .nfo sidecar.
	// Resolver must pick episode.mkv (larger) over small.mkv.
	makeFile(t, filepath.Join(jobDir, "episode.mkv"), 4096)
	makeFile(t, filepath.Join(jobDir, "small.mkv"), 512)
	makeFile(t, filepath.Join(jobDir, "metadata.nfo"), 200)

	job := &domain.UpscaleJob{
		ID:              "job-test-001",
		LibraryInfohash: infohash,
	}

	resolver := NewResolver(torrentsDir, stagingDir)
	localPath, err := resolver.Resolve(context.Background(), job)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}

	// The returned path must exist on disk.
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("Resolve() returned path %q doesn't exist: %v", localPath, err)
	}

	// Must be under {stagingDir}/{jobID}/
	expectedDir := filepath.Join(stagingDir, job.ID)
	if !filepath.IsAbs(localPath) {
		t.Errorf("localPath %q is not absolute", localPath)
	}
	dirPart := filepath.Dir(localPath)
	if dirPart != expectedDir {
		t.Errorf("localPath dir = %q, want %q", dirPart, expectedDir)
	}

	// Must have .mkv extension (picked the video file, not .nfo).
	if filepath.Ext(localPath) != ".mkv" {
		t.Errorf("localPath ext = %q, want .mkv", filepath.Ext(localPath))
	}

	// Must be named source.mkv (not small.mkv — largest wins).
	base := filepath.Base(localPath)
	if base != "source.mkv" {
		t.Errorf("localPath base = %q, want %q", base, "source.mkv")
	}

	// The copied file must have the same content length as episode.mkv (4096 bytes).
	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("Stat localPath: %v", err)
	}
	if info.Size() != 4096 {
		t.Errorf("copied file size = %d, want 4096", info.Size())
	}
}

func TestResolve_MissingInfohashDir_ErrSourceGone(t *testing.T) {
	torrentsDir := t.TempDir()
	stagingDir := t.TempDir()

	job := &domain.UpscaleJob{
		ID:              "job-missing-001",
		LibraryInfohash: "doesnotexist",
	}

	resolver := NewResolver(torrentsDir, stagingDir)
	_, err := resolver.Resolve(context.Background(), job)
	if err == nil {
		t.Fatalf("Resolve() error = nil, want ErrSourceGone")
	}
	if !errors.Is(err, ErrSourceGone) {
		t.Errorf("Resolve() error = %v, want errors.Is(err, ErrSourceGone)", err)
	}
}

func TestResolve_EmptyInfohashDir_ErrSourceGone(t *testing.T) {
	torrentsDir := t.TempDir()
	stagingDir := t.TempDir()

	const infohash = "emptyhash"
	// Create the directory but put no video files in it — only a .nfo.
	nfoDir := filepath.Join(torrentsDir, infohash)
	makeFile(t, filepath.Join(nfoDir, "info.nfo"), 100)

	job := &domain.UpscaleJob{
		ID:              "job-empty-001",
		LibraryInfohash: infohash,
	}

	resolver := NewResolver(torrentsDir, stagingDir)
	_, err := resolver.Resolve(context.Background(), job)
	if err == nil {
		t.Fatalf("Resolve() error = nil, want ErrSourceGone (no video files)")
	}
	if !errors.Is(err, ErrSourceGone) {
		t.Errorf("Resolve() error = %v, want errors.Is(err, ErrSourceGone)", err)
	}
}

func TestResolve_EmptyInfohash_Error(t *testing.T) {
	torrentsDir := t.TempDir()
	stagingDir := t.TempDir()

	job := &domain.UpscaleJob{
		ID:              "job-nohash-001",
		LibraryInfohash: "",
	}

	resolver := NewResolver(torrentsDir, stagingDir)
	_, err := resolver.Resolve(context.Background(), job)
	if err == nil {
		t.Fatalf("Resolve() error = nil, want error for empty infohash")
	}
}

func TestResolve_PicksMKVOverMp4WhenMKVIsLarger(t *testing.T) {
	torrentsDir := t.TempDir()
	stagingDir := t.TempDir()

	const infohash = "mixed-ext-hash"
	jobDir := filepath.Join(torrentsDir, infohash)

	// MKV is bigger → must be picked.
	makeFile(t, filepath.Join(jobDir, "episode.mp4"), 1024)
	makeFile(t, filepath.Join(jobDir, "episode.mkv"), 8192)

	job := &domain.UpscaleJob{
		ID:              "job-mixed-001",
		LibraryInfohash: infohash,
	}

	resolver := NewResolver(torrentsDir, stagingDir)
	localPath, err := resolver.Resolve(context.Background(), job)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if filepath.Ext(localPath) != ".mkv" {
		t.Errorf("expected .mkv (larger); got %q", filepath.Ext(localPath))
	}
}

func TestResolve_Idempotent_OverwritesExistingStaging(t *testing.T) {
	torrentsDir := t.TempDir()
	stagingDir := t.TempDir()

	const infohash = "idem-hash"
	jobDir := filepath.Join(torrentsDir, infohash)
	makeFile(t, filepath.Join(jobDir, "episode.mkv"), 2048)

	job := &domain.UpscaleJob{
		ID:              "job-idem-001",
		LibraryInfohash: infohash,
	}

	resolver := NewResolver(torrentsDir, stagingDir)

	// First resolve.
	p1, err := resolver.Resolve(context.Background(), job)
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}

	// Second resolve — must not fail even though staging already exists.
	p2, err := resolver.Resolve(context.Background(), job)
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}

	if p1 != p2 {
		t.Errorf("idempotent resolve returned different paths: %q vs %q", p1, p2)
	}
}
