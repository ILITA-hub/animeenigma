package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/analyticsclient"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	gorillaws "github.com/gorilla/websocket"
)

// Default timing constants for the WS control-plane pump.
const (
	defaultPongWait   = 60 * time.Second
	defaultPingPeriod = 30 * time.Second // must be < pongWait
	defaultWriteWait  = 10 * time.Second
	defaultMaxMsgSize = 64 * 1024 // 64 KiB read limit
)

// HubConfig allows timing constants to be overridden for tests.
type HubConfig struct {
	PongWait   time.Duration
	PingPeriod time.Duration
	WriteWait  time.Duration
	MaxMsgSize int64
}

func (c HubConfig) withDefaults() HubConfig {
	if c.PongWait == 0 {
		c.PongWait = defaultPongWait
	}
	if c.PingPeriod == 0 {
		c.PingPeriod = defaultPingPeriod
	}
	if c.WriteWait == 0 {
		c.WriteWait = defaultWriteWait
	}
	if c.MaxMsgSize == 0 {
		c.MaxMsgSize = defaultMaxMsgSize
	}
	return c
}

// Leaser is the interface the Hub uses to dispatch lease_req frames.
type Leaser interface {
	OnLeaseReq(ctx context.Context, workerID string) (*domain.UpscaleSegment, LeaseHandles, string, int, error)
}

// WorkerHeartbeater is the minimal WorkerRepository surface the Hub needs.
type WorkerHeartbeater interface {
	Heartbeat(ctx context.Context, workerID, jobID string, seg int, now time.Time) error
}

// Hub is the worker WebSocket connection registry keyed by worker_id.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*Conn

	leaser          Leaser
	workers         WorkerHeartbeater
	log             *logger.Logger
	cfg             HubConfig
	execRouter      ExecRouter             // optional; nil = ignore exec frames
	analyticsClient *analyticsclient.Client // optional; nil = telemetry forwarding disabled
}

// SetExecRouter wires an ExecRouter to handle exec_data and exec_close frames
// received from workers. Must be called before any worker connects.
func (h *Hub) SetExecRouter(r ExecRouter) {
	h.mu.Lock()
	h.execRouter = r
	h.mu.Unlock()
}

// SetAnalyticsClient wires an analyticsclient.Client to forward per-worker
// GPU telemetry to ClickHouse via the analytics service (CD-15). A nil client
// disables telemetry forwarding (safe no-op on Send). Must be called before
// any worker connects.
func (h *Hub) SetAnalyticsClient(c *analyticsclient.Client) {
	h.mu.Lock()
	h.analyticsClient = c
	h.mu.Unlock()
}

// NewHub constructs a Hub with default timing constants.
func NewHub(leaser Leaser, workers WorkerHeartbeater, log *logger.Logger) *Hub {
	return NewHubWithConfig(leaser, workers, log, HubConfig{})
}

// NewHubWithConfig constructs a Hub with explicit timing (useful in tests).
func NewHubWithConfig(leaser Leaser, workers WorkerHeartbeater, log *logger.Logger, cfg HubConfig) *Hub {
	return &Hub{
		conns:   make(map[string]*Conn),
		leaser:  leaser,
		workers: workers,
		log:     log,
		cfg:     cfg.withDefaults(),
	}
}

// Register adds a connection to the hub and starts its read/write pumps.
func (h *Hub) Register(conn *Conn) {
	h.mu.Lock()
	h.conns[conn.workerID] = conn
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	conn.ctx = ctx
	conn.cancel = cancel

	go conn.writePump()
	go conn.readPump()
}

