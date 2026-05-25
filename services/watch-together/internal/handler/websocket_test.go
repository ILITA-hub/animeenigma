package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/hub"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/service"
)

// nopCatalog is a CatalogValidator stub used by the WS handler tests. The
// handler tests never exercise state:change_* envelopes (those are covered
// directly in internal/service/inbound_test.go), so the stub just always
// returns Valid=true. The InboundRouter still needs a non-nil catalog to
// satisfy its constructor contract.
type nopCatalog struct{}

func (nopCatalog) ValidateEpisode(
	_ context.Context,
	_, _, _, _, _ string,
) (service.ValidateResult, error) {
	return service.ValidateResult{Valid: true}, nil
}

// fakeGrace satisfies the GraceCanceller interface and records every
// Cancel/Start invocation so tests can assert exact call sites for the
// Plan 05.1 wiring. cancelReturn lets a test pin the Cancel return value
// (true = recovered, false = no pending timer).
type fakeGrace struct {
	mu           sync.Mutex
	cancelCalls  []string
	startCalls   []string
	cancelReturn bool
	period       time.Duration
}

func newFakeGrace() *fakeGrace {
	return &fakeGrace{period: 5 * time.Minute}
}

func (g *fakeGrace) Cancel(roomID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cancelCalls = append(g.cancelCalls, roomID)
	return g.cancelReturn
}

func (g *fakeGrace) Start(roomID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.startCalls = append(g.startCalls, roomID)
}

func (g *fakeGrace) Period() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.period
}

func (g *fakeGrace) snapshotStart() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, len(g.startCalls))
	copy(out, g.startCalls)
	return out
}

func (g *fakeGrace) snapshotCancel() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, len(g.cancelCalls))
	copy(out, g.cancelCalls)
	return out
}

// wsFixture bundles the moving parts for an end-to-end WS handler test:
// a chi router with /ws mounted, a real httptest.Server, a miniredis-backed
// repo, and helpers to mint JWTs against the fixture's secret.
type wsFixture struct {
	t          *testing.T
	server     *httptest.Server
	cfg        *config.Config
	hub        *hub.Hub
	repo       *repo.RoomRepo
	roomSvc    *service.RoomService
	jwtManager *authz.JWTManager
	grace      *fakeGrace
	mr         *miniredis.Miniredis
}

// newWSFixture spins up a fresh fixture per test. JWT secret is fixed
// per-fixture so test-minted tokens match the handler's validator.
//
// `maxMembers` overrides the default 10-member cap. Pass 0 to keep the
// default; the capacity-full test uses 2 so it doesn't have to open
// 11 connections.
func newWSFixture(t *testing.T, maxMembers int) *wsFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	if maxMembers == 0 {
		maxMembers = 10
	}
	cfg := &config.Config{
		PublicBaseURL: "http://localhost:0", // Origin check satisfied by no-Origin requests from the test client.
		MaxMembers:    maxMembers,
		JWT: authz.JWTConfig{
			Secret:          "test-secret-do-not-use-in-prod",
			Issuer:          "animeenigma-test",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
		},
		AllowAllOrigins: true, // simpler than minting matching-Origin headers per-test
	}

	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)
	roomSvc := service.NewRoomService(r, log)
	// Deterministic room IDs so tests can assert exact values.
	roomSvc.SetIDProviderForTest(func() string { return "room-fixed-id" })
	roomSvc.SetClockForTest(func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) })

	wsHub := hub.NewHub(r, log, "test-instance-zzz")
	t.Cleanup(wsHub.Close)

	// 01.6 wiring: the WS handler now requires an InboundRouter so
	// inbound envelopes can be dispatched. Real router with real drift
	// engine + rate limiter — tests in 01.5 only exercise the upgrade /
	// snapshot / member-lifecycle path, so the router is exercised
	// indirectly when inbound messages flow through.
	drift := service.NewDriftEngine(log)
	rl := service.NewRateLimiter()
	inboundRouter := service.NewInboundRouter(r, wsHub, drift, rl, nopCatalog{}, log)
	// Plan 05.1 — fakeGrace records every Cancel/Start so tests can assert
	// the upgrade-cancels-grace + last-conn-starts-grace contracts.
	grace := newFakeGrace()
	wsHandler := NewWebSocketHandler(wsHub, r, roomSvc, inboundRouter, grace, cfg, log)

	httpRouter := chi.NewRouter()
	httpRouter.Get("/api/watch-together/ws", wsHandler.Upgrade)

	server := httptest.NewServer(httpRouter)
	t.Cleanup(server.Close)

	return &wsFixture{
		t:          t,
		server:     server,
		cfg:        cfg,
		hub:        wsHub,
		repo:       r,
		roomSvc:    roomSvc,
		jwtManager: authz.NewJWTManager(cfg.JWT),
		grace:      grace,
		mr:         mr,
	}
}

