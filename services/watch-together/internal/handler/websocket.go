// Package handler — websocket.go is the 01.5 WS upgrade entry point.
//
// Lifecycle (per 01.5-PLAN.md §<lifecycle_contract>):
//
//  1. Pre-upgrade JWT validation from ?token=... → 401 on failure.
//  2. Pre-upgrade room presence check via repo.Exists → 404 on miss.
//  3. Origin-header allowlist (production) or open (dev via cfg.AllowAllOrigins).
//  4. websocket.Upgrader.Upgrade.
//  5. Post-upgrade capacity check via hub.MemberCount → CAPACITY_FULL close frame.
//  6. repo.AddMember (member meta persisted to Redis).
//  7. hub.Register (starts read+write pumps via the 01.3 contract).
//  8. Send room:snapshot (the FIRST envelope the client sees).
//  9. Broadcast member:joined to everyone except the joining user.
// 10. Install OnClose callback that on disconnect:
//     a. RemoveMember from Redis ONLY when this is the user's last connection.
//     b. Broadcast member:left to remaining members.
//
// Auth path divergence from CONTEXT.md: CONTEXT.md mentions ROOM_NOT_FOUND as
// a close frame, but pre-upgrade HTTP 404 is more debuggable (browser
// dev-tools network panel surfaces it). The frontend treats both shapes
// identically (Phase 2). Documented in 01-05-SUMMARY.md.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/hub"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/service"
)

// closeFrameDeadline is how long we wait for the CAPACITY_FULL close frame
// to flush to the wire before giving up. Generous because the connection
// is being thrown away anyway and we want the frame to land in 95%+ of
// network conditions so the frontend can render a friendly error.
const closeFrameDeadline = 2 * time.Second

// WebSocketHandler serves GET /api/watch-together/ws. Owns the Origin
// allowlist, JWT validation, capacity gate, and the per-connection
// lifecycle hooks (snapshot on enter, member:left + RemoveMember on exit).
// Inbound message routing lives in 01.6 — this handler leaves
// Connection.OnMessage at nil with a TODO marker.
type WebSocketHandler struct {
	hub        *hub.Hub
	repo       *repo.RoomRepo
	roomSvc    *service.RoomService
	cfg        *config.Config
	log        *logger.Logger
	upgrader   websocket.Upgrader
	jwtManager *authz.JWTManager
}

// NewWebSocketHandler wires the dependencies and pre-builds the Origin
// allowlist + JWT manager so per-request work is minimal. Pass nil for log
// to fall back to logger.Default().
func NewWebSocketHandler(
	h *hub.Hub,
	r *repo.RoomRepo,
	roomSvc *service.RoomService,
	cfg *config.Config,
	log *logger.Logger,
) *WebSocketHandler {
	if log == nil {
		log = logger.Default()
	}
	return &WebSocketHandler{
		hub:        h,
		repo:       r,
		roomSvc:    roomSvc,
		cfg:        cfg,
		log:        log,
		upgrader:   newWSUpgrader(cfg),
		jwtManager: authz.NewJWTManager(cfg.JWT),
	}
}

