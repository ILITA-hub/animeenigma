package service

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"golang.org/x/sys/unix"
)

// statfsFunc is the Statfs call indirected through a function pointer
// so tests can plug in a fake. Production wiring uses unix.Statfs.
type statfsFunc func(path string, st *unix.Statfs_t) error

// DiskGuard probes free space on the torrent download directory and
// publishes it to the library_disk_free_bytes gauge. The enqueue
// handler consults Allow() before inserting a job so we never queue
// a download we can't fit.
//
// LIB-NF-01 (locked in 03-SPEC.md): 30s polling cadence, 20% default
// minimum free.
type DiskGuard struct {
	path    string
	metrics *metrics.LibraryMetrics
	log     *logger.Logger
	// statfs is the Statfs implementation. Tests overwrite this; in
	// production it defaults to unix.Statfs.
	statfs statfsFunc
}

// NewDiskGuard constructs a DiskGuard for the given path. metrics may
// be nil — Check / Allow still work, only the gauge update in Run()
// becomes a no-op.
func NewDiskGuard(path string, m *metrics.LibraryMetrics, log *logger.Logger) *DiskGuard {
	return &DiskGuard{
		path:    path,
		metrics: m,
		log:     log,
		statfs:  unix.Statfs,
	}
}

// Check returns the current freeBytes / totalBytes / freePct via
// unix.Statfs. freePct is integer floor(freeBytes * 100 / totalBytes)
// and is 0 when totalBytes == 0 (defensive against unmounted paths).
func (g *DiskGuard) Check() (freeBytes uint64, totalBytes uint64, freePct int, err error) {
	var st unix.Statfs_t
	if err = g.statfs(g.path, &st); err != nil {
		return 0, 0, 0, err
	}
	bsize := uint64(st.Bsize) //nolint:unconvert // Bsize is int32 on some platforms
	totalBytes = st.Blocks * bsize
	freeBytes = st.Bavail * bsize
	if totalBytes == 0 {
		return freeBytes, totalBytes, 0, nil
	}
	freePct = int(freeBytes * 100 / totalBytes)
	return freeBytes, totalBytes, freePct, nil
}

// Allow returns whether the freePct is at or above the configured
// minimum. The handler increments library_enqueue_rejected_total{
// reason="disk_full"} when this returns false. The freePct is
// returned for log context.
func (g *DiskGuard) Allow(minFreePct int) (allowed bool, freePct int, err error) {
	_, _, pct, err := g.Check()
	if err != nil {
		return false, 0, err
	}
	return pct >= minFreePct, pct, nil
}

// Run is the polling loop that refreshes the library_disk_free_bytes
// gauge every interval. Errors are logged at warn level — a transient
// Statfs failure shouldn't kill the whole worker.
func (g *DiskGuard) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	// Tick once immediately so the gauge is populated before the
	// first 30s window elapses.
	g.tick()

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.tick()
		}
	}
}

func (g *DiskGuard) tick() {
	free, _, pct, err := g.Check()
	if err != nil {
		if g.log != nil {
			g.log.Warnw("disk guard statfs failed", "path", g.path, "error", err)
		}
		return
	}
	if g.metrics != nil {
		g.metrics.SetDiskFreeBytes(free)
	}
	_ = pct // kept available for callers that want richer logging
}

// ErrStatfsUnsupported is returned by a test fake when the caller
// neglects to set up a mock. Production never sees this.
var ErrStatfsUnsupported = errors.New("statfs: no implementation")
