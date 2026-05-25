# Phase 1: Backend Foundation - Context

**Gathered:** 2026-05-25
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)
**Workstream:** watch-together (v1.0)
**Source design doc:** `docs/superpowers/specs/2026-05-25-watch-together-design.md`

<domain>
## Phase Boundary

Stand up `services/watch-together/` on port **8091** with Redis-only state. Provides:

- REST endpoints for room lifecycle (`POST /rooms`, `GET /rooms/{id}`, `DELETE /rooms/{id}`)
- WebSocket endpoint at `/ws?token=<jwt>&room=<roomId>` for sync/chat/reactions/state-changes
- Drift detection engine (1.5s soft / 5s hard correction; PERSISTENT_DRIFT after 5 sustained)
- Gateway routing (HTTP + WebSocket proxy under existing authMiddleware)
- docker-compose entry, Makefile targets (`redeploy-watch-together`, `logs-`, `restart-`)
- CLAUDE.md updates (Service Ports row + Gateway Routing row)

End-state: `curl POST /api/watch-together/rooms` returns a `{room_id, invite_url, ws_url}`; two `wscat` connections to `/ws?token=$JWT&room=$ROOM_ID` see each other's joins, exchange chat messages with broadcast echo, and receive a `room:snapshot` on connect.

**Requirements covered:** WT-FOUND-01 through WT-FOUND-10, WT-NF-01, WT-NF-02, WT-NF-03.

</domain>

<decisions>
## Implementation Decisions (Locked — from REQUIREMENTS.md + design doc)

### Service Layout
- New Go microservice at `services/watch-together/` mirroring `services/themes/` shape
- Port **8091** (next free after `notifications:8090`)
- Standard layout: `cmd/watch-together-api/main.go`, `internal/{config,domain,handler,repo,service,hub,transport}/`
- NO `migrations/` directory — Redis-only, no Postgres, no GORM, no AutoMigrate
- `services/watch-together/go.mod` joined to root `go.work`, with `require + replace` for every `libs/*` used (`libs/logger`, `libs/metrics`, `libs/jwt`, `libs/redis` if it exists, else direct `go-redis/redis/v9`)

### Redis State Schema (all keys prefixed `wt:`)
- `wt:room:{roomId}` HASH — `id`, `created_at`, `anime_id`, `episode_id`, `player`, `translation_id`, `playback_state`, `playback_time`, `playback_time_updated_at`, `host_user_id`
- `wt:room:{roomId}:members` HASH — `user_id` → `MemberMeta` JSON (`username`, `avatar_url`, `joined_at`, `last_seen_at`)
- `wt:room:{roomId}:messages` LIST — capped at 100 via `LPUSH` + `LTRIM`; each entry `ChatMessage` JSON (`id`, `user_id`, `username`, `body`, `ts`)
- `wt:room:{roomId}:events` PUBSUB — multi-instance fanout (subscriber wired in Phase 1 but no-op for single-instance v1.0)
- TTL: 900s sliding (refreshed on any inbound event). Last-member-disconnect → 5min grace timer (no refresh during grace; keys expire naturally).

### REST API (gateway-proxied under existing `authMiddleware`)
- `POST /api/watch-together/rooms` — body `{anime_id, episode_id, player, translation_id}`; returns `{room_id, invite_url, ws_url}`. Caller becomes `host_user_id` (cosmetic).
- `GET /api/watch-together/rooms/{id}` — returns `RoomSnapshot` (state + members + last 50 messages). 410 Gone if TTL expired.
- `DELETE /api/watch-together/rooms/{id}` — host-only force-close. 403 if not host, 204 on success. Broadcasts `room:closed` to all members, deletes Redis keys.
- All routes resolve auth via `authz.UserIDFromContext(ctx)` (existing project convention).

### WebSocket Endpoint
- `/ws` accepting `?token=<jwt>&room=<roomId>` query params
- Upgrade validates JWT against `JWT_SECRET`; rejects unauthenticated with HTTP 401 (not close frame)
- Validates room exists in Redis; rejects with `error: {code: 'ROOM_NOT_FOUND'}` close frame if not
- Validates room under capacity (10 members); rejects with `{"error":"CAPACITY_FULL"}` close frame if not
- Adds connection to internal hub keyed by room; immediately sends `room:snapshot`

