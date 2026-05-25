// ws_proxy_test.go — workstream watch-together v1.0 Phase 1 Plan 01.7.2.
//
// End-to-end coverage for the dedicated WebSocket reverse proxy at
// /api/watch-together/ws. The standard ProxyService.Forward path strips
// RFC 7230 §6.1 hop-by-hop headers (Upgrade, Connection, etc.) which is
// the CORRECT behaviour for normal HTTP but breaks the WS handshake.
// newWSProxy is the dedicated code path that preserves those headers.
//
// Fixture pattern: spin up a real httptest.NewServer hosting a
// gorilla/websocket upgrader (the "backend"), wrap newWSProxy around it in
// another httptest.NewServer (the "gateway"), then dial the gateway with
// gorilla/websocket and assert full upgrade + bidirectional frame flow.
package transport

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// echoWSBackend builds an httptest.Server that accepts WS upgrades and echoes
// every text frame it receives until the client closes the connection.
// queryCh receives the query string of the upgrade request so tests can
// assert ?token=... and ?room=... reached the backend verbatim.
type wsBackendOpts struct {
	queryCh        chan string
	subprotocolCh  chan string // first selected subprotocol observed by backend
	echo           bool        // when true, echo back text frames
	rejectUpgrade  bool        // when true, return 403 BEFORE upgrade
}

func newWSEchoBackend(t *testing.T, opts wsBackendOpts) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
		Subprotocols: []string{"watch-together-v1", "echo-test-v1"},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.queryCh != nil {
			select {
			case opts.queryCh <- r.URL.RawQuery:
			default:
			}
		}
		if opts.rejectUpgrade {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("backend upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		if opts.subprotocolCh != nil {
			select {
			case opts.subprotocolCh <- conn.Subprotocol():
			default:
			}
		}
		if !opts.echo {
			// Hold the connection open until the client closes.
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
}

// dialWSThroughProxy converts http://... → ws://... and dials.
func dialWSThroughProxy(t *testing.T, proxyHTTP string, path string, query string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u, err := url.Parse(proxyHTTP)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	u.Scheme = "ws"
	u.Path = path
	u.RawQuery = query
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second
	return dialer.Dial(u.String(), header)
}

// TestWSProxy_UpgradeRoundTrip — Behaviour 1+2+3: the gateway WS proxy
// forwards the upgrade handshake, the client receives a 101 response, and
// frames flow in both directions through the proxy.
func TestWSProxy_UpgradeRoundTrip(t *testing.T) {
	t.Parallel()
	backend := newWSEchoBackend(t, wsBackendOpts{echo: true})
	defer backend.Close()

	wsHandler, err := newWSProxy(backend.URL, logger.Default())
	if err != nil {
		t.Fatalf("newWSProxy: %v", err)
	}
	proxy := httptest.NewServer(wsHandler)
	defer proxy.Close()

	conn, resp, err := dialWSThroughProxy(t, proxy.URL, "/api/watch-together/ws", "", nil)
	if err != nil {
		t.Fatalf("dial: %v (resp=%+v)", err, resp)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d; want 101", resp.StatusCode)
	}

	// Client → backend frame, then backend → client echo.
	want := "hello-watch-together"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(want)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Errorf("echo = %q; want %q", string(got), want)
	}
}