// Upgrade is the chi.HandlerFunc-compatible entry point. Mounted at
// /api/watch-together/ws by transport.NewRouter — OUTSIDE the
// AuthMiddleware-wrapped subgroup because browsers can't set custom
// headers on the WS upgrade request. Auth lives in the ?token= query
// param and is validated here.
func (h *WebSocketHandler) Upgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Step 1: JWT validation. Pre-upgrade 401 so browser dev-tools shows
	// the failure clearly in the network panel (WT-NF-01).
	token := r.URL.Query().Get("token")
	if token == "" {
		h.log.Debugw("watch_together ws upgrade rejected: missing token")
		httputil.Unauthorized(w)
		return
	}
	claims, err := h.jwtManager.ValidateAccessToken(token)
	if err != nil {
		h.log.Debugw("watch_together ws upgrade rejected: invalid token", "err", err)
		httputil.Unauthorized(w)
		return
	}
	userID := claims.UserID
	username := claims.Username

	// Step 2: Room presence check. Pre-upgrade so the failure shows as
	// HTTP 404 in the network panel rather than a successful upgrade
	// followed by a mysterious immediate close. Diverges from CONTEXT.md's
	// "close-frame ROOM_NOT_FOUND" — documented in 01-05-SUMMARY.md.
	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		h.log.Debugw("watch_together ws upgrade rejected: missing room")
		httputil.BadRequest(w, "room query param is required")
		return
	}
	exists, err := h.repo.Exists(ctx, roomID)
	if err != nil {
		h.log.Errorw("watch_together ws exists check failed",
			"room_id", roomID,
			"user_id", userID,
			"err", err,
		)
		httputil.Error(w, err)
		return
	}
	if !exists {
		h.log.Infow("watch_together ws upgrade rejected: room not found",
			"room_id", roomID,
			"user_id", userID,
		)
		// libs/httputil.NotFound emits a JSON {success:false,error:{code,message}}
		// envelope keyed to the project's standard error shape. Code is
		// ROOM_NOT_FOUND for parity with the CONTEXT.md close-frame
		// vocabulary so the frontend can detect either path with the same
		// switch.
		httputil.NotFound(w, "ROOM_NOT_FOUND")
		return
	}

	// Step 3: Upgrade. Origin allowlist enforced inside the Upgrader's
	// CheckOrigin hook — built once in NewWebSocketHandler.
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote the HTTP error response (403 typically).
		// We just log; do NOT touch w further or we'll double-write.
		h.log.Warnw("watch_together ws upgrade failed",
			"room_id", roomID,
			"user_id", userID,
			"err", err,
		)
		return
	}

	// Step 4: Capacity check. Post-upgrade because we need a wire to send
	// the CAPACITY_FULL envelope on before closing. The plan's
	// <lifecycle_contract> step 5 specifies this ordering explicitly.
	if h.cfg.MaxMembers > 0 && h.hub.MemberCount(roomID) >= h.cfg.MaxMembers {
		h.log.Infow("watch_together ws capacity full",
			"room_id", roomID,
			"user_id", userID,
			"max_members", h.cfg.MaxMembers,
		)
		writeCloseFrameError(conn, domain.ErrCodeCapacityFull, "room at capacity")
		_ = conn.Close()
		return
	}

	// Step 5: Persist member meta. JoinedAt + LastSeenAt are the same
	// timestamp on first join; presence:heartbeat in 01.6 bumps LastSeenAt.
	// AvatarURL is empty for v1.0 (no user-profile lookup in this service);
	// the frontend renders a fallback avatar derived from username.
	now := time.Now()
	meta := domain.MemberMeta{
		Username:   username,
		AvatarURL:  "", // reserved for future profile lookup
		JoinedAt:   now.Unix(),
		LastSeenAt: now.Unix(),
	}
	if err := h.repo.AddMember(ctx, roomID, userID, meta); err != nil {
		h.log.Errorw("watch_together ws add_member failed",
			"room_id", roomID,
			"user_id", userID,
			"err", err,
		)
		_ = conn.Close()
		return
	}

	// Step 6: Register with the hub. The 01.3 contract starts both pumps
	// inside Register — we get back the *Connection so we can install
	// OnMessage (stubbed for 01.6) and OnClose (cleanup callback below).
	c, err := h.hub.Register(roomID, userID, username, conn)
	if err != nil {
		h.log.Errorw("watch_together ws hub register failed",
			"room_id", roomID,
			"user_id", userID,
			"err", err,
		)
		// Best-effort cleanup; we just added the member, undo it.
		if rmErr := h.repo.RemoveMember(context.Background(), roomID, userID); rmErr != nil {
			h.log.Warnw("watch_together ws remove_member after register failure",
				"room_id", roomID,
				"user_id", userID,
				"err", rmErr,
			)
		}
		_ = conn.Close()
		return
	}

	// TODO(01.6): wire c.OnMessage = inboundRouter.Dispatch. For now any
	// inbound frame is decoded by readPump and silently discarded.
	c.OnMessage = nil

	// Install the lifecycle cleanup hook BEFORE we send the snapshot so an
	// abnormal early disconnect (TCP RST between Register and Send) still
	// produces the proper member:left broadcast + repo.RemoveMember pair.
	c.OnClose = h.makeOnClose(roomID, userID)

	// Step 7: Send the room:snapshot envelope. This is the FIRST frame the
	// client sees and contains the full RoomSnapshot (state + members +
	// last 50 messages + protocol_version). The snapshot is built by the
	// roomSvc.Get fast path against Redis — it already includes the just-
	// added member because AddMember (above) ran before Register.
	if err := h.sendSnapshot(ctx, c, roomID); err != nil {
		h.log.Errorw("watch_together ws snapshot send failed",
			"room_id", roomID,
			"user_id", userID,
			"err", err,
		)
		// Snapshot failure → tear down the connection. The OnClose hook
		// will fire via hub.Unregister and handle member:left + RemoveMember.
		h.hub.Unregister(c)
		return
	}

	// Step 8: Broadcast member:joined to everyone EXCEPT the joining user.
	// Fire-and-forget — broadcast failure is non-fatal for the new
	// connection itself.
	h.broadcastMemberJoined(ctx, roomID, userID, meta)

	h.log.Infow("watch_together ws connected",
		"action", "ws_connect",
		"room_id", roomID,
		"user_id", userID,
		"username", username,
	)
	// Connection is now owned by the hub's pumps. We return; the
	// readPump's defer calls Hub.Unregister on disconnect, which fires
	// OnClose for cleanup.
}

