package service

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	libtorrent "github.com/ILITA-hub/animeenigma/services/library/internal/torrent"
)

// TorrentAdder is the surface the worker needs from the torrent
// facade. Pulling it out as an interface lets unit tests substitute
// a stub without spinning up an anacrolix Client.
type TorrentAdder interface {
	Add(ctx context.Context, magnetURI string) (libtorrent.DownloadHandle, error)
}

// JobStore is the slice of *repo.JobRepository the worker needs.
// Keeping it as an interface lets unit tests inject a stub backed
// by an in-memory map; production wiring passes *repo.JobRepository
// (which implements every method by signature).
type JobStore interface {
	Claim(ctx context.Context, statuses ...domain.JobStatus) (*domain.Job, error)
	GetByID(ctx context.Context, id string) (*domain.Job, error)
	UpdateProgress(ctx context.Context, id string, downloadedBytes, totalBytes int64, peers int) error
	UpdateStatus(ctx context.Context, id string, newStatus domain.JobStatus, errorText string) error
	Cancel(ctx context.Context, id string) error
}

// stallErrorText is the SPEC-locked error_text written when stall
// detection fires. The Phase 5 admin UI grep-matches on this string.
const stallErrorText = "stalled: no peers for 30 minutes"

// WorkerPool is N goroutines that race for queued jobs via the
// FOR UPDATE SKIP LOCKED Claim() path. Each goroutine drives one
// download at a time:
//
//	Claim queued → tc.Add(magnet) → tick(progressTick) ticker:
//	  - reread row → if cancelled, drop handle + return
//	  - update progress + emit metrics
//	  - stall check: zero peers for >= stallTimeout → fail
//	On handle.Done() → status=encoding (NOT done — Phase 4 owns the
//	encoder).
type WorkerPool struct {
	workers      int
	jobRepo      JobStore
	tc           TorrentAdder
	metrics      *metrics.LibraryMetrics
	stallTimeout time.Duration
	progressTick time.Duration
	pollInterval time.Duration
	log          *logger.Logger

	handlesMu sync.RWMutex
	handles   map[string]libtorrent.DownloadHandle

	wg sync.WaitGroup
}

// NewWorkerPool constructs a WorkerPool. workers must be >= 1.
// stallTimeout = how long a zero-peer download is tolerated before
// being flipped to failed. progressTick = how often the worker
// re-reads handle.Progress() and updates the row.
func NewWorkerPool(
	workers int,
	jobRepo JobStore,
	tc TorrentAdder,
	libMetrics *metrics.LibraryMetrics,
	stallTimeout time.Duration,
	progressTick time.Duration,
	log *logger.Logger,
) *WorkerPool {
	if workers < 1 {
		workers = 1
	}
	if stallTimeout <= 0 {
		stallTimeout = 30 * time.Minute
	}
	if progressTick <= 0 {
		progressTick = 5 * time.Second
	}
	return &WorkerPool{
		workers:      workers,
		jobRepo:      jobRepo,
		tc:           tc,
		metrics:      libMetrics,
		stallTimeout: stallTimeout,
		progressTick: progressTick,
		pollInterval: 2 * time.Second,
		log:          log,
		handles:      make(map[string]libtorrent.DownloadHandle),
	}
}

// Start launches the worker goroutines + a 5s active-count publisher.
// Returns immediately; goroutines exit on <-ctx.Done().
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.runWorker(ctx, i)
	}
	p.wg.Add(1)
	go p.publishActiveCount(ctx)
}

// runWorker is one goroutine: claim → process → loop. On empty queue
// it sleeps pollInterval before retrying.
func (p *WorkerPool) runWorker(ctx context.Context, idx int) {
	defer p.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := p.jobRepo.Claim(ctx)
		if err != nil {
			if p.log != nil {
				p.log.Warnw("worker claim failed", "worker", idx, "error", err)
			}
			if !p.sleep(ctx, p.pollInterval) {
				return
			}
			continue
		}
		if job == nil {
			if !p.sleep(ctx, p.pollInterval) {
				return
			}
			continue
		}
		p.processJob(ctx, job)
	}
}

