# Phase 2: Frontend Shell + Chat - Context

**Gathered:** 2026-05-25
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)
**Workstream:** watch-together (v1.0)

<domain>
## Phase Boundary

Build the Vue 3 frontend shell for Watch Together. Two browsers can join the same room, see each other in a member list, exchange chat messages and emoji reactions, and the player area is mounted (but does NOT yet sync — sync ships in Phase 3).

- New route `/watch/room/:roomId` with `requireAuth` guard
- New view `WatchTogetherView.vue`
- New composable `useWatchTogetherRoom.ts` (WS lifecycle, auto-reconnect, room state)
- New API client `frontend/web/src/api/watch-together.ts` (createRoom/getRoom/deleteRoom)
- Sidebar: `RoomSidebar`, `MemberList`, `ChatPanel`, `ReactionPalette`, `ReactionBurstOverlay`
- `InviteButton.vue` on existing `WatchView.vue` (player chrome area)
- Player area mounted via existing `<*Player>` component, with new `:room` prop accepted-but-ignored (Phase 3 wires it)
- i18n: new `watch_together.*` namespace in both en.json and ru.json; parity test mirrors the spotlight pattern
- Two-browser smoke: open invite link in second browser → both members visible, chat round-trips, reactions render as floating bursts

**Requirements covered:** WT-SHELL-01..08, WT-NF-04.
</domain>

<decisions>
## Implementation Decisions (Locked from REQUIREMENTS.md + design doc)

### Routing
- Route path: `/watch/room/:roomId`
- Mounted to `WatchTogetherView.vue`
- Uses existing `requireAuth` guard pattern (matches other auth-only routes — check `frontend/web/src/router/index.ts`); redirects unauthenticated to `/login?next=/watch/room/:roomId`
- On mount: fetches room snapshot via `GET /api/watch-together/rooms/{id}` (REST, not WS)
- On 410 Gone: render "This room has ended" empty state with a button back to the anime's watch page

### Composable `useWatchTogetherRoom.ts`
- Returns reactive `room`, `members`, `messages`, `reactions`, `connectionStatus`
- Emit methods: `emitPlay`, `emitPause`, `emitSeek`, `emitTimeTick`, `emitChangeEpisode`, `emitChangePlayer`, `emitChangeTranslation`, `sendChat`, `sendReaction`, `heartbeat` (used by Phase 3 player sync)
- Subscribe methods: `onPlaybackEvent(handler)`, `onStateChanged(handler)`, `onChatMessage(handler)`, `onReaction(handler)`, `onMemberJoined(handler)`, `onMemberLeft(handler)`, `onCorrection(handler)`, `onError(handler)`
- WebSocket lifecycle: open on call, auto-reconnect with exponential backoff (1s/2s/4s/8s, capped at 30s), close on unmount, replay `room:snapshot` on every reconnect
- Re-emission guard: when own user's `by_user_id` matches the inbound event, don't re-fire the local handler (prevents echo loops)
- Mint JWT: use the same auth flow as the rest of the app (existing `useAuthStore` JWT). Append as `?token=` query param on WS URL.

### API client `api/watch-together.ts`
- `createRoom({anime_id, episode_id, player, translation_id})` → returns `{room_id, invite_url, ws_url}`
- `getRoom(id)` → returns `RoomSnapshot`; throws on 410 (caller handles)
- `deleteRoom(id)` → 204; throws on 403 (caller handles)
- Use the existing axios/fetch client + types pattern (check `frontend/web/src/api/notifications.ts` or similar exemplar)

### Component layout
- **`RoomSidebar.vue`** — outer wrapper, holds:
  - `MemberList.vue` — avatars + usernames, `(host)` badge on `host_user_id`, `(You)` badge on own user
  - `ChatPanel.vue` — message list (auto-scroll on new), 500-char input, send-on-Enter; show username next to each message
  - `ReactionPalette.vue` — ~24 anime-friendly emoji as clickable chips; click sends `chat:reaction`
- **`ReactionBurstOverlay.vue`** — absolute-positioned over player area, `pointer-events: none`, animates incoming reactions as floating emoji that fade after 3s
- **`InviteButton.vue`** — mounted into `WatchView.vue` (player chrome area); click flow: createRoom → router.push(`/watch/room/${room_id}`) → copy invite_url to clipboard → toast "Invite link copied — share it with friends"

### Player mounting in WatchTogetherView
- Mount the appropriate `<*Player>` based on `room.player`:
  - `kodik` → `<KodikPlayer>`
  - `animelib` → `<AnimeLibPlayer>`
  - `ourenglish` → `<OurEnglishPlayer>`
  - `hanime` → `<HanimePlayer>`
  - `raw` → `<RawPlayer>`
- Pass `:room="room"` prop to each. **Phase 2:** player components ACCEPT the prop but IGNORE it (no sync logic). Phase 3 wires sync per-player.
- Desktop (>= 1024px): sidebar mounted to the right of player
- Mobile (< 1024px): sidebar as a tabbed bottom panel (chat / members / reactions tabs) — final mobile polish in Phase 5

