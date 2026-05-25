package hub

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// pubsubFrame is the envelope-wrapper put on the wt:room:{id}:events PUBSUB
// channel. We tag every publish with the originating Hub.instanceID so the
// subscriber loop on the SAME instance can drop its own echo (otherwise
// every broadcast would fan out twice on a single-instance deployment).
// v2 horizontal scale uses the same shape unchanged — other instances see
// a different InstanceID and apply the inner envelope locally.
type pubsubFrame struct {
	InstanceID string          `json:"instance_id"`
	Env        domain.Envelope `json:"env"`
}

// Hub is the per-room WebSocket connection registry. Public-API surface is
// frozen by the 01.3 plan §<hub_contract>:
//
//	Register, Unregister, Broadcast, SendTo, MemberCount, MemberUserIDs, Close
//
// All state-mutating methods are safe for concurrent use; reads use an
// RLock so MemberCount / MemberUserIDs called during a Broadcast don't
// block the broadcaster.
type Hub struct {
	// rooms maps roomID → set<*Connection>. Multi-tab users have multiple
	// entries in the set; MemberCount returns raw connection count (used for
	// capacity gate), MemberUserIDs dedups to logical users (used for snapshot).
	rooms map[string]map[*Connection]struct{}

	mu sync.RWMutex

	// repo is the Redis facade (publish + subscribe wrappers from 01.2).
	repo *repo.RoomRepo

	// instanceID tags every published pubsub frame so this hub can detect
	// and drop its own echoes (single-instance v1.0 behavior — v2
	// horizontal scale uses the same field to identify other instances).
	// Set once at construction; callers in main.go generate a UUID at boot.
	instanceID string

	// perRoomSub stores the cancel function for each per-room pubsub
	// subscriber goroutine. One subscriber per room, started lazily on
	// the first Register, torn down on the last Unregister.
	perRoomSub map[string]context.CancelFunc

	// ctx is the hub-wide background context; Close cancels it to drop
	// every subscriber. Each per-room subscriber is a child of this.
	ctx    context.Context
	cancel context.CancelFunc

	log *logger.Logger
}

// NewHub constructs a Hub bound to the given repo + instanceID. instanceID
// MUST be unique per running process (generated via uuid.NewString() at
// boot in cmd/watch-together-api/main.go). The repo is used for both the
// pubsub publish on Broadcast AND the per-room subscribe goroutine.
func NewHub(r *repo.RoomRepo, log *logger.Logger, instanceID string) *Hub {
	if log == nil {
		log = logger.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		rooms:      make(map[string]map[*Connection]struct{}),
		repo:       r,
		instanceID: instanceID,
		perRoomSub: make(map[string]context.CancelFunc),
		ctx:        ctx,
		cancel:     cancel,
		log:        log,
	}
}

// InstanceID returns the instance identifier used for pubsub origin-tagging.
// Exposed primarily for tests that synthesize pubsub frames; production
// callers should never need to read this.
func (h *Hub) InstanceID() string { return h.instanceID }

// Register attaches a new Connection to the room set and starts its two pumps.
// The websocket pointer is the real *gorilla/websocket.Conn coming out of the
// upgrade handler (01.5). Returns the new Connection so the caller can hold
// a reference (set OnMessage, query UserID for routing, etc).
//
// If the room set was empty before this call, Register also starts the
// per-room pubsub subscriber goroutine. The subscriber is torn down on the
// matching last-member Unregister.
func (h *Hub) Register(roomID, userID, username string, conn *websocket.Conn) (*Connection, error) {
	if roomID == "" || userID == "" {
		return nil, apperrors.InvalidInput("hub.Register: room_id and user_id are required")
	}
	if conn == nil {
		return nil, apperrors.InvalidInput("hub.Register: conn is nil")
	}
	return h.registerInternal(roomID, userID, username, conn), nil
}

// registerInternal is the wsConn-typed core of Register, factored out so
// hub_test.go can register fakeConn instances. The exported Register
// keeps the public signature locked to *websocket.Conn per the 01.3 contract.
func (h *Hub) registerInternal(roomID, userID, username string, conn wsConn) *Connection {
	c := newConnection(roomID, userID, username, conn, h.log)
	c.hub = h

	h.mu.Lock()
	set, ok := h.rooms[roomID]
	if !ok {
		set = make(map[*Connection]struct{})
		h.rooms[roomID] = set
	}
	set[c] = struct{}{}
	firstInRoom := len(set) == 1
	h.mu.Unlock()

	ActiveConnections.WithLabelValues(roomID).Inc()

	if firstInRoom {
		h.startRoomSubscriber(roomID)
	}

	// Start the pumps. They terminate independently — readPump on conn error
	// (and calls back into Unregister), writePump on sendCh close / conn
	// error (and calls Close which is idempotent with Unregister).
	go c.readPump(h.ctx)
	go c.writePump(h.ctx)

	h.log.Infow("watch_together hub register",
		"room_id", roomID,
		"user_id", userID,
		"username", username,
	)
	return c
}