// mintToken signs a fresh access token for the given user. Mirrors
// authz.JWTManager.GenerateTokenPair but only the access-side claims
// (refresh token isn't relevant for WS upgrade).
func (f *wsFixture) mintToken(userID, username string) string {
	f.t.Helper()
	now := time.Now()
	claims := authz.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    f.cfg.JWT.Issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(f.cfg.JWT.AccessTokenTTL)),
		},
		UserID:   userID,
		Username: username,
		Role:     authz.RoleUser,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(f.cfg.JWT.Secret))
	if err != nil {
		f.t.Fatalf("sign test token: %v", err)
	}
	return signed
}

// createRoom seeds a fresh room in Redis so the WS handler's pre-upgrade
// Exists check passes. Returns the room ID.
func (f *wsFixture) createRoom(hostUserID, hostUsername string) string {
	f.t.Helper()
	room, err := f.roomSvc.Create(context.Background(), hostUserID, hostUsername, service.CreateRoomInput{
		AnimeID:       "anime-uuid-1",
		EpisodeID:     "ep-1",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "translation-1",
	})
	if err != nil {
		f.t.Fatalf("seed Create: %v", err)
	}
	return room.ID
}

// wsURL turns the httptest.Server URL into a ws:// URL with the right
// query string. roomID == "" omits the param so the missing-room test
// can exercise the 400 branch.
func (f *wsFixture) wsURL(token, roomID string) string {
	u, _ := url.Parse(f.server.URL)
	u.Scheme = "ws"
	u.Path = "/api/watch-together/ws"
	q := u.Query()
	if token != "" {
		q.Set("token", token)
	}
	if roomID != "" {
		q.Set("room", roomID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// dial opens a WS connection using the gorilla client. Returns the
// connection, the HTTP response (so callers can assert the status on
// upgrade failure), and any error.
func (f *wsFixture) dial(token, roomID string) (*websocket.Conn, *http.Response, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	return dialer.Dial(f.wsURL(token, roomID), nil)
}

// readEnvelopeWithin reads a single envelope from the connection, failing
// the test if no frame arrives within timeout. Returns the decoded envelope.
func readEnvelopeWithin(t *testing.T, c *websocket.Conn, timeout time.Duration) domain.Envelope {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	mt, payload, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("unexpected message type %d, want TextMessage", mt)
	}
	var env domain.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("decode envelope: %v; raw=%s", err, string(payload))
	}
	return env
}

// ----------------------------------------------------------------------------
// Test 1: GET /ws without ?token=... → HTTP 401, no upgrade.
// ----------------------------------------------------------------------------

func TestWS_MissingToken_Returns401(t *testing.T) {
	fx := newWSFixture(t, 0)
	_, resp, err := fx.dial("", "any-room")
	if err == nil {
		t.Fatal("expected dial to fail with 401, but it succeeded")
	}
	if resp == nil {
		t.Fatalf("response is nil; err=%v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// ----------------------------------------------------------------------------
// Test 2: GET /ws with bad-signature token → HTTP 401, no upgrade.
// ----------------------------------------------------------------------------

func TestWS_InvalidToken_Returns401(t *testing.T) {
	fx := newWSFixture(t, 0)
	// Sign against a DIFFERENT secret so the handler's validator rejects it.
	now := time.Now()
	claims := authz.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    fx.cfg.JWT.Issuer,
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		UserID:   "alice",
		Username: "Alice",
		Role:     authz.RoleUser,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte("wrong-secret"))

	_, resp, err := fx.dial(signed, "any-room")
	if err == nil {
		t.Fatal("expected dial to fail with 401, but it succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", responseStatus(resp))
	}
}

// ----------------------------------------------------------------------------
// Test 3: valid token but missing ?room=... → HTTP 400.
// ----------------------------------------------------------------------------

func TestWS_MissingRoom_Returns400(t *testing.T) {
	fx := newWSFixture(t, 0)
	tok := fx.mintToken("alice", "Alice")
	_, resp, err := fx.dial(tok, "")
	if err == nil {
		t.Fatal("expected dial to fail with 400, but it succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", responseStatus(resp))
	}
}

// ----------------------------------------------------------------------------
// Test 4: valid token + non-existent room → HTTP 404.
// ----------------------------------------------------------------------------

func TestWS_NonExistentRoom_Returns404(t *testing.T) {
	fx := newWSFixture(t, 0)
	tok := fx.mintToken("alice", "Alice")
	_, resp, err := fx.dial(tok, "no-such-room")
	if err == nil {
		t.Fatal("expected dial to fail with 404, but it succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", responseStatus(resp))
	}
}

// ----------------------------------------------------------------------------
// Test 5: valid token + existing room → 101 Switching Protocols + room:snapshot.
// ----------------------------------------------------------------------------

func TestWS_Success_FirstFrameIsRoomSnapshot(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")
	tok := fx.mintToken("alice", "Alice")

	conn, resp, err := fx.dial(tok, roomID)
	if err != nil {
		t.Fatalf("dial: %v (status=%d)", err, responseStatus(resp))
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101 Switching Protocols", resp.StatusCode)
	}

	env := readEnvelopeWithin(t, conn, 2*time.Second)
	if env.Type != domain.MsgRoomSnapshot {
		t.Fatalf("first frame type = %q, want %q", env.Type, domain.MsgRoomSnapshot)
	}

	var snap domain.RoomSnapshot
	if err := json.Unmarshal(env.Data, &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snap.Room.ID != roomID {
		t.Errorf("snap.Room.ID = %q, want %q", snap.Room.ID, roomID)
	}
	if snap.ProtocolVersion != domain.ProtocolVersion {
		t.Errorf("snap.ProtocolVersion = %q, want %q", snap.ProtocolVersion, domain.ProtocolVersion)
	}

	// Verify the joining user appears in the member list (AddMember
	// runs before Register, so the snapshot built by roomSvc.Get
	// already includes them).
	if len(snap.Members) != 1 || snap.Members[0].UserID != "alice" {
		t.Errorf("snap.Members = %+v, want exactly [alice]", snap.Members)
	}
}

// ----------------------------------------------------------------------------
// Test 6: two clients in same room — second sees first in snapshot;
// first receives member:joined for the second.
// ----------------------------------------------------------------------------

func TestWS_TwoClients_FirstSeesMemberJoined(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")

	// Client A connects first.
	connA, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	defer connA.Close()

	// Drain Alice's room:snapshot so the next read is the member:joined for Bob.
	snapEnv := readEnvelopeWithin(t, connA, 2*time.Second)
	if snapEnv.Type != domain.MsgRoomSnapshot {
		t.Fatalf("Alice first frame = %q, want room:snapshot", snapEnv.Type)
	}

	// Client B connects.
	connB, _, err := fx.dial(fx.mintToken("bob", "Bob"), roomID)
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	defer connB.Close()

	// Bob's first frame must be room:snapshot containing Alice.
	bobSnap := readEnvelopeWithin(t, connB, 2*time.Second)
	if bobSnap.Type != domain.MsgRoomSnapshot {
		t.Fatalf("Bob first frame = %q, want room:snapshot", bobSnap.Type)
	}
	var snap domain.RoomSnapshot
	if err := json.Unmarshal(bobSnap.Data, &snap); err != nil {
		t.Fatalf("decode Bob snapshot: %v", err)
	}
	memberSet := map[string]bool{}
	for _, m := range snap.Members {
		memberSet[m.UserID] = true
	}
	if !memberSet["alice"] || !memberSet["bob"] {
		t.Errorf("Bob snapshot members = %v, want both alice and bob", snap.Members)
	}

	// Alice's NEXT frame must be member:joined for Bob.
	joinEnv := readEnvelopeWithin(t, connA, 2*time.Second)
	if joinEnv.Type != domain.MsgMemberJoined {
		t.Fatalf("Alice second frame = %q, want member:joined", joinEnv.Type)
	}
	var joinData domain.MemberJoinedData
	if err := json.Unmarshal(joinEnv.Data, &joinData); err != nil {
		t.Fatalf("decode member:joined: %v", err)
	}
	if joinData.UserID != "bob" {
		t.Errorf("member:joined user_id = %q, want bob", joinData.UserID)
	}
}

// ----------------------------------------------------------------------------
// Test 7: capacity full — third connection (MaxMembers=2) is upgraded then
// closed with a CAPACITY_FULL envelope. Validates both the close-frame path
// and the JSON error envelope sent before the close.
// ----------------------------------------------------------------------------

func TestWS_CapacityFull_ThirdConnectionRejected(t *testing.T) {
	fx := newWSFixture(t, 2)
	roomID := fx.createRoom("host", "Host")

	// Fill room with 2 connections.
	c1, _, err := fx.dial(fx.mintToken("u1", "U1"), roomID)
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	defer c1.Close()
	_ = readEnvelopeWithin(t, c1, 2*time.Second) // drain snapshot

	c2, _, err := fx.dial(fx.mintToken("u2", "U2"), roomID)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer c2.Close()
	_ = readEnvelopeWithin(t, c2, 2*time.Second)
	// Drain member:joined for u2 on c1 to keep its read pump unblocked.
	_ = readEnvelopeWithin(t, c1, 2*time.Second)

	// Wait for hub state to reflect both registrations (Register is sync
	// but reads of MemberCount inside Upgrade race with this test's local
	// view — a tiny sleep is the lowest-noise sync barrier short of
	// adding a test-only hook).
	deadline := time.Now().Add(time.Second)
	for fx.hub.MemberCount(roomID) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Third connection should upgrade then receive CAPACITY_FULL error
	// envelope followed by a close frame.
	c3, resp, err := fx.dial(fx.mintToken("u3", "U3"), roomID)
	if err != nil {
		t.Fatalf("dial 3 (expected upgrade then close): %v (status=%d)", err, responseStatus(resp))
	}
	defer c3.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("third dial status = %d, want 101 (upgrade + close-frame path)", resp.StatusCode)
	}

	// Read the error envelope.
	env := readEnvelopeWithin(t, c3, 2*time.Second)
	if env.Type != domain.MsgError {
		t.Fatalf("got envelope type %q, want %q", env.Type, domain.MsgError)
	}
	var errData domain.ErrorData
	if err := json.Unmarshal(env.Data, &errData); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if errData.Code != domain.ErrCodeCapacityFull {
		t.Errorf("error code = %q, want %q", errData.Code, domain.ErrCodeCapacityFull)
	}

	// Subsequent read should observe close. Gorilla returns
	// CloseError on a normal close frame.
	_ = c3.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := c3.ReadMessage()
	if readErr == nil {
		t.Errorf("expected read error after CAPACITY_FULL close, got nil")
	}
}

// ----------------------------------------------------------------------------
// Test 8: disconnect triggers repo.RemoveMember + member:left broadcast.
// ----------------------------------------------------------------------------

func TestWS_Disconnect_BroadcastsMemberLeftAndRemovesMember(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")

	connA, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	defer connA.Close()
	_ = readEnvelopeWithin(t, connA, 2*time.Second) // snapshot

	connB, _, err := fx.dial(fx.mintToken("bob", "Bob"), roomID)
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	_ = readEnvelopeWithin(t, connB, 2*time.Second) // snapshot
	_ = readEnvelopeWithin(t, connA, 2*time.Second) // member:joined bob

	// Verify Bob is in Redis members hash.
	members, err := fx.repo.ListMembers(context.Background(), roomID)
	if err != nil {
		t.Fatalf("ListMembers pre-disconnect: %v", err)
	}
	hasBob := false
	for _, m := range members {
		if m.UserID == "bob" {
			hasBob = true
		}
	}
	if !hasBob {
		t.Fatalf("Bob missing from Redis members pre-disconnect: %+v", members)
	}

	// Close Bob's connection (clean close to exercise the normal disconnect path).
	if err := connB.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
		time.Now().Add(2*time.Second),
	); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = connB.Close()

	// Alice should receive member:left for Bob.
	leftEnv := readEnvelopeWithin(t, connA, 3*time.Second)
	if leftEnv.Type != domain.MsgMemberLeft {
		t.Fatalf("Alice got envelope %q, want member:left", leftEnv.Type)
	}
	var leftData domain.MemberLeftData
	if err := json.Unmarshal(leftEnv.Data, &leftData); err != nil {
		t.Fatalf("decode member:left: %v", err)
	}
	if leftData.UserID != "bob" {
		t.Errorf("member:left user_id = %q, want bob", leftData.UserID)
	}

	// Verify Bob is gone from Redis (with a short retry loop because
	// OnClose runs on the hub goroutine slightly after the WS close).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		members, err = fx.repo.ListMembers(context.Background(), roomID)
		if err != nil {
			t.Fatalf("ListMembers post-disconnect: %v", err)
		}
		stillThere := false
		for _, m := range members {
			if m.UserID == "bob" {
				stillThere = true
			}
		}
		if !stillThere {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("Bob still in Redis members 2s after disconnect: %+v", members)
}

// ----------------------------------------------------------------------------
// Test 9: abrupt close (TCP-RST simulation via conn.Close without close-frame)
// still fires the OnClose cleanup callback.
// ----------------------------------------------------------------------------

func TestWS_AbruptClose_FiresOnCloseCleanup(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")

	connA, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	defer connA.Close()
	_ = readEnvelopeWithin(t, connA, 2*time.Second)

	connB, _, err := fx.dial(fx.mintToken("bob", "Bob"), roomID)
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	_ = readEnvelopeWithin(t, connB, 2*time.Second)
	_ = readEnvelopeWithin(t, connA, 2*time.Second) // member:joined bob

	// Drop Bob's underlying TCP without a close frame.
	if err := connB.NetConn().Close(); err != nil {
		t.Fatalf("net conn close: %v", err)
	}

	// Alice should still receive member:left.
	leftEnv := readEnvelopeWithin(t, connA, 3*time.Second)
	if leftEnv.Type != domain.MsgMemberLeft {
		t.Fatalf("Alice got envelope %q, want member:left after abrupt close", leftEnv.Type)
	}
}

// ----------------------------------------------------------------------------
// Test 10: multi-tab — same user with two connections; closing one keeps
// the user "present" (no member:left until the LAST tab closes).
//
// Implementation note: we use a goroutine + channel reader so that an
// expected-no-frame interval doesn't consume the connection's read
// deadline (gorilla closes the WS on a read-timeout, which would break
// the subsequent expected-frame assertion).
// ----------------------------------------------------------------------------

func TestWS_MultiTab_OnlyLastTabFiresMemberLeft(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("host", "Host")

	// Watcher (host) — receives every member:left so we can assert
	// it does NOT fire on the first multi-tab close.
	connHost, _, err := fx.dial(fx.mintToken("host", "Host"), roomID)
	if err != nil {
		t.Fatalf("dial host: %v", err)
	}
	defer connHost.Close()
	_ = readEnvelopeWithin(t, connHost, 2*time.Second) // snapshot

	// Alice opens two tabs.
	connA1, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial alice tab 1: %v", err)
	}
	_ = readEnvelopeWithin(t, connA1, 2*time.Second)   // alice tab1 snapshot
	_ = readEnvelopeWithin(t, connHost, 2*time.Second) // host sees member:joined alice

	connA2, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial alice tab 2: %v", err)
	}
	_ = readEnvelopeWithin(t, connA2, 2*time.Second)   // alice tab2 snapshot
	_ = readEnvelopeWithin(t, connHost, 2*time.Second) // host sees member:joined alice (second time)
	// NOTE: connA1 does NOT receive member:joined for tab2 — Hub.Broadcast
	// excludes ALL connections owned by the joining userID, so alice tab1
	// is excluded from her own tab2's member:joined fanout. No drain
	// needed on connA1 here.

	// Start a background reader on connHost that feeds every frame into a
	// channel. Avoids per-read deadlines that would close the connection
	// on the "expected no frame" assertion below.
	frames := make(chan domain.Envelope, 8)
	go func() {
		for {
			_, payload, rerr := connHost.ReadMessage()
			if rerr != nil {
				return
			}
			var env domain.Envelope
			if jerr := json.Unmarshal(payload, &env); jerr != nil {
				continue
			}
			frames <- env
		}
	}()

	// Close ONE of Alice's tabs. Host should NOT receive member:left
	// for alice within a short grace window — tab2 keeps her present.
	_ = connA1.Close()

	select {
	case env := <-frames:
		if env.Type == domain.MsgMemberLeft {
			var data domain.MemberLeftData
			_ = json.Unmarshal(env.Data, &data)
			if data.UserID == "alice" {
				t.Fatalf("premature member:left for alice while tab2 still open")
			}
		}
		// Any other frame type is unexpected but not fatal — the only
		// reasonable surprise is none. Continue to the next phase.
	case <-time.After(500 * time.Millisecond):
		// Good — no frame arrived. That's what we expected.
	}

	// Now close Alice's LAST tab.
	_ = connA2.Close()

	// THIS time, host MUST receive member:left for alice within a
	// reasonable window.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case env := <-frames:
			if env.Type != domain.MsgMemberLeft {
				// Could be a stray member:joined (e.g. echo) — keep reading.
				continue
			}
			var data domain.MemberLeftData
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("decode member:left: %v", err)
			}
			if data.UserID != "alice" {
				t.Errorf("member:left user_id = %q, want alice", data.UserID)
			}
			return
		case <-deadline:
			t.Fatalf("host did not receive member:left for alice within 3s of last-tab close")
		}
	}
}

