package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/worker/internal/upscale"
	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

// chanLeaseConn implements leaseConn by reading from a channel of
// LeaseGrantPayload values. Used to bridge the WS dispatch path to RunLeaseLoop.
type chanLeaseConn struct {
	ch <-chan wire.LeaseGrantPayload
}

func (c chanLeaseConn) ReadGrant(ctx context.Context) (wire.LeaseGrantPayload, error) {
	select {
	case <-ctx.Done():
		return wire.LeaseGrantPayload{}, ctx.Err()
	case g, ok := <-c.ch:
		if !ok {
			return wire.LeaseGrantPayload{}, fmt.Errorf("grant channel closed")
		}
		return g, nil
	}
}

const (
	// pongWait is the read deadline extension on each pong received from the server.
	// Must match the server's pongWait (60s).
	pongWait = 60 * time.Second

	// maxMsgSize is the read limit per frame (64 KiB, matches server).
	maxMsgSize = 64 * 1024

	// sendBuf is the capacity of the client's outbound frame channel.
	sendBuf = 64

	// defaultHeartbeatInterval / defaultMetricsInterval are the per-segment
	// telemetry cadences. Heartbeat keeps the worker row's current_job_id /
	// current_segment fresh (and feeds the server's cancel-in-flight FindByJob);
	// metrics drives the Prometheus + ClickHouse observability stack. They are
	// well under the server's 60s pongWait so the connection is also kept warm.
	defaultHeartbeatInterval = 5 * time.Second
	defaultMetricsInterval   = 10 * time.Second
)

// BackoffConfig controls reconnect backoff timing. Exposed as a field on Client
// so tests can substitute faster values without modifying production constants.
type BackoffConfig struct {
	Initial time.Duration
	Max     time.Duration
}

var defaultBackoff = BackoffConfig{
	Initial: 1 * time.Second,
	Max:     30 * time.Second,
}

// Client is the worker-side dial-home agent. It enrolls with the server,
// maintains a WebSocket connection, and reconnects on disconnect.
type Client struct {
	cfg     Config
	backoff BackoffConfig
	send    chan []byte

	// manager is the thread-safe model registry. Always contains at least "mock".
	// processSegment selects the model per-job from this registry.
	manager *upscale.Manager

	// stdoutMu guards concurrent writes to stdout (print can be called from
	// multiple goroutines; tests read the buffer concurrently).
	stdoutMu sync.Mutex
	// stdout is the writer for the neutral console tokens. Defaults to os.Stdout.
	// Overridable in tests. Always accessed under stdoutMu.
	stdout io.Writer

	// connMu guards grantCh — reset at the start of every runOnce call and
	// closed when the connection drops so RunLeaseLoop exits cleanly.
	connMu  sync.Mutex
	grantCh chan wire.LeaseGrantPayload

	// processorFn, when non-nil, overrides the per-job model selection in
	// processSegment. Tests set this to inject CopyProcessor (or another stub)
	// so the end-to-end WS wiring test does not require a real ffmpeg binary.
	processorFn func(cfg Config) (Processor, error)

	// commandHandler handles server-sent command frames (cancel, drain, shutdown, etc.).
	commandHandler *CommandHandler

	// execHandler handles server-sent exec_open frames.
	execHandler *ExecHandler

	// heartbeatInterval / metricsInterval control the per-segment Telemetry
	// cadence. Defaulted in NewClient; overridable by tests for fast emission.
	heartbeatInterval time.Duration
	metricsInterval   time.Duration
}

// NewClient constructs a Client with default backoff settings and a model
// Manager initialised from cfg.PreinstalledModels. The manager always contains
// at least the built-in "mock" model.
func NewClient(cfg Config) *Client {
	send := make(chan []byte, sendBuf)
	// noopCancel is a placeholder until a real segment context is wired in via
	// CommandHandler.SetCancel. It is a plain no-op func so no context leaks
	// (an unused context.WithCancel would leak its cancel goroutine/timer).
	noopCancel := func() {}
	hbInterval := defaultHeartbeatInterval
	if cfg.HeartbeatInterval > 0 {
		hbInterval = cfg.HeartbeatInterval
	}
	metInterval := defaultMetricsInterval
	if cfg.MetricsInterval > 0 {
		metInterval = cfg.MetricsInterval
	}

	// Construct the model Manager over cfg.ModelsDir — the SAME directory that
	// holds the image's baked PreinstalledModels weights AND is the extraction
	// target for pull-on-demand Install (T29). It MUST be non-empty for Install
	// to write pulled weights; an empty dir would make every pull-on-demand
	// install fail with "modelsDir is not set". cfg defaults MODELS_DIR to
	// "/models" (provisioned by the worker image).
	mgr := upscale.NewManager(cfg.ModelsDir, cfg.PreinstalledModels)

	return &Client{
		cfg:               cfg,
		backoff:           defaultBackoff,
		send:              send,
		manager:           mgr,
		stdout:            os.Stdout,
		commandHandler:    NewCommandHandler(noopCancel),
		execHandler:       NewExecHandler(send),
		heartbeatInterval: hbInterval,
		metricsInterval:   metInterval,
	}
}