// Unregister removes a Connection from its room set, closes the underlying
// websocket via Connection.Close (idempotent), and — if this was the last
// connection in the room — tears down the per-room pubsub subscriber.
//
// Safe to call from anywhere (the readPump's defer is the most common
// caller). Calling Unregister on a connection that's already gone is a
// no-op aside from the redundant Connection.Close, which is itself a no-op
// thanks to closeOnce.
func (h *Hub) Unregister(c *Connection) {
	if c == nil {
		return
	}

	h.mu.Lock()
	set, ok := h.rooms[c.RoomID]
	if ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.rooms, c.RoomID)
		}
	}
	lastInRoom := ok && len(set) == 0
	h.mu.Unlock()

	if ok {
		ActiveConnections.WithLabelValues(c.RoomID).Dec()
	}

	c.Close()

	if lastInRoom {
		h.stopRoomSubscriber(c.RoomID)
		// NOTE: grace-period handling (5min before Redis keys expire) is the
		// responsibility of the service layer in 01.5+, not the hub. Hub
		// only knows about live connection state.
		h.log.Infow("watch_together hub room empty",
			"room_id", c.RoomID,
			"note", "grace period managed by service layer",
		)
	}

	h.log.Infow("watch_together hub unregister",
		"room_id", c.RoomID,
		"user_id", c.UserID,
	)
}

// Broadcast sends env to every connection in roomID except those owned by
// excludeUserID (pass "" to broadcast to all). Returns the count of LOCAL
// recipients that received the message (skips drops + skipped users).
//
// Also publishes the wrapped envelope onto the wt:room:{id}:events Redis
// channel — forward-compat for v2 horizontal scale. The publish is
// fire-and-forget at the hub level; a publish error is logged but does not
// fail the call (local fanout has already happened or is happening).
func (h *Hub) Broadcast(ctx context.Context, roomID string, env domain.Envelope, excludeUserID string) (int, error) {
	if roomID == "" {
		return 0, apperrors.InvalidInput("hub.Broadcast: room_id is required")
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: marshal envelope failed")
	}

	delivered := h.localFanout(roomID, payload, env.Type, excludeUserID)

	// Wrap-and-publish to Redis pubsub. Failure here is non-fatal — local
	// recipients already got the message. v1.0 single-instance: the only
	// subscriber is ourselves and we drop our own echoes.
	frame := pubsubFrame{InstanceID: h.instanceID, Env: env}
	frameBytes, err := json.Marshal(frame)
	if err != nil {
		h.log.Warnw("watch_together hub pubsub marshal", "room_id", roomID, "err", err)
		return delivered, nil
	}
	if h.repo != nil {
		if perr := h.repo.Publish(ctx, roomID, frameBytes); perr != nil {
			h.log.Warnw("watch_together hub publish failed",
				"room_id", roomID,
				"event_type", env.Type,
				"err", perr,
			)
		}
	}
	return delivered, nil
}

