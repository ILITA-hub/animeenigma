// Package service — inbound.go is the WebSocket inbound message router for
// the watch-together service.
//
// Role: every `playback:*`, `state:change_*`, `chat:*`, `presence:*` envelope
// the readPump in hub/connection.go decodes hits Dispatch, which routes to
// one of 10 typed handlers, applies side effects against Redis via repo, and
// fans out the corresponding outbound envelope(s) via hub.Broadcast or
// hub.SendTo.
//
// The router is the production binding for Connection.OnMessage installed by
// the WS upgrade handler in 01.5 (handler/websocket.go). 01.5 left the binding
// at nil with a TODO(01.6) marker; 01.6.3 wires `c.OnMessage = router.Dispatch`
// and `c.OnClose` chains into router.OnDisconnect for drift + rate-limit
// cleanup.
//
// Design references:
//   - docs/superpowers/specs/2026-05-25-watch-together-design.md §Inbound &
//     §Server-outbound (canonical envelope shapes)
//   - .planning/workstreams/watch-together/REQUIREMENTS.md (WT-FOUND-05,
//     WT-FOUND-07, WT-NF-02)
//   - .planning/workstreams/watch-together/phases/01-backend-foundation/01.6-PLAN.md
//     §<dispatch_table> (handler-by-handler contract)
//
// Metric policy:
//   - wt_ws_messages_received_total{type} is bumped EXACTLY ONCE per inbound
//     envelope by hub/connection.go readPump BEFORE invoking OnMessage. The
//     router does NOT re-bump that counter — double-counting would break the
//     Phase 5 Grafana inbound-rate panel. (Decision recorded in 01-06-SUMMARY.md.)
//   - Router-specific counters live in service/metrics.go:
//     DriftCorrectionsTotal{severity}, RateLimitedTotal{type},
//     ChatMessagesTotal, ReactionsTotal.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// HubFanout is the narrow subset of hub.Hub the InboundRouter actually
// touches. Extracting an interface lets unit tests in inbound_test.go pass
// a fakeHub that captures Broadcast/SendTo calls without instantiating a
// real hub + WS connections. The real *hub.Hub satisfies this interface
// by signature (verified at compile time when the WS handler wires it in
// main.go).
type HubFanout interface {
	Broadcast(ctx context.Context, roomID string, env domain.Envelope, excludeUserID string) (int, error)
	SendTo(ctx context.Context, roomID, userID string, env domain.Envelope) (int, error)
}

// ConnectionCtx is the minimal Connection-shaped view the router needs.
// Defined here so tests don't have to construct a real hub.Connection
// (which carries goroutine pumps + private state). The real
// hub.Connection satisfies this via its three exported fields.
type ConnectionCtx struct {
	RoomID   string
	UserID   string
	Username string
}

// errCodeBadPayload is the error code returned to the sender when the
// router can't decode the inbound payload's Data into the expected shape.
// Not added to domain/ws_message.go because it's a router-implementation
// detail; the frontend should treat any error code starting with "BAD_"
// as "you sent malformed data".
const errCodeBadPayload = "BAD_PAYLOAD"

// errCodeUnknownType is the error code returned when env.Type doesn't match
// any of the 10 inbound message types. Logged at warn level too — a sustained
// stream of these signals either a client/server protocol mismatch or a
// malicious peer.
const errCodeUnknownType = "UNKNOWN_TYPE"

// chatBodyCharLimit is the maximum chat body length per WT-FOUND-05. Bodies
// exceeding this are rejected with ErrCodeChatTooLong sender-only — never
// truncated silently, never broadcast.
const chatBodyCharLimit = 500

