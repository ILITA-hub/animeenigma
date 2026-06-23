package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

const (
	// leaseTTL is how long a worker has to process a segment before it is
	// considered stale and re-queued by the sweeper.
	leaseTTL = 10 * time.Minute

	// graceWindow extends handle validity beyond the lease TTL so the worker
	// can complete an upload even if it finishes right at the deadline.
	graceWindow = 2 * time.Minute
)

// LeasedHandles is a value type mirroring protocol.LeaseHandles; returned by
// Leaser.OnLeaseReq so the hub can build the frame payload without importing
// the controlplane package from service.
type LeasedHandles = controlplane.LeaseHandles

// jobEligibleRepo is the minimal JobRepository interface the Leaser needs.
type jobEligibleRepo interface {
	NextEligible(ctx context.Context) (*domain.UpscaleJob, error)
	UpdateStatus(ctx context.Context, id string, status domain.JobStatus, errText string) error
}

// segmentLeaserRepo is the minimal SegmentRepository interface the Leaser needs.
type segmentLeaserRepo interface {
	LeaseNext(ctx context.Context, jobID, workerID string, ttl time.Duration) (*domain.UpscaleSegment, error)
	Counts(ctx context.Context, jobID string) (pending, leased, done int, err error)
}

// workerHeartbeater is the minimal WorkerRepository interface the Leaser needs.
type workerHeartbeater interface {
	Heartbeat(ctx context.Context, workerID, jobID string, seg int, now time.Time) error
}

// Leaser picks the next eligible job/segment and mints capability handles.
type Leaser struct {
	jobs    jobEligibleRepo
	segs    segmentLeaserRepo
	workers workerHeartbeater
	log     *logger.Logger
}

// NewLeaser constructs a Leaser with the default logger.
func NewLeaser(jobs jobEligibleRepo, segs segmentLeaserRepo, workers workerHeartbeater) *Leaser {
	return &Leaser{jobs: jobs, segs: segs, workers: workers, log: logger.Default()}
}

// NewLeaserWithLogger constructs a Leaser with an explicit logger.
func NewLeaserWithLogger(jobs jobEligibleRepo, segs segmentLeaserRepo, workers workerHeartbeater, log *logger.Logger) *Leaser {
	if log == nil {
		log = logger.Default()
	}
	return &Leaser{jobs: jobs, segs: segs, workers: workers, log: log}
}

// OnLeaseReq handles a lease_req frame from the given worker:
//   - Finds the next eligible job (via NextEligible).
//   - Tries to claim the next available segment (via LeaseNext).
//   - When all segments are done (LeaseNext returns nil AND pending=0,leased=0,done>0),
//     flips the job to JobFinalizing.
//   - Returns (nil, zero, nil) when there is nothing to lease (no jobs, or job
//     not yet segmented).
//   - On success, returns the claimed segment, pre-minted HMAC handles, and nil.
func (l *Leaser) OnLeaseReq(ctx context.Context, workerID string) (*domain.UpscaleSegment, LeasedHandles, error) {
	zero := LeasedHandles{}

	// Find the oldest eligible job.
	job, err := l.jobs.NextEligible(ctx)
	if err != nil {
		return nil, zero, err
	}
	if job == nil {
		return nil, zero, nil
	}

	// Try to claim the next segment.
	seg, err := l.segs.LeaseNext(ctx, job.ID, workerID, leaseTTL)
	if err != nil {
		return nil, zero, err
	}

	if seg == nil {
		// No segment available — check why.
		pending, leased, done, cerr := l.segs.Counts(ctx, job.ID)
		if cerr != nil {
			return nil, zero, cerr
		}
		if pending == 0 && leased == 0 && done > 0 {
			// All segments are done — flip job to finalizing.
			if uerr := l.jobs.UpdateStatus(ctx, job.ID, domain.JobFinalizing, ""); uerr != nil {
				return nil, zero, uerr
			}
		}
		// If done==0 as well, the job has no segments yet (still segmenting);
		// return nothing.
		return nil, zero, nil
	}

	// Mint capability handles valid for leaseTTL + graceWindow.
	handleTTL := leaseTTL + graceWindow
	getHandle, getExp, getSig := capability.MintJobHandle(job.ID, "segment-get", seg.Idx, handleTTL)
	putHandle, putExp, putSig := capability.MintJobHandle(job.ID, "segment-put", seg.Idx, handleTTL)

	handles := LeasedHandles{
		GetHandle: getHandle,
		GetExp:    getExp,
		GetSig:    getSig,
		PutHandle: putHandle,
		PutExp:    putExp,
		PutSig:    putSig,
	}

	// Record the worker's current assignment. A heartbeat failure here does NOT
	// fail the lease — the segment lease (LeaseNext) is already durable, so the
	// worker can proceed; the sweeper's liveness check is the safety net. Log at
	// warn for parity with the hub's heartbeat path (M-2/M-3).
	if err := l.workers.Heartbeat(ctx, workerID, job.ID, seg.Idx, time.Now()); err != nil {
		l.log.Warnw("leaser: worker heartbeat update failed (lease still granted)",
			"worker_id", workerID, "job_id", job.ID, "segment_idx", seg.Idx, "error", err)
	}

	return seg, handles, nil
}