// sleep is a ctx-aware time.Sleep. Returns false when the context
// fires.
func (p *WorkerPool) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// processJob drives a single claimed job through downloading →
// encoding (the Phase-3 stop point). Returns when the download
// completes, the job is cancelled, or stall detection fires.
func (p *WorkerPool) processJob(ctx context.Context, job *domain.Job) {
	if p.metrics != nil {
		p.metrics.IncJobsTotal(string(domain.JobStatusDownloading))
	}

	handle, err := p.tc.Add(ctx, job.Magnet)
	if err != nil {
		_ = p.jobRepo.UpdateStatus(ctx, job.ID, domain.JobStatusFailed, err.Error())
		if p.metrics != nil {
			p.metrics.IncJobsTotal(string(domain.JobStatusFailed))
		}
		if p.log != nil {
			p.log.Errorw("torrent add failed", "job_id", job.ID, "error", err)
		}
		return
	}

	p.registerHandle(job.ID, handle)
	defer p.unregisterHandle(job.ID)
	defer handle.Cancel() // defensive — releases peers on any exit path

	lastNonZeroPeerAt := time.Now()
	var lastReportedBytes int64

	tick := time.NewTicker(p.progressTick)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			// Service shutdown — leave row as downloading; the
			// graceful Stop() rewrites it back to queued.
			return

		case <-handle.Done():
			// Download finished (or Cancel() fired). Re-read the row
			// to decide whether to transition to encoding or accept
			// the cancellation already written by the DELETE path.
			fresh, err := p.jobRepo.GetByID(ctx, job.ID)
			if err == nil && fresh != nil && fresh.Status == domain.JobStatusCancelled {
				if p.metrics != nil {
					p.metrics.IncJobsTotal(string(domain.JobStatusCancelled))
				}
				return
			}
			// Otherwise drive to encoding — Phase 4 picks up here.
			if err := p.jobRepo.UpdateStatus(ctx, job.ID, domain.JobStatusEncoding, ""); err != nil {
				if p.log != nil {
					p.log.Errorw("update status encoding", "job_id", job.ID, "error", err)
				}
				return
			}
			if p.metrics != nil {
				p.metrics.IncJobsTotal(string(domain.JobStatusEncoding))
			}
			return

		case <-tick.C:
			downloaded, total, peers := handle.Progress()

			// Stall detection FIRST — if we've been at zero peers
			// for >= stallTimeout, fail the job and exit. We update
			// lastNonZeroPeerAt only when peers>0 so a long
			// peer-drought ages out the timestamp naturally.
			if peers > 0 {
				lastNonZeroPeerAt = time.Now()
			}
			if time.Since(lastNonZeroPeerAt) >= p.stallTimeout {
				_ = p.jobRepo.UpdateStatus(ctx, job.ID, domain.JobStatusFailed, stallErrorText)
				if p.metrics != nil {
					p.metrics.IncJobsTotal(string(domain.JobStatusFailed))
				}
				if p.log != nil {
					p.log.Warnw("download stalled", "job_id", job.ID, "stall_timeout", p.stallTimeout)
				}
				return
			}

			// Update progress + emit bytes delta.
			if err := p.jobRepo.UpdateProgress(ctx, job.ID, downloaded, total, peers); err != nil {
				if p.log != nil {
					p.log.Warnw("update progress failed", "job_id", job.ID, "error", err)
				}
			}
			if p.metrics != nil && downloaded > lastReportedBytes {
				p.metrics.AddDownloadBytes(downloaded - lastReportedBytes)
				lastReportedBytes = downloaded
			}

			// Re-read the row — if it's cancelled, exit. The DELETE
			// handler already wrote status=cancelled before signalling
			// the handle; we observe it on the next tick and exit
			// cleanly.
			fresh, err := p.jobRepo.GetByID(ctx, job.ID)
			if err == nil && fresh != nil && fresh.Status == domain.JobStatusCancelled {
				if p.metrics != nil {
					p.metrics.IncJobsTotal(string(domain.JobStatusCancelled))
				}
				return
			}
		}
	}
}

// CancelJob is the public cancel path called by the DELETE handler.
// Status flip happens FIRST in the DB, then the in-memory handle
// is signalled. This ordering (CONTEXT-locked) guarantees that even
// if the in-memory Cancel is lost in a crash, the next progress
// tick observes the cancelled status and exits gracefully.
func (p *WorkerPool) CancelJob(ctx context.Context, jobID string) error {
	if err := p.jobRepo.Cancel(ctx, jobID); err != nil {
		return err
	}
	p.handlesMu.RLock()
	h := p.handles[jobID]
	p.handlesMu.RUnlock()
	if h != nil {
		h.Cancel()
	}
	return nil
}

// Stop tears the pool down on SIGTERM. It cancels every in-memory
// handle, waits up to timeout for the worker goroutines to exit, then
// rewrites any row still in status='downloading' (because the worker
// was mid-flight) back to 'queued' so a future process re-claims it.
//
// This mirrors the startup ResumeInterruptedDownloads hook from the
// repo. Together they guarantee resumption semantics across restarts.
func (p *WorkerPool) Stop(ctx context.Context, timeout time.Duration) error {
	// Snapshot active job IDs before cancelling so we can rewrite them.
	p.handlesMu.RLock()
	active := make([]string, 0, len(p.handles))
	for id, h := range p.handles {
		active = append(active, id)
		h.Cancel()
	}
	p.handlesMu.RUnlock()

	// Wait for goroutines to exit, but not forever.
	doneCh := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(timeout):
		// Pool didn't drain in time — log and continue. The DB
		// rewrite below still happens so the next process can
		// resume.
		if p.log != nil {
			p.log.Warnw("worker pool stop timed out", "timeout", timeout)
		}
	}

	// Rewrite still-downloading rows back to queued so they get
	// re-claimed next time around. We use UpdateStatus per row so
	// the bookkeeping (updated_at) flows through.
	rewriteCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, id := range active {
		_ = p.jobRepo.UpdateStatus(rewriteCtx, id, domain.JobStatusQueued, "")
	}
	return nil
}

// ActiveCount returns the number of active in-memory handles.
func (p *WorkerPool) ActiveCount() int {
	p.handlesMu.RLock()
	defer p.handlesMu.RUnlock()
	return len(p.handles)
}

func (p *WorkerPool) registerHandle(id string, h libtorrent.DownloadHandle) {
	p.handlesMu.Lock()
	defer p.handlesMu.Unlock()
	p.handles[id] = h
}

func (p *WorkerPool) unregisterHandle(id string) {
	p.handlesMu.Lock()
	defer p.handlesMu.Unlock()
	delete(p.handles, id)
}

// publishActiveCount ticks every 5s and updates the active-torrents
// gauge from the in-memory handle map.
func (p *WorkerPool) publishActiveCount(ctx context.Context) {
	defer p.wg.Done()
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if p.metrics != nil {
				p.metrics.SetActiveTorrents(p.ActiveCount())
			}
		}
	}
}

