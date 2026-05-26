---
workstream: watch-together
created: 2026-05-25
last_updated: 2026-05-26
milestone_v1_0_status: complete
milestone_v1_0_closed: 2026-05-26
---

# Project State

## Current Position
**Status:** v1.0 Watch Together Foundation **SHIPPED** 2026-05-26. All 5 phases complete (5/5). 41 plans landed across the milestone; 51 requirements covered. Ready for the next milestone (`/gsd-new-milestone --ws watch-together`).
**Current Phase:** None — milestone closed
**Last Activity:** 2026-05-26
**Last Activity Description:** Phase 5 (Polish + Production-Ship) closed via Plan 05.9. Three commits land: smoke script `scripts/smoke-watch-together-v1.sh` (8 acceptance scenarios, 3 consecutive idempotent runs exit 0), `05-PHASE-SUMMARY.md` (200+ lines, 14 H2 sections, 30-row locked-decisions table, 4 documented deviations), ROADMAP/STATE close-out. Phase 5 deliverables across 9 plans: GraceManager (5min reconnect window, sync.Map race-winner primitive, 10 unit tests under -race), 5 new Prometheus metrics (wt_rooms_active gauge + members/chat/session histograms + persistent-drift counterVec), ReactionBurstOverlay polish (8-cap FIFO + 8-column stratification + scale-rise-wiggle 2.5s), RoomSidebar mobile bottom-sheet (dual-mode SFC, drag gestures, 2-tab bar), WatchTogetherView capacity (10/10) + room-closed redirect + auth-expired blocking modal + lastAnimeId sessionStorage cache, Grafana dashboard (13 panels, every wt_* metric), CLAUDE.md §Watch Together expanded to ~80 lines + WT-NF-05 dependency audit attestation (zero new heavyweight backend deps; zero new npm deps across all 5 phases), daily 7:13 UTC GitHub Actions Kodik canary cron. Soft prerequisite D-04-01 (hero-spotlight `platform_stats.go` build breakage) carries forward as documentation-only — does not block v1.0 ship; the watch-together-side code for the catalog validate endpoint is correct and unit-tested under `GOWORK=off`, only live deployment of the catalog-side endpoint is gated.

## Milestone Roll-Up

| Phase | Closed | Plans | Requirements |
|-------|--------|-------|--------------|
| Phase 1 — Backend Foundation | 2026-05-25 | 9 | WT-FOUND-01..10, WT-NF-01..03 |
| Phase 2 — Frontend Shell + Chat | 2026-05-25 | 10 | WT-SHELL-01..08, WT-NF-04 |
| Phase 3 — Player Sync — All 5 | 2026-05-25 | 7 | WT-SYNC-01..10 |
| Phase 4 — State Switching | 2026-05-25 | 6 | WT-STATE-01..05 |
| Phase 5 — Polish + Production-Ship | 2026-05-26 | 9 | WT-POLISH-01..08, WT-NF-05..07, WT-SYNC-10 cron |
| **Total** | | **41** | **51 covered** |

## v1.0 Promise — Delivered

2–10 logged-in friends share an invite link, land in the same room at `/watch/room/:roomId`, watch the same anime in lock-step. Every play, pause, seek, episode switch, player switch, and translation switch mirrors to all members in real time. Text chat and emoji reactions run alongside the player. All 5 players syncable — including Kodik via the undocumented `kodik_player_api` RPC. Reconnect grace, mobile bottom-sheet, capacity-full UX, room-expired redirect, auth-expired modal, Grafana dashboard, daily Kodik canary. Shipped 2026-05-26.

## Progress
**Phases Complete:** 5 / 5 — v1.0 COMPLETE
**Current Plan:** N/A — milestone closed

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
- TypeScript domain + REST client; composable; 6 watch-together components; view + route; player `room?` prop wiring (Phase 3-ready); InviteButton mounted into Anime.vue
- 27 `watch_together.*` i18n keys (en + ru parity-locked)
- Two-browser e2e shell spec (`watch-together-shell.spec.ts`)
- 170 unit/component tests; WatchTogetherView chunk 6.57 kB gz (78% under 30 kB WT-NF-04 budget)

## Phase 3 deliverables (shipped)
- `usePlayerSyncBridge.ts` composable (RAF 1Hz heartbeat, two-layer echo guard, soft-vs-hard drift correction)
- All 5 players adopt `props.room?` and invoke the bridge
- Kodik adapter via `kodik_player_api` postMessage RPC + boot-time smoke probe + fallback banner
- SyncToastStack + ConnectionStatusOverlay + 5 new i18n keys
- 2 e2e specs (sync + canary, totaling 21 listings)
- 188 unit/component tests

