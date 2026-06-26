package controlplane

import (
	"net/http"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

// newWSUpgrader returns a gorilla Upgrader configured to reject browser origins.
// Worker clients are server-side processes — they have no Origin header. Any
// connection carrying an Origin header is a browser (or a proxy spoofing one)
// and is rejected with 403.
func newWSUpgrader() gorillaws.Upgrader {
	return gorillaws.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			// Accept only connections with no Origin header — server clients
			// don't set it; browsers always do.
			return r.Header.Get("Origin") == ""
		},
	}
}

// UpgradeHandler returns an http.HandlerFunc for GET /worker/ws.
//
// Authentication: the worker must supply valid session query params:
//
//	?worker_id=<uuid>&exp=<unix-seconds>&sig=<hmac-hex>
//
// These are minted by GormEnrollStore.EnrollTx during the enroll flow and
// verified here with VerifySession (HMAC-SHA256 over workerID+exp via the
// shared job-capability secret).
//
// Security:
//   - Browser origins are rejected (CheckOrigin → Origin header must be absent).
//   - Missing or invalid session params → 401.
//   - Expired session → 401.
//
// The wired path in main.go is:
//
//	GormEnrollStore.EnrollTx → issues (workerID, exp, sig)
//	Worker dials /worker/ws?worker_id=…&exp=…&sig=…
//	UpgradeHandler verifies VerifySession → upgrades → hub.Register
func UpgradeHandler(hub *Hub) http.HandlerFunc {
	upgrader := newWSUpgrader()

	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Reject browser origins before doing anything else.
		if r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// 2. Extract and verify session params.
		workerID := r.URL.Query().Get("worker_id")
		exp := r.URL.Query().Get("exp")
		sig := r.URL.Query().Get("sig")

		if workerID == "" || exp == "" || sig == "" {
			http.Error(w, "missing session params", http.StatusUnauthorized)
			return
		}
		if !VerifySession(workerID, exp, sig, time.Now()) {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}

		// 3. Upgrade to WebSocket.
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// upgrader.Upgrade writes the HTTP error itself.
			return
		}

		// 4. Register the connection with the hub (starts read/write pumps).
		conn := newConn(workerID, ws, hub)
		hub.Register(conn)
		// Register is non-blocking; the pumps run in goroutines. The HTTP
		// handler can return — the net/http server keeps the underlying TCP
		// connection open because Upgrade hijacks it.
	}
}