// errUnauthorized is returned by runOnce when the WS upgrade is rejected with
// 401, signalling that the session has expired and re-enrollment is needed.
type errUnauthorized struct{ msg string }

func (e errUnauthorized) Error() string { return e.msg }

// ErrSessionRejected is a TERMINAL error: the worker's credential was rejected
// and could not be recovered by re-enrollment. With permanent sessions
// (SessionTTL ~= 100yr) a 401 no longer means "session expired, just re-enroll";
// it means the credential is genuinely invalid/consumed — a terminal state the
// operator must resolve by re-provisioning the worker with a fresh enroll token.
// main() maps this to a distinct non-zero exit code (ExitCodeSessionRejected)
// so an orchestrator does NOT silently restart into an infinite crash-loop.
type ErrSessionRejected struct{ msg string }

func (e ErrSessionRejected) Error() string { return e.msg }

// ExitCodeSessionRejected is the distinct process exit code main() uses when
// Run returns ErrSessionRejected — lets the operator/orchestrator tell a
// terminal credential failure apart from a transient/network fatal (exit 1).
const ExitCodeSessionRejected = 2

// errEnrollUnauthorized reports whether err is an enroll failure caused by the
// server rejecting the (single-use) enroll token with 401 — i.e. the token was
// already consumed or revoked. This is the terminal-credential signal.
func errEnrollUnauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), "server returned 401")
}

// Run enrolls the worker with the server and then maintains a persistent
// WebSocket connection, reconnecting with exponential backoff on disconnect.
//
// Server sessions are effectively permanent (SessionTTL ~= 100yr), so a 401 on
// the WS upgrade now means the credential is genuinely invalid/consumed rather
// than merely expired. Run still attempts ONE re-enroll on 401 (covers the rare
// case of a server-side session-row eviction where the original enroll token is
// still valid); if that re-enroll is itself rejected with 401, the single-use
// token is consumed and the state is terminal — Run returns ErrSessionRejected
// so main() exits with a distinct code instead of crash-looping forever.
//
// Run honours a server-sent shutdown/update command: a watcher goroutine cancels
// the root context when CommandHandler.ShutdownCh closes, so the in-flight
// segment finishes and the worker exits cleanly. It returns when ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	c.print("starting")

	// Derive a cancellable root context so a server "shutdown"/"update" command
	// can stop the worker after the in-flight segment (B4: ShutdownCh wiring).
	ctx, cancelRoot := context.WithCancel(ctx)
	defer cancelRoot()
	go func() {
		select {
		case <-ctx.Done():
		case <-c.commandHandler.ShutdownCh:
			// Drain has already been signalled by Shutdown(); cancelling the root
			// context tears down the active connection + lease loop after the
			// in-flight segment completes, and Run returns ctx.Err()==Canceled.
			fmt.Fprintln(os.Stderr, "worker: shutdown command received; exiting after in-flight segment")
			cancelRoot()
		}
	}()

	enroll, err := c.enroll(ctx)
	if err != nil {
		c.print("error")
		if errEnrollUnauthorized(err) {
			fmt.Fprintln(os.Stderr, "worker: enroll token rejected (401); re-provision this worker with a fresh enroll token")
			return ErrSessionRejected{msg: fmt.Sprintf("enroll: %v", err)}
		}
		return fmt.Errorf("enroll: %w", err)
	}

	c.print("connected")

	delay := c.backoff.Initial
	for {
		wsErr := c.runOnce(ctx, enroll)
		if wsErr == nil || ctx.Err() != nil {
			// Either a clean shutdown or context cancelled — do not reconnect.
			return ctx.Err()
		}

		// If the WS upgrade was rejected with 401, the session row is gone.
		// Attempt ONE re-enroll. With permanent sessions a 401 is no longer the
		// expected "session expired" path, so a re-enroll that ALSO 401s means
		// the single-use enroll token is consumed/revoked — a terminal state.
		if _, is401 := wsErr.(errUnauthorized); is401 {
			c.print("reconnecting")
			fresh, err := c.enroll(ctx)
			if err != nil {
				c.print("error")
				if errEnrollUnauthorized(err) {
					// Terminal: token already consumed. Clean exit, distinct code.
					fmt.Fprintln(os.Stderr, "worker: session rejected; re-provision this worker with a fresh enroll token")
					return ErrSessionRejected{msg: fmt.Sprintf("re-enroll after 401: %v", err)}
				}
				// Re-enroll failed for a transient/network reason — surface it.
				return fmt.Errorf("re-enroll after 401: %w", err)
			}
			enroll = fresh
			// Do not reset delay — keep backoff pressure in case of transient auth issues.
		} else {
			c.print("reconnecting")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > c.backoff.Max {
			delay = c.backoff.Max
		}
	}
}

