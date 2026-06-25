package upscale

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// maxModelArtifactBytes caps the artifact body read in Install to prevent an
// OOM from a hostile or corrupted artifact. 2 GiB comfortably covers the
// largest plausible realesrgan weight sets (real model pairs are ~10-200 MiB).
// A one-byte overflow is detected and causes an error; truncation never occurs.
const maxModelArtifactBytes = 2 << 30 // 2 GiB

// Manager is a thread-safe registry of upscale Models.
// It always contains the built-in "mock" model and any models registered via
// NewManager (from PREINSTALLED_MODELS) or Install (T29 pull-on-demand).
type Manager struct {
	mu       sync.RWMutex
	models   map[string]Model
	modelsDir string

	// installMu serialises Install calls for the same model name; the outer
	// registry write is still guarded by mu.
	installMu sync.Mutex
	// inFlight tracks names currently being installed so concurrent Install
	// calls for the same name wait rather than double-installing.
	inFlight map[string]*sync.Mutex
}

// NewManager constructs a Manager. It always registers the built-in mock model.
// For each name in preinstalled, it attempts to register a realesrgan model
// (using the name as both registry key and -n model name). If the weights file
// is absent in modelsDir, a warning is logged and the model is skipped — mock
// always remains.
//
// modelsDir is the directory where model weight files ({name}.param, {name}.bin)
// live. It is also the extraction target for Install.
// Pass an empty string to skip weight-presence checks (useful in tests where no
// real weights exist).
func NewManager(modelsDir string, preinstalled []string) *Manager {
	m := &Manager{
		models:    make(map[string]Model),
		modelsDir: modelsDir,
		inFlight:  make(map[string]*sync.Mutex),
	}

	// Built-in mock is ALWAYS present.
	m.models["mock"] = mockModel{}

	// Register preinstalled models.
	for _, name := range preinstalled {
		name = strings.TrimSpace(name)
		if name == "" || name == "mock" {
			continue
		}
		// If a modelsDir is set, verify the weight files exist before registering.
		if modelsDir != "" {
			paramPath := filepath.Join(modelsDir, name+".param")
			binPath := filepath.Join(modelsDir, name+".bin")
			if _, err := os.Stat(paramPath); err != nil {
				log.Printf("upscale: preinstalled model %q: .param not found at %s; skipping", name, paramPath)
				continue
			}
			if _, err := os.Stat(binPath); err != nil {
				log.Printf("upscale: preinstalled model %q: .bin not found at %s; skipping", name, binPath)
				continue
			}
		}
		m.models[name] = newRealesrgan(name, name, "realesrgan-ncnn-vulkan", modelsDir)
	}

	return m
}

// Available returns a sorted list of all model names in the registry.
// Safe for concurrent use.
func (m *Manager) Available() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.models))
	for name := range m.models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns the Model registered under name, and whether it was found.
// Safe for concurrent use.
func (m *Manager) Get(name string) (Model, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	model, ok := m.models[name]
	return model, ok
}

