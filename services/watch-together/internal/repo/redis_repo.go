package repo

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
)

// ErrNotFound is returned when GetRoom (or any read-after-write helper) cannot
// locate the requested room in Redis — either it never existed or its sliding
// TTL has expired. Handlers should `errors.Is(err, repo.ErrNotFound)` to map
// this onto HTTP 404 / 410 Gone semantics. Also exposed as the AppError
// CodeNotFound via apperrors.NotFound for the gateway error envelope.
var ErrNotFound = stderrors.New("watch-together: room not found")

// Field names allowed in UpdateRoomState. Mirrors the Redis HASH schema in
// .planning/.../01-CONTEXT.md §Redis State Schema. Unknown field → error.
var allowedRoomFields = map[string]struct{}{
	"id":                       {},
	"created_at":               {},
	"anime_id":                 {},
	"episode_id":               {},
	"player":                   {},
	"translation_id":           {},
	"playback_state":           {},
	"playback_time":            {},
	"playback_time_updated_at": {},
	"host_user_id":             {},
}

// Chat list cap (LTRIM 0 99 → keep at most 100 entries, newest at head).
const chatListMaxIndex = 99

// RoomRepo encapsulates every Redis op the watch-together service needs.
// Every state-mutating method refreshes the sliding TTL on all 3 persistent
// keys atomically via a single TxPipelined block — handlers cannot
// accidentally forget to bump the TTL. Read paths (GetRoom / ListMembers /
// GetMessages / CountMembers / Exists) intentionally do NOT refresh
// (sliding TTL is event-driven; see 01-CONTEXT.md).
type RoomRepo struct {
	client *redis.Client
	ttl    time.Duration
	log    *logger.Logger
}

// NewRoomRepo builds a repo over the supplied *redis.Client. `ttl` is
// the sliding TTL applied to wt:room:{id}, wt:room:{id}:members,
// wt:room:{id}:messages — read from cfg.RoomTTL (default 900s).
func NewRoomRepo(client *redis.Client, ttl time.Duration, log *logger.Logger) *RoomRepo {
	if log == nil {
		log = logger.Default()
	}
	return &RoomRepo{client: client, ttl: ttl, log: log}
}

// roomToFields encodes a Room into the snake_case Redis HASH fields. Floats
// use FormatFloat('f', -1, 64) for round-trippable precision; ints use
// FormatInt base-10. Keep this list in sync with allowedRoomFields above.
func roomToFields(r *domain.Room) map[string]interface{} {
	return map[string]interface{}{
		"id":                       r.ID,
		"created_at":               strconv.FormatInt(r.CreatedAt, 10),
		"anime_id":                 r.AnimeID,
		"episode_id":               r.EpisodeID,
		"player":                   r.Player,
		"translation_id":           r.TranslationID,
		"playback_state":           r.PlaybackState,
		"playback_time":            strconv.FormatFloat(r.PlaybackTime, 'f', -1, 64),
		"playback_time_updated_at": strconv.FormatInt(r.PlaybackTimeUpdatedAtMs, 10),
		"host_user_id":             r.HostUserID,
	}
}

// fieldsToRoom decodes the Redis HASH map[string]string back into a Room.
// Numeric parse failures are tolerated as zero values so a partial write
// from a future schema version cannot make the read crash — the schema is
// owned by this service so corruption is the only realistic way this fires.
func fieldsToRoom(m map[string]string) *domain.Room {
	createdAt, _ := strconv.ParseInt(m["created_at"], 10, 64)
	playbackTime, _ := strconv.ParseFloat(m["playback_time"], 64)
	updatedAt, _ := strconv.ParseInt(m["playback_time_updated_at"], 10, 64)
	return &domain.Room{
		ID:                      m["id"],
		CreatedAt:               createdAt,
		AnimeID:                 m["anime_id"],
		EpisodeID:               m["episode_id"],
		Player:                  m["player"],
		TranslationID:           m["translation_id"],
		PlaybackState:           m["playback_state"],
		PlaybackTime:            playbackTime,
		PlaybackTimeUpdatedAtMs: updatedAt,
		HostUserID:              m["host_user_id"],
	}
}

