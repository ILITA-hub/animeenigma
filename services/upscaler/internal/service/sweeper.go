// Package service contains domain-level orchestration services for the upscaler.
package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
)

const (
	// sweepInterval is how often the sweeper checks for expired leases and
	// stale workers.
	sweepInterval = 30 * time.Second

	// staleWorkerThreshold is the maximum time a worker can go without sending
	// a heartbeat before it is marked as gone.
	staleWorkerThreshold = 3 * time.Minute
)

// Sweeper re-leases expired segments and marks stale workers as gone.
// Call Run(ctx) to start the background loop; call Stop() to cancel it.
type Sweeper struct {
	segments *repo.SegmentRepository
	workers  *repo.WorkerRepository
	log      *logger.Logger

	// stopCh is closed by Stop() to signal Run() to exit. Created in New* so
	// Stop() is safe to call before Run() (closing a buffered channel of size 0
	// is fine; close on a nil channel panics so we always initialise it).
	stopCh chan struct{}
}

// NewSweeper constructs a Sweeper backed by the given repositories.
func NewSweeper(segments *repo.SegmentRepository, workers *repo.WorkerRepository) *Sweeper {
	return &Sweeper{
		segments: segments,
		workers:  workers,
		log:      logger.Default(),
		stopCh:   make(chan struct{}),
	}
}

// NewSweeperWithLogger constructs a Sweeper with an explicit logger.
func NewSweeperWithLogger(segments *repo.SegmentRepository, workers *repo.WorkerRepository, log *logger.Logger) *Sweeper {
	return &Sweeper{
		segments: segments,
		workers:  workers,
		log:      log,
		stopCh:   make(chan struct{}),
	}
}

// Run starts the sweeper loop. It blocks until ctx is cancelled or Stop() is
// called. The first sweep fires after sweepInterval.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// Stop signals the Run() loop to exit. Safe to call multiple times or before
// Run() — closing an already-closed channel would panic, so we use a
// sync.Once-like pattern via a buffered channel.
func (s *Sweeper) Stop() {
	// Non-blocking close: if stopCh is already closed this select falls through
	// to default and does nothing (no panic). If it's still open, we close it.
	select {
	case <-s.stopCh:
		// already closed — nothing to do
	default:
		close(s.stopCh)
	}
}

// sweep performs a single sweeper cycle: expire stale segment leases, then
// mark stale workers as gone.
func (s *Sweeper) sweep(ctx context.Context) {
	now := time.Now()

	// 1. Re-lease expired segments (flip leased→pending when lease_expires_at < now).
	n, err := s.segments.ExpireStale(ctx, now)
	if err != nil {
		s.log.Warnw("sweeper: ExpireStale failed", "error", err)
	} else if n > 0 {
		s.log.Infow("sweeper: expired stale segment leases", "count", n)
	}

	// 2. Mark stale workers as gone.
	// ListConnected(time.Time{}) returns ALL non-gone workers regardless of
	// heartbeat time (zero time is before any stored timestamp). We then filter
	// to those whose last heartbeat is older than staleWorkerThreshold.
	staleThreshold := now.Add(-staleWorkerThreshold)
	staleWorkers, err := s.workers.ListConnected(ctx, time.Time{})
	if err != nil {
		s.log.Warnw("sweeper: ListConnected failed", "error", err)
		return
	}
	for _, w := range staleWorkers {
		if w.LastHeartbeatAt == nil || w.LastHeartbeatAt.Before(staleThreshold) {
			if merr := s.workers.MarkGone(ctx, w.WorkerID); merr != nil {
				s.log.Warnw("sweeper: MarkGone failed", "worker_id", w.WorkerID, "error", merr)
			} else {
				s.log.Infow("sweeper: marked stale worker as gone", "worker_id", w.WorkerID)
			}
		}
	}
}