// ----------------------------------------------------------------------------
// Test 11: Origin allowlist — rejects WS upgrades from non-PublicBaseURL
// browsers when AllowAllOrigins is false. Uses the same fixture but flips
// the config off so the buildWSOriginCheck path activates.
// ----------------------------------------------------------------------------

func TestWS_OriginAllowlist_RejectsMismatchedBrowserOrigin(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{
		PublicBaseURL:   "https://animeenigma.ru",
		MaxMembers:      10,
		AllowAllOrigins: false,
		JWT: authz.JWTConfig{
			Secret:          "test-secret-origin-case",
			Issuer:          "animeenigma-test",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
		},
	}
	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)
	roomSvc := service.NewRoomService(r, log)
	wsHub := hub.NewHub(r, log, "test-instance-origin")
	t.Cleanup(wsHub.Close)

	driftOrigin := service.NewDriftEngine(log)
	rlOrigin := service.NewRateLimiter()
	originRouter := service.NewInboundRouter(r, wsHub, driftOrigin, rlOrigin, nopCatalog{}, log)
	wsHandler := NewWebSocketHandler(wsHub, r, roomSvc, originRouter, newFakeGrace(), cfg, log)
	httpRouter := chi.NewRouter()
	httpRouter.Get("/api/watch-together/ws", wsHandler.Upgrade)
	server := httptest.NewServer(httpRouter)
	t.Cleanup(server.Close)

	// Seed a room.
	room, err := roomSvc.Create(context.Background(), "alice", "Alice", service.CreateRoomInput{
		AnimeID:       "a",
		EpisodeID:     "e",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "t",
	})
	if err != nil {
		t.Fatalf("seed room: %v", err)
	}

	// Mint token.
	jm := authz.NewJWTManager(cfg.JWT)
	pair, err := jm.GenerateTokenPair("alice", "Alice", authz.RoleUser, "sess-1")
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	u.Path = "/api/watch-together/ws"
	q := u.Query()
	q.Set("token", pair.AccessToken)
	q.Set("room", room.ID)
	u.RawQuery = q.Encode()

	hdr := http.Header{}
	hdr.Set("Origin", "https://evil.example") // not in allowlist

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	_, resp, err := dialer.Dial(u.String(), hdr)
	if err == nil {
		t.Fatal("expected origin rejection, got successful upgrade")
	}
	if resp == nil {
		t.Fatalf("no response on origin reject; err=%v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

// ----------------------------------------------------------------------------
// Plan 05.1: HandleUpgrade calls graceMgr.Cancel BEFORE hub.Register so a
// returning member cancels any pending reconnect window.
// ----------------------------------------------------------------------------

func TestWS_HandleUpgrade_CancelsPendingGrace(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")
	// Pre-pin the fake grace so Cancel returns "recovered" — verifies the
	// log path; the test still passes if Cancel returns false because the
	// assertion is on call count + roomID.
	fx.grace.cancelReturn = true

	conn, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = readEnvelopeWithin(t, conn, 2*time.Second) // snapshot

	calls := fx.grace.snapshotCancel()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 Cancel call, got %d: %v", len(calls), calls)
	}
	if calls[0] != roomID {
		t.Errorf("Cancel called with roomID = %q, want %q", calls[0], roomID)
	}
}

// ----------------------------------------------------------------------------
// Plan 05.1: makeOnClose calls graceMgr.Start when hub.MemberCount(roomID)
// drops to 0 (last connection in the room).
// ----------------------------------------------------------------------------

func TestWS_OnClose_LastConnection_StartsGrace(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")

	connA, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	_ = readEnvelopeWithin(t, connA, 2*time.Second) // snapshot

	// Close Alice — she's the only member, so OnClose should fire grace.Start.
	if err := connA.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
		time.Now().Add(2*time.Second),
	); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = connA.Close()

	// Wait for OnClose to run on the hub goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fx.grace.snapshotStart()) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	starts := fx.grace.snapshotStart()
	if len(starts) != 1 {
		t.Fatalf("expected exactly 1 Start call, got %d: %v", len(starts), starts)
	}
	if starts[0] != roomID {
		t.Errorf("Start called with roomID = %q, want %q", starts[0], roomID)
	}
}