// sendSnapshot builds the canonical RoomSnapshot via the service layer and
// pushes it onto the connection's outbound channel as a room:snapshot
// envelope. Returns an error only if the snapshot fetch fails — a Send
// drop (full buffer at this stage is essentially impossible since the
// connection was just created with an empty buffer) is treated as a
// silent failure since the connection is already in trouble.
func (h *WebSocketHandler) sendSnapshot(ctx context.Context, c *hub.Connection, roomID string) error {
	snap, err := h.roomSvc.Get(ctx, roomID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	env := domain.Envelope{
		Type: domain.MsgRoomSnapshot,
		Data: data,
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}

	if !c.Send(payload) {
		// Buffer full at handshake time would imply a misconfigured
		// sendBufferSize (default 64); log and return nil so the caller
		// keeps the connection. The client will eventually time out
		// waiting for the snapshot and reconnect — better than a hard
		// failure that closes a healthy connection.
		h.log.Warnw("watch_together ws snapshot send dropped at handshake",
			"room_id", roomID,
			"user_id", c.UserID,
		)
	}
	return nil
}

// broadcastMemberJoined emits a member:joined envelope to every connection
// in roomID EXCEPT those owned by userID. Errors are logged-not-returned —
// a broadcast failure here is non-fatal for the new connection.
func (h *WebSocketHandler) broadcastMemberJoined(
	ctx context.Context,
	roomID, userID string,
	meta domain.MemberMeta,
) {
	data, err := json.Marshal(domain.MemberJoinedData{
		UserID: userID,
		Member: meta,
	})
	if err != nil {
		h.log.Warnw("watch_together ws marshal member_joined", "err", err)
		return
	}
	env := domain.Envelope{Type: domain.MsgMemberJoined, Data: data}
	if _, err := h.hub.Broadcast(ctx, roomID, env, userID); err != nil {
		h.log.Warnw("watch_together ws broadcast member_joined",
			"room_id", roomID,
			"user_id", userID,
			"err", err,
		)
	}
}

// makeOnClose returns a closure suitable for assigning to Connection.OnClose.
// The closure encapsulates the "last connection for this user" check so
// multi-tab disconnects only fire one repo.RemoveMember + member:left pair
// (per 01.5-PLAN.md §<tasks>/Test 10).
//
// The check uses hub.MemberUserIDs AFTER Unregister has already removed
// the leaving connection — so if the user has another tab open, their
// userID still appears in the deduplicated list and we skip cleanup.
func (h *WebSocketHandler) makeOnClose(roomID, userID string) func(*hub.Connection) {
	return func(_ *hub.Connection) {
		// Use a fresh background context — the request context is long
		// gone by the time the connection drops (could be hours later).
		// Generous timeout because a Redis hiccup here is recoverable
		// noise, not user-facing latency.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Has the leaving user got any other connections still in this
		// room? Check the post-Unregister state (Hub.Unregister has
		// already removed this connection from the room set before
		// firing OnClose — see hub.go).
		stillPresent := false
		for _, otherID := range h.hub.MemberUserIDs(roomID) {
			if otherID == userID {
				stillPresent = true
				break
			}
		}
		if stillPresent {
			// Multi-tab: another tab keeps the user present. Don't
			// remove from Redis, don't broadcast member:left — only
			// the LAST tab leaving counts as "the user left".
			h.log.Debugw("watch_together ws onclose multi-tab still present",
				"room_id", roomID,
				"user_id", userID,
			)
			return
		}

		if err := h.repo.RemoveMember(ctx, roomID, userID); err != nil {
			// Logged not propagated — the connection is already gone,
			// nothing left to do but record the failure.
			h.log.Warnw("watch_together ws remove_member on close",
				"room_id", roomID,
				"user_id", userID,
				"err", err,
			)
		}

		data, err := json.Marshal(domain.MemberLeftData{UserID: userID})
		if err != nil {
			h.log.Warnw("watch_together ws marshal member_left", "err", err)
			return
		}
		env := domain.Envelope{Type: domain.MsgMemberLeft, Data: data}
		// Empty excludeUserID — broadcast to everyone remaining in the room.
		// (The leaving user's connection is already removed from the hub
		// before OnClose fires, so they wouldn't receive it anyway.)
		if _, err := h.hub.Broadcast(ctx, roomID, env, ""); err != nil {
			h.log.Warnw("watch_together ws broadcast member_left",
				"room_id", roomID,
				"user_id", userID,
				"err", err,
			)
		}

		h.log.Infow("watch_together ws disconnected",
			"action", "ws_disconnect",
			"room_id", roomID,
			"user_id", userID,
		)
	}
}

// writeCloseFrameError sends a single MsgError envelope as a text frame
// (so the client's normal envelope decoder picks it up) AND a websocket
// close-control frame (so the WS API also emits a clean close event).
// Used for CAPACITY_FULL — the only post-upgrade rejection path.
//
// errors.Is on the close-frame side is unnecessary because we always
// supply CloseNormalClosure; the code field below it is for the WS
// protocol, not our domain error vocabulary.
func writeCloseFrameError(conn *websocket.Conn, code, message string) {
	// Best-effort text frame so the JSON envelope decoder sees the error.
	env := domain.Envelope{Type: domain.MsgError}
	if data, err := json.Marshal(domain.ErrorData{Code: code, Message: message}); err == nil {
		env.Data = data
	}
	if payload, err := json.Marshal(env); err == nil {
		_ = conn.SetWriteDeadline(time.Now().Add(closeFrameDeadline))
		_ = conn.WriteMessage(websocket.TextMessage, payload)
	}

	// Then the proper close control frame so the client observes a clean
	// close event with our reason string.
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, code),
		time.Now().Add(closeFrameDeadline),
	)
}

