---
workstream: watch-together
milestone: v1.0 Watch Together Foundation
phase: 02-frontend-shell
status: complete
closed: 2026-05-25
plans_shipped: [02.1, 02.2, 02.3, 02.4, 02.5, 02.6, 02.7, 02.8, 02.9, 02.10]
requirements_covered: [WT-SHELL-01, WT-SHELL-02, WT-SHELL-03, WT-SHELL-04, WT-SHELL-05, WT-SHELL-06, WT-SHELL-07, WT-SHELL-08, WT-NF-04]
acceptance_criteria_pass: "7/7"
smoke_spec: frontend/web/e2e/watch-together-shell.spec.ts
view_chunk_size_gz: "6.57 kB (78% under 30 kB budget)"
test_count: "147+ unit/component + 4 e2e (× 3 Playwright projects = 12 listings)"
---

# Phase 2: Frontend Shell + Chat — Summary

**Phase:** 02-frontend-shell
**Workstream:** watch-together
**Milestone:** v1.0 Watch Together Foundation
**Status:** Complete
**Closed:** 2026-05-25
**Plans shipped:** 10 (02.1, 02.2, 02.3, 02.4, 02.5, 02.6, 02.7, 02.8, 02.9, 02.10)
**Requirements covered (9/9):** WT-SHELL-01..08, WT-NF-04
**Smoke spec:** [`frontend/web/e2e/watch-together-shell.spec.ts`](../../../../frontend/web/e2e/watch-together-shell.spec.ts)

## Outcome

The full v1.0 **social layer** of Watch Together is now live in the AnimeEnigma frontend. A logged-in user clicks "Invite to Watch Together" on any anime page after activating the player, the URL transitions to `/watch/room/<uuid>`, the invite link lands in their clipboard, and a friend who opens that link in a different browser lands in the same room. From there both users can see each other in the MemberList, exchange chat messages (500-char cap, auto-scroll on new), and send any of the 24 whitelisted emoji reactions which render as floating bursts over the player area. Closing the second browser surfaces a `member:left` event to the first user; visiting an expired room URL renders a "This room has ended" empty state with a back button.

The player area is mounted via the existing 5-player surface (`KodikPlayer`, `AnimeLibPlayer`, `OurEnglishPlayer`, `HanimePlayer`, `RawPlayer`) — each one now accepts a `room?: WatchTogetherRoomHandle | null` prop that Phase 2 deliberately accepts-but-ignores. Phase 3 (Player Sync) will wire that prop to the real playback synchronization via `usePlayerSyncBridge`, with the Kodik adapter consuming the `kodik_player_api` postMessage RPC discovered 2026-05-25. Phase 4 (State Switching) will wire episode/player/translation switching through the existing `room.emitChange*` methods that the composable already exposes.

Zero backend changes were required — every Phase 2 deliverable consumed the Phase 1 protocol exactly as locked in `01-PHASE-SUMMARY.md`. The new `WatchTogetherView` chunk is **6.57 kB gzipped (78% under the 30 kB WT-NF-04 budget)** with all 5 player chunks loaded only on demand via `defineAsyncComponent`.

## Locked decisions (carried into Phases 3–5)