// SendTo targets every connection owned by userID in roomID. Multi-tab users
// receive on every tab they have open in this room (so "play" arriving on
// tab A is reflected on tab B). Returns the count of connections that
// received the message.
func (h *Hub) SendTo(ctx context.Context, roomID, userID string, env domain.Envelope) (int, error) {
	if roomID == "" || userID == "" {
		return 0, apperrors.InvalidInput("hub.SendTo: room_id and user_id are required")
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: marshal envelope failed")
	}

	h.mu.RLock()
	set := h.rooms[roomID]
	targets := make([]*Connection, 0, len(set))
	for c := range set {
		if c.UserID == userID {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	delivered := 0
	for _, c := range targets {
		if c.Send(payload) {
			MessagesSentTotal.WithLabelValues(env.Type).Inc()
			delivered++
		}
	}
	return delivered, nil
}

// MemberUserIDs returns the deduplicated set of user IDs currently connected
// to roomID. O(N) on the connection count; called from 01.5 to populate
// RoomSnapshot.Members. Iteration order is undefined (Go map semantics).
func (h *Hub) MemberUserIDs(roomID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.rooms[roomID]
	if len(set) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(set))
	for c := range set {
		seen[c.UserID] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for uid := range seen {
		out = append(out, uid)
	}
	return out
}

// MemberCount returns the raw connection count for roomID (multi-tab users
// counted once per tab). Used by 01.5 to enforce per-room capacity at
// upgrade time — capacity is on raw connections, not logical users, so
// abusive multi-tab spawning can't bypass the limit.
func (h *Hub) MemberCount(roomID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[roomID])
}

// Close shuts down the hub: cancels the master context (which drops every
// per-room subscriber), then closes every live connection. Called from
// main.go on SIGTERM. Safe to call exactly once.
func (h *Hub) Close() {
	h.cancel()

	h.mu.Lock()
	rooms := h.rooms
	h.rooms = make(map[string]map[*Connection]struct{})
	for _, cancel := range h.perRoomSub {
		cancel()
	}
	h.perRoomSub = make(map[string]context.CancelFunc)
	h.mu.Unlock()

	for _, set := range rooms {
		for c := range set {
			c.Close()
		}
	}
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

// localFanout sends payload to every connection in roomID except those owned
// by excludeUserID. Bumps wt_ws_messages_sent_total{type} per successful
// delivery; drops (Send returning false) are accounted for in
// wt_ws_messages_dropped_total inside Connection.Send.
func (h *Hub) localFanout(roomID string, payload []byte, eventType, excludeUserID string) int {
	h.mu.RLock()
	set := h.rooms[roomID]
	// Snapshot the recipients under the read lock so the actual Send calls
	// (which block briefly on the recipient's sendCh) happen lock-free.
	recipients := make([]*Connection, 0, len(set))
	for c := range set {
		if excludeUserID != "" && c.UserID == excludeUserID {
			continue
		}
		recipients = append(recipients, c)
	}
	h.mu.RUnlock()

	delivered := 0
	for _, c := range recipients {
		if c.Send(payload) {
			MessagesSentTotal.WithLabelValues(eventType).Inc()
			delivered++
		}
	}
	return delivered
}

// startRoomSubscriber spawns the per-room pubsub subscriber goroutine. The
// subscriber:
//
//   1. Subscribes to wt:room:{id}:events via repo.Subscribe.
//   2. For each incoming pubsubFrame, drops it if InstanceID == h.instanceID
//      (own echo — v1.0 single-instance default path).
//   3. Otherwise, does a local fanout of frame.Env (v2 horizontal scale path).
//
// Stored cancel function is invoked by stopRoomSubscriber on last-Unregister.
func (h *Hub) startRoomSubscriber(roomID string) {
	if h.repo == nil {
		return
	}

	subCtx, cancel := context.WithCancel(h.ctx)
	h.mu.Lock()
	// Replace any stale cancel (shouldn't happen, but defensive).
	if prev, ok := h.perRoomSub[roomID]; ok {
		prev()
	}
	h.perRoomSub[roomID] = cancel
	h.mu.Unlock()

	go h.roomSubscriberLoop(subCtx, roomID)
}

// stopRoomSubscriber cancels the per-room subscriber goroutine if one exists.
// Safe to call for rooms that never had a subscriber registered.
func (h *Hub) stopRoomSubscriber(roomID string) {
	h.mu.Lock()
	cancel, ok := h.perRoomSub[roomID]
	if ok {
		delete(h.perRoomSub, roomID)
	}
	h.mu.Unlock()
	if ok {
		cancel()
	}
}

// roomSubscriberLoop is the goroutine started by startRoomSubscriber. Exits
// when subCtx is cancelled (last-member-leaves OR hub.Close).
func (h *Hub) roomSubscriberLoop(ctx context.Context, roomID string) {
	sub := h.repo.Subscribe(ctx, roomID)
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			h.handlePubsubMessage(roomID, msg)
		}
	}
}

// handlePubsubMessage decodes a single pubsubFrame and either drops it (own
// echo) or fans it out locally (foreign instance). Extracted so the unit
// test can drive synthetic frames without spinning a real go-redis
// subscription.
func (h *Hub) handlePubsubMessage(roomID string, msg *redis.Message) {
	if msg == nil || msg.Payload == "" {
		return
	}
	var frame pubsubFrame
	if err := json.Unmarshal([]byte(msg.Payload), &frame); err != nil {
		h.log.Warnw("watch_together hub pubsub decode",
			"room_id", roomID,
			"err", err,
		)
		return
	}
	if frame.InstanceID == h.instanceID {
		// Own echo — drop. This is the v1.0 single-instance default path.
		return
	}
	// Foreign instance — apply the inner envelope as a local-only fanout.
	// We re-marshal Env (not the wrapper) so connections see the plain
	// envelope shape, identical to a same-instance Broadcast recipient.
	payload, err := json.Marshal(frame.Env)
	if err != nil {
		h.log.Warnw("watch_together hub pubsub apply marshal",
			"room_id", roomID,
			"err", err,
		)
		return
	}
	h.localFanout(roomID, payload, frame.Env.Type, "")
}