// expireAll pipelines EXPIRE on all 3 persistent keys for the given room. Used
// by every state-mutating method to keep the sliding TTL honest. EXPIRE on a
// non-existent key is a Redis no-op (returns 0), so calling this for a room
// that has not yet seen members/messages is safe.
func (r *RoomRepo) expireAll(pipe redis.Pipeliner, ctx context.Context, roomID string) {
	pipe.Expire(ctx, KeyRoom(roomID), r.ttl)
	pipe.Expire(ctx, KeyRoomMembers(roomID), r.ttl)
	pipe.Expire(ctx, KeyRoomMessages(roomID), r.ttl)
}

// CreateRoom HSETs every field of `room` into wt:room:{id} and refreshes the
// sliding TTL on all 3 persistent keys atomically. Overwrites without warning
// if the room already exists — handlers MUST check Exists() first if that
// matters (POST /rooms always allocates a fresh UUID so collisions are
// impossible in v1.0).
func (r *RoomRepo) CreateRoom(ctx context.Context, room *domain.Room) error {
	if room == nil || room.ID == "" {
		return apperrors.InvalidInput("room id is required")
	}

	fields := roomToFields(room)
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, KeyRoom(room.ID), fields)
		r.expireAll(pipe, ctx, room.ID)
		return nil
	})
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: create room failed")
	}

	r.log.Infow("watch_together repo op",
		"op", "create_room",
		"room_id", room.ID,
		"anime_id", room.AnimeID,
		"host_user_id", room.HostUserID,
	)
	return nil
}

// GetRoom returns the Room or ErrNotFound. Reads do not refresh the TTL
// (sliding TTL is driven by inbound events, not reads — see 01-CONTEXT.md).
func (r *RoomRepo) GetRoom(ctx context.Context, roomID string) (*domain.Room, error) {
	m, err := r.client.HGetAll(ctx, KeyRoom(roomID)).Result()
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: get room failed")
	}
	if len(m) == 0 {
		return nil, ErrNotFound
	}
	return fieldsToRoom(m), nil
}

// Exists returns true if the room HASH exists. Used by handlers to map
// 404 vs 410 Gone (post-TTL-expiry) — v1.0 collapses both into ErrNotFound,
// but the Exists hook is wired for the v1.2 named-room work.
func (r *RoomRepo) Exists(ctx context.Context, roomID string) (bool, error) {
	n, err := r.client.Exists(ctx, KeyRoom(roomID)).Result()
	if err != nil {
		return false, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: exists check failed")
	}
	return n > 0, nil
}

// UpdateRoomState partial-updates the wt:room:{id} HASH. Only fields in
// allowedRoomFields may be written — any other key returns InvalidInput and
// the call is rejected before touching Redis. Refreshes the sliding TTL on
// all 3 persistent keys atomically.
func (r *RoomRepo) UpdateRoomState(ctx context.Context, roomID string, fields map[string]interface{}) error {
	if roomID == "" {
		return apperrors.InvalidInput("room id is required")
	}
	if len(fields) == 0 {
		return apperrors.InvalidInput("at least one field is required")
	}

	encoded := make(map[string]interface{}, len(fields))
	for name, raw := range fields {
		if _, ok := allowedRoomFields[name]; !ok {
			return apperrors.InvalidInput(fmt.Sprintf("invalid field name: %q", name))
		}
		// Encode the value to match Redis HASH storage semantics — go-redis will
		// otherwise fmt.Sprintf("%v", v) which is fine for strings/ints but
		// surprises on floats (no precision control).
		switch v := raw.(type) {
		case float64:
			encoded[name] = strconv.FormatFloat(v, 'f', -1, 64)
		case float32:
			encoded[name] = strconv.FormatFloat(float64(v), 'f', -1, 64)
		case int64:
			encoded[name] = strconv.FormatInt(v, 10)
		case int:
			encoded[name] = strconv.Itoa(v)
		default:
			encoded[name] = v
		}
	}

	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, KeyRoom(roomID), encoded)
		r.expireAll(pipe, ctx, roomID)
		return nil
	})
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: update room failed")
	}

	r.log.Infow("watch_together repo op",
		"op", "update_room_state",
		"room_id", roomID,
		"field_count", len(encoded),
	)
	return nil
}

