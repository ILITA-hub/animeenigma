---
workstream: watch-together
created: 2026-05-25
last_updated: 2026-05-25
---

# Project State

## Current Position
**Status:** Phase 2 complete — frontend shell + chat shipped. Ready for `/gsd-plan-phase --ws watch-together 03-player-sync`.
**Current Phase:** None (Phase 3 next)
**Last Activity:** 2026-05-25
**Last Activity Description:** Phase 2 (Frontend Shell + Chat) closed out. Two-browser Playwright smoke spec authored: room create → join → chat round-trip → reactions → member:left. i18n smoke-verify (en + ru) covers raw-key-string detection at runtime. WatchTogetherView chunk size: **6.57 kB gzipped** (78% under the 30 kB WT-NF-04 budget). 170 unit/component tests + 4 e2e tests × 3 Playwright projects = 12 listings. All 7 ROADMAP Phase 2 acceptance criteria green. 02-PHASE-SUMMARY.md written.

## Progress
**Phases Complete:** 2 / 5
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

## Decisions locked in Phase 1
See `phases/01-backend-foundation/01-PHASE-SUMMARY.md` for the canonical decisions table. Highlights:
- Port 8091, Redis-only state, `wt:` key prefix, 15min sliding TTL, 10-member capacity, 1-seek/sec + 5-chat/sec rate limits, 500-char chat cap, protocol version "1.0", `?token=` query-param WS auth.

## Decisions locked in Phase 2
See `phases/02-frontend-shell/02-PHASE-SUMMARY.md §"Locked decisions"` for the canonical 16-row decisions table. Highlights:
- `watch_together.*` i18n namespace (27 keys, en+ru parity), 24-emoji REACTION_WHITELIST const re-exported from `@/types/watch-together`, `useWatchTogetherRoom` public API frozen, `WatchTogetherRoomHandle` type alias on every player's `room?` prop, route name `watch-together-room`, `sessionStorage.returnUrl` (not `?next=` query) for auth gate, `defineAsyncComponent` for all 5 players inside the view, echo guard scope = playback:event + room:state_changed only, reconnect backoff `[1, 2, 4, 8, 16, 30]s` with snapshot-reset.

## Session Continuity
**Stopped At:** Phase 2 close-out complete; STATE + ROADMAP updated; Playwright spec authored and parses cleanly across all 3 projects.
**Resume File:** None — next session can run `/gsd-plan-phase --ws watch-together 03-player-sync` to start the player-sync phase.
