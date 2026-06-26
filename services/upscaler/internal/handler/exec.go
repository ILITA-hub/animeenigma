package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/go-chi/chi/v5"
	gorillaws "github.com/gorilla/websocket"
)

// execRelayer is the interface ExecShellHandler uses to drive exec sessions.
// It is satisfied by *controlplane.ExecRelay.
type execRelayer interface {
	Open(workerID, adminID string, pty bool) (string, error)
	SendToWorker(sessionID string, data []byte) error
	CloseSession(sessionID string, exitCode *int)
	Subscribe(sessionID string) <-chan controlplane.Frame
}

// ExecShellHandler bridges the admin WebSocket to the exec relay.
// It is mounted as GET /api/upscale/workers/{id}/shell in the admin group.
type ExecShellHandler struct {
	relay    execRelayer
	log      *logger.Logger
	upgrader gorillaws.Upgrader
}

// NewExecShellHandler constructs an ExecShellHandler backed by relay.
func NewExecShellHandler(relay execRelayer, log *logger.Logger) *ExecShellHandler {
	if log == nil {
		log = logger.Default()
	}
	// The admin WS is reached via the gateway's admin-proxied path, not from a
	// browser directly, so we permit connections carrying an Origin header
	// (the gateway may forward one).  We do NOT allow arbitrary browser origins
	// from public IPs — that is enforced at the gateway (JWT + AdminRole) before
	// the request reaches us.
	upgrader := gorillaws.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			// Accept all origins here: the requireGatewayInternal middleware
			// already verified X-Gateway-Internal before this handler runs, so
			// we're on the admin-only surface (Docker-network-only path).
			return true
		},
	}
	return &ExecShellHandler{
		relay:    relay,
		log:      log,
		upgrader: upgrader,
	}
}

// ServeHTTP handles GET /api/upscale/workers/{id}/shell.
//
// Query params:
//   - ?pty=true  — request full PTY allocation on the worker (default: false)
//
// The admin is identified from X-Admin-ID header (injected by the gateway's
// JWT verification) or falls back to "admin" (gateway already proved AdminRole).
//
// Frame protocol over the admin WebSocket:
//   - Admin → handler: JSON {"type":"exec_data","payload":{"session_id":"...","data":<base64>}}
//   - Handler → admin: same Frame struct
//
// Returns 403 when the relay returns ErrRemoteShellDisabled.
// Returns 404 when the upgrader fails (worker not connected).
func (h *ExecShellHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")
	if strings.TrimSpace(workerID) == "" {
		http.Error(w, "worker id required", http.StatusBadRequest)
		return
	}

	// PTY mode from query param.
	pty := r.URL.Query().Get("pty") == "true"

	// Admin identity from X-Admin-ID (gateway-injected from JWT claims) or fallback.
	adminID := r.Header.Get("X-Admin-ID")
	if adminID == "" {
		adminID = "admin"
	}

	// Open the exec session before upgrading so we can return a plain HTTP
	// error when the relay rejects it (e.g. disabled or worker not connected).
	sessionID, err := h.relay.Open(workerID, adminID, pty)
	if err != nil {
		if errors.Is(err, controlplane.ErrRemoteShellDisabled) {
			http.Error(w, "remote shell disabled", http.StatusForbidden)
			return
		}
		if errors.Is(err, controlplane.ErrWorkerNotConnected) {
			http.Error(w, "worker not connected", http.StatusServiceUnavailable)
			return
		}
		h.log.Errorw("exec: open session failed", "worker_id", workerID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Upgrade to WebSocket.
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrader writes the HTTP error; clean up the session.
		h.relay.CloseSession(sessionID, nil)
		return
	}
	defer ws.Close()

	// Subscribe to frames from the worker.
	fromWorker := h.relay.Subscribe(sessionID)
	if fromWorker == nil {
		// Session was already torn down (race with worker disconnect).
		ws.WriteMessage(gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "session gone"))
		return
	}

	// fromWorker → admin WS pump (goroutine).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for f := range fromWorker {
			raw, err := json.Marshal(f)
			if err != nil {
				continue
			}
			if err := ws.WriteMessage(gorillaws.TextMessage, raw); err != nil {
				return
			}
		}
		// Channel closed (session ended) — send close frame to admin.
		ws.WriteMessage(gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "session closed"))
	}()

	// Admin WS → worker pump (inline, on the HTTP goroutine).
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			// Admin disconnected or WS error — close the session.
			break
		}
		var f controlplane.Frame
		if err := json.Unmarshal(msg, &f); err != nil {
			h.log.Warnw("exec: bad frame from admin", "worker_id", workerID, "error", err)
			continue
		}
		switch f.Type {
		case "exec_data":
			var payload controlplane.ExecPayload
			if err := f.Decode(&payload); err != nil {
				h.log.Warnw("exec: bad exec_data payload from admin", "worker_id", workerID, "error", err)
				continue
			}
			if err := h.relay.SendToWorker(sessionID, payload.Data); err != nil {
				h.log.Warnw("exec: SendToWorker failed", "worker_id", workerID, "session_id", sessionID, "error", err)
			}
		case "exec_close":
			// Admin requested close — fall through to the single cleanup close
			// below (do NOT call CloseSession here too, or the worker gets a
			// duplicate exec_close and the relay logs a spurious second close).
			goto cleanup
		default:
			h.log.Warnw("exec: unexpected frame type from admin", "type", f.Type)
		}
	}

cleanup:
	h.relay.CloseSession(sessionID, nil)
	<-done
}