// reactionWhitelist enumerates the 24 anime-friendly emoji accepted by
// handleReaction. Final list is locked in Phase 2 (UI ships the matching
// palette in WT-SHELL-04); for Phase 1 this is the placeholder list called
// out in 01.6-PLAN.md §handleReaction. Reactions outside the whitelist are
// silently dropped (UX-friendly: a client emoji-picker bug shouldn't surface
// as a hard error frame).
var reactionWhitelist = map[string]struct{}{
	"🔥":  {},
	"❤️": {}, // U+2764 U+FE0F — the dressed-with-VS16 variant most clients send
	"😂":  {},
	"😭":  {},
	"👀":  {},
	"🙏":  {},
	"🎉":  {},
	"✨":  {},
	"💀":  {},
	"🥺":  {},
	"😍":  {},
	"🤔":  {},
	"👏":  {},
	"🙌":  {},
	"😱":  {},
	"😎":  {},
	"🌸":  {},
	"⚡":  {}, // U+26A1 lightning bolt
	"💯":  {},
	"🎌":  {},
	"🍣":  {},
	"🌟":  {},
	"💢":  {},
	"🤯":  {},
}

// InboundRouter is the per-service dispatcher that owns every inbound
// envelope handler. Singleton at boot: main.go constructs one and passes it
// into the WS handler.
//
// Time / ID injection: now() and newID() are pluggable for tests (default
// time.Now / uuid.NewString in production). Injection lives behind methods,
// not exported fields — see SetClockForTest / SetIDProviderForTest.
type InboundRouter struct {
	repo  *repo.RoomRepo
	hub   HubFanout
	drift *DriftEngine
	rl    *RateLimiter
	log   *logger.Logger

	now   func() time.Time
	newID func() string
}

// NewInboundRouter wires the deps and installs the production now/newID
// providers. Pass nil for log to fall back to logger.Default().
func NewInboundRouter(
	r *repo.RoomRepo,
	h HubFanout,
	drift *DriftEngine,
	rl *RateLimiter,
	log *logger.Logger,
) *InboundRouter {
	if log == nil {
		log = logger.Default()
	}
	return &InboundRouter{
		repo:  r,
		hub:   h,
		drift: drift,
		rl:    rl,
		log:   log,
		now:   time.Now,
		newID: uuid.NewString,
	}
}

// SetClockForTest overrides the router's `now` provider. INTERNAL TEST USE
// ONLY — pin envelope server_ts / chat message TS to a deterministic instant.
func (r *InboundRouter) SetClockForTest(fn func() time.Time) {
	r.now = fn
}

// SetIDProviderForTest overrides the chat-message-ID generator. INTERNAL
// TEST USE ONLY — pin ChatMessage.ID values for deterministic asserts.
func (r *InboundRouter) SetIDProviderForTest(fn func() string) {
	r.newID = fn
}

// Dispatch is the Connection.OnMessage callback installed by 01.5/01.6 in
// the WS handler. Receives the connection (for room/user context) and the
// decoded envelope (Type + Data) from hub/connection.go readPump.
//
// Concurrency: invoked from the connection's readPump goroutine, one
// invocation per inbound frame. Multiple connections in the same room
// each have their own readPump, so Dispatch can run concurrently for
// different members. All side effects flow through repo (Redis-atomic
// pipelines) and hub (mutex-protected fanout), so no router-level
// serialization is required.
func (r *InboundRouter) Dispatch(conn ConnectionCtx, env domain.Envelope) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch env.Type {
	case domain.MsgPlaybackPlay:
		r.handlePlaybackEvent(ctx, conn, env.Data, "play", domain.StatePlaying)
	case domain.MsgPlaybackPause:
		r.handlePlaybackEvent(ctx, conn, env.Data, "pause", domain.StatePaused)
	case domain.MsgPlaybackSeek:
		r.handleSeek(ctx, conn, env.Data)
	case domain.MsgPlaybackTimeTick:
		r.handleTimeTick(ctx, conn, env.Data)
	case domain.MsgStateChangeEpisode:
		r.handleStateChange(ctx, conn, env.Data, "episode_id")
	case domain.MsgStateChangePlayer:
		r.handleStateChange(ctx, conn, env.Data, "player")
	case domain.MsgStateChangeTrans:
		r.handleStateChange(ctx, conn, env.Data, "translation_id")
	case domain.MsgChatMessage:
		r.handleChat(ctx, conn, env.Data)
	case domain.MsgChatReaction:
		r.handleReaction(ctx, conn, env.Data)
	case domain.MsgPresenceHeartbeat:
		r.handleHeartbeat(ctx, conn, env.Data)
	default:
		r.log.Warnw("watch_together inbound unknown type",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"type", env.Type,
		)
		r.sendErrorToSelf(ctx, conn, errCodeUnknownType, fmt.Sprintf("unknown message type: %s", env.Type), "")
	}
}