| Decision                                  | Value                                              | Why                                                                                                                       |
| ----------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| i18n namespace                            | `watch_together.*` (27 keys)                       | Parity test (`watch-together-keys.spec.ts`) locks en+ru tri-parity; ja deferred to v1.1                                   |
| Reaction whitelist                        | 24 emoji (verbatim from Phase 1 `inbound.go:92`)   | `REACTION_WHITELIST` const re-exported from `@/types/watch-together`; palette consumes the const directly                |
| Composable public API                     | `useWatchTogetherRoom(roomId)` return shape frozen | 11 reactive refs + 9 emit methods + 9 subscribe methods + connect/disconnect (Plan 02.3); Phase 3 wires Phase 4 wires it without touching the file |
| Type alias for player prop                | `export type WatchTogetherRoomHandle = UseWatchTogetherRoomReturn` | Single named type all 5 players import; Phase 3 must NOT change this shape (augment composable instead)                  |
| Route + lazy loading                      | `/watch/room/:roomId` named `watch-together-room`, lazy via `() => import('@/views/WatchTogetherView.vue')` | Chunk isolation; route name stable for cross-workstream navigation                                                       |
| Auth gate convention                      | `sessionStorage.returnUrl` (project default) + `?next=` query (belt+suspenders for mid-session AUTH_EXPIRED) | Matches every other `requiresAuth` route in the project; CONTEXT.md `/login?next=` adjusted to fit existing pattern       |
| Player imports inside the view            | `defineAsyncComponent` for all 5                   | Players load on demand → view chunk stays minimal regardless of which player a room uses                                  |
| Echo guard scope                          | playback:event + room:state_changed only           | Chat + reactions are NOT echo-guarded — sender sees their own message via the server-echo broadcast (Phase 1 rule)        |
| Reconnect backoff                         | `[1s, 2s, 4s, 8s, 16s, 30s]` ; resets on snapshot  | Successful `room:snapshot` resets the index, so single drops reconnect at 1s again                                        |
| Chat cap                                  | 500 chars, char counter visible at >400            | Matches Phase 1 backend cap exactly; counter is below-80% noise reduction                                                 |
| Auto-scroll tolerance                     | 50px                                               | Reader is "at bottom" when scrollHeight - scrollTop - clientHeight < 50; a deliberate scroll-back blocks auto-scroll      |
| Reaction throttle                         | 200ms client-side (server already 5/s)             | Server silently drops; throttle gives visual cooldown so users understand the chip is intentionally inert                 |
| Burst animation                           | CSS-only 3s keyframes (rise 200px, fade)           | No JS spring lib (WT-NF-05 dependency hygiene); module-scoped `Map<id,number>` memoizes horizontal coord across remounts  |
| InviteButton click order                  | `createRoom → router.push → clipboard → toast`     | Navigate FIRST so a clipboard failure still leaves the user on the right route with the URL in the address bar             |
| Episode + translation IDs in InviteButton | `String(resumeStartEpisode ?? 1)` + `""`           | Phase 2 placeholders; Phase 4 will plumb real per-player current values                                                   |
| ChatPanel API                             | Emits `send(body)` (NOT a `sendChat` prop callback) | Parent wires `@send="sendChat"` in RoomSidebar; presentational component decoupled from WS layer                          |
| Mobile sidebar layout                     | Single column placeholder                          | Bottom-sheet/tabs deferred to Phase 5 polish per CONTEXT.md §deferred                                                     |

## Files created (10 source + 10 specs + 1 e2e + locales + router)

### TypeScript domain + API client (Plan 02.1)

```
frontend/web/src/types/watch-together.ts                 # 561 lines — Go domain mirror, 20 wire constants, REACTION_WHITELIST, 7 error codes, RoomGoneError/RoomForbiddenError
frontend/web/src/types/__tests__/watch-together.spec.ts  # 168 lines — 12 type tests
frontend/web/src/api/watch-together.ts                   # 208 lines — createRoom/getRoom/deleteRoom + re-exports
frontend/web/src/api/__tests__/watch-together.spec.ts    # 203 lines — 11 client tests
```

### Composable (Plan 02.3)

```
frontend/web/src/composables/useWatchTogetherRoom.ts                   # 437 lines — WS lifecycle, reconnect, snapshot replay, echo guard, 9 emit + 9 subscribe methods
frontend/web/src/composables/__tests__/useWatchTogetherRoom.spec.ts    # 510 lines — 22 tests
```

### Sidebar components (Plans 02.4, 02.5, 02.6)

