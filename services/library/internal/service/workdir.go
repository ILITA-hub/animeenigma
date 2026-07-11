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

// NewWorkDir stores the canonicalized (symlink-resolved) root so the symlink-escape
// checks in Resolve compare real paths against a real root. If root doesn't exist on
// disk yet, it falls back to the lexically-cleaned path.
func NewWorkDir(root string) *WorkDir {
	clean := filepath.Clean(root)
	if real, err := filepath.EvalSymlinks(clean); err == nil {
		clean = real
	}
	return &WorkDir{root: clean}
}

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
	// Symlink-escape guard. EvalSymlinks only works on a fully-existing path, so a
	// non-existing leaf under an existing symlinked ancestor (e.g. root/escape ->
	// /outside, then "escape/newfile.mkv") would otherwise slip through with err!=nil
	// and be returned as the un-resolved lexical path — a real escape at read/serve/
	// delete time. Instead, canonicalize the deepest ANCESTOR that actually exists and
	// require it to stay inside root; the not-yet-existing tail is purely lexical and
	// cannot introduce a new symlink.
	if err := wd.assertJailedRealpath(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// assertJailedRealpath walks up from abs to the deepest existing ancestor, resolves
// its symlinks, and confirms the real path is still inside root. Fails safe: if not
// even root exists (nothing resolves inside the jail) or the deepest ancestor can't be
// canonicalized (e.g. a dangling symlink), it rejects.
func (wd *WorkDir) assertJailedRealpath(abs string) error {
	probe := abs
	for {
		// Lstat (not Stat) so a symlink is detected AT itself and gets canonicalized,
		// rather than transparently followed outside the jail.
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		if probe == wd.root {
			// Root itself isn't on disk yet — nothing can resolve inside the jail.
			return errors.InvalidInput("path escapes working dir")
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return errors.InvalidInput("path escapes working dir")
		}
		probe = parent
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return errors.InvalidInput("path escapes working dir")
	}
	if real != wd.root && !strings.HasPrefix(real, wd.root+string(os.PathSeparator)) {
		return errors.InvalidInput("path escapes working dir")
	}
	return nil
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
