// Package torrent wraps github.com/anacrolix/torrent behind a thin,
// service-shaped facade so the rest of the library service never has
// to import the upstream package directly. There is exactly one
// *torrent.Client per process (locked in 03-CONTEXT.md), shared by N
// download workers. The facade exposes only what the worker needs:
//
//   - Client.Add(ctx, magnetURI) → DownloadHandle
//   - DownloadHandle.{ID, Progress, Cancel, Done}
//   - Client.Close()
//
// Everything else (DHT bootstrap tuning, piece selection, encryption
// policy) stays at upstream defaults.
package torrent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	anacrolix "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"
)

// InfoHashDir is the per-torrent storage directory: {downloadDir}/{infohash}/.
// It is the single source of truth for the on-disk layout — the torrent client
// writes each torrent's payload here (via the storage path maker below) and the
// encoder's DefaultSourceResolver reads from the same place. Keeping both sides
// behind this one helper prevents the layout drift that produced zero autocache
// episodes (client wrote flat files; resolver looked under {infohash}/).
func InfoHashDir(downloadDir, infoHashHex string) string {
	return filepath.Join(downloadDir, infoHashHex)
}

// Config controls the per-client tuning. Defaults are picked to be
// gentle on a self-hosted home server (80 connected peers / 1MiB/s
// upload cap / 24h seed window).
type Config struct {
	DownloadDir    string
	MaxPeers       int
	UploadRateKBPS int
	SeedDuration   time.Duration
}

// DownloadHandle is the per-torrent view the worker manipulates. The
// methods are safe to call from any goroutine.
type DownloadHandle interface {
	// ID returns the lowercase hex infohash. Stable for the lifetime
	// of the torrent — different magnet URIs for the same content
	// resolve to the same ID.
	ID() string

	// Progress reports current download state. total == -1 means
	// metadata has not arrived yet (anacrolix is still asking peers
	// for the .torrent dict); callers should treat that as
	// "wait, no progress yet".
	Progress() (downloaded, total int64, peers int)

	// Cancel stops the download and releases all peers. It is
	// idempotent — calling it twice is a no-op.
	Cancel()

	// Done resolves when the torrent finishes downloading (Complete
	// signal), when Cancel() is called, or when the seed-duration
	// timer expires.
	Done() <-chan struct{}
}

// Client wraps a single underlying *anacrolix.Client.
type Client struct {
	cfg      Config
	anacrolix *anacrolix.Client
	// storage is the file storage we hand to anacrolix. anacrolix only
	// auto-closes the storage it creates itself (when DefaultStorage is
	// nil); a caller-provided one is ours to Close().
	storage storage.ClientImplCloser
}

// NewClient builds the underlying anacrolix client with the supplied
// tuning. The download directory is created (MkdirAll) before the
// client is constructed so anacrolix has somewhere to write piece
// stores on first add.
func NewClient(cfg Config) (*Client, error) {
	if cfg.DownloadDir == "" {
		return nil, errors.New("torrent: DownloadDir is required")
	}
	if cfg.MaxPeers <= 0 {
		cfg.MaxPeers = 80
	}
	if cfg.SeedDuration <= 0 {
		cfg.SeedDuration = 24 * time.Hour
	}

	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("torrent: mkdir download dir: %w", err)
	}

	// Store each torrent's payload under {DownloadDir}/{infohash}/ instead of
	// anacrolix's default flat-by-name layout, so the encoder's resolver (which
	// stats {DownloadDir}/{infohash}/) finds it. Without this, completed
	// downloads sat flat on disk and every encode failed "resolve source: stat
	// .../<infohash>: no such file or directory" — the autocache pool stayed
	// empty. We own this storage and Close() it in Client.Close().
	store := storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir: cfg.DownloadDir,
		TorrentDirMaker: func(baseDir string, _ *metainfo.Info, infoHash metainfo.Hash) string {
			return InfoHashDir(baseDir, infoHash.HexString())
		},
	})

	acfg := anacrolix.NewDefaultClientConfig()
	acfg.DefaultStorage = store
	acfg.Seed = true
	acfg.EstablishedConnsPerTorrent = cfg.MaxPeers
	if cfg.UploadRateKBPS > 0 {
		bps := rate.Limit(cfg.UploadRateKBPS * 1024)
		acfg.UploadRateLimiter = rate.NewLimiter(bps, cfg.UploadRateKBPS*1024)
	}

	c, err := anacrolix.NewClient(acfg)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("torrent: new client: %w", err)
	}
	return &Client{cfg: cfg, anacrolix: c, storage: store}, nil
}

