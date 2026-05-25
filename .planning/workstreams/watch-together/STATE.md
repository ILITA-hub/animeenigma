---
workstream: watch-together
created: 2026-05-25
last_updated: 2026-05-25
---

# Project State

## Current Position
**Status:** Phase 4 complete — validated state-switch propagation (episode / player / translation) across all 5 players, sender-only error UX, two-browser e2e spec shipped. Ready for `/gsd-plan-phase --ws watch-together 05-polish`.
**Current Phase:** None (Phase 5 next)
**Last Activity:** 2026-05-25
**Last Activity Description:** Phase 4 (State Switching) closed out. 6 plans landed across 4 waves: catalog `/internal/anime/{id}/episodes/validate` endpoint (04.1 — 28 unit/HTTP tests), watch-together `CatalogClient` with 3s timeout + 5s positive cache + 2 new error codes (04.2 — 8 tests), validated `handleChangeEpisode/Player/Translation` handlers replacing Phase 1 pass-throughs (04.3 — 12 TestStateChange_* tests + exported CatalogValidator interface + applyStateChange shared tail), PlayerTabBar.vue + WatchTogetherView player remount via :key + error toasts + 8 i18n keys (04.4 — 14 view + component tests, 7.08 kB gz WatchTogetherView chunk), per-player switcher routing through `props.room.emitChangeXxx` in all 5 player SFCs (04.5), and the two-browser state-switching e2e spec (04.6 — 622 lines, 4 tests × 3 Playwright projects = 12 listings). All 5/5 ROADMAP Phase 4 acceptance criteria addressed. 04-PHASE-SUMMARY.md written with 14 H2 sections + 27-row locked-decisions table + 6 documented deviations.

## Progress
**Phases Complete:** 4 / 5
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
See `phases/03-player-sync/03-PHASE-SUMMARY.md §"Locked decisions"` for the canonical 23-row decisions table.