// Unregister removes a connection from the hub and closes it.
// It also notifies the ExecRouter (if wired) so any active exec sessions for
// the departing worker are torn down and their admin channels receive exec_close.
func (h *Hub) Unregister(workerID string) {
	h.mu.Lock()
	c, ok := h.conns[workerID]
	if ok {
		delete(h.conns, workerID)
	}
	er := h.execRouter
	h.mu.Unlock()

	if ok {
		c.close()
	}

	// Notify ExecRelay (if wired) after releasing the lock so the relay's own
	// mutex cannot deadlock with ours during cleanup.
	if er != nil {
		// WorkerGone must be a method on the ExecRouter that accepts a workerID.
		// ExecRelay implements this via WorkerGone; the interface only exposes
		// DeliverFromWorker, so we type-assert here to keep the interface minimal.
		type workerGoner interface {
			WorkerGone(workerID string)
		}
		if wg, ok := er.(workerGoner); ok {
			wg.WorkerGone(workerID)
		}
	}
}

// Send marshals f and enqueues it on the named worker's send channel.
// Returns an error when the worker is not connected or the channel is full.
func (h *Hub) Send(workerID string, f Frame) error {
	h.mu.RLock()
	c, ok := h.conns[workerID]
	h.mu.RUnlock()
	if !ok {
		return errWorkerNotFound
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	select {
	case c.send <- raw:
		return nil
	default:
		return errSendBufferFull
	}
}

// Broadcast marshals f and enqueues it on every connected worker's send channel.
func (h *Hub) Broadcast(f Frame) {
	raw, err := json.Marshal(f)
	if err != nil {
		return
	}
	h.mu.RLock()
	conns := make([]*Conn, 0, len(h.conns))
	for _, c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		select {
		case c.send <- raw:
		default:
			// Buffer full — skip this connection; sweeper will clean it up.
		}
	}
}

// ── Sentinel errors ───────────────────────────────────────────────────────────

type hubError string

func (e hubError) Error() string { return string(e) }

const (
	errWorkerNotFound hubError = "hub: worker not connected"
	errSendBufferFull hubError = "hub: send buffer full"
)

// ── Conn ─────────────────────────────────────────────────────────────────────

const connSendBuf = 64

// Conn represents one worker WebSocket connection.
type Conn struct {
	workerID  string
	ws        *gorillaws.Conn
	send      chan []byte
	hub       *Hub
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	// leaseInFlight is a per-connection single-flight guard for the lease path.
	// A worker only ever has one lease in flight at a time, so a second
	// lease_req arriving while the first is still being resolved (NextEligible +
	// LeaseNext TX + Heartbeat) is a duplicate and is dropped. This bounds the
	// number of lease goroutines to ≤1 per connection.
	leaseInFlight atomic.Bool

	// lastJobID and lastSegmentIdx carry the job/segment attribution for the
	// most recent heartbeat frame. They are READ by the metrics frame handler
	// and WRITTEN by the heartbeat frame handler. Both fields are accessed only
	// from the single per-connection read-pump goroutine (dispatch is called
	// sequentially by readPump; only the lease path is offloaded to a goroutine
	// and it does NOT touch these fields) — so NO locking is needed.
	lastJobID      string
	lastSegmentIdx int // initialised to -1 (sentinel: no heartbeat yet; 0 is a valid first segment)
}

func newConn(workerID string, ws *gorillaws.Conn, hub *Hub) *Conn {
	return &Conn{
		workerID:       workerID,
		ws:             ws,
		send:           make(chan []byte, connSendBuf),
		hub:            hub,
		lastSegmentIdx: -1, // sentinel: no heartbeat received yet
	}
}

// close cancels the connection context and closes the WebSocket once.
func (c *Conn) close() {
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.ws.Close()
	})
}

// readPump reads frames from the WebSocket and dispatches them.
// It unregisters the connection on return.
func (c *Conn) readPump() {
	defer c.hub.Unregister(c.workerID)

	cfg := c.hub.cfg
	c.ws.SetReadLimit(cfg.MaxMsgSize)
	c.ws.SetReadDeadline(time.Now().Add(cfg.PongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(cfg.PongWait))
	})

	for {
		_, msg, err := c.ws.ReadMessage()
		if err != nil {
			// Connection closed or errored — stop the pump.
			if gorillaws.IsUnexpectedCloseError(err,
				gorillaws.CloseGoingAway,
				gorillaws.CloseNormalClosure,
				gorillaws.CloseAbnormalClosure,
			) {
				c.hub.log.Warnw("controlplane: unexpected ws close", "worker_id", c.workerID, "error", err)
			}
			return
		}

		var f Frame
		if err := json.Unmarshal(msg, &f); err != nil {
			c.hub.log.Warnw("controlplane: bad frame JSON", "worker_id", c.workerID, "error", err)
			continue
		}

		c.dispatch(f)
	}
}