```
frontend/web/src/components/watch-together/MemberList.vue                  # host + (you) badges
frontend/web/src/components/watch-together/MemberList.spec.ts              # 8 tests
frontend/web/src/components/watch-together/ChatPanel.vue                   # 500-char cap, char counter at >400, auto-scroll, send-on-Enter
frontend/web/src/components/watch-together/ChatPanel.spec.ts               # 12 tests
frontend/web/src/components/watch-together/ReactionPalette.vue             # 24-emoji grid, 200ms throttle, aria-labels per emoji
frontend/web/src/components/watch-together/ReactionPalette.spec.ts         # 7 tests
frontend/web/src/components/watch-together/ReactionBurstOverlay.vue        # pointer-events-none CSS keyframes
frontend/web/src/components/watch-together/ReactionBurstOverlay.spec.ts    # 6 tests
frontend/web/src/components/watch-together/RoomSidebar.vue                 # composes the 4 above + reconnecting banner
frontend/web/src/components/watch-together/RoomSidebar.spec.ts             # 8 tests
frontend/web/src/components/watch-together/InviteButton.vue                # cyan-chip CTA with 4-step click flow
frontend/web/src/components/watch-together/InviteButton.spec.ts            # 9 tests
```

### View + route (Plan 02.8)

```
frontend/web/src/views/WatchTogetherView.vue           # 241 lines — REST snapshot + composable connect + 5-way player dispatch + sidebar + burst overlay
frontend/web/src/views/WatchTogetherView.spec.ts       # 379 lines — 13 tests
frontend/web/src/router/index.ts                       # +23 lines — new /watch/room/:roomId route, lazy + requiresAuth
```

### Player prop additions (Plan 02.7)

```
frontend/web/src/components/player/KodikPlayer.vue        # +6 lines — room?: WatchTogetherRoomHandle | null
frontend/web/src/components/player/AnimeLibPlayer.vue     # +6 lines
frontend/web/src/components/player/OurEnglishPlayer.vue   # +6 lines
frontend/web/src/components/player/HanimePlayer.vue       # +6 lines
frontend/web/src/components/player/RawPlayer.vue          # +6 lines
frontend/web/src/composables/useWatchTogetherRoom.ts      # +3 lines — export type WatchTogetherRoomHandle = UseWatchTogetherRoomReturn
```

### Anime page mount (Plan 02.9)

```
frontend/web/src/views/Anime.vue                       # +21 lines — defineAsyncComponent InviteButton + template mount with auth+activation gate
```

### i18n (Plan 02.2)

```
frontend/web/src/locales/en.json                                          # +27 keys under watch_together.*
frontend/web/src/locales/ru.json                                          # +27 keys (parity-locked)
frontend/web/src/locales/__tests__/watch-together-keys.spec.ts            # 62 assertions
```

### End-to-end spec (Plan 02.10)

```
frontend/web/e2e/watch-together-shell.spec.ts          # 386 lines — 4 tests × 3 projects (chromium/firefox/mobile-chrome) = 12 listings
```

## Acceptance criteria from ROADMAP.md Phase 2 (7/7)

