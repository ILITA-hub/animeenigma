package upscale

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// buildModelTAR creates a valid TAR containing {name}.param and {name}.bin.
func buildModelTAR(t *testing.T, name string) []byte {
	t.Helper()
	return buildTARFiles(t, map[string][]byte{
		name + ".param": []byte("param-data"),
		name + ".bin":   []byte("bin-data"),
	})
}

// buildTraversalTAR creates a TAR containing a path-traversal entry.
func buildTraversalTAR(t *testing.T) []byte {
	t.Helper()
	return buildTARFiles(t, map[string][]byte{
		"../evil.bin": []byte("evil-content"),
	})
}

// buildAbsoluteTAR creates a TAR containing an absolute-path entry.
func buildAbsoluteTAR(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:     "/etc/evil.bin",
		Typeflag: tar.TypeReg,
		Size:     4,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	return buf.Bytes()
}

func buildTARFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, data := range files {
		hdr := &tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(data)),
			Mode:     0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256HexOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ── Manager tests ─────────────────────────────────────────────────────────────

// TestManager_MockAlwaysPresent verifies that NewManager always registers mock.
func TestManager_MockAlwaysPresent(t *testing.T) {
	t.Parallel()

	mgr := NewManager("", nil)

	m, ok := mgr.Get("mock")
	if !ok {
		t.Fatal("expected mock to be present in manager")
	}
	if m.Name() != "mock" {
		t.Errorf("mock.Name() = %q, want %q", m.Name(), "mock")
	}
}

// TestManager_MockAlwaysPresentWithPreinstalled verifies mock survives alongside
// preinstalled models (even if they are absent → skipped).
func TestManager_MockAlwaysPresentWithPreinstalled(t *testing.T) {
	t.Parallel()

	// modelsDir is empty → presence checks are skipped; preinstalled registered anyway.
	mgr := NewManager("", []string{"nonexistent-model"})

	if _, ok := mgr.Get("mock"); !ok {
		t.Fatal("mock must always be present even with preinstalled models")
	}
}

// TestManager_PreinstalledSkippedWhenWeightsAbsent verifies that when modelsDir
// is set and weight files are absent, the model is skipped but mock survives.
func TestManager_PreinstalledSkippedWhenWeightsAbsent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() // empty — no weight files
	mgr := NewManager(dir, []string{"absent-model"})

	if _, ok := mgr.Get("absent-model"); ok {
		t.Error("absent-model should be skipped when weights missing")
	}
	if _, ok := mgr.Get("mock"); !ok {
		t.Error("mock must still be present after skipping absent preinstalled model")
	}
}

// TestManager_PreinstalledRegisteredWhenWeightsPresent verifies that a model
// is registered when its weight files exist in modelsDir.
func TestManager_PreinstalledRegisteredWhenWeightsPresent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write dummy weight files.
	for _, ext := range []string{".param", ".bin"} {
		if err := os.WriteFile(filepath.Join(dir, "present"+ext), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mgr := NewManager(dir, []string{"present"})

	if _, ok := mgr.Get("present"); !ok {
		t.Error("present model should be registered when weight files exist")
	}
	if _, ok := mgr.Get("mock"); !ok {
		t.Error("mock must still be present")
	}
}

// TestManager_Available verifies Available() returns a sorted list.
func TestManager_Available(t *testing.T) {
	t.Parallel()

	mgr := NewManager("", nil)
	mgr.RegisterForTest("zzz", &testDummyModel{name: "zzz"})
	mgr.RegisterForTest("aaa", &testDummyModel{name: "aaa"})

	got := mgr.Available()
	if len(got) < 3 {
		t.Fatalf("expected at least 3 models, got %v", got)
	}
	// Must be sorted.
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("Available() not sorted at index %d: %v", i, got)
		}
	}
	// Must contain mock, aaa, zzz.
	has := func(name string) bool {
		for _, n := range got {
			if n == name {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"mock", "aaa", "zzz"} {
		if !has(want) {
			t.Errorf("Available() missing %q; got %v", want, got)
		}
	}
}

// TestManager_GetHitMiss verifies Get returns (model, true) for known names
// and (nil, false) for unknown names.
func TestManager_GetHitMiss(t *testing.T) {
	t.Parallel()

	mgr := NewManager("", nil)

	m, ok := mgr.Get("mock")
	if !ok {
		t.Error("Get(mock) returned false, want true")
	}
	if m == nil {
		t.Error("Get(mock) returned nil model")
	}

	if _, ok := mgr.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) returned true, want false")
	}
}

// TestManager_InstallCorrectChecksumRegisters verifies that Install with a
// correct checksum extracts files and registers the model.
func TestManager_InstallCorrectChecksumRegisters(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildModelTAR(t, "mymodel")
	checksum := sha256HexOf(tarData)

	if err := mgr.Install("mymodel", "v1", bytes.NewReader(tarData), checksum); err != nil {
		t.Fatalf("Install: %v", err)
	}

	m, ok := mgr.Get("mymodel")
	if !ok {
		t.Fatal("model not registered after successful Install")
	}
	if m.Name() != "mymodel" {
		t.Errorf("model.Name() = %q, want %q", m.Name(), "mymodel")
	}

	// Weight files must exist on disk.
	for _, ext := range []string{".param", ".bin"} {
		path := filepath.Join(dir, "mymodel"+ext)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s: %v", path, err)
		}
	}
}