// OnDisconnect is the per-connection lifecycle hook invoked from the WS
// handler's OnClose callback. Drops the member's drift state + rate-limit
// buckets so a long-running service doesn't accumulate dead user state.
//
// Idempotent — calling on a member who never had state is a no-op.
func (r *InboundRouter) OnDisconnect(roomID, userID string) {
	if r.drift != nil {
		r.drift.Reset(roomID, userID)
	}
	if r.rl != nil {
		r.rl.Forget(userID)
	}
}

// ----------------------------------------------------------------------------
// Handler — playback:play / playback:pause
//
// Both share the same shape (PlaybackPlayData / PlaybackPauseData both carry
// just `time`) and the same side-effect pattern (HSET state + time +
// updated_at, broadcast playback:event with attribution). The `kind` and
// `state` arguments differentiate them.
// ----------------------------------------------------------------------------

func (r *InboundRouter) handlePlaybackEvent(
	ctx context.Context,
	conn ConnectionCtx,
	data json.RawMessage,
	kind, newState string,
) {
	var payload domain.PlaybackPlayData
	if err := json.Unmarshal(data, &payload); err != nil {
		r.sendBadPayload(ctx, conn, kind, err)
		return
	}

	now := r.now()
	nowMs := now.UnixMilli()

	if err := r.repo.UpdateRoomState(ctx, conn.RoomID, map[string]interface{}{
		"playback_state":           newState,
		"playback_time":            payload.Time,
		"playback_time_updated_at": nowMs,
	}); err != nil {
		r.log.Errorw("watch_together update room state",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"kind", kind,
			"err", err,
		)
		return
	}

	// playback:event broadcast — excludes sender (they already know they
	// did it; design doc §Server-outbound).
	out, err := buildEnvelope(domain.MsgPlaybackEvent, domain.PlaybackEventData{
		Kind:     kind,
		Time:     payload.Time,
		ByUserID: conn.UserID,
		ServerTS: nowMs,
	})
	if err != nil {
		r.log.Errorw("watch_together marshal playback_event", "err", err)
		return
	}
	if _, err := r.hub.Broadcast(ctx, conn.RoomID, out, conn.UserID); err != nil {
		r.log.Warnw("watch_together broadcast playback_event",
			"room_id", conn.RoomID,
			"err", err,
		)
	}
}

// ----------------------------------------------------------------------------
// Handler — playback:seek
//
// Same shape as play/pause but ALSO rate-limited (1/sec/user, WT-NF-02).
// Rejection path: send RATE_LIMITED to sender only, do NOT update Redis.
// ----------------------------------------------------------------------------

func (r *InboundRouter) handleSeek(ctx context.Context, conn ConnectionCtx, data json.RawMessage) {
	if !r.rl.AllowSeek(conn.UserID) {
		RateLimitedTotal.WithLabelValues(domain.MsgPlaybackSeek).Inc()
		r.sendErrorToSelf(ctx, conn, domain.ErrCodeRateLimited, "seek rate limit exceeded (1/sec)", "")
		return
	}

	var payload domain.PlaybackSeekData
	if err := json.Unmarshal(data, &payload); err != nil {
		r.sendBadPayload(ctx, conn, "seek", err)
		return
	}

	now := r.now()
	nowMs := now.UnixMilli()

	if err := r.repo.UpdateRoomState(ctx, conn.RoomID, map[string]interface{}{
		"playback_time":            payload.Time,
		"playback_time_updated_at": nowMs,
	}); err != nil {
		r.log.Errorw("watch_together update room (seek)",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"err", err,
		)
		return
	}

	// playback:event broadcast — exclude sender.
	out, err := buildEnvelope(domain.MsgPlaybackEvent, domain.PlaybackEventData{
		Kind:     "seek",
		Time:     payload.Time,
		ByUserID: conn.UserID,
		ServerTS: nowMs,
	})
	if err != nil {
		r.log.Errorw("watch_together marshal seek event", "err", err)
		return
	}
	if _, err := r.hub.Broadcast(ctx, conn.RoomID, out, conn.UserID); err != nil {
		r.log.Warnw("watch_together broadcast seek event", "err", err)
	}
}