// enroll sends the one-time token to the server and obtains a session triple
// (worker_id, exp, sig) that is required for the WebSocket upgrade.
func (c *Client) enroll(ctx context.Context) (wire.EnrollResponse, error) {
	body, err := json.Marshal(wire.EnrollRequest{Token: c.cfg.EnrollToken})
	if err != nil {
		return wire.EnrollResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.ServerURL+"/worker/enroll", bytes.NewReader(body))
	if err != nil {
		return wire.EnrollResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return wire.EnrollResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return wire.EnrollResponse{}, fmt.Errorf("enroll: server returned %d", resp.StatusCode)
	}

	var enroll wire.EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&enroll); err != nil {
		return wire.EnrollResponse{}, fmt.Errorf("enroll: decode response: %w", err)
	}
	return enroll, nil
}

// runOnce opens a single WebSocket connection, sends the register frame, runs
// the read/write pumps, and returns when the connection closes.
func (c *Client) runOnce(ctx context.Context, enroll wire.EnrollResponse) error {
	wsURL := buildWSURL(c.cfg.ServerURL, enroll.WorkerID, enroll.Exp, enroll.Sig)

	dialer := gorillaws.Dialer{
		// Do NOT set HandshakeTimeout here — context controls the deadline.
		HandshakeTimeout: 30 * time.Second,
		// Explicitly zero-out the Jar so no Origin header leaks from any default.
	}

	// Server rejects connections with an Origin header (browser guard), so we
	// omit it. Gorilla does not add Origin by default when using a plain Dialer
	// (it only adds it for browser-origin requests in the default http client
	// path). We also set no request headers here.
	conn, resp, err := dialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		// Gorilla returns the HTTP response even on upgrade failure so we can
		// detect 401 and signal the caller to re-enroll.
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return errUnauthorized{msg: "ws upgrade: 401 unauthorized"}
		}
		return err
	}
	defer conn.Close()

	// Configure read-side constraints.
	conn.SetReadLimit(maxMsgSize)
	conn.SetReadDeadline(time.Now().Add(pongWait)) //nolint:errcheck
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Send the register frame immediately (seq=1).
	// ModelsAvailable reflects the current manager state.
	regFrame, err := wire.NewFrame("register", 1, wire.RegisterPayload{
		WorkerID:        enroll.WorkerID,
		GPUInfo:         "unknown",
		ImageVersion:    os.Getenv("IMAGE_VERSION"),
		ModelsAvailable: c.manager.Available(),
	})
	if err != nil {
		return fmt.Errorf("build register frame: %w", err)
	}
	raw, err := json.Marshal(regFrame)
	if err != nil {
		return fmt.Errorf("marshal register frame: %w", err)
	}
	if err := conn.WriteMessage(gorillaws.TextMessage, raw); err != nil {
		return fmt.Errorf("send register frame: %w", err)
	}

	c.print("idle")

	// Create a per-connection grant channel. dispatch() pushes lease_grant
	// frames here; RunLeaseLoop drains it via chanLeaseConn.
	grantCh := make(chan wire.LeaseGrantPayload, 8)
	c.connMu.Lock()
	c.grantCh = grantCh
	c.connMu.Unlock()

	// Start the lease loop in a goroutine; it exits when loopCtx is cancelled
	// or grantCh is closed (connection dropped).
	loopCtx, cancelLoop := context.WithCancel(ctx)
	defer cancelLoop()
	go func() {
		c.RunLeaseLoop(loopCtx, enroll.WorkerID, chanLeaseConn{ch: grantCh}) //nolint:errcheck
	}()

	// Run pumps; readPump returns when the connection closes and signals writePump
	// via the done channel. loopCtx is the connection context: it is cancelled
	// (via the deferred cancelLoop) when this connection drops, which tears down
	// any in-flight exec sessions so no orphaned process survives a reconnect.
	done := make(chan struct{})
	go c.writePump(ctx, conn, done)

	err = c.readPump(loopCtx, conn)

	// Close grantCh so chanLeaseConn.ReadGrant returns an error and the
	// lease loop goroutine exits cleanly.
	c.connMu.Lock()
	if c.grantCh != nil {
		close(c.grantCh)
		c.grantCh = nil
	}
	c.connMu.Unlock()

	close(done)
	return err
}

