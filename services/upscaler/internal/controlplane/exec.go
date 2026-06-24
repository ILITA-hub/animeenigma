package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// ErrRemoteShellDisabled is returned by ExecRelay.Open when the feature is disabled.
var ErrRemoteShellDisabled = errors.New("controlplane: remote shell is disabled")

// ErrSessionNotFound is returned when the session ID is not known to the relay.
var ErrSessionNotFound = errors.New("controlplane: exec session not found")

// ExecRouter is the interface the Hub uses to dispatch incoming exec frames
// (exec_data, exec_close) received from workers.
type ExecRouter interface {
	DeliverFromWorker(f Frame)
}

// execConfig holds ExecRelay tunables.
type execConfig struct {
	Enabled     bool
	IdleTimeout time.Duration // reset on any exec_data through the relay
}

// ExecRelayConfig is the public configuration type for NewExecRelay.
type ExecRelayConfig struct {
	Enabled     bool
	IdleTimeout time.Duration // default 10 minutes when zero
}

// execSession holds per-session state.
type execSession struct {
	sessionID string
	workerID  string
	adminID   string
	pty       bool
	toAdmin   chan Frame     // relay writes worker frames here; admin reads
	cancel    context.CancelFunc
	timer     *time.Timer
	openedAt  time.Time
}

// ExecRelay manages admin-initiated exec sessions relayed over worker WS connections.
// Thread-safe; sessions are keyed by sessionID.
type ExecRelay struct {
	hub      hubSender
	cfg      execConfig
	mu       sync.Mutex
	sessions map[string]*execSession // sessionID → session
	// workerSessions indexes workerID → set of sessionIDs (for WorkerGone cleanup)
	workerSessions map[string]map[string]struct{}
	log            *logger.Logger
	audit          io.Writer // audit log destination; os.Stderr in production
}

// NewExecRelay constructs an ExecRelay.
// audit receives one-line EXEC_OPEN / EXEC_CLOSE audit records; pass os.Stderr
// in production, a *bytes.Buffer in tests.
func NewExecRelay(hub hubSender, cfg ExecRelayConfig, log *logger.Logger, audit io.Writer) *ExecRelay {
	timeout := cfg.IdleTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if log == nil {
		log = logger.Default()
	}
	if audit == nil {
		audit = io.Discard
	}
	return &ExecRelay{
		hub: hub,
		cfg: execConfig{
			Enabled:     cfg.Enabled,
			IdleTimeout: timeout,
		},
		sessions:       make(map[string]*execSession),
		workerSessions: make(map[string]map[string]struct{}),
		log:            log,
		audit:          audit,
	}
}

// Open creates a new exec session to workerID on behalf of adminID.
// Returns ErrRemoteShellDisabled when the feature is disabled.
// On success it delivers an exec_open frame to the worker and writes an audit line.
func (r *ExecRelay) Open(workerID, adminID string, pty bool) (string, error) {
	if !r.cfg.Enabled {
		return "", ErrRemoteShellDisabled
	}

	// Generate a cryptographically random session ID.
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("controlplane: generate session id: %w", err)
	}
	sessionID := "exec-" + workerID + "-" + hex.EncodeToString(buf)

	// Build the exec_open frame.
	payload := ExecPayload{
		SessionID: sessionID,
		Pty:       pty,
	}
	f, err := NewFrame("exec_open", 0, payload)
	if err != nil {
		return "", fmt.Errorf("controlplane: marshal exec_open frame: %w", err)
	}

	// Create the session before sending the frame so DeliverFromWorker can
	// route a very fast response back without racing.
	ctx, cancel := context.WithCancel(context.Background())

	sess := &execSession{
		sessionID: sessionID,
		workerID:  workerID,
		adminID:   adminID,
		pty:       pty,
		toAdmin:   make(chan Frame, 64),
		cancel:    cancel,
		openedAt:  time.Now(),
	}

	// Idle timer: restarted on every exec_data delivery.
	sess.timer = time.AfterFunc(r.cfg.IdleTimeout, func() {
		r.log.Infow("exec: idle timeout, closing session", "session_id", sessionID, "worker_id", workerID)
		r.CloseSession(sessionID, nil)
	})

	r.mu.Lock()
	r.sessions[sessionID] = sess
	if r.workerSessions[workerID] == nil {
		r.workerSessions[workerID] = make(map[string]struct{})
	}
	r.workerSessions[workerID][sessionID] = struct{}{}
	r.mu.Unlock()

	// Send exec_open to the worker. If it fails, clean up and return the error.
	if err := r.hub.Send(workerID, f); err != nil {
		r.mu.Lock()
		delete(r.sessions, sessionID)
		if ws := r.workerSessions[workerID]; ws != nil {
			delete(ws, sessionID)
		}
		r.mu.Unlock()
		sess.timer.Stop()
		cancel()
		if errors.Is(err, errWorkerNotFound) {
			return "", ErrWorkerNotConnected
		}
		return "", fmt.Errorf("controlplane: send exec_open: %w", err)
	}

	// Audit: EXEC_OPEN.
	r.writeAudit(fmt.Sprintf("EXEC_OPEN  ts=%s session=%s worker=%s admin=%s pty=%v",
		time.Now().UTC().Format(time.RFC3339), sessionID, workerID, adminID, pty))

	_ = ctx // held by timer goroutine via cancel
	return sessionID, nil
}