## Phase 4 deliverables (shipped)
- Catalog `/internal/anime/{id}/episodes/validate` endpoint (strict for kodik/animelib, permissive for ourenglish/hanime/raw)
- Watch-together `CatalogClient` (3s timeout + 5s positive cache)
- Validated `handleChangeEpisode/Player/Translation` handlers + 2 new error code constants
- PlayerTabBar.vue (5-tab switcher with data-player + aria-selected)
- Per-player switcher routing in all 5 SFCs
- 8 new i18n keys (player_tab_<kind> × 5 + state_change_<field>_unavailable × 3)
- Two-browser state-switching e2e spec (4 tests × 3 projects = 12 listings)
- 50 new backend unit tests + 27 frontend vitest cases

## Phase 5 deliverables (shipped)
- `GraceManager` (5min reconnect window) + 10 unit tests under `-race`
- 5 new Prometheus metrics (`wt_rooms_active`, `wt_members_per_room`, `wt_chat_messages_per_room`, `wt_session_duration_seconds`, `wt_persistent_drift_total{user_role}`) + shared `observeRoomTeardown` helper
- `room:closed` broadcast on host DELETE (closes 01.4 TODO)
- ReactionBurstOverlay polish (8-cap FIFO + 8-column stratification + scale-rise-wiggle 2.5s)
- RoomSidebar mobile bottom-sheet (dual-mode SFC, drag gestures, 2-tab bar, member count strip)
- WatchTogetherView polish (capacity 10/10 + room-closed redirect + auth-expired blocking modal + lastAnimeId sessionStorage cache + onAuthExpired composable channel)
- Grafana dashboard `infra/grafana/dashboards/watch-together.json` (13 panels covering every `wt_*` metric)
- `CLAUDE.md` §Watch Together expanded from 7 to ~80 lines + WT-NF-05 dependency audit attestation
- `.github/workflows/watch-together-kodik-canary.yml` (daily 7:13 UTC cron, Chromium-only, Telegram alert on failure)
- 6 new i18n keys (bottom_sheet_tab_chat + bottom_sheet_tab_reactions + room_ended_redirect_toast + auth_expired_modal_title/body/login_button) en + ru parity-locked
- `scripts/smoke-watch-together-v1.sh` (8 acceptance scenarios, idempotent — 3 consecutive runs exit 0)
- ~25 new backend unit tests + ~36 new frontend tests on plan-touched specs

## Decisions locked across the milestone

See the per-phase summaries for canonical decisions tables:
- Phase 1: `phases/01-backend-foundation/01-PHASE-SUMMARY.md` (~25 decisions)
- Phase 2: `phases/02-frontend-shell/02-PHASE-SUMMARY.md` §"Locked decisions" (16 rows)
- Phase 3: `phases/03-player-sync/03-PHASE-SUMMARY.md` §"Locked decisions" (23 rows)
- Phase 4: `phases/04-state-switching/04-PHASE-SUMMARY.md` §"Locked decisions" (27 rows)
- Phase 5: `phases/05-polish/05-PHASE-SUMMARY.md` §"Locked decisions" (30+ rows)

## Soft prerequisite (carried from Phase 4 — does NOT block v1.0)
- D-04-01 (pre-existing `services/catalog/internal/service/spotlight/cards/platform_stats.go` build breakage from hero-spotlight workstream commit `b17bbb3`) blocks `make redeploy-catalog`. Plan 04.1's endpoint code is on the branch but cannot be deployed live until D-04-01 is resolved in the hero-spotlight workstream. Watch-together backend tests run green under `GOWORK=off`. Smoke section 7 SKIPs the live `/internal/anime/.../episodes/validate` check with structured rationale. v1.0 ships unblocked; D-04-01 carries forward to whichever workstream resolves it first.

## Concerns / Risks (forward-looking)
- **Kodik undocumented RPC is a single point of failure for RU sync.** The `kodik_player_api` postMessage dispatcher is undocumented; Kodik can change/remove it in any bundle update. Phase 3 ships a boot probe + fallback banner; Phase 5 ships the daily Playwright canary (`.github/workflows/watch-together-kodik-canary.yml`) + Telegram alert as the early-warning signal.
- **Concurrent-agent `git add -A` risk.** Documented in MEMORY.md → `feedback_worktree_from_head`. Phase 5 agents continued staging files individually by path. No incidents in Phase 5.

## Session Continuity
**Stopped At:** v1.0 milestone closed. Phase 5 (Polish + Production-Ship) shipped via 9 plans (05.1–05.9). STATE + ROADMAP + 05-PHASE-SUMMARY all updated. Smoke `scripts/smoke-watch-together-v1.sh` exits 0 on 3 consecutive runs against the live stack. Operator manual VR checkpoint (VR1–VR7) deferred for post-merge walkthrough — not a ship blocker.
**Resume File:** None.
**Next:** Run `/gsd-new-milestone --ws watch-together` to start v1.1 (Per-User Player), v1.2 (Persistent Named Rooms), or v1.3 (Voice Piggyback) — per `MILESTONES.md`.
