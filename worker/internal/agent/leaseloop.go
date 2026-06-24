package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

const (
	// idleDelay is how long the worker waits before re-requesting a lease
	// when the server returns an empty grant (no work available).
	idleDelay = 2 * time.Second

	// leaseReqSeqStart is the seq number for the first lease_req frame sent
	// after register. Subsequent requests increment by 1.
	leaseReqSeqStart = 2
)

// RunLeaseLoop sends lease_req frames over the Client's send channel,
// processes granted segments via proc, and loops. It runs until ctx is
// cancelled or a fatal error occurs.
//
// Segments are processed single-flight: one segment in flight at a time.
// The server already enforces single-flight on its side (one lease_req
// per connection at a time), so this is defence-in-depth.
//
// RunLeaseLoop is called from the read pump after the register frame is
// confirmed; it shares the Client's send channel for outbound frames.
//
// workerID is the session worker_id from enrollment, used to bind data-plane
// requests to the lease (X-Worker-Id header).
func (c *Client) RunLeaseLoop(ctx context.Context, workerID string, proc Processor, conn leaseConn) error {
	seq := leaseReqSeqStart
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Request a lease.
		if err := c.sendLeaseReq(seq); err != nil {
			return fmt.Errorf("lease_req seq=%d: %w", seq, err)
		}
		seq++

		// Wait for the grant.
		grant, err := conn.ReadGrant(ctx)
		if err != nil {
			return fmt.Errorf("read lease_grant: %w", err)
		}

		// Empty grant = no work; idle and retry.
		if grant.JobID == "" {
			c.print("idle")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(idleDelay):
			}
			continue
		}

		c.print("leased")

		// Process the granted segment.
		if err := c.processSegment(ctx, workerID, grant, proc); err != nil {
			// Non-fatal: log to stderr and loop (server will re-grant or timeout).
			fmt.Fprintf(os.Stderr, "worker: segment %s/%d: %v\n", grant.JobID, grant.Idx, err)
		}
	}
}

// sendLeaseReq enqueues a lease_req frame on the send channel.
func (c *Client) sendLeaseReq(seq int) error {
	f, err := wire.NewFrame("lease_req", seq, wire.LeaseReqPayload{})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	select {
	case c.send <- raw:
		return nil
	default:
		return fmt.Errorf("send channel full")
	}
}

// processSegment downloads the input segment, processes it via proc, uploads
// the output, and deletes both local files (process-and-delete pattern).
func (c *Client) processSegment(ctx context.Context, workerID string, grant wire.LeaseGrantPayload, proc Processor) error {
	// Build the base segment URL on the server's data plane.
	segBaseURL := fmt.Sprintf("%s/worker/segments/%s/%d", c.cfg.ServerURL, grant.JobID, grant.Idx)

	// Temp paths for local in/out files.
	inPath := fmt.Sprintf("%s/seg-%s-%d-in", os.TempDir(), grant.JobID, grant.Idx)
	outPath := fmt.Sprintf("%s/seg-%s-%d-out", os.TempDir(), grant.JobID, grant.Idx)

	// Always clean up local files regardless of outcome.
	defer func() {
		os.Remove(inPath)  //nolint:errcheck
		os.Remove(outPath) //nolint:errcheck
	}()

	c.print("processing")

	// 1. Download input segment using the GET capability handle.
	if err := Download(ctx, c.cfg, workerID,
		segBaseURL,
		grant.Handles.GetHandle,
		grant.Handles.GetExp,
		grant.Handles.GetSig,
		inPath,
	); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// 2. Process the segment (stub in Task 15; real pipeline in Task 17).
	if _, err := proc.Process(ctx, inPath, outPath); err != nil {
		return fmt.Errorf("process: %w", err)
	}

	// 3. Upload output segment using the PUT capability handle.
	// A successful PUT implicitly marks the segment done on the server.
	if err := Upload(ctx, c.cfg, workerID,
		segBaseURL,
		grant.Handles.PutHandle,
		grant.Handles.PutExp,
		grant.Handles.PutSig,
		outPath,
	); err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	// Local files are deleted by the deferred cleanup above.
	return nil
}

// leaseConn is the minimal interface the lease loop needs to receive grant
// frames. In production this is backed by a gorillaws.Conn read via the
// readPump; in tests it is a fake that can inject specific grant sequences.
type leaseConn interface {
	// ReadGrant blocks until a lease_grant frame arrives or ctx is cancelled.
	// An empty LeaseGrantPayload (JobID=="") means no work is available.
	ReadGrant(ctx context.Context) (wire.LeaseGrantPayload, error)
}
