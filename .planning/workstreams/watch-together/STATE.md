---
workstream: watch-together
created: 2026-05-25
last_updated: 2026-05-25
---

# Project State

## Current Position
**Status:** Phase 3 complete — synchronized playback across all 5 players + Kodik RPC adapter + daily canary + sender-attribution toasts + connection-status overlay. Ready for `/gsd-plan-phase --ws watch-together 04-state-switching`.
**Current Phase:** None (Phase 4 next)
**Last Activity:** 2026-05-25
**Last Activity Description:** Phase 3 (Player Sync — All 5) closed out. `usePlayerSyncBridge.ts` composable (17 Vitest cases) wired into AnimeLib + OurEnglish + Hanime + Raw via `if (props.room) usePlayerSyncBridge(videoRef, props.room)`. KodikPlayer.vue extended with `postCommand` helper + 500ms-delayed/2s-timeout boot probe + fallback banner when undocumented `kodik_player_api` RPC fails. `SyncToastStack.vue` + `ConnectionStatusOverlay.vue` mounted in WatchTogetherView with 5 new i18n keys (en+ru parity-locked). Two e2e specs shipped: `watch-together-sync.spec.ts` (6 tests × 3 projects = 18 listings across all 5 players + CDP-throttled drift correction) and `kodik-rpc-probe.spec.ts` (`[canary]`-tagged, 354 lines). 188 unit/component tests green, 9/9 ROADMAP Phase 3 acceptance criteria addressed. 03-PHASE-SUMMARY.md written.

## Progress
**Phases Complete:** 3 / 5
**Current Plan:** N/A (between phases)

## Phase 1 deliverables (shipped)
- `services/watch-together/` Go microservice on port 8091
- REST: POST/GET/DELETE /api/watch-together/rooms[/{id}]
- WebSocket: /api/watch-together/ws with full inbound router (10 message types)
- Drift engine + per-user rate limits + 500-char chat cap
- Redis-only state under `wt:` key prefix with 15min sliding TTL
- Gateway: HTTP proxy + dedicated `httputil.NewSingleHostReverseProxy` WS reverse proxy
- docker-compose entry + Makefile targets + CLAUDE.md updates
- 98 unit tests across watch-together module (+ 10 gateway integration)
- `scripts/smoke-watch-together.sh` (Phase 1 acceptance check, idempotent)

## Phase 2 deliverables (shipped)
- TypeScript domain + REST client: `frontend/web/src/types/watch-together.ts`, `frontend/web/src/api/watch-together.ts` (mirror of Phase 1 Go domain; 20 wire constants + REACTION_WHITELIST + custom error subclasses)
- Composable: `frontend/web/src/composables/useWatchTogetherRoom.ts` — WS lifecycle, exponential reconnect backoff, snapshot replay, echo guard, 9 emit + 9 subscribe methods (frozen public API for Phase 3)
- 6 components in `frontend/web/src/components/watch-together/`: MemberList, ChatPanel (500-char cap + char counter + auto-scroll), ReactionPalette (24 emoji + 200ms throttle), ReactionBurstOverlay (CSS-only @keyframes), RoomSidebar (composes the above), InviteButton (4-step click flow with clipboard fallback)
- View + route: `frontend/web/src/views/WatchTogetherView.vue` (lazy-loaded at `/watch/room/:roomId` route name `watch-together-room`) — REST snapshot fetch + composable connect + 5-way `<*Player>` dispatch via `defineAsyncComponent` + sidebar + burst overlay
- All 5 `<*Player>` components accept `room?: WatchTogetherRoomHandle | null` prop (Phase 3 wires sync via `usePlayerSyncBridge`); type alias `WatchTogetherRoomHandle` exported from the composable
- `InviteButton` mounted into `Anime.vue` player chrome, gated by `authStore.isAuthenticated && playerActivated && anime`
- i18n: `watch_together.*` namespace (27 keys) in both en.json + ru.json with parity test (62 assertions)
- E2E: `frontend/web/e2e/watch-together-shell.spec.ts` — two-browser room flow + expired-room URL + i18n smoke (en + ru) = 4 tests × 3 Playwright projects = 12 listings
- 170 unit/component tests green across 11 spec files; WatchTogetherView chunk 6.57 kB gz (78% under 30 kB WT-NF-04 budget)