// ----------------------------------------------------------------------------
// Handler — playback:time_tick
//
// Drift-detection only; never broadcasts. The drift engine reads the
// canonical room state from Redis, decides if the reported_time is
// in/soft/hard/persistent drift band, and the router either sends a
// playback:correction (soft/hard) or an error:PERSISTENT_DRIFT (persistent)
// envelope to the sender ONLY.
// ----------------------------------------------------------------------------

func (r *InboundRouter) handleTimeTick(ctx context.Context, conn ConnectionCtx, data json.RawMessage) {
	var payload domain.PlaybackTimeTickData
	if err := json.Unmarshal(data, &payload); err != nil {
		r.sendBadPayload(ctx, conn, "time_tick", err)
		return
	}

	nowMs := r.now().UnixMilli()
	correction, err := r.drift.OnTimeTick(ctx, r.repo, conn.RoomID, conn.UserID, payload.Time, nowMs)
	if err != nil {
		// Includes repo.ErrNotFound when the room TTL'd out mid-session.
		// We don't close the connection from here — the WS handler will
		// notice the room is gone on the next state-mutating call.
		r.log.Debugw("watch_together drift skipped",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"err", err,
		)
		return
	}
	if correction == nil {
		return // in-sync; no-op
	}

	switch correction.Severity {
	case DriftPersistent:
		r.sendErrorToSelf(ctx, conn, domain.ErrCodePersistentDrift,
			"drift exceeds correction threshold for 5 consecutive ticks", "reload")
	case DriftSoft, DriftHard:
		out, err := buildEnvelope(domain.MsgPlaybackCorrection, domain.PlaybackCorrectionData{
			Time:     correction.Time,
			ServerTS: correction.ServerTS,
		})
		if err != nil {
			r.log.Errorw("watch_together marshal correction", "err", err)
			return
		}
		if _, err := r.hub.SendTo(ctx, conn.RoomID, conn.UserID, out); err != nil {
			r.log.Warnw("watch_together send correction",
				"room_id", conn.RoomID,
				"user_id", conn.UserID,
				"err", err,
			)
		}
	}
}

// ----------------------------------------------------------------------------
// Handler — state:change_episode / state:change_player / state:change_translation
//
// Phase 1 surface only — no catalog validation (deferred to Phase 4
// WT-STATE-02). Each change:
//   - Updates the corresponding HASH field
//   - Resets playback_time=0 and playback_state=paused (so all members start
//     the new content together)
//   - Broadcasts room:state_changed{field, value, by_user_id} to ALL members
//     (sender INCLUDED — they need the same source-of-truth update)
// ----------------------------------------------------------------------------

func (r *InboundRouter) handleStateChange(
	ctx context.Context,
	conn ConnectionCtx,
	data json.RawMessage,
	field string,
) {
	// All three Data shapes have a single string field — decode into a
	// flexible map[string]interface{} so we handle episode_id / player /
	// translation_id with one code path.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		r.sendBadPayload(ctx, conn, "state:change_"+field, err)
		return
	}
	val, ok := raw[field]
	if !ok {
		r.sendBadPayload(ctx, conn, "state:change_"+field, fmt.Errorf("missing field %q", field))
		return
	}
	valStr, ok := val.(string)
	if !ok || valStr == "" {
		r.sendBadPayload(ctx, conn, "state:change_"+field, fmt.Errorf("field %q must be non-empty string", field))
		return
	}

	now := r.now()
	nowMs := now.UnixMilli()

	if err := r.repo.UpdateRoomState(ctx, conn.RoomID, map[string]interface{}{
		field:                      valStr,
		"playback_time":            float64(0),
		"playback_state":           domain.StatePaused,
		"playback_time_updated_at": nowMs,
	}); err != nil {
		r.log.Errorw("watch_together update room (state change)",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"field", field,
			"err", err,
		)
		return
	}

	out, err := buildEnvelope(domain.MsgRoomStateChanged, domain.RoomStateChangedData{
		Field:    field,
		Value:    valStr,
		ByUserID: conn.UserID,
	})
	if err != nil {
		r.log.Errorw("watch_together marshal state_changed", "err", err)
		return
	}
	// Broadcast to ALL (sender included) — single source of truth.
	if _, err := r.hub.Broadcast(ctx, conn.RoomID, out, ""); err != nil {
		r.log.Warnw("watch_together broadcast state_changed",
			"room_id", conn.RoomID,
			"err", err,
		)
	}
}

