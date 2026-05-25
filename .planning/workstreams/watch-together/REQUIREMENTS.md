# Requirements: AnimeEnigma `watch-together` workstream — v1.0

**Milestone:** v1.0 Watch Together Foundation
**Defined:** 2026-05-25
**Core value:** 2–10 logged-in friends click an invite link, land in the same room, watch the same anime in lock-step across all 5 players (incl. Kodik via undocumented RPC), with text chat + emoji reactions, ephemeral state. Two friends in different time zones never have to coordinate "1, 2, 3, play" again.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-25-watch-together-design.md`

## v1.0 Requirements

### Backend — Watch Together Service Foundation (Phase 1)

- [ ] **WT-FOUND-01**: New Go microservice at `services/watch-together/` on port **8091** (next free after `notifications:8090`). Standard project layout: `cmd/watch-together-api/main.go`, `internal/{config,domain,handler,repo,service,hub,transport}/`. No `migrations/` directory — Redis-only, no Postgres. `GET /health` returns 200 `{"status":"ok"}`. Prometheus `/metrics` registered via `libs/metrics` with `service="watch-together"` label. Joins the existing multi-module workspace (`go.work` extended). Ships as `services/watch-together/Dockerfile` mirroring `services/themes/Dockerfile`'s shape. `services/watch-together/go.mod` includes `require + replace` directives for every `libs/*` module it depends on (`libs/logger`, `libs/metrics`, `libs/jwt`, `libs/redis` if it exists, otherwise direct `go-redis/redis/v9`).

- [ ] **WT-FOUND-02**: Redis-only state schema (no Postgres tables, no GORM, no AutoMigrate). All keys prefixed `wt:`. Schema implemented in `internal/repo/redis_repo.go`:
  - `wt:room:{roomId}` HASH — `id`, `created_at`, `anime_id`, `episode_id`, `player`, `translation_id`, `playback_state`, `playback_time`, `playback_time_updated_at`, `host_user_id`
  - `wt:room:{roomId}:members` HASH — `user_id` → `MemberMeta` JSON (`username`, `avatar_url`, `joined_at`, `last_seen_at`)
  - `wt:room:{roomId}:messages` LIST — capped at 100 via `LPUSH` + `LTRIM`; each entry is a `ChatMessage` JSON (`id`, `user_id`, `username`, `body`, `ts`)
  - `wt:room:{roomId}:events` PUBSUB — multi-instance fanout (subscriber wired in Phase 1 but trivially a no-op for single-instance v1.0)
  - TTL: all `wt:room:{roomId}*` keys set to 900 sec sliding (refreshed on any inbound event). Last-member-disconnect triggers a 5min grace timer; if no reconnect, keys are not refreshed and expire naturally.

- [ ] **WT-FOUND-03**: Public HTTP API (gateway-routed in WT-FOUND-08):
  - `POST /api/watch-together/rooms` — body `{anime_id, episode_id, player, translation_id}`; creates room, returns `{room_id, invite_url, ws_url}`. Caller becomes `host_user_id` (cosmetic only).
  - `GET /api/watch-together/rooms/{id}` — returns `RoomSnapshot` (current state + members + last 50 messages). Returns 410 Gone if room TTL has expired.
  - `DELETE /api/watch-together/rooms/{id}` — host-only force-close (sends `room:closed` to all members, deletes Redis keys). Returns 403 if not host, 204 on success. Cosmetic — rooms also auto-close when empty + grace expires.
  - All routes resolve authenticated user via `authz.UserIDFromContext(ctx)` (existing project convention).

- [ ] **WT-FOUND-04**: WebSocket endpoint at `/ws` accepting `?token=<jwt>&room=<roomId>` query params. Upgrade handler validates JWT against `JWT_SECRET`; rejects unauthenticated with 401. Validates room exists in Redis; rejects with 404 if not. Validates room is under capacity (10 members); rejects with `{"error":"CAPACITY_FULL"}` close frame if not. Adds connection to internal hub keyed by room, sends `room:snapshot` immediately on successful upgrade.

- [ ] **WT-FOUND-05**: WebSocket inbound message router (`internal/handler/ws_inbound.go`) handles all 10 inbound message types per design doc (`playback:play|pause|seek|time_tick`, `state:change_episode|change_player|change_translation`, `chat:message`, `chat:reaction`, `presence:heartbeat`). Each handler validates payload, updates Redis state where applicable, and broadcasts via the hub. Seek messages enforce a 1s per-user rate limit (in-process token bucket); rate-limited messages return `error: {code: 'RATE_LIMITED'}` to the sender only. Chat messages cap at 500 chars; over-cap is dropped with `error: {code: 'CHAT_TOO_LONG'}`.

- [ ] **WT-FOUND-06**: WebSocket outbound broadcaster (`internal/hub/hub.go`) maintains per-room connection sets, broadcasts events to all room members (excluding sender for `playback:event` echo via the `by_user_id` field), and supports per-recipient sends (used for `playback:correction` and `error`). Hub subscribes to Redis pubsub channel `wt:room:{id}:events` for forward-compatible multi-instance fanout (no-op for single-instance v1.0). Hub also publishes to that channel when broadcasting; the publisher-and-subscriber-in-one-process pattern lets v2 horizontal scale work without protocol change.

- [ ] **WT-FOUND-07**: Drift detection engine (`internal/service/sync.go`). On each member's inbound `playback:time_tick {time}`:
  - Compute `expected_time = room.playback_time + (now - playback_time_updated_at)/1000` if state is `"playing"`, else `room.playback_time`.
  - Compute `drift = abs(member.reported_time - expected_time)`.
  - If `1.5s < drift <= 5s`: send `playback:correction {time: expected_time, server_ts: now}` to that member only (soft).
  - If `drift > 5s`: same correction (hard — client decides whether to soft-nudge or hard-seek).
  - Tracks the last 5 corrections per member; if all 5 exceed 5s drift, sends `error: {code: 'PERSISTENT_DRIFT', hint: 'reload'}` and stops correcting (avoids spam).

- [ ] **WT-FOUND-08**: Gateway integration — `services/gateway/internal/config/config.go` gains `WatchTogetherURL string` field defaulting to `http://watch-together:8091`. `services/gateway/internal/router/routes.go` adds:
  - HTTP proxy `/api/watch-together/*` → `WatchTogetherURL/*` under existing `authMiddleware`
  - WebSocket proxy on `/api/watch-together/ws` with `Upgrade: websocket` handling (use the existing WS proxy pattern from the rooms service if present, otherwise add one with `httputil.NewSingleHostReverseProxy` configured for WS).
  - Internal `/internal/watch-together/*` is NOT proxied (no internal endpoints in v1.0; defensive default for forward compat).

- [ ] **WT-FOUND-09**: Docker / Compose wiring — `docker/docker-compose.yml` gets a new `watch-together` service block (image build context `./services/watch-together`, depends_on redis, port 8091 exposed internally only, env vars from `.env`). `docker/.env.example` documents new env vars: `WATCH_TOGETHER_PORT=8091`, `WATCH_TOGETHER_REDIS_ADDR=redis:6379`, `WATCH_TOGETHER_JWT_SECRET=${JWT_SECRET}`, `WATCH_TOGETHER_SERVICE_URL=http://watch-together:8091` (for gateway). `Makefile` gets `redeploy-watch-together`, `logs-watch-together`, `restart-watch-together` targets mirroring existing services. `CLAUDE.md` gets a new row in Service Ports table and Gateway Routing table.

- [ ] **WT-FOUND-10**: End-to-end smoke test in Phase 1 summary: `curl POST /api/watch-together/rooms` returns a `room_id`, `wscat -c "ws://gateway:8000/api/watch-together/ws?token=$JWT&room=$ROOM_ID"` connects and immediately receives a `room:snapshot` frame. Sending `{"type":"chat:message","data":{"body":"hello"}}` echoes back as `{"type":"chat:message","data":{"message":...}}` (the sender also receives it). Opening a second `wscat` against the same room receives a `member:joined` frame on the first connection.

### Frontend — Shell + Chat + Invite (Phase 2)

- [ ] **WT-SHELL-01**: New Vue 3 route `/watch/room/:roomId` mounted to `WatchTogetherView.vue`. Route uses the existing `requireAuth` guard (redirect to `/login?next=/watch/room/:roomId` if not logged in). On mount, fetches room snapshot via `GET /api/watch-together/rooms/{id}` — on 410 Gone, shows "This room has ended" with a button to return to the anime's watch page.

- [ ] **WT-SHELL-02**: New composable `frontend/web/src/composables/useWatchTogetherRoom.ts` exposing the reactive interface in the design doc (`room`, `members`, `messages`, `reactions`, `connectionStatus`, emit methods, subscribe methods). Handles WebSocket lifecycle: open on call, auto-reconnect with exponential backoff (1s/2s/4s/8s, capped at 30s), close on unmount, replay `room:snapshot` on every reconnect. Strips re-emission of events that originated locally (sender's `by_user_id` matches own user id → don't re-fire the local handler).

- [ ] **WT-SHELL-03**: New API client `frontend/web/src/api/watch-together.ts` with `createRoom(payload)`, `getRoom(id)`, `deleteRoom(id)`. Standard error handling and types matching the backend.

- [ ] **WT-SHELL-04**: New component `RoomSidebar.vue` containing:
  - `MemberList.vue` — avatar + username per member, "(host)" badge on `host_user_id`, "You" badge on own user
  - `ChatPanel.vue` — message list (auto-scroll to bottom on new), text input with 500-char limit, send-on-Enter
  - `ReactionPalette.vue` — ~24 anime-friendly emoji as clickable chips; click sends `chat:reaction` (whitelist defined inline; final list approved during Phase 2 planning)
  - `ReactionBurstOverlay.vue` — absolute-positioned overlay (pointer-events: none) on top of the player area; animates incoming reactions as floating emoji that fade after 3s

- [ ] **WT-SHELL-05**: New component `InviteButton.vue` mounted into `WatchView.vue` (player chrome area — final placement decided during Phase 2 UI mockup). Click flow: `createRoom({anime_id, episode_id, player, translation_id})` → router.push(`/watch/room/${room_id}`) → copy `invite_url` to clipboard → toast "Invite link copied — share it with friends".

- [ ] **WT-SHELL-06**: `WatchTogetherView.vue` mounts the appropriate `<*Player>` based on `room.player`, passing `:room="room"` prop (player adapter wiring lands in Phase 3 — for Phase 2 the prop is accepted but the players ignore it). Sidebar mounted to the right on desktop, as a tabbed bottom panel on mobile (`<lg` breakpoint).

- [ ] **WT-SHELL-07**: i18n strings — new `watch_together.*` namespace in both `frontend/web/src/locales/en.json` and `ru.json`. Keys for: title, subtitle, members heading, "(host)" badge, "You" badge, empty-chat placeholder, chat input placeholder, send button label, reaction palette title, "Invite link copied" toast, "Room ended" empty state, "Reconnecting…" indicator, "Capacity full" error, "Auth expired" error. Locale parity test (mirror the spotlight pattern from `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts`) ensures both files have identical keys.

- [ ] **WT-SHELL-08**: Two-browser smoke test in Phase 2 summary: open two browsers (different accounts), one creates a room, copies the link, the other navigates to it. Both see the same member list, exchange chat messages in both directions, send reactions that appear as bursts on both screens. Player area is visible but does not yet sync (sync ships in Phase 3).

### Frontend — Player Sync (Phase 3)

- [ ] **WT-SYNC-01**: HTML5 player adapter — generic logic in `frontend/web/src/composables/usePlayerSyncBridge.ts` that wires a `videoRef` to a `WatchTogetherRoom`. Listens to `@play`, `@pause`, `@seeked` on the video; emits room events. Subscribes `room.onPlaybackEvent` and applies to local video. Includes a re-emission guard: when applying a remote event, suppress the next local `@play|@pause|@seeked` (uses a "last applied" timestamp + small tolerance window).

- [ ] **WT-SYNC-02**: Wire `usePlayerSyncBridge` into `AnimeLibPlayer.vue`, `OurEnglishPlayer.vue`, `HanimePlayer.vue`, `RawPlayer.vue` via the new optional `room?: WatchTogetherRoom` prop. When prop is null/undefined, players behave exactly as today (zero regression). When set, sync is active.

- [ ] **WT-SYNC-03**: Kodik adapter in `KodikPlayer.vue` — extend the existing `handleKodikMessage` handler at `KodikPlayer.vue:278-326` to also consume `kodik_player_play`, `kodik_player_seek`, `kodik_player_video_started`, `kodik_player_video_ended` outbound events (currently only `kodik_player_time_update` and `kodik_player_pause` are consumed). Add an outbound `postCommand(method, payload)` helper that `iframe.contentWindow.postMessage({key: 'kodik_player_api', value: {method, ...payload}}, '*')`. When `room` prop is set: outbound events from Kodik → room emits; room events → `postCommand`.

- [ ] **WT-SYNC-04**: Kodik boot-time smoke probe — on `KodikPlayer.vue` mount in a room context, send `{key: 'kodik_player_api', value: {method: 'get_time'}}` and start a 2s timer. If a `kodik_player_time` outbound reply arrives within the timeout, mark `kodikSyncAvailable = true`. Otherwise, mark `false`, show a banner "Kodik sync unavailable for this bundle version — use voice chat to coordinate", and disable outbound sync from this client (inbound still consumed, since we still receive `kodik_player_time_update`/`kodik_player_pause`).

- [ ] **WT-SYNC-05**: `playback:time_tick` heartbeat — every player (when `room` prop set) emits `room.emitTimeTick(currentTime)` once per second of playback. Use `requestAnimationFrame`-throttled loop with a `lastTick = 0` gate, not `setInterval`, to avoid drift when the tab is throttled.

- [ ] **WT-SYNC-06**: `playback:correction` handler — `usePlayerSyncBridge` subscribes to `room.onPlaybackCorrection`; on receive, computes `target = correction.time + (now - correction.server_ts)/1000` (correct for network delay), then either soft-nudges (if `|currentTime - target| < 1s`) or hard-seeks (otherwise). Corrections never trigger any UI feedback (silent).

- [ ] **WT-SYNC-07**: Sender attribution UI — when an incoming `playback:event` is from another member (`by_user_id !== ownUserId`), show a subtle fadeable toast at the bottom of the player ("Alice paused", "Bob seeked to 12:34"). 1.5s display + 0.5s fade. Toasts stack vertically, max 3 visible.

- [ ] **WT-SYNC-08**: `connectionStatus` UI — when `connectionStatus === 'reconnecting'` or `'closed'`, overlay the player with a non-blocking indicator ("Reconnecting…" with a small spinner). Player keeps playing locally during reconnect; on successful reconnect, drift correction will pull it back into sync.

- [ ] **WT-SYNC-09**: Two-browser sync smoke test in Phase 3 summary: same setup as WT-SHELL-08, but now both browsers' players play/pause/seek in sync. Test against each of the 5 players individually (one anime per player) and verify drift correction kicks in when one browser is throttled (DevTools Performance tab → 4x slowdown).

- [ ] **WT-SYNC-10**: Kodik probe daily regression test — Playwright test (runs in CI nightly) that loads a Kodik iframe in a standalone test page, sends `{method:'get_time'}`, asserts a `kodik_player_time` reply arrives within 5s. If this ever fails in CI, the team knows Kodik changed their bundle and the inbound RPC may have shifted or been removed. Test failure file path: `frontend/web/e2e/kodik-rpc-probe.spec.ts`.

### Backend + Frontend — State Switching (Phase 4)

- [ ] **WT-STATE-01**: Backend handlers for `state:change_episode`, `state:change_player`, `state:change_translation` in `internal/handler/ws_inbound.go`. Each updates the corresponding field in `wt:room:{roomId}` HASH, resets `playback_time` to 0 and `playback_state` to `"paused"` (so all members start the new content together), and broadcasts `room:state_changed {field, value, by_user_id}` to all members.

- [ ] **WT-STATE-02**: Backend validation — `state:change_episode` validates that the episode exists for the room's current anime via a catalog HTTP call (`GET /internal/anime/{id}/episodes?player=...&translation=...`); if not found, returns `error: {code: 'EPISODE_UNAVAILABLE'}` to the sender only and does NOT mutate state. Similar for `state:change_translation` (must be valid for the room's current `player`) and `state:change_player` (must have at least one episode for the room's current anime).

- [ ] **WT-STATE-03**: Frontend handler — `WatchTogetherView.vue` subscribes to `room.onStateChanged`; on `field === 'player'`, swaps which player component is mounted; on `field === 'episode_id'` or `'translation_id'`, the existing player's source props change and the player's existing reactivity reloads the source. All 5 players already support reactive source changes (verified via current `WatchView.vue`); the only new work is ensuring the room context survives the player swap.

- [ ] **WT-STATE-04**: Frontend trigger UI — the existing player chrome's episode/player/translation switchers, when in a room context, emit `room.emitChangeEpisode|emitChangePlayer|emitChangeTranslation` instead of (or in addition to) the local switch. Implementation: the existing switchers already emit events; add an "in a room?" prop and re-route the emit. Local state change is suppressed; the change applies when `room:state_changed` arrives back (single source of truth).

- [ ] **WT-STATE-05**: Two-browser state-switch smoke test in Phase 4 summary: same setup, host clicks next-episode → both browsers' players switch and start the new episode paused at 0. Host changes player (Kodik → AniLib) → both browsers swap player. Host changes translation → both swap.

### Polish (Phase 5)

- [ ] **WT-POLISH-01**: Reaction burst animations — emoji float up from bottom-left of player, gentle wiggle on horizontal axis, fade over 2.5s, scale from 0.8→1.2→1.0. CSS-only animation (no JS spring lib), inline in `ReactionBurstOverlay.vue`. Max 8 simultaneous bursts; older bursts drop off when a 9th arrives.

- [ ] **WT-POLISH-02**: Reconnect grace period — Backend keeps room alive for 5 minutes after last member disconnect (sliding TTL no longer refreshed, but base TTL hasn't expired yet). If any member reconnects within 5min, room state is intact and they rejoin with full snapshot. After 5min, room is gone and `GET /api/watch-together/rooms/{id}` returns 410.

- [ ] **WT-POLISH-03**: Mobile bottom-sheet layout — `<lg` breakpoint switches sidebar to a bottom-anchored panel with a draggable handle. Two tabs: Chat | Reactions. Player stays at top, full width. Sheet expands to ~60% height when actively used (chat focused, palette open), collapses to ~10% (just member count badge + tab dots) otherwise.

- [ ] **WT-POLISH-04**: Capacity-full UX — Frontend handles `error: {code: 'CAPACITY_FULL'}` from WebSocket close by showing a friendly "This room is full (10/10)" page with a button to "Browse other anime". Does NOT auto-retry.

- [ ] **WT-POLISH-05**: Room-expired redirect — On 410 from `GET /api/watch-together/rooms/{id}`, redirect to `/anime/:animeId/watch` (decoded from URL state if possible, or to home if not) with toast "This Watch Together room has ended."

- [ ] **WT-POLISH-06**: Auth-expired handling — On `error: {code: 'AUTH_EXPIRED'}` WebSocket close, prompt re-login with `next=` preserving the room URL. After successful re-login, return to room and rejoin.

- [ ] **WT-POLISH-07**: i18n complete — all user-facing strings introduced in Phases 1-5 are keyed in `watch_together.*` namespace in both en.json and ru.json. Locale parity test green. Smoke-verify in browser per [feedback_smoke_verify_i18n.md] — load the room view in both languages and confirm no raw key strings render.

- [ ] **WT-POLISH-08**: Grafana panel — new dashboard panel "Watch Together" with: active rooms count (gauge), total members across all rooms (gauge), messages per minute (counter rate), drift corrections per minute (counter rate), Kodik probe success rate over last 24h (single-stat). Metrics emitted from the Go service via `libs/metrics`. Dashboard JSON committed to `infra/grafana/dashboards/` (or wherever existing dashboards live).

### Non-functional / cross-cutting (`WT-NF-*`)

- [ ] **WT-NF-01** (Phase 1): WebSocket connection upgrades validate JWT exactly the same way as HTTP requests — same `JWT_SECRET`, same claim parsing via `libs/jwt`. Rejected connections receive a 401 HTTP response (not a WebSocket close frame) so failures surface in the browser's network panel.
- [ ] **WT-NF-02** (Phase 1): Per-user rate limits — `playback:seek` capped at 1/sec/user (in-process token bucket); `chat:message` capped at 5/sec/user (same primitive). Over-limit messages return `error: {code: 'RATE_LIMITED'}` to sender only; not broadcast.
- [ ] **WT-NF-03** (Phase 1): Structured logs via `libs/logger` for every WebSocket open/close, every state mutation, every error. Log entries include `room_id`, `user_id`, `event_type` for grep-ability.
- [ ] **WT-NF-04** (Phase 2): Frontend bundle size impact — measure the WatchTogetherView + composable + components against the existing baseline; aim for <30KB gz added to the lazy-loaded route chunk. If exceeded, audit imports (likely culprit: a heavy emoji picker — use a tiny static palette instead).
- [ ] **WT-NF-05** (Phase 5): All new dependencies (frontend and backend) audited for license + maintenance. Default-stack only: `go-redis`, `gorilla/websocket`, existing Vue 3 / Pinia. No new heavyweight libs.
- [ ] **WT-NF-06** (Phase 5): Telemetry / observability — at minimum: counter `watch_together_rooms_created_total`, gauge `watch_together_active_rooms`, gauge `watch_together_active_members`, counter `watch_together_messages_total{type}`, counter `watch_together_drift_corrections_total`, counter `watch_together_kodik_probe_failures_total`.
- [ ] **WT-NF-07** (Phase 5): Documentation — update `CLAUDE.md` with: new service in Service Ports table, new gateway routing in Gateway Routing table, new section "Watch Together" explaining the architecture briefly and pointing at the design doc.