## Phase 3 deliverables (shipped)
- Bridge composable: `frontend/web/src/composables/usePlayerSyncBridge.ts` (356 lines) — RAF heartbeat at 1Hz, two-layer echo guard (`applyingRemote` flag + 250ms watchdog + lastAppliedTime ±0.5s tolerance), soft drift correction (playbackRate 0.97/1.03 for 5s) for <1s drift, hard `currentTime` seek for ≥1s drift. 17-case Vitest suite (TDD RED → GREEN)
- Wired into all 4 HTML5 players via `if (props.room) usePlayerSyncBridge(videoRef, props.room)`: AnimeLibPlayer, OurEnglishPlayer, HanimePlayer, RawPlayer. Phase 2 `void props.room` anchors removed entirely. Zero behavior change when `props.room === null`
- Kodik adapter: `KodikPlayer.vue` extended +179 lines — `postCommand(method, payload)` helper using `{key: 'kodik_player_api', value: {method, ...payload}}`; `handleKodikMessage` now consumes `kodik_player_play/seek/video_started/video_ended/time`; outbound RPCs gated by `kodikSyncAvailable` boolean; 500ms-delayed iframe boot grace + 2s `get_time` reply timeout sets `kodikSyncAvailable=false` + renders i18n fallback banner; 300ms `applyingRemote` re-entry guard
- UX overlays in WatchTogetherView (mount order: ConnectionStatusOverlay → players → SyncToastStack → ReactionBurstOverlay):
  - `SyncToastStack.vue` (146 lines, 11 tests) — subscribes to `room.onPlaybackEvent`, sender-attribution toasts ("Alice paused" / "Bob seeked to 12:34"), max-3 stack, 2000ms auto-removal, vue `transition-group` fade. Echo-guarded by composable so own-user events don't toast
  - `ConnectionStatusOverlay.vue` (77 lines, 9 tests) — renders only for `reconnecting`/`closed`, silent on `idle`/`connecting`/`open`/`failed`. `pointer-events-none` outer + `pointer-events-auto` inner; CSS-only `animate-spin` spinner
- 5 new i18n keys (en+ru parity-locked, +3 interpolation-preservation tests): `kodik_sync_unavailable`, `sync_toast_played`, `sync_toast_paused`, `sync_toast_seeked`, `connection_status_closed`
- E2E specs:
  - `frontend/web/e2e/watch-together-sync.spec.ts` (493 lines, 6 tests × 3 projects = 18 listings) — 5 player-sync tests (all 5 players exercised) + 1 drift-correction test (CDP `Emulation.setCPUThrottlingRate rate:4` on browser B)
  - `frontend/web/e2e/kodik-rpc-probe.spec.ts` (354 lines, `[canary]`-tagged) — daily Playwright canary; 7-step probe of `get_time` → `play` → `pause` → `seek`; skip vs fail policy: stack-down/catalog-drift/iframe-not-found = `test.skip`, `get_time` → no reply = hard fail
- 188 unit/component tests green (Phase 2 baseline 170 + Phase 3 delta 18); `bunx tsc --noEmit` + `bunx eslint` clean across every touched file

## Decisions locked in Phase 1
See `phases/01-backend-foundation/01-PHASE-SUMMARY.md` for the canonical decisions table. Highlights:
- Port 8091, Redis-only state, `wt:` key prefix, 15min sliding TTL, 10-member capacity, 1-seek/sec + 5-chat/sec rate limits, 500-char chat cap, protocol version "1.0", `?token=` query-param WS auth.

## Decisions locked in Phase 2
See `phases/02-frontend-shell/02-PHASE-SUMMARY.md §"Locked decisions"` for the canonical 16-row decisions table. Highlights:
- `watch_together.*` i18n namespace (27 keys, en+ru parity), 24-emoji REACTION_WHITELIST const re-exported from `@/types/watch-together`, `useWatchTogetherRoom` public API frozen, `WatchTogetherRoomHandle` type alias on every player's `room?` prop, route name `watch-together-room`, `sessionStorage.returnUrl` (not `?next=` query) for auth gate, `defineAsyncComponent` for all 5 players inside the view, echo guard scope = playback:event + room:state_changed only, reconnect backoff `[1, 2, 4, 8, 16, 30]s` with snapshot-reset.