// DeleteRoom removes all 3 persistent keys (HASH + members + messages). The
// pubsub channel is not a key — it self-cleans when the last subscriber drops.
func (r *RoomRepo) DeleteRoom(ctx context.Context, roomID string) error {
	keys := []string{
		KeyRoom(roomID),
		KeyRoomMembers(roomID),
		KeyRoomMessages(roomID),
	}
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: delete room failed")
	}
	r.log.Infow("watch_together repo op", "op", "delete_room", "room_id", roomID)
	return nil
}

// AddMember writes meta as JSON into wt:room:{id}:members under user_id.
// Refreshes the sliding TTL on all 3 keys. Idempotent — calling twice
// with the same user_id overwrites the meta (used for presence:heartbeat
// last_seen_at bumps in 01.5).
func (r *RoomRepo) AddMember(ctx context.Context, roomID, userID string, meta domain.MemberMeta) error {
	if roomID == "" || userID == "" {
		return apperrors.InvalidInput("room_id and user_id are required")
	}

	payload, err := json.Marshal(meta)
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: marshal member meta failed")
	}

	_, err = r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, KeyRoomMembers(roomID), userID, string(payload))
		r.expireAll(pipe, ctx, roomID)
		return nil
	})
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: add member failed")
	}

	r.log.Infow("watch_together repo op", "op", "add_member", "room_id", roomID, "user_id", userID)
	return nil
}

// RemoveMember HDELs the user_id entry from the members HASH and refreshes the
// TTL on all 3 keys (a leave is still an "inbound event" per 01-CONTEXT.md;
// the room only expires once the last member triggers the 5min grace path).
func (r *RoomRepo) RemoveMember(ctx context.Context, roomID, userID string) error {
	if roomID == "" || userID == "" {
		return apperrors.InvalidInput("room_id and user_id are required")
	}

	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HDel(ctx, KeyRoomMembers(roomID), userID)
		r.expireAll(pipe, ctx, roomID)
		return nil
	})
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: remove member failed")
	}

	r.log.Infow("watch_together repo op", "op", "remove_member", "room_id", roomID, "user_id", userID)
	return nil
}

// ListMembers returns every member of the room as a []domain.Member. Order is
// not guaranteed (Redis HGETALL has no ordering); callers that need stable
// ordering should sort by Meta.JoinedAt.
func (r *RoomRepo) ListMembers(ctx context.Context, roomID string) ([]domain.Member, error) {
	m, err := r.client.HGetAll(ctx, KeyRoomMembers(roomID)).Result()
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: list members failed")
	}

	members := make([]domain.Member, 0, len(m))
	for userID, raw := range m {
		var meta domain.MemberMeta
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			// Skip the corrupt entry but log it — this should never happen in
			// practice because AddMember is the only writer.
			r.log.Warnw("watch_together repo decode",
				"op", "list_members",
				"room_id", roomID,
				"user_id", userID,
				"err", err,
			)
			continue
		}
		members = append(members, domain.Member{UserID: userID, Meta: meta})
	}
	return members, nil
}

// CountMembers returns HLEN on the members HASH. Cheap O(1) — used by the WS
// upgrade handler to enforce the per-room capacity limit (WT-NF-02 → 10).
func (r *RoomRepo) CountMembers(ctx context.Context, roomID string) (int, error) {
	n, err := r.client.HLen(ctx, KeyRoomMembers(roomID)).Result()
	if err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: count members failed")
	}
	return int(n), nil
}