// DeliverFromWorker routes an exec_data or exec_close frame received from a
// worker to the appropriate admin channel.
// This is called from Hub.dispatch for exec_data and exec_close frame types.
func (r *ExecRelay) DeliverFromWorker(f Frame) {
	var payload ExecPayload
	if err := f.Decode(&payload); err != nil {
		r.log.Warnw("exec: bad exec frame payload from worker", "type", f.Type, "error", err)
		return
	}
	sessionID := payload.SessionID

	r.mu.Lock()
	sess, ok := r.sessions[sessionID]
	r.mu.Unlock()

	if !ok {
		// Session already closed or unknown — silently ignore.
		return
	}

	// Reset idle timer on exec_data frames.
	if f.Type == "exec_data" {
		sess.timer.Reset(r.cfg.IdleTimeout)
	}

	// Forward the frame to the admin channel (non-blocking drop on full buffer).
	select {
	case sess.toAdmin <- f:
	default:
		r.log.Warnw("exec: admin channel full, dropping frame", "session_id", sessionID, "type", f.Type)
	}

	// If the worker sent exec_close, terminate the session from the worker side.
	if f.Type == "exec_close" {
		r.closeSessionInternal(sessionID, payload.ExitCode, false /* already delivered exec_close */)
	}
}

// SendToWorker relays admin stdin data (admin→worker exec_data frame).
func (r *ExecRelay) SendToWorker(sessionID string, data []byte) error {
	r.mu.Lock()
	sess, ok := r.sessions[sessionID]
	r.mu.Unlock()

	if !ok {
		return ErrSessionNotFound
	}

	// Reset idle timer on admin activity.
	sess.timer.Reset(r.cfg.IdleTimeout)

	payload := ExecPayload{
		SessionID: sessionID,
		Data:      data,
	}
	f, err := NewFrame("exec_data", 0, payload)
	if err != nil {
		return fmt.Errorf("controlplane: marshal exec_data: %w", err)
	}
	if err := r.hub.Send(sess.workerID, f); err != nil {
		if errors.Is(err, errWorkerNotFound) {
			return ErrWorkerNotConnected
		}
		return fmt.Errorf("controlplane: send exec_data: %w", err)
	}
	return nil
}

// CloseSession terminates a session from the admin side.
// Sends exec_close to the worker and removes the session.
func (r *ExecRelay) CloseSession(sessionID string, exitCode *int) {
	r.closeSessionInternal(sessionID, exitCode, true /* send exec_close to worker */)
}

// closeSessionInternal is the internal teardown path.
// sendToWorker controls whether an exec_close frame is sent to the worker
// (true when closing from admin/timeout side; false when the worker already
// sent exec_close and we're just cleaning up state).
func (r *ExecRelay) closeSessionInternal(sessionID string, exitCode *int, sendToWorker bool) {
	r.mu.Lock()
	sess, ok := r.sessions[sessionID]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.sessions, sessionID)
	if ws := r.workerSessions[sess.workerID]; ws != nil {
		delete(ws, sessionID)
	}
	r.mu.Unlock()

	// Stop the idle timer.
	sess.timer.Stop()
	// Cancel the session context.
	sess.cancel()

	if sendToWorker {
		// Best-effort: tell the worker to close the pty/shell.
		payload := ExecPayload{
			SessionID: sessionID,
			ExitCode:  exitCode,
		}
		if f, err := NewFrame("exec_close", 0, payload); err == nil {
			_ = r.hub.Send(sess.workerID, f) //nolint:errcheck // best-effort
		}
	}

	// Close the admin channel so Subscribe readers exit.
	close(sess.toAdmin)

	// Audit: EXEC_CLOSE.
	exitStr := "nil"
	if exitCode != nil {
		exitStr = fmt.Sprintf("%d", *exitCode)
	}
	r.writeAudit(fmt.Sprintf("EXEC_CLOSE ts=%s session=%s worker=%s admin=%s exit=%s",
		time.Now().UTC().Format(time.RFC3339), sessionID, sess.workerID, sess.adminID, exitStr))
}

// WorkerGone is called when a worker connection drops. It terminates all exec
// sessions for that worker and delivers exec_close to their admin channels.
func (r *ExecRelay) WorkerGone(workerID string) {
	r.mu.Lock()
	sessionIDs := make([]string, 0, len(r.workerSessions[workerID]))
	for sid := range r.workerSessions[workerID] {
		sessionIDs = append(sessionIDs, sid)
	}
	r.mu.Unlock()

	for _, sid := range sessionIDs {
		// Deliver exec_close to the admin channel before teardown.
		r.mu.Lock()
		sess, ok := r.sessions[sid]
		r.mu.Unlock()

		if ok {
			// Push an exec_close frame to the admin channel so the WS handler
			// can relay it to the admin before the channel closes.
			if f, err := NewFrame("exec_close", 0, ExecPayload{SessionID: sid}); err == nil {
				select {
				case sess.toAdmin <- f:
				default:
				}
			}
		}

		// Tear down the session (don't send exec_close back to worker — it's gone).
		r.closeSessionInternal(sid, nil, false)
	}
}

// Subscribe returns the admin receive channel for a session.
// Returns nil when the session is not found.
func (r *ExecRelay) Subscribe(sessionID string) <-chan Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	sess, ok := r.sessions[sessionID]
	if !ok {
		return nil
	}
	return sess.toAdmin
}

// writeAudit writes a single-line audit record.
func (r *ExecRelay) writeAudit(line string) {
	fmt.Fprintln(r.audit, line) //nolint:errcheck
}