// ----------------------------------------------------------------------------
// Handler — chat:message
//
// Order matters: char-cap check FIRST (drop oversized payloads before
// they touch the rate limiter, so a spammer can't exhaust the limiter
// with garbage), then rate-limit check, then persist + broadcast.
// Broadcast is to ALL (sender INCLUDED) — the sender's UI listens for
// their own echo as the persistence confirmation per WT-FOUND-10
// success criterion #4.
// ----------------------------------------------------------------------------

func (r *InboundRouter) handleChat(ctx context.Context, conn ConnectionCtx, data json.RawMessage) {
	var payload domain.ChatMessageInData
	if err := json.Unmarshal(data, &payload); err != nil {
		r.sendBadPayload(ctx, conn, "chat:message", err)
		return
	}

	if len(payload.Body) > chatBodyCharLimit {
		r.sendErrorToSelf(ctx, conn, domain.ErrCodeChatTooLong,
			fmt.Sprintf("chat body exceeds %d chars", chatBodyCharLimit), "")
		return
	}

	if !r.rl.AllowChat(conn.UserID) {
		RateLimitedTotal.WithLabelValues(domain.MsgChatMessage).Inc()
		r.sendErrorToSelf(ctx, conn, domain.ErrCodeRateLimited,
			"chat rate limit exceeded (5/sec)", "")
		return
	}

	msg := domain.ChatMessage{
		ID:       r.newID(),
		UserID:   conn.UserID,
		Username: conn.Username,
		Body:     payload.Body,
		TS:       r.now().UnixMilli(),
	}

	if err := r.repo.AppendMessage(ctx, conn.RoomID, msg); err != nil {
		r.log.Errorw("watch_together append chat",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"err", err,
		)
		// Don't send error to sender — they'll see no echo, which the
		// frontend can interpret as failure (matches WT-FOUND-10 #4 success
		// semantics: echo == persistence confirmation, no echo == failure).
		return
	}
	ChatMessagesTotal.Inc()

	out, err := buildEnvelope(domain.MsgChatMessageOut, domain.ChatMessageOutData{Message: msg})
	if err != nil {
		r.log.Errorw("watch_together marshal chat_message_out", "err", err)
		return
	}
	if _, err := r.hub.Broadcast(ctx, conn.RoomID, out, ""); err != nil {
		r.log.Warnw("watch_together broadcast chat",
			"room_id", conn.RoomID,
			"err", err,
		)
	}
}

// ----------------------------------------------------------------------------
// Handler — chat:reaction
//
// Ephemeral — NOT persisted in the Redis LIST. Whitelist-checked silently
// (out-of-whitelist emoji is dropped without error). Broadcast to ALL
// (sender included — gives a "yes my reaction landed" feedback signal).
// ----------------------------------------------------------------------------

func (r *InboundRouter) handleReaction(ctx context.Context, conn ConnectionCtx, data json.RawMessage) {
	var payload domain.ChatReactionInData
	if err := json.Unmarshal(data, &payload); err != nil {
		r.sendBadPayload(ctx, conn, "chat:reaction", err)
		return
	}
	if _, ok := reactionWhitelist[payload.Emoji]; !ok {
		// Silent drop — a misbehaving client palette shouldn't surface
		// as a hard error. Logged at debug for forensic visibility.
		r.log.Debugw("watch_together reaction not in whitelist",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"emoji", payload.Emoji,
		)
		return
	}

	out, err := buildEnvelope(domain.MsgChatReactionOut, domain.ChatReactionOutData{
		UserID: conn.UserID,
		Emoji:  payload.Emoji,
	})
	if err != nil {
		r.log.Errorw("watch_together marshal reaction", "err", err)
		return
	}
	if _, err := r.hub.Broadcast(ctx, conn.RoomID, out, ""); err != nil {
		r.log.Warnw("watch_together broadcast reaction",
			"room_id", conn.RoomID,
			"err", err,
		)
		return
	}
	ReactionsTotal.Inc()
}