// ----------------------------------------------------------------------------
// Plan 05.1: makeOnClose does NOT call graceMgr.Start when other members
// remain in the room (only the last-connection-in-room triggers the timer).
// ----------------------------------------------------------------------------

func TestWS_OnClose_NotLastConnection_NoGraceStart(t *testing.T) {
	fx := newWSFixture(t, 0)
	roomID := fx.createRoom("alice", "Alice")

	connA, _, err := fx.dial(fx.mintToken("alice", "Alice"), roomID)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	defer connA.Close()
	_ = readEnvelopeWithin(t, connA, 2*time.Second)

	connB, _, err := fx.dial(fx.mintToken("bob", "Bob"), roomID)
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	_ = readEnvelopeWithin(t, connB, 2*time.Second)
	_ = readEnvelopeWithin(t, connA, 2*time.Second) // member:joined bob

	// Close Bob — Alice remains, so grace.Start should NOT fire.
	if err := connB.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
		time.Now().Add(2*time.Second),
	); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = connB.Close()

	// Wait for member:left to fire on Alice — that's the signal OnClose
	// completed on the server side.
	leftEnv := readEnvelopeWithin(t, connA, 3*time.Second)
	if leftEnv.Type != domain.MsgMemberLeft {
		t.Fatalf("got %q, want member:left", leftEnv.Type)
	}

	// Grace.Start MUST NOT have been called (Alice is still there).
	if starts := fx.grace.snapshotStart(); len(starts) != 0 {
		t.Errorf("expected 0 Start calls (Alice still present), got %d: %v", len(starts), starts)
	}
}

