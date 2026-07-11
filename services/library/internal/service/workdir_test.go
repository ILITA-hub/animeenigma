package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkDir_JailRejectsTraversal(t *testing.T) {
	wd := NewWorkDir(t.TempDir())
	for _, bad := range []string{"../etc", "/etc/passwd", "a/../../b", ".."} {
		if _, err := wd.Resolve(bad); err == nil {
			t.Fatalf("Resolve(%q) should be rejected", bad)
		}
	}
}

func TestWorkDir_ListAndDelete(t *testing.T) {
	root := t.TempDir()
	wd := NewWorkDir(root)
	ih := filepath.Join(root, "abcd1234")
	if err := os.MkdirAll(ih, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ih, "video.mkv"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	top, err := wd.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Name != "abcd1234" || !top[0].IsDir {
		t.Fatalf("unexpected top listing: %+v", top)
	}
	inside, err := wd.List("abcd1234")
	if err != nil {
		t.Fatal(err)
	}
	if len(inside) != 1 || inside[0].Name != "video.mkv" || inside[0].Size != 5 {
		t.Fatalf("unexpected inner listing: %+v", inside)
	}
	if err := wd.Delete("abcd1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ih); !os.IsNotExist(err) {
		t.Fatal("expected dir removed")
	}
}

func TestWorkDir_DeleteRootRefused(t *testing.T) {
	wd := NewWorkDir(t.TempDir())
	if err := wd.Delete(""); err == nil {
		t.Fatal("deleting the root must be refused")
	}
}

func TestWorkDir_SymlinkEscapeRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	wd := NewWorkDir(root)
	if _, err := wd.Resolve("escape/secret.txt"); err == nil {
		t.Fatal("Resolve through a symlink escaping root should be rejected")
	}
	if _, err := wd.List("escape"); err == nil {
		t.Fatal("List through a symlink escaping root should be rejected")
	}
	if err := wd.Delete("escape"); err == nil {
		t.Fatal("Delete of a symlink escaping root should be rejected")
	}
	if _, err := os.Lstat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Fatalf("target outside root must be untouched: %v", err)
	}
}

// TestWorkDir_SymlinkEscapeNonExistentLeaf guards the EvalSymlinks-ENOENT bypass:
// a not-yet-existing leaf under a symlinked ancestor that escapes root must still be
// rejected, since a later Download/Delete would otherwise act OUTSIDE root.
func TestWorkDir_SymlinkEscapeNonExistentLeaf(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	wd := NewWorkDir(root)
	if _, err := wd.Resolve("escape/newleaf.mkv"); err == nil {
		t.Fatal("Resolve of a non-existent leaf under an escaping symlink should be rejected")
	}
	if err := wd.Delete("escape/newleaf.mkv"); err == nil {
		t.Fatal("Delete of a non-existent leaf under an escaping symlink should be rejected")
	}
}

// TestWorkDir_NewLeafUnderRealDirAllowed is the positive counterpart: a not-yet-
// existing leaf under a genuine (non-symlink) subdirectory of root must resolve, so
// the walk-up guard doesn't over-reject legitimate new paths (e.g. Download targets).
func TestWorkDir_NewLeafUnderRealDirAllowed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "abcd1234"), 0o755); err != nil {
		t.Fatal(err)
	}
	wd := NewWorkDir(root)
	got, err := wd.Resolve("abcd1234/newfile.mkv")
	if err != nil {
		t.Fatalf("new leaf under a real dir should resolve: %v", err)
	}
	if want := filepath.Join(wd.root, "abcd1234", "newfile.mkv"); got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}
