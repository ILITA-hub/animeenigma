# Watch Together — Design

**Date:** 2026-05-25
**Status:** Approved for planning
**Workstream:** `watch-together` (`.planning/workstreams/watch-together/`)
**Author:** Claude (with 0neymik0)

## Goal

Build **Watch Together**, a synchronized co-watch experience for AnimeEnigma. Two to ten logged-in friends share an invite link, land in the same room, and watch the same anime in lock-step — every play, pause, seek, episode switch, player switch, and translation switch is mirrored across all members in real time. Text chat and emoji reactions run alongside the player. Rooms are ephemeral (die when empty) and require no schema changes outside the new service.

The single moment that defines the feature: **two friends, one in Moscow and one in Tokyo, click a link, watch the same Frieren episode together with sub-second sync, chat about it, and never have to coordinate "1, 2, 3, play" over voice again**.

## Non-goals

| Excluded | Reason |
|---|---|
| Public room directory / lobby browsing | Scope is private friend rooms only. Discoverable rooms multiply moderation surface (reports, NSFW gating, kick/ban, capacity caps for large rooms). Punt to v2. |
| Persistent named rooms ("Saturday Anime Night") | Ephemeral keeps state in Redis only; no Postgres tables, no chat retention policy, no rejoin-tomorrow UX. Punt to v2. |
| Voice or video chat | Discord / Telegram already do this well. Building voice infra (WebRTC SFU, TURN servers, codecs) is a separate-product-size effort. |
| Per-user player choice ("RU friend on Kodik, EN friend on OurEnglish, both synced to the same scene") | Genuinely useful for mixed-language friend groups and AnimeEnigma is uniquely positioned to do it. Defer to v2 — the v1 protocol commits to "everything shared". |
| Guest access without an account | All members must be logged in. WebSocket auth uses the existing gateway JWT. |
| In-platform invite notifications | Shareable link only. Friends already have a chat channel (Discord, SMS, anywhere). |
| Hanime-specific room-level age gating | Treats all 5 players uniformly. Relies on the user-level 18+ acknowledgement that already gates Hanime visibility. |
| Host promotion / kick / mute | "Anyone can control" model — control is democratic, no host privileges in v1. |
| Watch-history attribution per member | Watch history continues to be written per-user as if they were watching solo. Co-watch attribution would require a `watched_in_room_id` column and a separate UX. Punt. |
| Cross-browser-instance fanout (horizontal scaling) | v1 runs one instance of the service. Redis pubsub channel scheme is set up so v2 can scale horizontally without protocol changes. |

## Constraints / Decisions