// ----------------------------------------------------------------------------
// Test 12: buildWSOriginCheck unit test — same-origin + missing-Origin
// behavior + AllowAllOrigins precedence.
// ----------------------------------------------------------------------------

func TestBuildWSOriginCheck(t *testing.T) {
	cases := []struct {
		name           string
		baseURL        string
		allowAll       bool
		origin         string
		expectAccepted bool
	}{
		{"allow_all_overrides_everything", "https://animeenigma.ru", true, "https://evil.example", true},
		{"no_origin_header_allowed", "https://animeenigma.ru", false, "", true},
		{"matching_origin_allowed", "https://animeenigma.ru", false, "https://animeenigma.ru", true},
		{"mismatched_origin_rejected", "https://animeenigma.ru", false, "https://evil.example", false},
		{"malformed_origin_rejected", "https://animeenigma.ru", false, "::::not-a-url", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{PublicBaseURL: tc.baseURL, AllowAllOrigins: tc.allowAll}
			check := buildWSOriginCheck(cfg)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := check(req); got != tc.expectAccepted {
				t.Errorf("got %v, want %v", got, tc.expectAccepted)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// responseStatus returns 0 if resp is nil (gorilla's dialer sometimes does
// that on connection-refused / handshake-aborted paths), or the StatusCode
// otherwise. Useful for the t.Fatalf format strings in the negative tests.
func responseStatus(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

// usedImports is a no-op reference to defeat "imported and not used" if
// strings becomes unused across refactors. Avoids touching the import
// block during minor edits.
var _ = strings.HasPrefix
var _ sync.Once