### i18n
- New `watch_together.*` namespace in BOTH `frontend/web/src/locales/en.json` and `ru.json`
- Required keys: `title`, `subtitle`, `members_heading`, `host_badge`, `you_badge`, `empty_chat`, `chat_input_placeholder`, `send_button`, `reaction_palette_title`, `invite_copied_toast`, `room_ended_title`, `room_ended_back_button`, `reconnecting_indicator`, `capacity_full_error`, `auth_expired_error`
- Locale parity test at `frontend/web/src/locales/__tests__/watch-together-keys.spec.ts` — mirror `spotlight-keys.spec.ts` pattern exactly

### Bundle size
- WT-NF-04: `WatchTogetherView` chunk should be < 30KB gzipped after route-based lazy-loading
- Use dynamic `() => import('@/views/WatchTogetherView.vue')` in router config
- If chunk exceeds 30KB after final implementation, document why in the phase summary and plan a follow-up

### Connection status indicator
- "Reconnecting…" indicator shown when WebSocket is in reconnecting state
- Hide on stable connection

### Capacity / errors
- On `CAPACITY_FULL` close-frame: show "Room is full (10/10)" page with a return-to-anime button
- On `AUTH_EXPIRED`: redirect to login with `?next=` preserved

### Reaction whitelist (PLACEHOLDER for Phase 2 to finalize)
Initial 24 emoji proposal (subject to final designer/PM review in Phase 2 planning):
😂 😭 🔥 💯 👀 😎 🤯 🥹 💀 👍 👎 🤔 ❤️ 💖 ✨ ⭐ 🎉 🙌 👏 😱 😅 🥺 ☠️ 😴

### Claude's Discretion
- Component file-level organization (one big SFC vs split sub-components — your call based on existing project patterns)
- CSS framework: Tailwind utility-only (matches CLAUDE.md spotlight rules: `font-medium`/`font-semibold` weights, no arbitrary numeric weights; `p-4 md:p-6 lg:p-8` padding scheme)
- Toast library: use the project's existing toast (search `frontend/web/src/composables/useToast` or similar)
- Avatar fallback: use existing user-avatar pattern from the app
- Clipboard write: `navigator.clipboard.writeText` (HTTPS-required); display a manual copy-fallback if unavailable

</decisions>

<canonical_refs>
## Canonical References

Downstream agents MUST read:

### Source design + requirements
- `docs/superpowers/specs/2026-05-25-watch-together-design.md` — Message protocol, room:snapshot shape, error semantics
- `.planning/workstreams/watch-together/REQUIREMENTS.md` — WT-SHELL-*, WT-NF-04
- `.planning/workstreams/watch-together/phases/01-backend-foundation/01-PHASE-SUMMARY.md` — What backend protocol Phase 2 builds against

### Backend protocol the frontend consumes
- `services/watch-together/internal/domain/ws_message.go` — All 20 WS message types, Envelope shape
- `services/watch-together/internal/handler/rooms.go` — REST endpoint signatures (POST/GET/DELETE)
- `services/watch-together/internal/handler/websocket.go` — WS upgrade + room:snapshot timing
- `services/watch-together/internal/service/inbound.go` — Inbound message handlers (what data shape each accepts)

### Frontend exemplars (closest analogs)
- `frontend/web/src/composables/` — Look for any composable that wraps a WebSocket (none may exist; if absent, this is the first; use the spotlight composable pattern for structure)
- `frontend/web/src/api/` — API client pattern (look for spotlight.ts, notifications.ts, or similar)
- `frontend/web/src/views/WatchView.vue` — Current watch view; you'll add the InviteButton to it
- `frontend/web/src/components/player/{Kodik,AnimeLib,OurEnglish,Hanime,Raw}Player.vue` — Players you'll need to add a `room?` prop to (just the prop in Phase 2 — wiring is Phase 3)
- `frontend/web/src/components/home/spotlight/` — Component organization pattern for grouped feature components
- `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` — Locale parity test pattern to mirror
- `frontend/web/src/stores/auth.ts` — JWT access for WS connection

### Project conventions
- `CLAUDE.md` — Frontend conventions (bun, Tailwind utility-only, spotlight UI-SPEC contract about font-weights and padding)

</canonical_refs>

<specifics>
## Specific Ideas

### Two-browser smoke
The phase summary should document:
- Browser A: visits an anime, clicks Invite → URL changes to `/watch/room/abc123`, invite URL is in clipboard
- Browser B (different account): opens the URL → lands in same room, sees A in member list
- Both send chat → both see them
- Both send reactions → both see floating bursts
- B closes tab → A sees `member:left` ("Bob left the room")
- Visiting an expired room URL → "This room has ended" page

### WS URL construction
The room snapshot from `getRoom(id)` contains `ws_url` from the backend. Use it directly; don't construct it client-side.

### Auto-reconnect UX
On disconnect, show "Reconnecting…" indicator. On success, replay snapshot. If reconnect fails after 30s capped backoff, show "Unable to reconnect" with a "Refresh" button.

</specifics>

<deferred>
## Deferred Ideas

- Per-player sync wiring (Phase 3)
- Kodik undocumented postMessage RPC integration (Phase 3)
- Drift correction UI nudges (Phase 3)
- Episode/player/translation switching propagation (Phase 4)
- Reaction burst polish animations (Phase 5)
- Mobile bottom-sheet final polish (Phase 5)
- Capacity / auth-expired full UX (Phase 5)
- Grafana dashboard frontend metrics (Phase 5)

</deferred>

---

*Phase: 02-frontend-shell*
*Context auto-generated: 2026-05-25 via workflow.skip_discuss*