// Install installs a new realesrgan model from an artifact stream.
//
// The artifact must be a TAR archive containing exactly {name}.param and
// {name}.bin. The SHA-256 checksum of the entire artifact stream is verified
// against expectedChecksumHex before any files are written; a mismatch causes
// Install to return an error without modifying the registry or disk.
//
// Tar entries with path traversal components (absolute paths or entries
// containing "..") are rejected and cause Install to return an error.
// Non-regular TAR entry types (symlinks, hardlinks, devices, fifos) are also
// rejected. If either {name}.param or {name}.bin is missing from the archive
// after extraction, Install returns an error and cleans up any partial files.
//
// The artifact body read is bounded by maxModelArtifactBytes; an artifact that
// exceeds this cap causes Install to fail rather than silently truncating.
//
// Install is idempotent: if a model with the given name is already registered,
// it returns nil immediately. Concurrent Install calls for the same name are
// serialised — only one runs the extract+register path.
//
// On any failure, the registry is left unchanged. Partial files written to
// modelsDir during a failed extraction are cleaned up on a best-effort basis.
//
// The version parameter is reserved for T29 provenance tracking; it is not
// used in this implementation.
func (m *Manager) Install(name, _ string, artifact io.Reader, expectedChecksumHex string) error {
	if name == "" {
		return fmt.Errorf("upscale: Install: name must not be empty")
	}
	if name == "mock" {
		return fmt.Errorf("upscale: Install: cannot overwrite built-in mock model")
	}

	// Worker-side name traversal guard: reject names that could escape the models
	// directory when used in filepath.Join or as the realesrgan -n argument.
	// The server also validates on upload, but the worker must not trust the grant.
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("upscale: Install %q: name contains path-traversal characters", name)
	}

	// Fast path: already registered.
	m.mu.RLock()
	_, exists := m.models[name]
	m.mu.RUnlock()
	if exists {
		return nil
	}

	// Acquire per-name install lock so concurrent Install(name, ...) calls
	// are serialised rather than both racing to write the same files.
	nameMu := m.perNameMu(name)
	nameMu.Lock()
	defer nameMu.Unlock()

	// Re-check under the name lock in case a concurrent Install just finished.
	m.mu.RLock()
	_, exists = m.models[name]
	m.mu.RUnlock()
	if exists {
		return nil
	}

	if m.modelsDir == "" {
		return fmt.Errorf("upscale: Install: modelsDir is not set")
	}

	// Read the artifact through a SHA-256 hasher so we can verify the checksum
	// before writing any files. Wrap with io.LimitReader so an oversized or
	// hostile artifact cannot OOM the worker. We read one byte beyond the cap to
	// detect overflow without silently truncating.
	hasher := sha256.New()
	limited := io.LimitReader(artifact, maxModelArtifactBytes+1)
	data, err := io.ReadAll(io.TeeReader(limited, hasher))
	if err != nil {
		return fmt.Errorf("upscale: Install %q: read artifact: %w", name, err)
	}
	if int64(len(data)) > maxModelArtifactBytes {
		return fmt.Errorf("upscale: Install %q: artifact exceeds %d-byte cap (got >%d bytes)", name, maxModelArtifactBytes, maxModelArtifactBytes)
	}

	// Verify checksum before touching disk.
	gotHex := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(gotHex, expectedChecksumHex) {
		return fmt.Errorf("upscale: Install %q: checksum mismatch: got %s, want %s", name, gotHex, expectedChecksumHex)
	}

	// Extract the TAR into modelsDir. Track files written for cleanup on failure.
	var written []string
	extractErr := extractTAR(strings.NewReader(string(data)), m.modelsDir, name, &written)
	if extractErr != nil {
		// Best-effort cleanup of any partially-written files.
		for _, p := range written {
			os.Remove(p) //nolint:errcheck
		}
		return fmt.Errorf("upscale: Install %q: extract: %w", name, extractErr)
	}

	// Assert both required weight files were extracted. A TAR that silently omits
	// {name}.param or {name}.bin would produce a model that fails at runtime —
	// reject it here and clean up whatever was written.
	wantParam := filepath.Join(m.modelsDir, name+".param")
	wantBin := filepath.Join(m.modelsDir, name+".bin")
	var missingFiles []string
	if _, err := os.Stat(wantParam); err != nil {
		missingFiles = append(missingFiles, name+".param")
	}
	if _, err := os.Stat(wantBin); err != nil {
		missingFiles = append(missingFiles, name+".bin")
	}
	if len(missingFiles) > 0 {
		for _, p := range written {
			os.Remove(p) //nolint:errcheck
		}
		return fmt.Errorf("upscale: Install %q: artifact is missing required weight file(s): %s", name, strings.Join(missingFiles, ", "))
	}

	// Register the model.
	// IMMUTABILITY NOTE: model names are content-address keys. A running worker
	// caches a model by name for its lifetime — Install is a no-op for a name
	// that is already registered. Changing a model's weights requires registering
	// them under a NEW name (e.g. "mymodel-v2") and restarting workers; there is
	// NO in-place update path.
	model := newRealesrgan(name, name, "realesrgan-ncnn-vulkan", m.modelsDir)
	m.mu.Lock()
	m.models[name] = model
	m.mu.Unlock()

	return nil
}

// perNameMu returns a per-name mutex, creating it if needed.
// The outer installMu guards the inFlight map itself.
func (m *Manager) perNameMu(name string) *sync.Mutex {
	m.installMu.Lock()
	defer m.installMu.Unlock()
	mu, ok := m.inFlight[name]
	if !ok {
		mu = &sync.Mutex{}
		m.inFlight[name] = mu
	}
	return mu
}

// extractTAR extracts {name}.param and {name}.bin from r into destDir.
// It rejects:
//   - any tar entry whose path is absolute or contains ".."
//   - any entry that is not a regular file (tar.TypeReg); symlinks, hardlinks,
//     character/block devices, FIFOs, and directories-as-files are all rejected
//
// Paths written are appended to *written for caller cleanup on failure.
func extractTAR(r io.Reader, destDir, name string, written *[]string) error {
	wantParam := name + ".param"
	wantBin := name + ".bin"
	allowed := map[string]bool{wantParam: true, wantBin: true}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}

		// Reject non-regular entry types (symlinks, hardlinks, char/block devices,
		// FIFOs, etc.) — only plain files are permitted.
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("tar entry %q has non-regular type %d (only regular files are allowed)", hdr.Name, hdr.Typeflag)
		}

		// Guard against path traversal.
		base := filepath.Base(hdr.Name)
		if filepath.IsAbs(hdr.Name) {
			return fmt.Errorf("tar entry has absolute path: %q", hdr.Name)
		}
		if strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("tar entry contains path traversal: %q", hdr.Name)
		}

		// Only extract expected files; skip others silently.
		if !allowed[base] {
			continue
		}

		destPath := filepath.Join(destDir, base)
		// Guard: clean path must still be inside destDir.
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry would escape destination: %q", hdr.Name)
		}

		if err := writeFile(destPath, tr); err != nil {
			return fmt.Errorf("write %s: %w", base, err)
		}
		*written = append(*written, destPath)
	}
	return nil
}

// writeFile creates or truncates dst and copies from r into it.
func writeFile(dst string, r io.Reader) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// RegisterForTest adds m to the registry under its Name(). This is exported
// for use in test packages (e.g. agent) that need to inject fake models into a
// Manager without going through Install. It is not meant for production use.
// It silently overwrites an existing entry with the same name.
func (m *Manager) RegisterForTest(name string, model Model) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models[name] = model
}