// dispatch routes a decoded frame to the appropriate handler.
//
// The lease path is offloaded to a goroutine so readPump returns to
// ReadMessage immediately — otherwise the 3+ DB round-trips inside
// OnLeaseReq (NextEligible + LeaseNext TX + Heartbeat) would block the read
// loop, so an inbound PONG control frame couldn't be processed and a lease
// taking >pongWait under DB contention would tear the connection down (I-2).
// heartbeat stays inline — it is one fast DB call.
func (c *Conn) dispatch(f Frame) {
	ctx := c.ctx

	switch f.Type {
	case "lease_req":
		c.handleLeaseReq(f.Seq)

	case "heartbeat":
		var hb HeartbeatPayload
		if err := f.Decode(&hb); err != nil {
			c.hub.log.Warnw("controlplane: bad heartbeat payload", "worker_id", c.workerID, "error", err)
			return
		}
		// Update job/segment attribution for the next metrics frame.
		// Safe without a lock: both fields are accessed only from this
		// single per-connection read-pump goroutine.
		c.lastJobID = hb.JobID
		c.lastSegmentIdx = hb.SegmentIdx
		if err := c.hub.workers.Heartbeat(ctx, c.workerID, hb.JobID, hb.SegmentIdx, time.Now()); err != nil {
			c.hub.log.Warnw("controlplane: heartbeat DB error", "worker_id", c.workerID, "error", err)
		}

	case "metrics":
		var mp MetricsPayload
		if err := f.Decode(&mp); err != nil {
			c.hub.log.Warnw("controlplane: bad metrics payload", "worker_id", c.workerID, "error", err)
			return
		}
		// Prometheus (low-cardinality, reduced field set) — keep exactly as-is.
		metrics.RecordWorkerTelemetry(mp.GPUModel, mp.ImageVersion, mp.GPUUtil, mp.VRAMUsedBytes, mp.DecodeFPS, mp.InferenceFPS, mp.EncodeFPS)
		// ClickHouse (high-cardinality, full payload) via analyticsclient (CD-15).
		// worker_id and ts are stamped SERVER-SIDE; job_id/segment_idx come from
		// the last heartbeat frame (both read from the same read-pump goroutine —
		// no lock needed). A nil client is a safe no-op.
		c.hub.mu.RLock()
		ac := c.hub.analyticsClient
		c.hub.mu.RUnlock()
		ac.Send(analyticsclient.UpscaleTelemetryRow{
			TS:           time.Now(),
			WorkerID:     c.workerID,
			GPUModel:     mp.GPUModel,
			ImageVersion: mp.ImageVersion,
			JobID:        c.lastJobID,
			SegmentIdx:   int32(c.lastSegmentIdx),
			GPUUtil:      float32(mp.GPUUtil),
			VRAMUsedB:    uint64(mp.VRAMUsedBytes),
			VRAMTotalB:   uint64(mp.VRAMTotalBytes),
			GPUTempC:     float32(mp.GPUTempC),
			GPUPowerW:    float32(mp.GPUPowerW),
			DecodeFPS:    float32(mp.DecodeFPS),
			InferenceFPS: float32(mp.InferenceFPS),
			EncodeFPS:    float32(mp.EncodeFPS),
		})

	case "register":
		// Optional — worker metadata update. Currently a no-op: the enrollment
		// flow already created the worker row; future work can update gpu_info etc.

	case "exec_data", "exec_close":
		// Route exec frames to the ExecRelay (if wired); ignore otherwise.
		c.hub.mu.RLock()
		er := c.hub.execRouter
		c.hub.mu.RUnlock()
		if er != nil {
			er.DeliverFromWorker(f)
		}

	default:
		// Unknown frame types are silently ignored so new server-side frame types
		// don't break old workers (forward-compat).
	}
}