### Inbound Message Router (10 types per design doc)
- `playback:play|pause|seek|time_tick`
- `state:change_episode|change_player|change_translation` (handled in Phase 1 surface but full validation in Phase 4)
- `chat:message` (cap 500 chars; over-cap → `error: {code: 'CHAT_TOO_LONG'}` sender-only)
- `chat:reaction`
- `presence:heartbeat`
- Seek messages: per-user 1s in-process token bucket; rate-limited → `error: {code: 'RATE_LIMITED'}` sender-only

### Outbound Broadcaster (`internal/hub/hub.go`)
- Per-room connection sets
- Broadcasts to all room members, excluding sender for `playback:event` echo (sender attribution via `by_user_id` field)
- Per-recipient sends (`playback:correction`, `error`)
- Subscribes to `wt:room:{id}:events` Redis pubsub for forward-compat multi-instance fanout (no-op single-instance v1.0)
- Publishes to that channel when broadcasting (publisher+subscriber-in-one-process pattern lets v2 horizontal scale work without protocol change)

### Drift Detection (`internal/service/sync.go`)
On each `playback:time_tick {time}`:
- Compute `expected_time = room.playback_time + (now - playback_time_updated_at)/1000` if state=`playing`, else `room.playback_time`
- `drift = abs(member.reported_time - expected_time)`
- `1.5s < drift <= 5s` → send `playback:correction {time, server_ts}` to member only (soft)
- `drift > 5s` → same correction (hard — client decides nudge vs hard-seek)
- Track last 5 corrections per member; if all 5 exceed 5s → send `error: {code: 'PERSISTENT_DRIFT', hint: 'reload'}` and stop correcting (anti-spam)

### Gateway Integration
- `services/gateway/internal/config/config.go` gains `WatchTogetherURL string` defaulting to `http://watch-together:8091`
- `services/gateway/internal/router/routes.go`:
  - HTTP proxy `/api/watch-together/*` → `WatchTogetherURL/*` under existing `authMiddleware`
  - WebSocket proxy on `/api/watch-together/ws` with `Upgrade: websocket` handling — reuse the existing WS proxy pattern from rooms service if present, otherwise add one with `httputil.NewSingleHostReverseProxy` configured for WS
  - Internal `/internal/watch-together/*` NOT proxied (defensive default for forward compat; no internal endpoints in v1.0)

### Docker / Compose / Makefile / CLAUDE.md
- `docker/docker-compose.yml`: new `watch-together` service block (build context `./services/watch-together`, depends_on redis, port 8091 internal-only, env vars from `.env`)
- `docker/.env.example` documents: `WATCH_TOGETHER_PORT=8091`, `WATCH_TOGETHER_REDIS_ADDR=redis:6379`, `WATCH_TOGETHER_JWT_SECRET=${JWT_SECRET}`, `WATCH_TOGETHER_SERVICE_URL=http://watch-together:8091` (for gateway)
- `Makefile`: `redeploy-watch-together`, `logs-watch-together`, `restart-watch-together` mirroring existing services
- `CLAUDE.md`: new row in Service Ports table AND Gateway Routing table

### Authentication & Authorization
- WT-NF-01: gateway-injected JWT only — no plaintext auth in WS protocol
- Anonymous users CANNOT create rooms or connect WS — `requireAuth` middleware applies
- Room ownership: cosmetic only via `host_user_id` (any member can drive playback)

### Capacity / Limits
- 10 members per room (WT-NF-02). Configurable via `WATCH_TOGETHER_MAX_MEMBERS` env, default 10
- Chat: 500 char message cap
- Seek rate limit: 1 per second per user
- Chat retention: 100 messages per room (LIST capped via LTRIM)

### Metrics (WT-NF-06 — full set lands in Phase 5; Phase 1 ships baseline)
- `/metrics` endpoint registered via `libs/metrics` with `service="watch-together"` label
- Standard HTTP histograms (`http_requests_total`, `http_request_duration_seconds`)
- Phase 1 adds: `wt_room_create_total`, `wt_ws_connections_active` (gauge), `wt_ws_messages_received_total{type}`, `wt_drift_corrections_total{severity}`