// TestWSProxy_PreservesQueryString — Behaviour 4: ?token=...&room=... reaches
// the backend verbatim. Watch-together's WS handler authenticates from
// ?token= because browsers can't set Authorization on WS upgrades.
func TestWSProxy_PreservesQueryString(t *testing.T) {
	t.Parallel()
	queryCh := make(chan string, 1)
	backend := newWSEchoBackend(t, wsBackendOpts{queryCh: queryCh, echo: true})
	defer backend.Close()

	wsHandler, err := newWSProxy(backend.URL, logger.Default())
	if err != nil {
		t.Fatalf("newWSProxy: %v", err)
	}
	proxy := httptest.NewServer(wsHandler)
	defer proxy.Close()

	conn, _, err := dialWSThroughProxy(t, proxy.URL, "/api/watch-together/ws",
		"token=abc.def.ghi&room=room-xyz", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	select {
	case got := <-queryCh:
		if !strings.Contains(got, "token=abc.def.ghi") || !strings.Contains(got, "room=room-xyz") {
			t.Errorf("backend query = %q; want token=abc.def.ghi & room=room-xyz", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for backend to receive upgrade")
	}
}

// TestWSProxy_PreservesSubprotocol — Behaviour 5: the
// Sec-WebSocket-Protocol header negotiation passes through the proxy. The
// backend selects from its supported list; the client should observe the
// negotiated subprotocol on conn.Subprotocol().
func TestWSProxy_PreservesSubprotocol(t *testing.T) {
	t.Parallel()
	subCh := make(chan string, 1)
	backend := newWSEchoBackend(t, wsBackendOpts{subprotocolCh: subCh, echo: true})
	defer backend.Close()

	wsHandler, err := newWSProxy(backend.URL, logger.Default())
	if err != nil {
		t.Fatalf("newWSProxy: %v", err)
	}
	proxy := httptest.NewServer(wsHandler)
	defer proxy.Close()

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second
	dialer.Subprotocols = []string{"watch-together-v1"}

	u, _ := url.Parse(proxy.URL)
	u.Scheme = "ws"
	u.Path = "/api/watch-together/ws"
	conn, resp, err := dialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d; want 101", resp.StatusCode)
	}
	if got := conn.Subprotocol(); got != "watch-together-v1" {
		t.Errorf("negotiated subprotocol = %q; want watch-together-v1", got)
	}
	select {
	case got := <-subCh:
		if got != "watch-together-v1" {
			t.Errorf("backend saw subprotocol = %q; want watch-together-v1", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for backend subprotocol observation")
	}
}

// TestWSProxy_BackendDown_Returns502 — Behaviour 6: when the upstream
// watch-together service is unreachable, the gateway returns 502 Bad
// Gateway instead of panicking. Points at 127.0.0.1:1 (port 1 reserved,
// guaranteed-closed) to provoke a connection-refused.
func TestWSProxy_BackendDown_Returns502(t *testing.T) {
	t.Parallel()
	wsHandler, err := newWSProxy("http://127.0.0.1:1", logger.Default())
	if err != nil {
		t.Fatalf("newWSProxy: %v", err)
	}
	proxy := httptest.NewServer(wsHandler)
	defer proxy.Close()

	// Plain HTTP probe — easier to assert the 502 than going through the
	// gorilla dialer (which surfaces dial failures as separate error types
	// depending on whether the response was even written).
	resp, err := http.Get(proxy.URL + "/api/watch-together/ws")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d; want 502", resp.StatusCode)
	}
}

// TestWSProxy_PathForwardedVerbatim — defensive: the watch-together service
// mounts at /api/watch-together/ws natively (no path rewrite). The proxy
// must forward whatever path it receives to the backend untouched so we
// don't have to maintain a path-rewrite table.
func TestWSProxy_PathForwardedVerbatim(t *testing.T) {
	t.Parallel()
	queryCh := make(chan string, 1)
	pathCh := make(chan string, 1)

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case pathCh <- r.URL.Path:
		default:
		}
		select {
		case queryCh <- r.URL.RawQuery:
		default:
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer backend.Close()

	wsHandler, err := newWSProxy(backend.URL, logger.Default())
	if err != nil {
		t.Fatalf("newWSProxy: %v", err)
	}
	proxy := httptest.NewServer(wsHandler)
	defer proxy.Close()

	conn, _, err := dialWSThroughProxy(t, proxy.URL, "/api/watch-together/ws", "", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()

	select {
	case got := <-pathCh:
		if got != "/api/watch-together/ws" {
			t.Errorf("backend path = %q; want /api/watch-together/ws", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for backend to receive request")
	}
}