| Decision | Choice | Rationale |
|---|---|---|
| **Audience** | Private friend rooms only (invite link) | Smallest feasible scope; no public discovery; matches the existing notifications + spotlight DNA of "self-hosted small-group platform". |
| **Sync model** | Anyone can control (last-write-wins) | Friend groups trust each other; no host concept simplifies protocol and UI. Implicit anti-grief: emit a 1s rate-limit per user on seek events (cheap, hidden). |
| **Chat** | Text chat + emoji reactions | Reactions for low-friction "I felt that"; text for actual conversation. Both fit naturally in the WebSocket protocol with no extra infra. |
| **Guest access** | Login required | One auth path (gateway JWT) covers HTTP + WebSocket. No guest-token endpoint, no temporary identity, no abuse surface. |
| **Shared state** | Everything (anime + episode + player + translation + time + play/pause) | One TV, multiple chairs. Smallest protocol, smallest mental model. |
| **Lifetime** | Ephemeral — dies when empty (+ 5min grace) | No Postgres tables, no chat retention, no migrations. Pure Redis. |
| **Hanime gating** | None | User-level 18+ flag is sufficient. Don't introduce a room-level age flag for v1. |
| **Invite mechanism** | Shareable link only | Friends already have a chat channel; we don't need to be one. |
| **Player support** | All 5 (Kodik, AniLib, OurEnglish, Hanime, Raw) | Kodik is fully syncable via the discovered `kodik_player_api` inbound postMessage RPC — undocumented (Kodik's developer docs are closed as of 2026-05) but verified in player bundle disassembly on 2026-05-25 (full method table in "Sync engine — Kodik adapter" below). |
| **Service placement** | New `services/watch-together/` (port 8091) | Greenfield boundary. The existing `services/rooms/` service is shaped for the anime-guess game; coupling watch-together to it would tangle two unrelated domains. Pays a small "another service" tax for forever-clean separation. |
| **Storage** | Redis only — no Postgres | Ephemeral state, ≤10 members per room, last-100-messages chat buffer. Fits trivially. No GORM, no migrations. |
| **Capacity** | 2–10 members per room | Friend-group scale. Hard cap enforced at the service. Anything bigger should be a v2 public-rooms decision. |
| **Drift tolerance** | ±1.5s before correction; soft seek if reachable, hard seek if >5s drift | Hides network jitter for normal play; doesn't let followers drift into next-scene territory. See "Sync engine" below. |
| **Reconnect grace** | 5 minutes after last member disconnect | Handles tab refresh, brief network blip, and one-friend-stepping-away cases. After 5 min of zero members, room and chat history are deleted. |

## Architecture Overview

```
┌─────────────────────────────────┐
│  Browser                        │
│  ┌───────────────────────────┐  │
│  │  WatchTogetherView.vue    │  │
│  │  ├─ <PlayerAdapter>       │  │  HTTPS
│  │  │   ├─ KodikPlayer       │  │  ───────┐
│  │  │   ├─ AnimeLibPlayer    │  │         │
│  │  │   ├─ OurEnglishPlayer  │  │  WSS    │
│  │  │   ├─ HanimePlayer      │  │  ───┐   │
│  │  │   └─ RawPlayer         │  │     │   │
│  │  └─ <RoomSidebar>         │  │     │   │
│  │      ├─ MemberList        │  │     │   │
│  │      ├─ ChatPanel         │  │     │   │
│  │      └─ ReactionBurst     │  │     │   │
│  │                           │  │     │   │
│  │  composables/             │  │     │   │
│  │   useWatchTogetherRoom()  │  │     │   │
│  └───────────────────────────┘  │     │   │
└─────────────────────────────────┘     │   │
                                        │   ▼
                       ┌────────────────┴───────────────────────┐
                       │  gateway (:8000)                       │
                       │  /api/watch-together/*  ───────────────┤
                       │  ws://…/api/watch-together/ws   ───────┤
                       │  (existing JWT middleware)             │
                       └───────────────┬────────────────────────┘
                                       │
                                       ▼
              ┌──────────────────────────────────────────┐
              │  watch-together (NEW, :8091)             │
              │  Go service                              │
              │  ├─ HTTP: POST/GET/DELETE /rooms         │
              │  ├─ WS hub: per-room broadcast           │
              │  ├─ Sync engine: drift correction        │
              │  ├─ State: Redis only                    │
              │  └─ JWT validation on upgrade            │
              └────────────────┬─────────────────────────┘
                               │
                               ▼
                      ┌──────────────────┐
                      │  Redis           │
                      │  • room state    │
                      │  • messages list │
                      │  • members hash  │
                      │  • pubsub        │
                      └──────────────────┘
```

## Data Model — Redis schema

All keys prefixed `wt:` for namespace isolation. No Postgres tables.

```
wt:room:{roomId}                HASH    canonical state
  ├─ id                           string
  ├─ created_at                   unix ts
  ├─ anime_id                     string (UUID)
  ├─ episode_id                   string
  ├─ player                       "kodik" | "animelib" | "ourenglish" | "hanime" | "raw"
  ├─ translation_id               string (provider-scoped)
  ├─ playback_state               "playing" | "paused"
  ├─ playback_time                float (sec)
  ├─ playback_time_updated_at     unix ms
  └─ host_user_id                 string (cosmetic only)

wt:room:{roomId}:members        HASH    user_id → MemberMeta JSON
  └─ MemberMeta                   { username, avatar_url, joined_at, last_seen_at }

wt:room:{roomId}:messages       LIST    capped at 100 via LPUSH+LTRIM
  └─ ChatMessage                  { id, user_id, username, body, ts }

wt:room:{roomId}:events         PUBSUB  multi-instance fanout (future)

TTL policy:
  All wt:room:{roomId}* keys      900 sec sliding (refreshed on any inbound event)
                                  + 5min grace after last member disconnects
                                  → not refreshed → keys expire and room is gone
```

**Why these shapes:**
- One HASH for canonical state — atomic field updates via `HSET`, fits in <1KB.
- Members as a HASH (not SET) so per-member metadata travels with membership without a second roundtrip.
- Messages as a capped LIST — bounded memory; the last-100 visible on reconnect is the right ergonomic for an ephemeral room.
- Pubsub channel `wt:room:{id}:events` is forward-looking for horizontal scaling. v1 runs one instance, but the channel is wired so v2 doesn't need a protocol change.

## REST API

| Method | Path | Body | Returns | Auth |
|---|---|---|---|---|
| `POST` | `/api/watch-together/rooms` | `{anime_id, episode_id, player, translation_id}` | `{room_id, invite_url, ws_url}` | JWT |
| `GET` | `/api/watch-together/rooms/{id}` | — | `RoomSnapshot` or 410 Gone | JWT |
| `DELETE` | `/api/watch-together/rooms/{id}` | — | 204 (host-only force-close; cosmetic) | JWT |

All mutating sync flows over WebSocket. REST is purely lifecycle.

## WebSocket Protocol

Endpoint: `wss://animeenigma.ru/api/watch-together/ws?token=<jwt>&room=<roomId>`
Frame format: JSON envelope `{type, data}`.

### Client → server (inbound)

| `type` | `data` shape | Effect |
|---|---|---|
| `playback:play` | `{ time }` | Broadcast play; update room state |
| `playback:pause` | `{ time }` | Broadcast pause; update room state |
| `playback:seek` | `{ time }` | Broadcast seek; update room state. Server enforces 1s per-user rate limit. |
| `playback:time_tick` | `{ time }` | 1Hz heartbeat for drift detection. Server consumes; NOT rebroadcast. |
| `state:change_episode` | `{ episode_id }` | Switch episode for everyone; reset time to 0 |
| `state:change_player` | `{ player }` | Switch player for everyone |
| `state:change_translation` | `{ translation_id }` | Switch translation for everyone |
| `chat:message` | `{ body }` (max 500 chars) | Append to chat list, broadcast |
| `chat:reaction` | `{ emoji }` (whitelist: ~24 anime-friendly emoji) | Broadcast reaction burst; not persisted |
| `presence:heartbeat` | `{}` | 5s interval; refresh `last_seen_at` |

### Server → client (outbound)

| `type` | `data` shape | Sent when |
|---|---|---|
| `room:snapshot` | `{ room, members, messages }` | On connect — full state for new joiner |
| `room:state_changed` | `{ field, value, by_user_id }` | After any state-mutating inbound |
| `playback:event` | `{ kind, time, by_user_id, server_ts }` | After any playback inbound |
| `playback:correction` | `{ time, server_ts }` | Drift correction nudge (per-recipient, not broadcast) |
| `member:joined` | `{ user_id, member }` | New member joins |
| `member:left` | `{ user_id }` | Member's connection drops |
| `chat:message` | `{ message }` | New chat message |
| `chat:reaction` | `{ user_id, emoji }` | Reaction burst |
| `room:closed` | `{ reason }` | Room is being torn down |
| `error` | `{ code, message }` | Recoverable error (e.g. `CAPACITY_FULL`, `RATE_LIMITED`) |

**Conscious choices:**
- `time_tick` is sender-to-server only — rebroadcasting would create 10 members × 1 Hz × N members of traffic. Server keeps the most recent tick per member.
- `by_user_id` on broadcasts so the UI can show "Alice paused" toasts (subtle, fadeable).
- `server_ts` on playback events so clients can compute network delay and adjust seek targets: `actual_time = event.time + (now - server_ts)`.
- `playback:correction` is a personalized message (only sent to the drifting member), not a broadcast — avoids correction storms.

## Sync engine

The server treats playback as a stream of events, not a polling loop.

### Drift detection (server-side)

For each room, server keeps the canonical playback state (last `play`/`pause`/`seek` event + the wall-clock at which it was received). On each member's `playback:time_tick`:

```
expected_time = room.playback_time + (now - room.playback_time_updated_at) / 1000
                                    if state == "playing" else room.playback_time
drift = abs(member.reported_time - expected_time)

if drift > 1.5s and drift <= 5s:
    send playback:correction {time: expected_time} to that member only (soft nudge)
elif drift > 5s:
    send playback:correction {time: expected_time} to that member only (hard seek)
```

Soft nudge = client smoothly catches up via small `currentTime` adjustment (or accepts the visual jump if too small to seek over). Hard seek = client jumps. The 1.5s tolerance hides typical network jitter on HLS players.

### Kodik adapter

Kodik is the only iframe-based player. The discovered `kodik_player_api` postMessage RPC (full reference in memory) handles all 5 actions:

| Action | Outbound postMessage to iframe |
|---|---|
| play | `{key:'kodik_player_api', value:{method:'play'}}` |
| pause | `{key:'kodik_player_api', value:{method:'pause'}}` |
| seek | `{key:'kodik_player_api', value:{method:'seek', seconds: N}}` |
| volume | `{key:'kodik_player_api', value:{method:'volume', volume: 0..1}}` |
| get_time | `{key:'kodik_player_api', value:{method:'get_time'}}` → reply via outbound `kodik_player_time` |

**Boot-time smoke probe (mandatory):** On `KodikPlayer.vue` mount in a room context, send `get_time` and wait ≤2s for the `kodik_player_time` reply. If no reply, fall back gracefully: room shows banner "Kodik sync unavailable for this bundle version — use voice chat to coordinate" and disables outbound playback sync from this client. Inbound events still work (we still receive `kodik_player_time_update`/`kodik_player_pause`).

`HTMLVideoElement`-based players (4 of 5) need no adapter — direct `videoRef.value.play() / .pause() / .currentTime = N` and `@timeupdate`/`@play`/`@pause`/`@seeked` event listeners are already present in those player components.

## Frontend integration

### New view

`frontend/web/src/views/WatchTogetherView.vue` mounted at route `/watch/room/:roomId`. On mount:
1. `GET /api/watch-together/rooms/{id}` to pre-fetch snapshot (handles "room expired" UX before opening WebSocket).
2. Open WebSocket via `useWatchTogetherRoom(id)`.
3. Mount the appropriate `<*Player room="...">` based on `room.player`.
4. Mount `<RoomSidebar>` (chat + members + reaction burst overlay).

### New composable

`frontend/web/src/composables/useWatchTogetherRoom.ts`:

```ts
export function useWatchTogetherRoom(roomId: Ref<string>) {
  return {
    room,        // ref<RoomSnapshot>
    members,     // ref<Member[]>
    messages,    // ref<ChatMessage[]>
    reactions,   // ref<ReactionBurst[]>  (auto-cleared after 3s)
    connectionStatus, // ref<'connecting'|'open'|'reconnecting'|'closed'>

    emitPlay(time): void
    emitPause(time): void
    emitSeek(time): void
    emitTimeTick(time): void
    emitChangeEpisode(id): void
    emitChangePlayer(id): void
    emitChangeTranslation(id): void
    sendChat(body): Promise<void>
    sendReaction(emoji): void

    onPlaybackEvent(handler): UnsubscribeFn
    onPlaybackCorrection(handler): UnsubscribeFn
    onStateChanged(handler): UnsubscribeFn
  }
}
```

### Player adapter prop

Each of the 5 player components gains an optional `room?: WatchTogetherRoom` prop. When set, the component:
- Wires outbound: `@play` → `room.emitPlay(currentTime)`, `@pause` → `room.emitPause`, `@seeked` → `room.emitSeek`, every 1s → `room.emitTimeTick`.
- Subscribes inbound: `room.onPlaybackEvent` → calls `.play()` / `.pause()` / `.currentTime = N` on local element (with a re-emission guard: don't re-emit the event we just applied).
- Subscribes corrections: `room.onPlaybackCorrection` → applies silently (no toast).
- Subscribes state changes: `room.onStateChanged` → re-mounts player when `episode_id` / `translation_id` / `player` changes.

### Entry point

On the existing `WatchView.vue` (anime watch page), add an "Invite to Watch Together" button in the player chrome area. Click:
1. `POST /api/watch-together/rooms` with current `{anime_id, episode_id, player, translation_id}`.
2. Navigate to `/watch/room/{room_id}`.
3. Copy `invite_url` to clipboard with a toast "Invite link copied — share it with friends".

## UI/UX layout

### Desktop (≥1024px)

```
┌──────────────────────────────────────────────────────────────┐
│  Header (existing)                                            │
├──────────────────────────────────────┬───────────────────────┤
│                                      │  Members (3)         │
│           Player (16:9)              │  ● Alice    (host)   │
│                                      │  ● Bob               │
│                                      │  ● You               │
│                                      │                       │
│                                      ├───────────────────────┤
│                                      │  Chat                 │
│                                      │  Alice: This OP slaps │
│                                      │  Bob:   no skip!!     │
│                                      │  You: …               │
│                                      │                       │
│                                      │  [Reaction palette]   │
│                                      │  [Type a message…   ] │
│  ┌─ Invite link: animeenigma.ru/… ──┐│                       │
│  └──────────────────────── [Copy] ──┘│                       │
└──────────────────────────────────────┴───────────────────────┘
```

### Mobile (<1024px)

```
┌─────────────────────────────────────┐
│  Header (existing, shorter)         │
├─────────────────────────────────────┤
│           Player (16:9)             │
│                                     │
│                                     │
├─────────────────────────────────────┤
│  Members (3)             [▼]        │  ← collapsible
├─────────────────────────────────────┤
│  Tab: Chat | Reactions              │
│                                     │
│  Alice: This OP slaps               │
│  Bob:   no skip!!                   │
│  You: …                             │
│                                     │
│  [Type a message…              ▶ ]  │
└─────────────────────────────────────┘
```

Reaction bursts overlay the player (absolute-positioned, bottom-left for the last 3 reactions, fade after 3s).

## Error handling & recovery

| Scenario | Handling |
|---|---|
| WebSocket connection drops mid-room | Frontend auto-reconnects with exponential backoff (1s, 2s, 4s, 8s, capped at 30s). UI shows "Reconnecting…" indicator. On reconnect, server replays `room:snapshot`. |
| Member's tab refreshes | Same as drop — auto-reconnect within 5min grace = they rejoin the same room with full snapshot. |
| Last member disconnects | Server marks room "draining", waits 5min. If someone reconnects, drain is cancelled. Otherwise room is deleted. |
| Room hits capacity (10 members) | Server rejects connection with `error: {code: 'CAPACITY_FULL'}`. Frontend shows "Room is full (10/10)". |
| Seek rate-limit hit (>1/s per user) | Server drops the message, sends `error: {code: 'RATE_LIMITED'}`. Frontend silently ignores (rate-limit is a UX detail, not a user-facing problem). |
| Kodik smoke probe times out | Frontend falls back to "Kodik sync unavailable" banner; outbound sync disabled, inbound events still consumed. |
| Episode/player/translation switch fails (e.g. provider has no episode N+1) | Initiating client receives `error: {code: 'EPISODE_UNAVAILABLE'}`. State is NOT mutated. Other members see nothing. |
| Member's auth JWT expires mid-session | WebSocket closed with `error: {code: 'AUTH_EXPIRED'}`. Frontend prompts re-login, preserves room URL for return. |
| Service crash / restart | All rooms gone (state was in Redis with TTL; Redis survives, but pubsub subscribers are gone). Clients reconnect, find room expired, redirect to anime page with toast "Room ended". |

## Testing strategy

| Layer | Tool | What |
|---|---|---|
| Go unit | `go test` | Handler logic, message router, drift-detection math, rate limiter |
| Go integration | `go test -tags=integration` + testcontainers Redis | Full WebSocket flow: connect → snapshot → emit → broadcast → reconnect |
| Frontend unit | Vitest | `useWatchTogetherRoom` composable, player adapter wiring, message envelope parsing |
| E2E | Playwright | Two-browser-context test: create room, join, play together, seek-and-followers-follow, chat exchange, reaction burst, host leaves and follower keeps watching |
| Kodik regression | Manual + scripted | Boot probe must keep working — add a daily smoke test that loads a Kodik iframe, sends `get_time`, asserts a reply. If it ever breaks, we know Kodik changed their bundle. |

## Triple-metric scoring (per project convention)

- **UXΔ** = `+4 (Better)` — Co-watching is a category-defining feature for anime communities. Removes the need for voice-coordinated "3, 2, 1, play". The +4 (not +5) reflects v1's deferred "per-user player" feature which is the actual platform-differentiator.
- **CDI** = `0.04 * 55` — Spread: touches new service, gateway routes, 5 player components, 1 new view, 1 new composable, 1 new sidebar component, i18n. Shift: small (additive only; no existing behavior changes). Effort_Fib 55 covers the full v1 (5 phases below).
- **MVQ** = `Griffin 88% / 75%` — Griffin (powerful & graceful, not flashy). Match% 88: the design straightforwardly serves the stated value. Slop-resistance 75: small risk of v2 scope creep ("per-user player" or "persistent named rooms" already loud in the brain).

## Phase decomposition

Detailed phases live in [`.planning/workstreams/watch-together/ROADMAP.md`](../../../.planning/workstreams/watch-together/ROADMAP.md). Brief outline:

| Phase | Title | Independently demoable end-state |
|---|---|---|
| 1 | Backend Foundation | New `services/watch-together/` on :8091. REST `POST /rooms` + WebSocket `/ws`. `curl` + `wscat` can create a room, join, send chat, see snapshot, see broadcast. Gateway routes done. |
| 2 | Frontend Shell + Chat | `WatchTogetherView.vue` route, `useWatchTogetherRoom` composable, sidebar with members + chat + reactions. "Invite to Watch Together" button on WatchView creates room + copies link. Two browsers can join the same room and exchange chat/reactions (no player sync yet). |
| 3 | Player Sync (all 5) | HTML5 adapter wired into AniLib / OurEnglish / Hanime / Raw. Kodik adapter wired via `kodik_player_api` RPC with boot-time smoke probe. Drift correction. Two browsers play/pause/seek in sync on any of the 5 players. |
| 4 | State Switching | Episode / player / translation switching propagates to all members. Players cleanly re-mount on state change. Host changes episode → all followers' players switch and resync. |
| 5 | Polish | Reaction burst animations, reconnect grace period, mobile bottom-sheet layout, i18n (en + ru), capacity-full UX, room-expired redirect, Grafana panel. Production-shippable. |

## Open questions for phase planning

- Where exactly in the existing `WatchView.vue` does the "Invite to Watch Together" button live? (Player chrome corner? Dedicated row? Modal?) — defer to UI mockup during Phase 2 planning.
- Should we expose a debug overlay (member RTT, drift values, last 10 events) for power users / troubleshooting? — likely yes, behind a `?debug=1` query param. Decide in Phase 5.
- Reaction emoji whitelist — pick during Phase 2 planning (currently described as "~24 anime-friendly emoji").
- Mobile chat: bottom-sheet vs full-screen overlay — decide in Phase 5.