### Claude's Discretion
- File-level organization within `internal/` (handler grouping, service splits, etc.) — use Go service conventions
- Specific Go library choices for WebSocket (`gorilla/websocket` is standard in repo; mirror `services/rooms/` if it has WS, otherwise pick gorilla)
- Redis client choice: prefer `libs/redis` if it exists in the repo; else `go-redis/redis/v9`
- Message-ID generation (UUIDs vs ULIDs vs nanoid) — match project convention
- Test layout: mirror `services/themes/` pattern; testcontainers for Redis if used elsewhere

</decisions>

<canonical_refs>
## Canonical References

Downstream agents MUST read these:

### Source design + requirements
- `docs/superpowers/specs/2026-05-25-watch-together-design.md` — Full design doc with message protocol, sequence diagrams, error semantics
- `.planning/workstreams/watch-together/REQUIREMENTS.md` — All WT-FOUND-*, WT-NF-* IDs
- `.planning/workstreams/watch-together/ROADMAP.md` — Phase boundaries

### Service-shape exemplars (closest analogs)
- `services/themes/` — Closest small Go service; same Dockerfile shape, same internal layout, mirrors what watch-together needs
- `services/notifications/` — Recent additive service with `JWT_SECRET`, Redis, gateway integration; very close pattern match for Phase 1
- `services/rooms/` — Has WebSocket handling for the existing game-rooms feature; reuse its WS upgrade pattern if useful

### Libs
- `libs/logger/` — Structured logging
- `libs/metrics/` — Prometheus `/metrics` registration
- `libs/jwt/` — JWT validation utilities
- `libs/redis/` (if exists) — Redis client wrapper

### Gateway integration anchors
- `services/gateway/internal/config/config.go` — Add `WatchTogetherURL` field
- `services/gateway/internal/router/routes.go` — Add HTTP + WS proxy routes
- `services/gateway/internal/middleware/auth.go` — Existing `authMiddleware`

### Infra
- `docker/docker-compose.yml` — Add service block
- `docker/.env.example` — Document env vars
- `Makefile` — Add 3 targets
- `CLAUDE.md` — Update 2 tables

</canonical_refs>

<specifics>
## Specific Ideas

### Smoke test (Success Criterion #4)
End-to-end smoke in the phase summary:
```bash
# Create room via gateway with API key auth
ROOM=$(curl -s -X POST http://localhost:8000/api/watch-together/rooms \
  -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"anime_id":"<uuid>","episode_id":"1","player":"animelib","translation_id":"<id>"}')

ROOM_ID=$(echo "$ROOM" | jq -r .room_id)

# Two wscat sessions in the same room
wscat -c "ws://localhost:8000/api/watch-together/ws?token=$JWT&room=$ROOM_ID" &
wscat -c "ws://localhost:8000/api/watch-together/ws?token=$JWT&room=$ROOM_ID" &

# Both should receive `member:joined` for the other; sending chat from one
# should echo back to both as `chat:message`
```

### Drift correction unit tests
Test the drift logic in isolation (no Redis, no WS) — pure function:
```go
TestComputeDrift(
  state=playing, room_time=100, updated_at=now-2s, reported_time=99.5
)
// drift=0.5+2 = 2.5s → soft correction
```

### WebSocket protocol versioning
- Server sends `room:snapshot` with a `protocol_version: "1.0"` field
- Clients can reject incompatible versions (forward-compat hook)

</specifics>

<deferred>
## Deferred Ideas

- Multi-instance horizontal scale (pubsub fanout is wired but no-op in v1.0)
- Postgres persistence (out of scope — v1.2 introduces persistent named rooms)
- Voice piggyback (v1.3 conditional)
- Per-user player (v1.1)
- Reaction burst animation (Phase 5)
- Mobile bottom-sheet layout (Phase 5)
- i18n strings beyond skeleton (Phase 5 finishes)
- Grafana dashboard panel (Phase 5)

</deferred>

---

*Phase: 01-backend-foundation*
*Context auto-generated: 2026-05-25 via workflow.skip_discuss*