// ----------------------------------------------------------------------------
// Handler — presence:heartbeat
//
// Idempotent — re-PUT the MemberMeta with refreshed last_seen_at. No
// broadcast (other members don't care about a heartbeat — they only care
// about absence, which the WS disconnect path handles).
//
// The handler doesn't have access to the original JoinedAt/AvatarURL the
// member used at upgrade time, so it preserves them via a read-modify-write
// pattern: ListMembers to find the existing MemberMeta, mutate last_seen_at,
// AddMember to re-insert. If the member isn't found in the HASH (TTL'd or
// removed concurrently), the heartbeat is a no-op.
// ----------------------------------------------------------------------------

func (r *InboundRouter) handleHeartbeat(ctx context.Context, conn ConnectionCtx, _ json.RawMessage) {
	members, err := r.repo.ListMembers(ctx, conn.RoomID)
	if err != nil {
		r.log.Debugw("watch_together heartbeat list members failed",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"err", err,
		)
		return
	}

	var existing *domain.MemberMeta
	for _, m := range members {
		if m.UserID == conn.UserID {
			meta := m.Meta
			existing = &meta
			break
		}
	}
	if existing == nil {
		// Member is no longer in the room HASH (could happen if a previous
		// disconnect already cleaned them up but the readPump hasn't
		// exited yet). Skip — the next inbound from this user will be a
		// no-op anyway because the connection is being torn down.
		return
	}

	existing.LastSeenAt = r.now().Unix()

	if err := r.repo.AddMember(ctx, conn.RoomID, conn.UserID, *existing); err != nil {
		r.log.Warnw("watch_together heartbeat add_member failed",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"err", err,
		)
	}
}

// ----------------------------------------------------------------------------
// Shared helpers
// ----------------------------------------------------------------------------

// buildEnvelope marshals body into Envelope.Data and returns the envelope.
// Centralizes the json.Marshal-then-wrap pattern used by every handler.
func buildEnvelope(msgType string, body interface{}) (domain.Envelope, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return domain.Envelope{}, err
	}
	return domain.Envelope{Type: msgType, Data: data}, nil
}

// sendErrorToSelf is the canonical sender-only error path. Used for
// RATE_LIMITED, CHAT_TOO_LONG, PERSISTENT_DRIFT, UNKNOWN_TYPE, BAD_PAYLOAD.
// Always hub.SendTo — never Broadcast — so other members don't see the
// rejection.
func (r *InboundRouter) sendErrorToSelf(
	ctx context.Context,
	conn ConnectionCtx,
	code, message, hint string,
) {
	out, err := buildEnvelope(domain.MsgError, domain.ErrorData{
		Code:    code,
		Message: message,
		Hint:    hint,
	})
	if err != nil {
		r.log.Errorw("watch_together marshal error envelope", "err", err)
		return
	}
	if _, err := r.hub.SendTo(ctx, conn.RoomID, conn.UserID, out); err != nil {
		r.log.Warnw("watch_together send error envelope",
			"room_id", conn.RoomID,
			"user_id", conn.UserID,
			"code", code,
			"err", err,
		)
	}
}

// sendBadPayload is the shorthand for malformed-JSON errors. Logs at warn
// (sustained BAD_PAYLOAD is a protocol bug worth investigating) and sends
// the BAD_PAYLOAD code to the sender only.
func (r *InboundRouter) sendBadPayload(ctx context.Context, conn ConnectionCtx, msgType string, decodeErr error) {
	r.log.Warnw("watch_together inbound bad payload",
		"room_id", conn.RoomID,
		"user_id", conn.UserID,
		"type", msgType,
		"err", decodeErr,
	)
	r.sendErrorToSelf(ctx, conn, errCodeBadPayload,
		fmt.Sprintf("malformed %s payload: %v", msgType, decodeErr), "")
}
