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
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"

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

	// processorFn, when non-nil, overrides the default NewPipelineProcessor
	// factory. Tests set this to inject CopyProcessor (or another stub) so
	// the end-to-end WS wiring test does not require a real ffmpeg binary.
	processorFn func(cfg Config) (Processor, error)

	// commandHandler handles server-sent command frames (cancel, drain, shutdown, etc.).
	commandHandler *CommandHandler

	// execHandler handles server-sent exec_open frames.
	execHandler *ExecHandler
}

// NewClient constructs a Client with default backoff settings.
func NewClient(cfg Config) *Client {
	send := make(chan []byte, sendBuf)
	_, noopCancel := context.WithCancel(context.Background())
	return &Client{
		cfg:            cfg,
		backoff:        defaultBackoff,
		send:           send,
		stdout:         os.Stdout,
		commandHandler: NewCommandHandler(noopCancel),
		execHandler:    NewExecHandler(send),
	}
}

// errUnauthorized is returned by runOnce when the WS upgrade is rejected with
// 401, signalling that the session has expired and re-enrollment is needed.
type errUnauthorized struct{ msg string }

func (e errUnauthorized) Error() string { return e.msg }

// Run enrolls the worker with the server and then maintains a persistent
// WebSocket connection, reconnecting with exponential backoff on disconnect.
// On 401 (session expired after SessionTTL), it re-enrolls automatically.
// It returns when ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	c.print("starting")

	enroll, err := c.enroll(ctx)
	if err != nil {
		c.print("error")
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

		// If the WS upgrade was rejected with 401, the session has expired.
		// Re-enroll to get a fresh session before the next reconnect attempt.
		if _, is401 := wsErr.(errUnauthorized); is401 {
			c.print("reconnecting")
			fresh, err := c.enroll(ctx)
			if err != nil {
				// If re-enroll also fails (e.g. token revoked), surface the error.
				c.print("error")
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
			return errUnauthorized{msg: fmt.Sprintf("ws upgrade: 401 unauthorized")}
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
	regFrame, err := wire.NewFrame("register", 1, wire.RegisterPayload{
		WorkerID:        enroll.WorkerID,
		GPUInfo:         "unknown",
		ImageVersion:    os.Getenv("IMAGE_VERSION"),
		ModelsAvailable: []string{},
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

	// Build the Processor for this connection. Tests may inject processorFn;
	// production uses NewPipelineProcessor with the configured model.
	var proc Processor
	if c.processorFn != nil {
		var err error
		proc, err = c.processorFn(c.cfg)
		if err != nil {
			// Fallback to CopyProcessor so the WS machinery stays alive.
			fmt.Fprintf(os.Stderr, "worker: processorFn error, falling back to CopyProcessor: %v\n", err)
			proc = CopyProcessor{}
		}
	} else if c.cfg.Model != "" {
		workDir := c.cfg.WorkDir
		if workDir == "" {
			workDir = os.TempDir()
		}
		pp, ppErr := NewPipelineProcessor(c.cfg.Model, c.cfg.Scale, workDir)
		if ppErr != nil {
			// Model not found (e.g. unregistered name).
			// Fall back to CopyProcessor so connectivity remains functional.
			fmt.Fprintf(os.Stderr, "worker: NewPipelineProcessor(%q): %v; falling back to CopyProcessor\n", c.cfg.Model, ppErr)
			proc = CopyProcessor{}
		} else {
			proc = pp
		}
	} else {
		// No model configured — use the no-op CopyProcessor (Task-15 stub).
		proc = CopyProcessor{}
	}

	// Start the lease loop in a goroutine; it exits when loopCtx is cancelled
	// or grantCh is closed (connection dropped).
	loopCtx, cancelLoop := context.WithCancel(ctx)
	defer cancelLoop()
	go func() {
		c.RunLeaseLoop(loopCtx, enroll.WorkerID, proc, chanLeaseConn{ch: grantCh}) //nolint:errcheck
	}()

	// Run pumps; readPump returns when the connection closes and signals writePump
	// via the done channel.
	done := make(chan struct{})
	go c.writePump(ctx, conn, done)

	err = c.readPump(conn)

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
func (c *Client) readPump(conn *gorillaws.Conn) error {
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

		c.dispatch(f)
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
func (c *Client) dispatch(f wire.Frame) {
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
		c.execHandler.Handle(context.Background(), p)
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