## Decisions locked in Phase 3
See `phases/03-player-sync/03-PHASE-SUMMARY.md §"Locked decisions"` for the canonical 23-row decisions table. Highlights:
- `usePlayerSyncBridge(videoRef, room): void` signature frozen; bridge call placed AFTER `videoRef` declaration (NOT at Phase 2 `void props.room` anchor) because `<script setup>` `const` doesn't hoist
- RAF + Date.now() gate at 1Hz heartbeat (NOT `setInterval`); two-layer echo guard (`applyingRemote` flag + 250ms watchdog + lastAppliedTime ±0.5s tolerance)
- Soft drift correction: `playbackRate` nudge to 0.97/1.03 for 5s for <1s drift; hard `currentTime` seek for ≥1s drift; corrections silent (no UI feedback per WT-SYNC-06)
- Kodik adapter: extend existing `handleKodikMessage` switch (don't replace); `postCommand` shape verbatim from `reference_kodik_inbound_postmessage_api.md`; 500ms iframe-boot grace + 2s `get_time` reply timeout; fallback banner gates outbound sync, inbound still consumed for progress-save
- Sender toast: 2000ms total lifetime, max-3 stack, mm:ss client-side time format (locale carries only `{time}` slot), `"someone"` verbatim component constant for username fallback (not an i18n key)
- ConnectionStatusOverlay scope: `reconnecting` + `closed` only; silent on `failed` (owned by WatchTogetherView's terminal-state branches); mounted in WatchTogetherView not in players
- Canary skip vs fail: stack-down/catalog-drift/iframe-not-found = `test.skip`; `get_time` → no reply = hard fail (the only meaningful signal)
- Two-browser e2e timeout: 2000ms (not 500ms target) — accounts for CI scheduling jitter without flaking

## Outstanding work for Phase 4
- Plumb the active episode + translation from each `<*Player>` up to `Anime.vue` / `WatchTogetherView.vue` so InviteButton can stop passing `translation_id=""`
- Wire each player's episode/player/translation switchers to re-route emits through `room.emitChangeEpisode/Player/Translation` instead of local state mutation when `room` is provided
- Subscribe `WatchTogetherView` to `room.onStateChanged(handler)` to swap player / re-mount on broadcast
- Add catalog-side validation (WT-STATE-02 — "does this episode exist for this anime+player+translation combo?") to backend state-change handlers
- (Optional) Add 3 new toast kinds (`sync_toast_episode_changed`, `sync_toast_player_switched`, `sync_toast_translation_changed`) — the locale parity test will enforce both en+ru in lockstep

## Concerns / Risks
- **Kodik undocumented RPC is a single point of failure for RU sync.** The `kodik_player_api` postMessage dispatcher is undocumented; Kodik can change/remove it in any bundle update. The Phase 3 boot probe + fallback banner provide graceful degradation; the daily Playwright canary (`frontend/web/e2e/kodik-rpc-probe.spec.ts`) is the only early-warning signal we have. Phase 5 must wire the nightly cron + alerting (Telegram via `TELEGRAM_ADMIN_CHAT_ID`) — without it the canary is silent.
- **Concurrent-agent `git add -A` risk.** Plan 03.6's smoke spec was absorbed into Plan 03.5's commit hash (`696c1e7`) when the 03.5 executor ran `git add -A` against the workstream rule documented in MEMORY.md → `feedback_worktree_from_head`. Content is correct on disk; only commit metadata is wrong. Phase 4 agents must continue staging files individually by path, never `git add -A`.

## Session Continuity
**Stopped At:** Phase 3 close-out complete; STATE + ROADMAP updated; 03-PHASE-SUMMARY.md written; bridge composable + Kodik adapter + UX overlays + e2e canary all shipping and green.
**Resume File:** None — next session can run `/gsd-plan-phase --ws watch-together 04-state-switching` to start the state-switching phase.