// readPump reads frames from the WebSocket and dispatches them.
// It returns when the connection is closed or an error occurs.
//
// connCtx is the per-connection context; it is passed to dispatch so exec
// sessions are bound to the connection's lifetime and torn down on WS drop.
func (c *Client) readPump(connCtx context.Context, conn *gorillaws.Conn) error {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var f wire.Frame
		if err := json.Unmarshal(msg, &f); err != nil {
			// Bad frame — log to stderr, continue.
			fmt.Fprintf(os.Stderr, "worker: bad frame JSON: %v\n", err)
			continue
		}

		c.dispatch(connCtx, f)
	}
}

// writePump drains the send channel, forwarding frames to the WebSocket.
// It returns when done is closed (connection dropped) or ctx is cancelled.
func (c *Client) writePump(ctx context.Context, conn *gorillaws.Conn, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			conn.WriteControl( //nolint:errcheck
				gorillaws.CloseMessage,
				gorillaws.FormatCloseMessage(gorillaws.CloseGoingAway, "shutdown"),
				time.Now().Add(5*time.Second),
			)
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
			if err := conn.WriteMessage(gorillaws.TextMessage, msg); err != nil {
				fmt.Fprintf(os.Stderr, "worker: write error: %v\n", err)
				return
			}
		}
	}
}

// dispatch routes an inbound frame to the appropriate handler.
// Unknown frame types are silently ignored (forward-compat with new server frames).
//
// connCtx is the per-connection context; exec sessions are bound to it so they
// are torn down when the connection drops.
func (c *Client) dispatch(connCtx context.Context, f wire.Frame) {
	switch f.Type {
	case "lease_grant":
		var g wire.LeaseGrantPayload
		if err := f.Decode(&g); err != nil {
			fmt.Fprintf(os.Stderr, "worker: decode lease_grant: %v\n", err)
			return
		}
		c.connMu.Lock()
		ch := c.grantCh
		c.connMu.Unlock()
		if ch != nil {
			select {
			case ch <- g:
			default:
				fmt.Fprintf(os.Stderr, "worker: grant channel full, dropping lease_grant for job %s/%d\n", g.JobID, g.Idx)
			}
		}
	case "command":
		var p wire.CommandPayload
		if err := f.Decode(&p); err != nil {
			fmt.Fprintf(os.Stderr, "worker: decode command payload: %v\n", err)
			return
		}
		if err := c.commandHandler.Handle(p.Cmd, p.Args); err != nil {
			fmt.Fprintf(os.Stderr, "worker: command %q: %v\n", p.Cmd, err)
		}
	case "exec_open":
		var p wire.ExecPayload
		if err := f.Decode(&p); err != nil {
			fmt.Fprintf(os.Stderr, "worker: decode exec_open payload: %v\n", err)
			return
		}
		// Bind the exec session to the connection context so a WS drop /
		// worker shutdown kills the process/PTY (no orphaned process).
		c.execHandler.Handle(connCtx, p)
	case "exec_close":
		// Admin-initiated teardown of a running exec session: kill the
		// process + close the PTY for the named session.
		var p wire.ExecPayload
		if err := f.Decode(&p); err != nil {
			fmt.Fprintf(os.Stderr, "worker: decode exec_close payload: %v\n", err)
			return
		}
		c.execHandler.Close(p.SessionID)
	default:
		// Forward-compat: unknown frames are ignored silently.
	}
}

// print writes a neutral console token to stdout.
// ONLY the following tokens are allowed: connected, leased, processing, idle,
// error, reconnecting, starting.
// No URLs, hostnames, worker IDs, or paths are ever written to stdout.
func (c *Client) print(token string) {
	c.stdoutMu.Lock()
	defer c.stdoutMu.Unlock()
	fmt.Fprintln(c.stdout, token)
}

// buildWSURL constructs the WebSocket upgrade URL from the base server URL
// and the session triple obtained during enrollment.
// All query values are URL-escaped so future base64 sigs (containing +/=)
// cannot corrupt the URL.
func buildWSURL(serverURL, workerID, exp, sig string) string {
	// Replace http(s):// prefix with ws(s)://.
	wsBase := serverURL
	if len(wsBase) >= 7 && wsBase[:7] == "http://" {
		wsBase = "ws://" + wsBase[7:]
	} else if len(wsBase) >= 8 && wsBase[:8] == "https://" {
		wsBase = "wss://" + wsBase[8:]
	}
	q := url.Values{}
	q.Set("worker_id", workerID)
	q.Set("exp", exp)
	q.Set("sig", sig)
	return wsBase + "/worker/ws?" + q.Encode()
}