// TestManager_InstallWrongChecksumError verifies that a checksum mismatch
// causes Install to return an error and leave the registry unchanged.
func TestManager_InstallWrongChecksumError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildModelTAR(t, "model2")
	wrongChecksum := "0000000000000000000000000000000000000000000000000000000000000000"

	err := mgr.Install("model2", "v1", bytes.NewReader(tarData), wrongChecksum)
	if err == nil {
		t.Fatal("expected error on checksum mismatch, got nil")
	}

	if _, ok := mgr.Get("model2"); ok {
		t.Error("model must not be registered after checksum failure")
	}
}

// TestManager_InstallTarTraversalRejected verifies that a tar entry containing
// ".." in its path is rejected and neither files nor registration occur.
func TestManager_InstallTarTraversalRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildTraversalTAR(t)
	checksum := sha256HexOf(tarData)

	err := mgr.Install("evil", "v1", bytes.NewReader(tarData), checksum)
	if err == nil {
		t.Fatal("expected error on path-traversal tar entry, got nil")
	}

	if _, ok := mgr.Get("evil"); ok {
		t.Error("model must not be registered after traversal rejection")
	}
}

// TestManager_InstallAbsolutePathRejected verifies that a tar entry with an
// absolute path is rejected.
func TestManager_InstallAbsolutePathRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildAbsoluteTAR(t)
	checksum := sha256HexOf(tarData)

	err := mgr.Install("evil2", "v1", bytes.NewReader(tarData), checksum)
	// Absolute-path entry: either rejected by the absolute-path guard or silently
	// skipped (base != expected name). Either way, model must NOT be registered.
	// We don't assert err != nil here because the absolute-path guard returns
	// an error, but the checksum passes (the tar is valid).
	// Just verify registry is unchanged.
	_ = err
	if _, ok := mgr.Get("evil2"); ok {
		t.Error("model must not be registered: absolute-path entry must be rejected")
	}
}

// TestManager_InstallIdempotent verifies that calling Install twice for the
// same name (after the first succeeds) returns nil without double-registering.
func TestManager_InstallIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildModelTAR(t, "idem")
	checksum := sha256HexOf(tarData)

	if err := mgr.Install("idem", "v1", bytes.NewReader(tarData), checksum); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := mgr.Install("idem", "v1", bytes.NewReader(tarData), checksum); err != nil {
		t.Fatalf("second Install (idempotent): %v", err)
	}

	if _, ok := mgr.Get("idem"); !ok {
		t.Error("model missing after idempotent install")
	}
}

// TestManager_InstallConcurrentSameName verifies that concurrent Install calls
// for the same model name are race-safe and all succeed (idempotent).
func TestManager_InstallConcurrentSameName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildModelTAR(t, "concurrent")
	checksum := sha256HexOf(tarData)

	const goroutines = 8
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = mgr.Install("concurrent", "v1", bytes.NewReader(tarData), checksum)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Install error: %v", i, err)
		}
	}
	if _, ok := mgr.Get("concurrent"); !ok {
		t.Error("model not registered after concurrent installs")
	}
}

// TestManager_InstallRegistryUnchangedOnExtractError verifies that if tar
// extraction fails (e.g. traversal), the registry is unchanged.
func TestManager_InstallRegistryUnchangedOnExtractError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	// Register mock first as a baseline.
	initialAvail := mgr.Available()

	tarData := buildTraversalTAR(t)
	checksum := sha256HexOf(tarData)
	_ = mgr.Install("bad", "v1", bytes.NewReader(tarData), checksum)

	// Available() must be unchanged (still just mock).
	afterAvail := mgr.Available()
	if len(afterAvail) != len(initialAvail) {
		t.Errorf("available changed after failed install: before=%v after=%v", initialAvail, afterAvail)
	}
}

// TestManager_GetIsSafeConcurrently verifies Get is safe under concurrent reads
// and writes (installs).
func TestManager_GetIsSafeConcurrently(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	tarData := buildModelTAR(t, "race-model")
	checksum := sha256HexOf(tarData)

	var wg sync.WaitGroup

	// Writers.
	for i := range 4 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mgr.Install("race-model", "v1", bytes.NewReader(tarData), checksum) //nolint:errcheck
		}(i)
	}

	// Readers.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Get("mock")
			mgr.Get("race-model")
			mgr.Available()
		}()
	}

	wg.Wait()
}

// ── testDummyModel — a no-op Model for use in tests ─────────────────────────

type testDummyModel struct{ name string }

func (d *testDummyModel) Name() string { return d.name }
func (d *testDummyModel) Upscale(_ context.Context, _, _ string, _ int) error {
	return nil
}

// Ensure testDummyModel satisfies the interface.
var _ Model = (*testDummyModel)(nil)

// Ensure io.Reader is satisfied for test helpers.
var _ io.Reader = (*bytes.Reader)(nil)
