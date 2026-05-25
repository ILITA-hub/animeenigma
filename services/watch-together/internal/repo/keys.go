// Package repo holds the Redis persistence layer for the watch-together
// service. All `wt:`-prefixed Redis keys MUST be constructed through the
// helpers in this file — no string concatenation in handlers, services,
// or the hub. Centralizing keys here keeps the schema (see
// .planning/workstreams/watch-together/phases/01-backend-foundation/01-CONTEXT.md
// §Redis State Schema) in one auditable place.
package repo

import "fmt"

// KeyPrefix is the common prefix for every Redis key this service writes.
// Exposed for log scraping and admin tooling; production code should call
// the named builders below instead of concatenating against KeyPrefix.
const KeyPrefix = "wt:"

// KeyRoom returns the Redis key for the room HASH containing canonical
// playback state. Schema: see Room struct in internal/domain/room.go.
func KeyRoom(roomID string) string { return fmt.Sprintf("wt:room:%s", roomID) }

// KeyRoomMembers returns the Redis key for the per-room members HASH
// (user_id → MemberMeta JSON).
func KeyRoomMembers(roomID string) string { return fmt.Sprintf("wt:room:%s:members", roomID) }

// KeyRoomMessages returns the Redis key for the per-room chat LIST,
// capped at 100 entries via LPUSH + LTRIM 0 99 (newest at head).
func KeyRoomMessages(roomID string) string { return fmt.Sprintf("wt:room:%s:messages", roomID) }

// KeyRoomEvents returns the Redis PUBSUB channel used for forward-compat
// multi-instance fanout. Wired in Phase 1 but no-op for single-instance v1.0.
func KeyRoomEvents(roomID string) string { return fmt.Sprintf("wt:room:%s:events", roomID) }