// Add validates the magnet URI, registers it with anacrolix, and
// spawns the lifecycle goroutine that drives download → seed → drop.
// Caller receives a DownloadHandle.
func (c *Client) Add(ctx context.Context, magnetURI string) (DownloadHandle, error) {
	if _, err := metainfo.ParseMagnetUri(magnetURI); err != nil {
		return nil, fmt.Errorf("torrent: invalid magnet: %w", err)
	}
	t, err := c.anacrolix.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("torrent: add magnet: %w", err)
	}

	h := &handle{
		t:            t,
		done:         make(chan struct{}),
		seedDuration: c.cfg.SeedDuration,
	}
	go h.run(ctx)
	return h, nil
}

// Close tears down the underlying anacrolix client, dropping all
// active torrents and releasing peer connections. Idempotent.
func (c *Client) Close() error {
	if c == nil || c.anacrolix == nil {
		return nil
	}
	var errs []error
	if cerrs := c.anacrolix.Close(); len(cerrs) > 0 {
		errs = append(errs, fmt.Errorf("torrent: close client: %v", cerrs))
	}
	// anacrolix does NOT close a caller-provided DefaultStorage; we own it.
	if c.storage != nil {
		if err := c.storage.Close(); err != nil {
			errs = append(errs, fmt.Errorf("torrent: close storage: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("torrent: close: %v", errs)
	}
	return nil
}

// handle is the concrete DownloadHandle implementation.
type handle struct {
	t            *anacrolix.Torrent
	done         chan struct{}
	closeOnce    sync.Once
	cancelOnce   sync.Once
	seedDuration time.Duration
}

// ID is the lowercase hex infohash.
func (h *handle) ID() string {
	if h.t == nil {
		return ""
	}
	return h.t.InfoHash().HexString()
}

// Progress reports current download state. total is -1 until metadata
// arrives (anacrolix's Info() returns nil before the .torrent dict is
// resolved); we still report peer count even at that point because
// the worker uses zero-peer signal to drive stall detection.
func (h *handle) Progress() (downloaded, total int64, peers int) {
	if h.t == nil {
		return 0, -1, 0
	}
	peers = len(h.t.PeerConns())
	info := h.t.Info()
	if info == nil {
		return 0, -1, peers
	}
	return h.t.BytesCompleted(), info.TotalLength(), peers
}

// Cancel drops the torrent (releasing peers + on-disk state per
// anacrolix's Drop semantics) and closes Done(). Idempotent.
func (h *handle) Cancel() {
	h.cancelOnce.Do(func() {
		if h.t != nil {
			h.t.Drop()
		}
	})
	h.closeDone()
}

// Done resolves when the lifecycle goroutine exits.
func (h *handle) Done() <-chan struct{} { return h.done }

// closeDone is sync.Once-protected — multiple paths (run() exit,
// Cancel(), Close()) all funnel through here without panicking on
// double-close of the chan.
func (h *handle) closeDone() {
	h.closeOnce.Do(func() { close(h.done) })
}

// run drives the lifecycle: wait for metadata, start DownloadAll,
// wait for completion or cancellation, then schedule a seed window
// before dropping.
//
// The caller has up to two influences on this loop:
//   - ctx.Done() (the worker's root context) — short-circuits both
//     waits and triggers Drop.
//   - Cancel() (admin DELETE path) — Drops immediately and closes
//     the done channel.
func (h *handle) run(ctx context.Context) {
	defer h.closeDone()
	if h.t == nil {
		return
	}

	// 1. Wait for metadata (the magnet → .torrent step).
	select {
	case <-ctx.Done():
		h.t.Drop()
		return
	case <-h.t.Closed():
		// Cancel()/Drop() closed the torrent while we were awaiting metadata
		// (a dead magnet never fires GotInfo()) — stop instead of leaking this
		// goroutine forever (audit #30). Already dropped, so don't Drop again.
		return
	case <-h.t.GotInfo():
	}

	// 2. Begin downloading every piece.
	h.t.DownloadAll()

	// 3. Wait for completion or cancellation.
	select {
	case <-ctx.Done():
		h.t.Drop()
		return
	case <-h.t.Closed():
		// Dropped mid-download — unblock (Complete() may never fire).
		return
	case <-h.t.Complete().On():
		// Download complete — fall through to seed window.
	}

	// 4. Seed for the configured window, then drop.
	if h.seedDuration <= 0 {
		h.t.Drop()
		return
	}
	timer := time.NewTimer(h.seedDuration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		h.t.Drop()
	case <-h.t.Closed():
		// Cancelled mid-seed — already dropped, just stop.
	case <-timer.C:
		h.t.Drop()
	}
}