## Decisions locked in Phase 4
See `phases/04-state-switching/04-PHASE-SUMMARY.md §"Locked decisions"` for the canonical 27-row decisions table. Highlights:
- Catalog endpoint is a sibling (`/internal/anime/{id}/episodes/validate`) not a flag on `/episodes` (keeps NOTIF-DET-01 frozen); soft-negative HTTP 200 `{valid, reason}` contract (NOT 4xx)
- Strict validation for kodik/animelib delegates to existing `EpisodesLookupService` (5min Redis cache reuse from NOTIF-DET-01); permissive v1.0 for ourenglish/hanime/raw with `// TODO(v1.1): tighten validation` markers at both call sites
- CatalogClient: 3s timeout, 5s positive cache (mutex protects map only, HTTP call outside lock), negative results NEVER cached, clock injection for tests
- Transport-error policy: NEVER mutate Redis on 5xx/DNS/timeout — sender-only error mapped to user-changed field's code (UX consistency)
- Bogus player short-circuits as BAD_PAYLOAD before catalog round-trip; `episode_id` resets to literal `"1"` sentinel on player change
- Exported `CatalogValidator` interface (mirrors HubFanout pattern) — cross-package test stub via `nopCatalog`
- PlayerTabBar overlaid `absolute top-2 left-2 z-20` (not sibling above player); stable iteration order kodik→animelib→ourenglish→hanime→raw (matches WatchTogetherView dispatch + PlayerKind union); `:key="`player-${livePlayer}`"` re-mount mechanism
- Per-player guard pattern: `if (props.room) { props.room.emitChangeXxx(String(id)); return }` as FIRST executable line of user-click selectors; programmatic `_selectXxx` siblings intentionally NOT guarded
- Raw subtitle picker NOT routed (per-user UX); OurEnglish server picker NOT routed (different axis); Hanime auto-advance DOES flow through the guard (both members advance together)
- E2E test hook: `window.__wtTestRoom` exposed only in dev/test builds via VITE_TEST_HOOK; production builds skip cleanly with structured notes; 2000ms wire-propagation budget per assertion (500ms target × 4 CI slack) Highlights:
- `usePlayerSyncBridge(videoRef, room): void` signature frozen; bridge call placed AFTER `videoRef` declaration (NOT at Phase 2 `void props.room` anchor) because `<script setup>` `const` doesn't hoist
- RAF + Date.now() gate at 1Hz heartbeat (NOT `setInterval`); two-layer echo guard (`applyingRemote` flag + 250ms watchdog + lastAppliedTime ±0.5s tolerance)
- Soft drift correction: `playbackRate` nudge to 0.97/1.03 for 5s for <1s drift; hard `currentTime` seek for ≥1s drift; corrections silent (no UI feedback per WT-SYNC-06)
- Kodik adapter: extend existing `handleKodikMessage` switch (don't replace); `postCommand` shape verbatim from `reference_kodik_inbound_postmessage_api.md`; 500ms iframe-boot grace + 2s `get_time` reply timeout; fallback banner gates outbound sync, inbound still consumed for progress-save
- Sender toast: 2000ms total lifetime, max-3 stack, mm:ss client-side time format (locale carries only `{time}` slot), `"someone"` verbatim component constant for username fallback (not an i18n key)
- ConnectionStatusOverlay scope: `reconnecting` + `closed` only; silent on `failed` (owned by WatchTogetherView's terminal-state branches); mounted in WatchTogetherView not in players
- Canary skip vs fail: stack-down/catalog-drift/iframe-not-found = `test.skip`; `get_time` → no reply = hard fail (the only meaningful signal)
- Two-browser e2e timeout: 2000ms (not 500ms target) — accounts for CI scheduling jitter without flaking

## Phase 4 deliverables (shipped)
- Catalog `/internal/anime/{id}/episodes/validate` endpoint (Plan 04.1) — strict for kodik/animelib via existing EpisodesLookupService, permissive for ourenglish/hanime/raw (`// TODO(v1.1): tighten validation`); soft-negative HTTP 200 + `{valid, reason}` contract; mounted on root router with NO middleware (docker-network-only)
- Watch-together CatalogClient (`internal/service/catalog_client.go`, Plan 04.2) — 3s timeout + 5s positive-result cache (mutex protects map only, HTTP call outside lock); clock injection for deterministic TTL tests; 2 new error code constants (`ErrCodePlayerUnavailable`, `ErrCodeTranslationUnavailable`); `CATALOG_URL` config field with trailing-slash trim
- Validated `handleChangeEpisode/Player/Translation` handlers in `internal/service/inbound.go` (Plan 04.3) — Phase 1 pass-through fully removed; exported `CatalogValidator` interface (cross-package test stub); shared `applyStateChange` tail centralizing HSET + broadcast + playback reset; bogus player short-circuits as BAD_PAYLOAD before catalog round-trip; `episode_id` resets to literal `"1"` sentinel on player change
- PlayerTabBar.vue + WatchTogetherView wiring (Plan 04.4) — 5-tab overlaid switcher with `data-player` + `aria-selected`; `:key="`player-${livePlayer}`"` re-mount on all 5 player branches; `onError` toast surfacing for the 3 new sender-only error codes BEFORE the CAPACITY_FULL / AUTH_EXPIRED checks; 8 i18n keys (`player_tab_<kind>` × 5 + `state_change_<field>_unavailable` × 3) en+ru parity-locked
- Per-player switcher routing (Plan 04.5) — `if (props.room) { props.room.emitChangeXxx(String(id)); return }` guard at the head of every user-click selector in all 5 SFCs; programmatic `_selectXxx` siblings intentionally NOT guarded; Raw subtitle picker explicitly per-user; Hanime auto-advance flows through the guard (both members advance together)
- Two-browser state-switching e2e spec (Plan 04.6) — `frontend/web/e2e/watch-together-state-switching.spec.ts` (622 lines, 4 tests × 3 Playwright projects = 12 listings); episode/player/translation switch propagation + invalid-episode sender-only safety property; `window.__wtTestRoom` hook for 3 tests (skip cleanly when absent in prod), PlayerTabBar UI direct for Test 2
- 50 new backend unit tests + 27 frontend vitest cases on the plan-touched specs + 12 e2e listings; full workstream vitest sweep green; `bunx tsc --noEmit` + `bunx eslint` clean across every touched file

## Outstanding work for Phase 5
- Wire the nightly Kodik canary cron (spec already shipped in Phase 3 at `frontend/web/e2e/kodik-rpc-probe.spec.ts`) + Telegram alerting via `TELEGRAM_ADMIN_CHAT_ID`
- Reaction-burst animation polish (CSS-only @keyframes per WT-POLISH-01; max 8 simultaneous bursts)
- 5-minute reconnect grace period in backend `internal/service/grace.go` (last-member-disconnect timer; WT-POLISH-02)
- Mobile bottom-sheet sidebar layout (<lg breakpoint; WT-POLISH-03)
- Capacity-full UX page (10/10 with return-to-anime button; WT-POLISH-04)
- Room-expired redirect (410 from GET /rooms/{id} → toast + back to anime; WT-POLISH-05)
- Auth-expired re-login flow with `next=` preservation (WT-POLISH-06)
- Grafana dashboard panel (active rooms, members, drift corrections, Kodik probe success — WT-POLISH-08 + WT-NF-06)
- CLAUDE.md "Watch Together" section + Service Ports + Gateway Routing table rows (WT-NF-07)

## Soft prerequisite (carried from Phase 4)
- D-04-01 (pre-existing `services/catalog/internal/service/spotlight/cards/platform_stats.go` build breakage from hero-spotlight workstream commit `b17bbb3`) blocks `make redeploy-catalog`. Plan 04.1's endpoint code is on the branch but cannot be deployed live until D-04-01 is resolved in the hero-spotlight workstream. Watch-together backend tests run green under `GOWORK=off` (module mode bypasses the workspace genproto conflict + skips the broken spotlight/cards build). Documented in `phases/04-state-switching/deferred-items.md`. Phase 5 is NOT blocked — `make redeploy-watch-together` works; only the catalog-side endpoint is dark-shipped pending the hero-spotlight fix.

## Concerns / Risks
- **Kodik undocumented RPC is a single point of failure for RU sync.** The `kodik_player_api` postMessage dispatcher is undocumented; Kodik can change/remove it in any bundle update. The Phase 3 boot probe + fallback banner provide graceful degradation; the daily Playwright canary (`frontend/web/e2e/kodik-rpc-probe.spec.ts`) is the only early-warning signal we have. Phase 5 must wire the nightly cron + alerting (Telegram via `TELEGRAM_ADMIN_CHAT_ID`) — without it the canary is silent.
- **Concurrent-agent `git add -A` risk.** Plan 03.6's smoke spec was absorbed into Plan 03.5's commit hash (`696c1e7`) when the 03.5 executor ran `git add -A` against the workstream rule documented in MEMORY.md → `feedback_worktree_from_head`. Content is correct on disk; only commit metadata is wrong. Phase 4 agents must continue staging files individually by path, never `git add -A`.

## Session Continuity
**Stopped At:** Phase 4 close-out complete; STATE + ROADMAP updated; 04-PHASE-SUMMARY.md written; validated state-switch handlers + CatalogClient + PlayerTabBar + per-player guards + e2e spec all shipping and green. Soft prerequisite D-04-01 (hero-spotlight `platform_stats.go` build breakage) carries forward — not blocking Phase 5 work, only blocks the live catalog-side endpoint deployment via `make redeploy-catalog`.
**Resume File:** None — next session can run `/gsd-plan-phase --ws watch-together 05-polish` to start the production-ship phase.
