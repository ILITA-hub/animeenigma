package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// WorkDirEntry is one listing row from the torrent working dir.
type WorkDirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// WorkDir is a path-jailed view of LIBRARY_TORRENT_DOWNLOAD_DIR. Every rel path is
// lexically cleaned and confirmed to stay inside root (rejecting .., absolute paths,
// and symlink escapes) before any FS op — the file manager never touches disk outside
// the torrent working dir.
type WorkDir struct{ root string }

func NewWorkDir(root string) *WorkDir { return &WorkDir{root: filepath.Clean(root)} }

// Resolve returns the jailed absolute path for rel, or an error if it escapes root.
//
// Unlike naively force-anchoring rel under "/" and letting filepath.Join eat any ".."
// components (which would silently reinterpret "../etc" as a same-named subdirectory
// of root instead of rejecting it), this explicitly rejects absolute paths and any
// cleaned relative path that still climbs above root via "..".
func (wd *WorkDir) Resolve(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.InvalidInput("path escapes working dir")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.InvalidInput("path escapes working dir")
	}
	abs := filepath.Join(wd.root, clean)
	// Defense in depth: confirm the resolved path is still lexically inside root.
	if abs != wd.root && !strings.HasPrefix(abs, wd.root+string(os.PathSeparator)) {
		return "", errors.InvalidInput("path escapes working dir")
	}
	// Reject symlink escape: if the path exists, its real path must still be inside root.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		if real != wd.root && !strings.HasPrefix(real, wd.root+string(os.PathSeparator)) {
			return "", errors.InvalidInput("path escapes working dir")
		}
	}
	return abs, nil
}

// List returns the entries directly under rel (one level, not recursive).
func (wd *WorkDir) List(rel string) ([]WorkDirEntry, error) {
	abs, err := wd.Resolve(rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("path not found")
		}
		return nil, err
	}
	out := make([]WorkDirEntry, 0, len(des))
	for _, de := range des {
		e := WorkDirEntry{Name: de.Name(), IsDir: de.IsDir()}
		if info, ierr := de.Info(); ierr == nil {
			e.Size = info.Size()
		}
		out = append(out, e)
	}
	return out, nil
}

// Delete removes the file or directory (recursively) at rel. The root itself cannot
// be deleted.
func (wd *WorkDir) Delete(rel string) error {
	abs, err := wd.Resolve(rel)
	if err != nil {
		return err
	}
	if abs == wd.root {
		return errors.InvalidInput("refusing to delete the working-dir root")
	}
	return os.RemoveAll(abs)
}