// MessageCount returns LLEN on the chat messages LIST. Cheap O(1). Used by
// Plan 05.2 RoomService.Delete + GraceManager.fire to observe the final
// chat activity level into wt_chat_messages_per_room before deleting keys.
// Empty room → 0, not an error.
func (r *RoomRepo) MessageCount(ctx context.Context, roomID string) (int, error) {
	n, err := r.client.LLen(ctx, KeyRoomMessages(roomID)).Result()
	if err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: count messages failed")
	}
	return int(n), nil
}

// AppendMessage LPUSHes the JSON-encoded message onto wt:room:{id}:messages
// and immediately LTRIMs the LIST to [0, 99] (cap 100, newest at head).
// Refreshes the sliding TTL on all 3 keys atomically.
func (r *RoomRepo) AppendMessage(ctx context.Context, roomID string, msg domain.ChatMessage) error {
	if roomID == "" {
		return apperrors.InvalidInput("room_id is required")
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: marshal chat message failed")
	}

	_, err = r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, KeyRoomMessages(roomID), payload)
		pipe.LTrim(ctx, KeyRoomMessages(roomID), 0, chatListMaxIndex)
		r.expireAll(pipe, ctx, roomID)
		return nil
	})
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: append message failed")
	}

	return nil
}

// GetMessages returns up to `limit` most-recent messages in CHRONOLOGICAL
// (oldest-first) order. Redis stores newest-at-head (LPUSH) so we LRANGE
// 0..limit-1 then reverse the slice in-place.
//
// Pass limit <= 0 to fetch nothing; values above the list cap (100) are
// silently capped by LTRIM so the LRANGE is bounded.
func (r *RoomRepo) GetMessages(ctx context.Context, roomID string, limit int) ([]domain.ChatMessage, error) {
	if limit <= 0 {
		return nil, nil
	}

	raw, err := r.client.LRange(ctx, KeyRoomMessages(roomID), 0, int64(limit-1)).Result()
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: get messages failed")
	}

	msgs := make([]domain.ChatMessage, 0, len(raw))
	for _, entry := range raw {
		var m domain.ChatMessage
		if err := json.Unmarshal([]byte(entry), &m); err != nil {
			r.log.Warnw("watch_together repo decode",
				"op", "get_messages",
				"room_id", roomID,
				"err", err,
			)
			continue
		}
		msgs = append(msgs, m)
	}

	// LRANGE returned newest-first; reverse in-place for chronological oldest-first.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// RefreshTTL bumps the sliding TTL on all 3 persistent keys to the configured
// value via a single TxPipelined block. Called by presence:heartbeat in 01.5
// to keep idle-but-still-watching rooms alive past 900s.
func (r *RoomRepo) RefreshTTL(ctx context.Context, roomID string) error {
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		r.expireAll(pipe, ctx, roomID)
		return nil
	})
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: refresh ttl failed")
	}
	return nil
}

// Publish writes the raw payload onto the wt:room:{id}:events PUBSUB channel.
// Currently the only subscriber is in-process (the hub in 01.3) but the
// indirection lets v2 horizontal scale work without changing the protocol.
// PUBLISH is a no-op when zero subscribers are listening (Redis returns 0).
func (r *RoomRepo) Publish(ctx context.Context, roomID string, payload []byte) error {
	if err := r.client.Publish(ctx, KeyRoomEvents(roomID), payload).Err(); err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "watch-together: publish failed")
	}
	return nil
}

// Subscribe opens a Redis SUBSCRIBE on wt:room:{id}:events. Caller is
// responsible for invoking sub.Close() when done — the hub in 01.3 owns the
// lifecycle (one subscription per room while connected members > 0).
func (r *RoomRepo) Subscribe(ctx context.Context, roomID string) *redis.PubSub {
	return r.client.Subscribe(ctx, KeyRoomEvents(roomID))
}