// newWSUpgrader builds the gorilla websocket.Upgrader with the right
// Origin-allowlist semantics:
//
//   - cfg.AllowAllOrigins=true → CheckOrigin returns true unconditionally
//     (local dev across Vite ports). NEVER enable in prod.
//   - Otherwise → allow only Origin headers whose scheme://host matches
//     cfg.PublicBaseURL. Same-origin requests (no Origin header) are
//     ALLOWED — non-browser clients like wscat / smoke tests don't send
//     Origin, and rejecting them would break the smoke-test acceptance
//     criterion in the plan.
func newWSUpgrader(cfg *config.Config) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     buildWSOriginCheck(cfg),
	}
}

// buildWSOriginCheck pre-computes the allowed origin set so each request
// is a single map lookup. Mirrors the services/rooms helper but tightened
// to allow an empty Origin (wscat / curl-based smoke tests) when we're
// NOT in AllowAllOrigins mode — a real browser always sends Origin, so
// the absence of one signals a non-browser client which we want to
// support for ops tooling.
func buildWSOriginCheck(cfg *config.Config) func(r *http.Request) bool {
	if cfg.AllowAllOrigins {
		return func(*http.Request) bool { return true }
	}

	allowed := map[string]struct{}{}
	if u, err := url.Parse(cfg.PublicBaseURL); err == nil && u.Host != "" {
		allowed[u.Scheme+"://"+u.Host] = struct{}{}
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// No Origin header — non-browser client (wscat / smoke).
			// Allow so ops tooling works; browsers always set Origin so
			// this branch can't be exploited from a malicious site.
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		_, ok := allowed[u.Scheme+"://"+u.Host]
		return ok
	}
}