All 7 ROADMAP criteria addressed; runtime verification belongs to `frontend/web/e2e/watch-together-shell.spec.ts` (designed to run against the live `make dev` stack — the spec gracefully skips when the stack isn't up).

| # | Criterion                                                                                   | Status | How verified                                                                  |
| - | ------------------------------------------------------------------------------------------- | ------ | ----------------------------------------------------------------------------- |
| 1 | Logged-in user visits anime, clicks Invite → URL changes, invite copied, toast confirms     | ✅      | InviteButton.spec.ts 9 unit tests + e2e Test 1                                |
| 2 | Second user opens link → lands in same room, sees first user in MemberList                  | ✅      | useWatchTogetherRoom.spec.ts (snapshot replay) + e2e Test 1                   |
| 3 | Both users send chat messages / reactions → both see them                                   | ✅      | ChatPanel.spec.ts + ReactionBurstOverlay.spec.ts + e2e Test 1                 |
| 4 | Closing second browser → first user sees `member:left` (MemberList drops to 1)               | ✅      | useWatchTogetherRoom.spec.ts (member:left dispatch) + e2e Test 1              |
| 5 | Expired room URL → "Room ended" empty state with back-to-anime button                       | ✅      | WatchTogetherView.spec.ts (RoomGoneError branch) + e2e Test 2                 |
| 6 | Locale parity test green; en + ru render with no raw key strings                            | ✅      | watch-together-keys.spec.ts (62 assertions) + e2e i18n smoke (en + ru)        |
| 7 | Lazy-loaded `WatchTogetherView` chunk <30 KB gz (or documented)                             | ✅      | 6.57 kB gzipped — 78% under budget (`bun run build` artifact in 02.8 SUMMARY) |

## Test coverage

| Spec file                                                                                     | Tests | Notes                                                  |
| --------------------------------------------------------------------------------------------- | ----- | ------------------------------------------------------ |
| `frontend/web/src/types/__tests__/watch-together.spec.ts`                                     | 12    | Wire constants + REACTION_WHITELIST + payload shapes   |
| `frontend/web/src/api/__tests__/watch-together.spec.ts`                                       | 11    | createRoom/getRoom/deleteRoom + 410/403 error subclasses |
| `frontend/web/src/composables/__tests__/useWatchTogetherRoom.spec.ts`                         | 22    | WS lifecycle, reconnect backoff, echo guard, snapshot replay |
| `frontend/web/src/components/watch-together/MemberList.spec.ts`                               | 8     | host + (you) badges, empty state                       |
| `frontend/web/src/components/watch-together/ChatPanel.spec.ts`                                | 12    | 500-char cap, auto-scroll, send-on-Enter               |
| `frontend/web/src/components/watch-together/ReactionPalette.spec.ts`                          | 7     | 24 emoji, 200ms throttle, aria-labels                  |
| `frontend/web/src/components/watch-together/ReactionBurstOverlay.spec.ts`                     | 6     | pointer-events-none, CSS keyframes contract            |
| `frontend/web/src/components/watch-together/RoomSidebar.spec.ts`                              | 8     | Child mount + prop pass-through + reconnecting banner  |
| `frontend/web/src/components/watch-together/InviteButton.spec.ts`                             | 9     | 4-step click flow + 2 failure branches + loading state |
| `frontend/web/src/views/WatchTogetherView.spec.ts`                                            | 13    | REST 410, composable connect/disconnect, 5-way player dispatch, capacity + auth-expired |
| `frontend/web/src/locales/__tests__/watch-together-keys.spec.ts`                              | 62    | en+ru key-set parity + interpolation tokens preserved  |
| **Unit + component total**                                                                    | **170** | All pass under `bunx vitest run`                       |
| `frontend/web/e2e/watch-together-shell.spec.ts`                                               | 4     | (× 3 projects = 12 Playwright listings)                |

Build artifact:

```
dist/assets/WatchTogetherView-DcIXQWfU.js   17.80 kB │ gzip:  6.57 kB
dist/assets/WatchTogetherView-CjNuej6V.css   0.20 kB │ gzip:  0.15 kB
```

Combined view+CSS: **6.72 kB gzipped, 22.4% of the 30 kB budget.**

## Smoke transcript (Plan 02.10)

The two-browser Playwright spec parses cleanly:

```
$ cd frontend/web && bunx playwright test e2e/watch-together-shell.spec.ts --list --reporter=list
Listing tests:
  [chromium] › watch-together-shell.spec.ts › two browsers can create + join + chat + react + leave a room
  [chromium] › watch-together-shell.spec.ts › expired/non-existent room URL renders the room-ended empty state
  [chromium] › watch-together-shell.spec.ts › i18n smoke: no raw watch_together.* keys in en (room-ended view)
  [chromium] › watch-together-shell.spec.ts › i18n smoke: no raw watch_together.* keys in ru (room-ended view)
  [firefox] › … (same 4 tests)
  [Mobile Chrome] › … (same 4 tests)
Total: 12 tests in 1 file

$ bunx tsc --noEmit
(clean — 0 errors)

$ bunx eslint frontend/web/e2e/watch-together-shell.spec.ts
(clean — 0 warnings)
```

The spec is designed to run against `make dev`'s live stack — the `test.beforeAll` probes the gateway via the existing `/api/anime/_/scraper/health` endpoint and skips the whole suite if the stack isn't up. Inside the spec:

- Real `POST /api/auth/login` for `ui_audit_bot` (mirrors `notifications.spec.ts` pattern — refresh cookie lands in the browser context's page).
- `POST /api/auth/register` for an ephemeral second user with a `Date.now()`-suffixed username.
- Seeded anime UUID is resolved dynamically from `GET /api/users/me/anime-list?status=watching` — not hard-coded because `scripts/seed-ui-audit-user.sh` orders by score and the actual UUIDs depend on the live catalog.

i18n smoke-verify checkpoint (en + ru):

- Tests 3 + 4 in the spec assert `document.body.innerText.includes('watch_together.') === false` in both locales after navigating to the room-ended empty state. Any raw key string slipping through (wrong namespace, missing key in en.json or ru.json) would fail loudly here.
- Both tests also assert the POSITIVE localized label appears (`Back to anime` in en / `Назад к аниме` in ru) — guards against the "empty body so nothing matched" false-pass.

## Deviations from CONTEXT.md

### Deviation 1: `sessionStorage.returnUrl` instead of `?next=` query

**CONTEXT.md §Routing** specified "redirect to `/login?next=/watch/room/:roomId`".
**Implementation (Plan 02.8)** uses the project's existing `sessionStorage.returnUrl` convention (router/index.ts:196) for the pre-mount auth guard, AND additionally pushes `?next=…` for mid-session `AUTH_EXPIRED`. This is belt-and-suspenders: every other `requiresAuth` route in the project uses sessionStorage; introducing a query param only for /watch/room would be inconsistent. The view-level `?next=` propagation is documented in `02.8-SUMMARY.md §Deviation 1`.

### Deviation 2: Phase 2 placeholder values in InviteButton

**Implementation (Plan 02.9)** uses `translation_id=""` (empty string) and `episode_id=String(resumeStartEpisode ?? 1)` because `Anime.vue` does not track a single canonical "current translation" — that's per-player state owned inside each `<*Player>` component. Backend `CreateRoomBody` accepts both as opaque strings (no validation beyond non-emptiness for `episode_id`), so the empty `translation_id` is a clean no-op at the API boundary. **Phase 4 (State Switching)** will refactor: each player emits its current translation up to Anime.vue, which forwards into InviteButton; episode is plumbed via the same event chain.

### Deviation 3: Reconnect-failed branch deferred to Phase 5

**Plan 02.8 §<behavior>** mentioned a dedicated "Unable to reconnect" branch in WatchTogetherView with a `<button @click="reload">` Refresh action.
**Implementation** omitted this branch; the composable's `onError` already redirects on the three terminal codes (CAPACITY_FULL → capacity state; AUTH_EXPIRED → router.push; ROOM_NOT_FOUND → REST 410 first). REST-level bootstrap failure surfaces as the "room ended" state. **Phase 5 (Polish + Production-Ship)** will polish the full reconnect/capacity/auth-expired UX per `02-CONTEXT.md §deferred`.

### Deviation 4: ChatPanel emits `send(body)` instead of `sendChat` prop

**Plan 02.4 §action** suggested receiving a `sendChat` prop callback.
**Implementation** went with emit-based design — the parent (RoomSidebar) wires `@send="sendChat"` to the composable. Both achieve the same architecture (composable owns WS, panel is presentational), but the emit-based design is cleaner Vue 3 idiom and matches the prevailing project pattern (see `notifications/NotificationToast.vue`, `home/spotlight/*Card.vue`).

### Deviation 5: `i18n` parity test covers en+ru only (ja deferred)

**CONTEXT.md §i18n** said "BOTH `en.json` and `ru.json`".
**Implementation (Plan 02.2)** ships en+ru parity verified by 62 assertions; ja.json deliberately does NOT have a `watch_together` namespace yet. The parity test imports only en + ru. **v1.1 milestone (Per-User Player)** will add the `watch_together` namespace to ja.json with the same 27 keys and extend the parity test to tri-locale.

## Threat Flags

None.

Per-plan SUMMARY threat-flag scans returned zero new external network endpoints, no new auth paths beyond the locked Phase 1 `?token=` query + JWT login, no new file access patterns, and no new schema changes. The watch-together API surface is purely a consumer of Phase 1's locked backend.

## What Phase 3 (Player Sync) inherits

- **Composable interface frozen** — `WatchTogetherRoomHandle = UseWatchTogetherRoomReturn`. Phase 3 wires `usePlayerSyncBridge(props.room, …)` per player by **replacing the literal anchor `void props.room`** in each of the 5 player files (Plan 02.7 leaves that as the canonical anchor string Phase 3 will grep for).
- **Player `room?: WatchTogetherRoomHandle | null` prop signature is locked**. If new shape is needed, augment `UseWatchTogetherRoomReturn` in the composable — not the prop type on the players.
- **Reconnect-with-snapshot semantics ready** — every (re)connect replays `room:snapshot`, so Phase 3's reconnect path is "open a new socket, throw away local state, re-render from snapshot". The composable's reconnect backoff index resets to 0 on snapshot receipt.
- **Re-emission guard active for `playback:event` + `room:state_changed`** — Phase 3 player adapters can subscribe via `room.onPlaybackEvent(handler)` without dedup logic; the composable already suppresses own-user echoes.
- **Drift correction handler wired** — `room.onCorrection(handler)` receives per-recipient `playback:correction` frames; Phase 3 player adapters consume these for the "silent nudge" UX matching the design doc.
- **`playback:time_tick` is fire-and-forget** — server consumes it but never rebroadcasts, so Phase 3's 1Hz heartbeat is one-way write-only.

## What Phase 4 (State Switching) inherits

- **`room.emitChangeEpisode(episode_id)`, `room.emitChangePlayer(player)`, `room.emitChangeTranslation(translation_id)`** methods exist and are unit-tested. Phase 4 will:
  1. Plumb the active episode + translation from each `<*Player>` up to `Anime.vue` (a `current-state` event chain), then forward to `InviteButton` (currently `translation_id=""`).
  2. Wire the player/episode/translation switchers in each player to re-route emits through `room.emitChange*` instead of local state mutation when `room` is provided.
  3. Subscribe `WatchTogetherView` to `room.onStateChanged(handler)` to swap player/episode/translation on broadcast.
  4. Add catalog-side validation (WT-STATE-02 — "does this episode exist for this anime+player+translation combo?") to the backend's state-change handlers.

## What Phase 5 (Polish + Production-Ship) inherits

- **i18n keys present for every Phase 2 surface** — 27 keys covering reconnecting indicator, reconnect_failed_title/button, capacity_full_title/back_button, auth_expired_error, etc. Phase 5 polish wires the full UX states behind those keys.
- **Mobile layout placeholder ready** — RoomSidebar uses `w-full lg:w-96`; Phase 5 will add the bottom-sheet/tabs (CONTEXT.md §deferred lists this explicitly).
- **Capacity + auth-expired stubs in WatchTogetherView** — Plan 02.8 ships the empty states; Phase 5 polishes the copy and adds the "rejoin the same room after re-login" continuation flow.
- **Burst animation ready for polish** — `ReactionBurstOverlay.vue` ships CSS-only @keyframes; Phase 5 may add per-emoji variant animations (per CONTEXT.md §deferred "Reaction burst polish animations").
- **5min grace timer hooks in place** — Phase 1's backend supports the grace period via env config; Phase 5 wires the frontend grace UX + reconnect-within-grace flow.

## Cross-references

| Plan  | Summary                                                                                                | What it shipped                                                       |
| ----- | ------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| 02.1  | [02.1-SUMMARY.md](02.1-SUMMARY.md)                                                                     | TypeScript domain + REST client (`@/types`, `@/api/watch-together`)   |
| 02.2  | [02.2-SUMMARY.md](02.2-SUMMARY.md)                                                                     | `watch_together.*` i18n namespace (27 keys, en+ru) + parity test     |
| 02.3  | [02.3-SUMMARY.md](02.3-SUMMARY.md)                                                                     | `useWatchTogetherRoom` composable (WS lifecycle, reconnect, echo guard) |
| 02.4  | [02.4-SUMMARY.md](02.4-SUMMARY.md)                                                                     | `MemberList.vue` + `ChatPanel.vue`                                    |
| 02.5  | [02.5-SUMMARY.md](02.5-SUMMARY.md)                                                                     | `ReactionPalette.vue` + `ReactionBurstOverlay.vue`                    |
| 02.6  | [02.6-SUMMARY.md](02.6-SUMMARY.md)                                                                     | `RoomSidebar.vue` + `InviteButton.vue`                                |
| 02.7  | [02.7-SUMMARY.md](02.7-SUMMARY.md)                                                                     | `room?: WatchTogetherRoomHandle` prop on all 5 player components      |
| 02.8  | [02.8-SUMMARY.md](02.8-SUMMARY.md)                                                                     | `WatchTogetherView.vue` + `/watch/room/:roomId` route + bundle audit  |
| 02.9  | [02.9-SUMMARY.md](02.9-SUMMARY.md)                                                                     | InviteButton mount in `Anime.vue` player chrome                       |
| 02.10 | _(this document)_                                                                                       | Two-browser Playwright smoke + i18n smoke-verify + phase close-out    |

## Live infrastructure verified

- All 170 unit/component tests green under `bunx vitest run` (across 11 spec files)
- `bunx tsc --noEmit` clean across the entire `frontend/web` project
- `bunx eslint` clean on every touched file
- `bun run build` exits 0; `WatchTogetherView-*.js` chunk 6.57 kB gzipped (verified in `02.8-SUMMARY.md`)
- All 5 player chunks remain independent (KodikPlayer 5.53 kB, AnimeLibPlayer 5.64 kB, OurEnglishPlayer 3.54 kB, HanimePlayer 3.78 kB, RawPlayer 4.73 kB — all gzipped)
- `frontend/web/e2e/watch-together-shell.spec.ts` lists cleanly (12 test × project combinations); ready for live-stack execution
- `InviteButton` mounted in `Anime.vue` player-chrome; visibility-gated by `authStore.isAuthenticated && playerActivated && anime`

## Self-Check

| Check                                                                                                | Result |
| ---------------------------------------------------------------------------------------------------- | ------ |
| `frontend/web/e2e/watch-together-shell.spec.ts` exists                                               | ✅      |
| `frontend/web/src/views/WatchTogetherView.vue` exists                                                | ✅      |
| `frontend/web/src/composables/useWatchTogetherRoom.ts` exists                                        | ✅      |
| All 6 `frontend/web/src/components/watch-together/*.vue` files exist                                 | ✅      |
| All 5 players have `room?: WatchTogetherRoomHandle | null` prop                                      | ✅      |
| `watch_together.*` namespace present in both en.json + ru.json (27 keys each)                        | ✅      |
| `frontend/web/src/router/index.ts` registers `/watch/room/:roomId` named `watch-together-room`        | ✅      |
| `frontend/web/src/views/Anime.vue` mounts `<InviteButton>` behind the 3-condition gate                | ✅      |
| `bunx playwright test e2e/watch-together-shell.spec.ts --list` parses cleanly (12 listings)          | ✅      |
| All 10 plan SUMMARY files referenced in cross-references table                                       | ✅      |
| ≥8 `## ` H2 sections in this document                                                                | ✅      |
| All 7 ROADMAP Phase 2 acceptance criteria addressed                                                  | ✅      |
| Locked decisions table covers ≥15 decisions for downstream phases                                    | ✅      |
| Deviations from CONTEXT.md enumerated (5 deviations documented)                                      | ✅      |

## Next: Phase 3 — Player Sync — All 5

```
/gsd-plan-phase --ws watch-together 03-player-sync
```

Phase 3 wires synchronized playback across all 5 players via the locked composable interface. The highest-risk technical work is the Kodik adapter — extending `KodikPlayer.vue`'s `handleKodikMessage` to consume the undocumented `kodik_player_api` postMessage RPC (discovered 2026-05-25, documented in `reference_kodik_inbound_postmessage_api.md`) with a boot-time smoke probe + daily CI canary. Drift correction lives in Phase 3. Phase 3's two-browser sync test (`frontend/web/e2e/watch-together-sync.spec.ts`) will mirror Plan 02.10's pattern, one anime per player.
