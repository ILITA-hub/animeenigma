package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	libtorrent "github.com/ILITA-hub/animeenigma/services/library/internal/torrent"
)

// TestStorageLayout_ResolverFindsInfoHashDir locks the contract between the
// torrent client's on-disk layout and the encoder's DefaultSourceResolver.
//
// The autocache pipeline produced ZERO episodes because the torrent client
// wrote files FLAT into {downloadDir}/ (anacrolix default path maker) while the
// resolver only stats {downloadDir}/{infohash}/ — so every encode failed
// "resolve source: stat .../<infohash>: no such file or directory". The client
// now stores each torrent under libtorrent.InfoHashDir(downloadDir, infohash);
// this test fails if the two ever diverge again.
func TestStorageLayout_ResolverFindsInfoHashDir(t *testing.T) {
	base := t.TempDir()
	infohash := "0123456789abcdef0123456789abcdef01234567"

	// The torrent client writes payloads here…
	dir := libtorrent.InfoHashDir(base, infohash)
	if dir != filepath.Join(base, infohash) {
		t.Fatalf("InfoHashDir = %q, want %q", dir, filepath.Join(base, infohash))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	video := filepath.Join(dir, "Show S01E01 1080p.mkv")
	if err := os.WriteFile(video, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}

	// …and the encoder resolves them from the same place.
	r := NewDefaultSourceResolver(base)
	got, err := r.Resolve(context.Background(), &domain.Job{ID: "j"}, infohash)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != video {
		t.Fatalf("Resolve = %q, want %q", got, video)
	}
}
