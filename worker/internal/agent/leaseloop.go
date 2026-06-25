package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ILITA-hub/animeenigma/worker/internal/upscale"
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
// processes granted segments via the per-job model selection, and loops.
// It runs until ctx is cancelled or a fatal error occurs.
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
func (c *Client) RunLeaseLoop(ctx context.Context, workerID string, conn leaseConn) error {
	// drainCh is the server-sent "drain" signal: once closed, the worker stops
	// requesting NEW leases, finishes any in-flight segment, and then idles until
	// ctx is cancelled (B4 graceful-drain wiring). nil-safe: a nil commandHandler
	// (only possible in narrow unit tests) yields a nil channel, which a select
	// treats as "never ready" — i.e. drain is simply inert.
	var drainCh <-chan struct{}
	if c.commandHandler != nil {
		drainCh = c.commandHandler.DrainCh
	}

	seq := leaseReqSeqStart
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// If a drain was requested, stop requesting new work and park until the
		// connection/root context is torn down. Any segment already in flight has
		// completed by this point (processSegment below is synchronous).
		select {
		case <-drainCh:
			c.print("idle")
			<-ctx.Done()
			return ctx.Err()
		default:
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

		// Empty grant = no work; idle and retry. Honour drain during the idle wait
		// so a drain that arrives while idle is acted on without a full idleDelay.
		if grant.JobID == "" {
			c.print("idle")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-drainCh:
				c.print("idle")
				<-ctx.Done()
				return ctx.Err()
			case <-time.After(idleDelay):
			}
			continue
		}

		c.print("leased")

		// Process the granted segment.
		if err := c.processSegment(ctx, workerID, grant); err != nil {
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

// processSegment downloads the input segment, processes it via the per-job
// model (selected from the manager or the processorFn test seam), uploads the
// output, and deletes both local files (process-and-delete pattern).
//
// Model selection order:
//  1. If c.processorFn is set (test seam), call it to build a Processor.
//  2. Otherwise, resolve the model name from grant.Model (empty → "mock") via
//     c.manager.Get. If found, wrap it in a PipelineProcessor and process.
//  3. If the model is not locally available, fail the segment cleanly and log
//     a clear error so the server re-leases to another worker.
//     // T29: pull-on-demand fetch here
func (c *Client) processSegment(ctx context.Context, workerID string, grant wire.LeaseGrantPayload) error {
	// Build the base segment URL on the server's data plane.
	segBaseURL := fmt.Sprintf("%s/worker/segments/%s/%d", c.cfg.ServerURL, grant.JobID, grant.Idx)

	// Temp paths for local in/out files. Use cfg.WorkDir so the operator can
	// point the worker at a fast NVMe scratch volume for segment staging.
	workDir := c.cfg.WorkDir
	if workDir == "" {
		workDir = os.TempDir()
	}
	inPath := fmt.Sprintf("%s/seg-%s-%d-in", workDir, grant.JobID, grant.Idx)
	outPath := fmt.Sprintf("%s/seg-%s-%d-out", workDir, grant.JobID, grant.Idx)

	// Always clean up local files regardless of outcome.
	defer func() {
		os.Remove(inPath)  //nolint:errcheck
		os.Remove(outPath) //nolint:errcheck
	}()

	// Per-segment cancellable context (I4): a server "cancel" command aborts the
	// in-flight download/ffmpeg/upload so the worker stops burning metered GPU on
	// a job the server no longer wants. segCtx is derived from the loop ctx, so it
	// is also cancelled when the connection drops or the worker shuts down. Its
	// cancel func is registered with the command handler for the duration of this
	// segment and reset to a no-op in the defer once the segment finishes, so a
	// late cancel can never tear down the NEXT segment. The same context bounds
	// per-segment telemetry, so the (jobID, segIdx) attribution Telemetry.Run
	// stamps is always for the segment actually in flight.
	segCtx, cancelSeg := context.WithCancel(ctx)
	defer cancelSeg()
	if c.commandHandler != nil {
		c.commandHandler.SetCancel(cancelSeg)
		defer c.commandHandler.SetCancel(nil) // reset to no-op when the segment ends
	}

	// Resolve the processor for this segment.
	var proc Processor

	if c.processorFn != nil {
		// Test seam: processorFn overrides per-job selection entirely.
		var err error
		proc, err = c.processorFn(c.cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "worker: processorFn error, falling back to CopyProcessor: %v\n", err)
			proc = CopyProcessor{}
		}
	} else {
		// Per-job model selection from the manager.
		modelName := grant.Model
		if modelName == "" {
			modelName = "mock"
		}
		model, ok := c.manager.Get(modelName)
		if !ok {
			// T29: pull-on-demand fetch here
			return fmt.Errorf("model %q not available locally; server will re-lease", modelName)
		}

		scale := grant.Scale
		if scale <= 0 {
			scale = c.cfg.Scale
		}
		proc = newModelProcessor(model, scale, workDir)
	}

	// statsFn surfaces the processor's REAL measured fps when it implements
	// StatsSource (PipelineProcessor); processors that don't (the CopyProcessor
	// stub) contribute zero fps but still emit GPU/heartbeat data.
	var statsFn func() Stats
	if ss, ok := proc.(StatsSource); ok {
		statsFn = ss.LiveStats
	}
	tel := NewTelemetry(c.send, c.heartbeatInterval, c.metricsInterval, statsFn)
	go tel.Run(segCtx, grant.JobID, grant.Idx)

	c.print("processing")

	// 1. Download input segment using the GET capability handle.
	if err := Download(segCtx, c.cfg, workerID,
		segBaseURL,
		grant.Handles.GetHandle,
		grant.Handles.GetExp,
		grant.Handles.GetSig,
		inPath,
	); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// 2. Process the segment via the per-segment cancellable context so a server
	// cancel aborts the in-flight ffmpeg/model run.
	if _, err := proc.Process(segCtx, inPath, outPath); err != nil {
		return fmt.Errorf("process: %w", err)
	}

	// 3. Upload output segment using the PUT capability handle.
	// A successful PUT implicitly marks the segment done on the server.
	if err := Upload(segCtx, c.cfg, workerID,
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

// newModelProcessor wraps an upscale.Model in a PipelineProcessor-equivalent
// Processor using the pipeline package's Process function.
func newModelProcessor(model upscale.Model, scale int, workDir string) Processor {
	return &PipelineProcessor{
		model:   model,
		scale:   scale,
		workDir: workDir,
	}
}

// leaseConn is the minimal interface the lease loop needs to receive grant
// frames. In production this is backed by a gorillaws.Conn read via the
// readPump; in tests it is a fake that can inject specific grant sequences.
type leaseConn interface {
	// ReadGrant blocks until a lease_grant frame arrives or ctx is cancelled.
	// An empty LeaseGrantPayload (JobID=="") means no work is available.
	ReadGrant(ctx context.Context) (wire.LeaseGrantPayload, error)
}
