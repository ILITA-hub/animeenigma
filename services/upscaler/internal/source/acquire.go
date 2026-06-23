package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

// ErrSourceGone is returned by Resolver.Resolve when the torrent directory
// for the job's infohash is absent or contains no recognisable video files.
// The caller (handler) should surface "source unavailable — re-acquire via
// library" to the operator.
var ErrSourceGone = errors.New("source file no longer present in torrent volume")

// videoExtensions is the set of file extensions we treat as candidate video
// files. Detection is by extension; ffprobe validates the actual codec later.
var videoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".webm": true,
	".mov":  true,
	".ts":   true,
	".m2ts": true,
}

// Resolver copies the original torrent video file from the library torrents
// volume into a per-job staging directory, ready for ffprobe + segmentation.
type Resolver struct {
	torrentsDir string // read-only mount of library_torrents volume
	stagingDir  string // writable staging area (e.g. /data/upscale-staging)
}

// NewResolver constructs a Resolver.
//   - torrentsDir: path to the library_torrents volume mount (cfg.Upscaler.TorrentsDir)
//   - stagingDir:  path to the staging area (cfg.Upscaler.StagingDir)
func NewResolver(torrentsDir, stagingDir string) *Resolver {
	return &Resolver{torrentsDir: torrentsDir, stagingDir: stagingDir}
}

// Resolve locates the largest video file under
// {torrentsDir}/{job.LibraryInfohash}/, copies it to
// {stagingDir}/{job.ID}/source{ext}, and returns that destination path.
//
// Returns ErrSourceGone when:
//   - LibraryInfohash is empty
//   - the {infohash}/ directory does not exist
//   - the directory exists but contains no recognisable video files
//
// Returns a plain error for I/O failures during the copy.
func (r *Resolver) Resolve(ctx context.Context, job *domain.UpscaleJob) (string, error) {
	if job.LibraryInfohash == "" {
		return "", fmt.Errorf("job %s has no library infohash: %w", job.ID, ErrSourceGone)
	}

	infohashDir := filepath.Join(r.torrentsDir, job.LibraryInfohash)

	// Walk {torrentsDir}/{infohash}/ and find the largest video file.
	entries, err := os.ReadDir(infohashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("torrent directory %q absent (dropped after seed window): %w",
				infohashDir, ErrSourceGone)
		}
		return "", fmt.Errorf("reading torrent directory %q: %w", infohashDir, err)
	}

	var bestPath string
	var bestSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !videoExtensions[ext] {
			continue
		}
		fullPath := filepath.Join(infohashDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue // skip unreadable entries
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			bestPath = fullPath
		}
	}

	if bestPath == "" {
		return "", fmt.Errorf("no video file found in %q: %w", infohashDir, ErrSourceGone)
	}

	// Prepare the per-job staging directory.
	jobStaging := filepath.Join(r.stagingDir, job.ID)
	if err := os.MkdirAll(jobStaging, 0o755); err != nil {
		return "", fmt.Errorf("creating staging dir %q: %w", jobStaging, err)
	}

	ext := filepath.Ext(bestPath)
	destPath := filepath.Join(jobStaging, "source"+ext)

	if err := copyFileAtomic(ctx, bestPath, destPath); err != nil {
		return "", fmt.Errorf("copying %q → %q: %w", bestPath, destPath, err)
	}

	return destPath, nil
}

// copyFileAtomic copies src to dst atomically: it writes to dst+".tmp",
// fsyncs, then renames to dst on success. On ANY error (open/read/write/
// fsync/ctx-cancel) the temp file is removed so no partial or corrupt file
// is ever left at dst. Rename is atomic on the same filesystem, which holds
// because staging is a single volume. This also closes the
// ctx-cancel-mid-copy leak (a cancelled copy leaves nothing behind).
func copyFileAtomic(ctx context.Context, src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	tmp := dst + ".tmp"
	// O_TRUNC so a leftover .tmp from a prior aborted run is overwritten.
	tmpFile, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open temp dest: %w", err)
	}

	// On any failure path, remove the temp file. committed is set true only
	// after the rename succeeds, so a good destination file is never deleted.
	committed := false
	defer func() {
		tmpFile.Close() // idempotent; safe even after an explicit Close below
		if !committed {
			_ = os.Remove(tmp)
		}
	}()

	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := srcFile.Read(buf)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write temp dest: %w", writeErr)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read source: %w", readErr)
		}
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("fsync temp dest: %w", err)
	}
	// Close before rename so all buffered data is flushed.
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp dest: %w", err)
	}

	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("rename temp dest: %w", err)
	}
	committed = true
	return nil
}