// handleLeaseReq resolves a lease OFF the read loop.
//
// Single-flight: if a lease is already being resolved for this connection the
// duplicate request is dropped (a worker holds at most one lease at a time, so
// this is correct and bounds lease goroutines to ≤1 per connection). The
// goroutine clears the flag when done and stops if the connection closes.
func (c *Conn) handleLeaseReq(reqSeq int) {
	if !c.leaseInFlight.CompareAndSwap(false, true) {
		// A lease is already in flight for this connection — drop the duplicate.
		c.hub.log.Warnw("controlplane: duplicate lease_req while one is in flight, ignoring", "worker_id", c.workerID)
		return
	}

	go func() {
		defer c.leaseInFlight.Store(false)

		seg, handles, model, scale, err := c.hub.leaser.OnLeaseReq(c.ctx, c.workerID)
		if err != nil {
			c.hub.log.Warnw("controlplane: lease_req error", "worker_id", c.workerID, "error", err)
			return
		}
		if seg == nil {
			// Nothing to lease right now — silently ignore (worker will retry).
			return
		}

		// Build the lease grant payload with model/scale. For non-mock, non-empty
		// models, mint a model-fetch capability handle using the same TTL convention
		// as the segment handles (leaseTTL + graceWindow = handleTTL).
		//
		// The worker uses this handle to fetch the model binary from:
		//   GET {SERVER_URL}/worker/models/{Model}?exp=&sig=
		// T27 builds the server-side model-fetch handler.
		handleTTL := 12 * time.Minute // leaseTTL(10m) + graceWindow(2m)
		payload := LeaseGrantPayload{
			JobID:   seg.JobID,
			Idx:     seg.Idx,
			Handles: handles,
			Model:   model,
			Scale:   scale,
		}
		if model != "" && model != "mock" {
			_, exp, sig := capability.MintJobHandle(seg.JobID, "model", 0, handleTTL)
			payload.ModelHandle = &ModelHandle{Exp: exp, Sig: sig}
		}

		grant, err := NewFrame("lease_grant", reqSeq+1, payload)
		if err != nil {
			c.hub.log.Warnw("controlplane: marshal lease_grant", "worker_id", c.workerID, "error", err)
			return
		}

		// Stop if the connection closed while we were resolving the lease.
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Route through Hub.Send for identical non-blocking semantics + the
		// declared sentinel errors (M-1 / removes the M-4 dead-code).
		if err := c.hub.Send(c.workerID, grant); err != nil {
			switch {
			case errors.Is(err, errSendBufferFull):
				c.hub.log.Warnw("controlplane: send buffer full, dropping lease_grant", "worker_id", c.workerID)
			case errors.Is(err, errWorkerNotFound):
				// Connection dropped between resolve and send — benign.
			default:
				c.hub.log.Warnw("controlplane: send lease_grant failed", "worker_id", c.workerID, "error", err)
			}
		}
	}()
}

// writePump drains the send channel and periodically sends WS pings.
func (c *Conn) writePump() {
	cfg := c.hub.cfg
	ticker := time.NewTicker(cfg.PingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
			if !ok {
				// Channel closed.
				c.ws.WriteMessage(gorillaws.CloseMessage, []byte{})
				return
			}
			if err := c.ws.WriteMessage(gorillaws.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.ws.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
			if err := c.ws.WriteMessage(gorillaws.PingMessage, nil); err != nil {
				return
			}

		case <-c.ctx.Done():
			c.ws.SetWriteDeadline(time.Now().Add(cfg.WriteWait))
			c.ws.WriteControl(gorillaws.CloseMessage,
				gorillaws.FormatCloseMessage(gorillaws.CloseGoingAway, "server shutdown"),
				time.Now().Add(cfg.WriteWait))
			return
		}
	}
}
