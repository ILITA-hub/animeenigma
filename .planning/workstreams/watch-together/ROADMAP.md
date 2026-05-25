# Roadmap: AnimeEnigma `watch-together` workstream

**Workstream:** watch-together (parallel to root `v3.x` scraper work, parallel to other workstreams `notifications`, `raw-jp`, `social`, `ui-ux-audit`, `hero-spotlight`)
**Active milestone:** v1.0 Watch Together Foundation
**Phase numbering:** Workstream-local — restarts at 1 inside each milestone (`v1.0` phases live at `phases/01-*`..`phases/05-*`; future `v1.1` phases at `milestones/v1.1-phases/01-*`).
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-25-watch-together-design.md`
**Requirements:** `REQUIREMENTS.md`

## Milestones

- ⏳ **v1.0 Watch Together Foundation** — Active (3/5 phases shipped). Ephemeral private friend rooms (2–10), all 5 players syncable (Kodik via undocumented `kodik_player_api` RPC), text chat + emoji reactions. Phase 1 (Backend Foundation) closed 2026-05-25; Phase 2 (Frontend Shell + Chat) closed 2026-05-25; Phase 3 (Player Sync — All 5) closed 2026-05-25.
- ⏳ **v1.1 Per-User Player** — Deferred. Mixed-language friend groups watch in their own language while sharing the timeline. Needs its own brainstorm.
- ⏳ **v1.2 Persistent Named Rooms** — Conditional. "Saturday Anime Night" rooms that survive past empty state. Adds Postgres + chat retention.
- ⏳ **v1.3 Voice Piggyback** — Conditional, post v1.1 usage data.

## v1.0 phases (planning)

| Phase | Title | Status |
|-------|-------|--------|
| 1 | Backend Foundation | ✅ Complete (2026-05-25 — 9 plans, 8/8 acceptance, smoke green 3× — see [`01-PHASE-SUMMARY.md`](phases/01-backend-foundation/01-PHASE-SUMMARY.md)) |
| 2 | Frontend Shell + Chat | ✅ Complete (2026-05-25 — 10 plans, 7/7 acceptance, two-browser Playwright smoke spec authored, chunk 6.57 kB gz / 30 kB budget — see [`02-PHASE-SUMMARY.md`](phases/02-frontend-shell/02-PHASE-SUMMARY.md)) |
| 3 | Player Sync — All 5 | ✅ Complete (2026-05-25 — 7 plans, 9/9 acceptance, two-browser sync e2e spec authored across all 5 players + drift via CDP throttling, Kodik RPC daily canary shipped, 188 unit/component tests + 21 e2e listings — see [`03-PHASE-SUMMARY.md`](phases/03-player-sync/03-PHASE-SUMMARY.md)) |
| 4 | State Switching | ⏳ Not started |
| 5 | Polish + Production-Ship | ⏳ Not started |

## Next

After v1.0 ships:

```
/gsd-new-milestone --ws watch-together
```

## Goal (v1.0)

2–10 logged-in AnimeEnigma users share an invite link, land in the same room at `/watch/room/:roomId`, and watch the same anime in lock-step. Every play, pause, seek, episode switch, player switch, and translation switch mirrors to all members in real time. Text chat and emoji reactions run alongside the player. All 5 players are syncable — including Kodik, via the undocumented `kodik_player_api` postMessage RPC discovered 2026-05-25.

The two-friends-in-different-time-zones-co-watch-Frieren-without-coordinating-1-2-3-play moment is what the milestone delivers.

State is ephemeral (Redis only, no Postgres), rooms die when empty + 5min grace. Login required for everyone. Invite link is the only distribution mechanism.

**UXΔ = +4 (Better)** | **CDI = 0.04 × 55** | **MVQ = Griffin 88%/75%**

## Vertical-slice phasing rationale

Five phases, each independently demoable and atomically committable:

- **Phase 1 (Backend Foundation)** ends with a working REST + WebSocket service that `curl` and `wscat` can fully exercise. Two `wscat` sessions in the same room can exchange chat and see each other's join events. No frontend touches; the WebSocket protocol surface is settled. Future phases can build against it without infra coordination.
- **Phase 2 (Frontend Shell + Chat)** is pure frontend — new view, new composable, new sidebar components, invite button on the existing watch page. Two browsers can join, see each other, exchange chat and reactions. The player is mounted but does NOT yet sync — sync ships in Phase 3. This phase delivers the *social* layer of the feature, which is independently valuable: friends can use the chat sidebar even when watching at slightly different speeds.
- **Phase 3 (Player Sync — All 5)** is where the synchronized-playback magic lands. Kodik's adapter (with the boot-time smoke probe) is the highest-risk technical work in the whole milestone — it's also the work that proves the feature can ship across all 5 players uniformly. Drift correction lives here. Phase 3 is the demo-day phase: "look, two browsers play Frieren in sync."
- **Phase 4 (State Switching)** lifts the feature from "watch one specific episode together" to "watch a whole series together" — episode switches, player switches, translation switches all propagate. This is mostly wire-up work since the underlying players already support reactive source changes.
- **Phase 5 (Polish + Production-Ship)** is the difference between a working prototype and a feature that's ready for users — reaction burst animations, reconnect grace, mobile bottom-sheet layout, i18n, capacity UX, room-expired redirects, auth-expired handling, and the Grafana panel.

The Kodik regression test (`WT-SYNC-10`) is intentionally a daily Playwright run, not a one-time check — Kodik's bundle can change at any time, so we want continuous canary signal on the RPC.

## Phases

### Phase 1: Backend Foundation

**Goal:** Stand up `services/watch-together/` on port 8091 with Redis-only state, REST endpoints for room lifecycle, WebSocket endpoint for sync/chat/reactions/state-changes, drift detection engine, gateway routing, docker-compose entry, Makefile targets, CLAUDE.md updates. End-state: `curl POST /api/watch-together/rooms` returns a `room_id`; two `wscat` connections to `/ws?token=$JWT&room=$ROOM_ID` see each other's joins, exchange chat messages with broadcast echo, and receive a `room:snapshot` on connect.

**Depends on:** Nothing — additive backend module. Touches existing files in `services/gateway/` (config + router) and `docker/docker-compose.yml` (one new service block) and `Makefile` (3 new targets) and `CLAUDE.md` (two new table rows).
**Requirements:** WT-FOUND-01, WT-FOUND-02, WT-FOUND-03, WT-FOUND-04, WT-FOUND-05, WT-FOUND-06, WT-FOUND-07, WT-FOUND-08, WT-FOUND-09, WT-FOUND-10, WT-NF-01, WT-NF-02, WT-NF-03
**SPEC:** `phases/01-backend-foundation/01-SPEC.md` (to be written by gsd-plan-phase)
**Touches:**
- `services/watch-together/cmd/watch-together-api/main.go` (new)
- `services/watch-together/internal/{config,domain,handler,repo,service,hub,transport}/` (new)
- `services/watch-together/Dockerfile` (new)
- `services/watch-together/go.mod` (new — joined to `go.work`; require + replace for every `libs/*` used)
- `go.work` (extend)
- `docker/docker-compose.yml` (new watch-together service block + depends_on redis)
- `docker/.env.example` (new `WATCH_TOGETHER_*` env vars, `WATCH_TOGETHER_SERVICE_URL` for gateway)
- `services/gateway/internal/config/config.go` (new `WatchTogetherURL` field)
- `services/gateway/internal/router/routes.go` (new `/api/watch-together/*` HTTP + WebSocket proxy under `authMiddleware`)
- `Makefile` (`make redeploy-watch-together`, `make logs-watch-together`, `make restart-watch-together`)
- `CLAUDE.md` (Service Ports row + Gateway Routing row)

**Success criteria:**
1. `make redeploy-watch-together` builds and starts the container clean; `make health` includes `watch-together:8091 - healthy`.
2. `curl http://localhost:8091/health` → 200 `{"status":"ok"}` directly; `curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" -X POST http://localhost:8000/api/watch-together/rooms -d '{"anime_id":"...","episode_id":"1","player":"animelib","translation_id":"..."}'` → 200 with `{room_id, invite_url, ws_url}` (gateway proxy + auth both work).
3. `wscat -c "ws://localhost:8000/api/watch-together/ws?token=$JWT&room=$ROOM_ID"` connects successfully and immediately receives a `room:snapshot` frame.
4. Two `wscat` sessions in the same room each receive a `member:joined` for the other. Sending `{"type":"chat:message","data":{"body":"hi"}}` from one is broadcast as `{"type":"chat:message","data":{"message":{...}}}` to both.
5. WebSocket without a valid `?token=` is rejected with HTTP 401 (not a close frame).
6. WebSocket with `?room=` referencing a non-existent room is rejected with `error: {code: 'ROOM_NOT_FOUND'}` close frame.
7. Spam `seek` events from one client → server emits `error: {code: 'RATE_LIMITED'}` after the second seek within 1s.
8. After 15min of zero events on a room, the Redis keys expire and `GET /api/watch-together/rooms/{id}` returns 410.

### Phase 2: Frontend Shell + Chat

**Goal:** New route `/watch/room/:roomId`, new view `WatchTogetherView.vue`, new composable `useWatchTogetherRoom`, sidebar with member list + chat panel + reaction palette + reaction burst overlay, "Invite to Watch Together" button on the existing `WatchView.vue` that creates a room and copies the invite link. Two browsers can join the same room, see each other, exchange chat and reactions. Player is mounted but does NOT yet sync.

**Depends on:** Phase 1 (REST + WebSocket endpoints must exist).
**Requirements:** WT-SHELL-01, WT-SHELL-02, WT-SHELL-03, WT-SHELL-04, WT-SHELL-05, WT-SHELL-06, WT-SHELL-07, WT-SHELL-08, WT-NF-04
**SPEC:** `phases/02-frontend-shell/02-SPEC.md`
**Touches:**
- `frontend/web/src/views/WatchTogetherView.vue` (new)
- `frontend/web/src/composables/useWatchTogetherRoom.ts` (new)
- `frontend/web/src/api/watch-together.ts` (new)
- `frontend/web/src/components/watch-together/{RoomSidebar,MemberList,ChatPanel,ReactionPalette,ReactionBurstOverlay,InviteButton}.vue` (new)
- `frontend/web/src/router/index.ts` (add route, mark `requireAuth`)
- `frontend/web/src/views/WatchView.vue` (mount `<InviteButton>`)
- `frontend/web/src/locales/{en,ru}.json` (new `watch_together.*` namespace + parity test)
- `frontend/web/src/locales/__tests__/watch-together-keys.spec.ts` (new — mirror spotlight parity test)

**Success criteria:**
1. Logged-in user visits an anime, clicks "Invite to Watch Together" → URL changes to `/watch/room/abc123`, invite link is in clipboard, toast confirms.
2. Second user opens the invite link in a different browser → lands in the same room, sees first user in member list.
3. Both users send chat messages; both see them in real time. Both send reactions; both see floating bursts.
4. Closing the second browser → first user sees `member:left` ("Bob left the room").
5. Returning to a room URL after the room has expired → "This room has ended" page with a button back to the anime.
6. Locale parity test green; both en and ru render the room view with no raw key strings (smoke-verify per [feedback_smoke_verify_i18n.md]).
7. Lazy-loaded `WatchTogetherView` chunk is <30KB gz (or documented why not, with a plan).

### Phase 3: Player Sync — All 5

**Goal:** Sync playback across all 5 players. New `usePlayerSyncBridge` composable for HTML5 players. Kodik adapter via `kodik_player_api` postMessage RPC with boot-time smoke probe. Drift correction. Sender-attribution toasts. Connection-status overlay. Daily Playwright regression test for the Kodik RPC.

**Depends on:** Phase 2 (composable + view + sidebar exist).
**Requirements:** WT-SYNC-01, WT-SYNC-02, WT-SYNC-03, WT-SYNC-04, WT-SYNC-05, WT-SYNC-06, WT-SYNC-07, WT-SYNC-08, WT-SYNC-09, WT-SYNC-10
**SPEC:** `phases/03-player-sync/03-SPEC.md`
**Touches:**
- `frontend/web/src/composables/usePlayerSyncBridge.ts` (new)
- `frontend/web/src/components/player/AnimeLibPlayer.vue` (add `room?` prop + bridge wiring)
- `frontend/web/src/components/player/OurEnglishPlayer.vue` (same)
- `frontend/web/src/components/player/HanimePlayer.vue` (same)
- `frontend/web/src/components/player/RawPlayer.vue` (same)
- `frontend/web/src/components/player/KodikPlayer.vue` (extend `handleKodikMessage` + add `postCommand` + boot-time smoke probe + `room?` prop)
- `frontend/web/e2e/kodik-rpc-probe.spec.ts` (new — daily CI regression)
- `frontend/web/e2e/watch-together-sync.spec.ts` (new — two-browser-context sync test, one anime per player)

**Success criteria:**
1. Two browsers in the same room, host clicks Play → follower's video starts playing within 500ms.
2. Host pauses → follower pauses within 500ms.
3. Host seeks to 5:00 → follower seeks to 5:00 ± 1s (network jitter tolerance).
4. Host slows tab to 4x (DevTools Performance) → server-side drift correction nudges host back to expected time within 5s.
5. Sender-attribution toast appears on follower when host acts ("Alice paused", "Alice seeked to 5:00").
6. Connection-status overlay appears during forced WebSocket disconnect; clears on reconnect.
7. Kodik boot probe: in a room, opening a Kodik anime → `get_time` is sent within 500ms of mount; `kodik_player_time` reply arrives within 2s; no fallback banner.
8. Forced Kodik probe failure (mock the postMessage reply) → fallback banner visible, outbound sync disabled, but incoming `kodik_player_time_update` / `kodik_player_pause` still update the local UI.
9. CI nightly run of `kodik-rpc-probe.spec.ts` passes (this is the canary for Kodik bundle changes).

### Phase 4: State Switching

**Goal:** Episode / player / translation switching propagates to all room members. Backend validates the requested switch (e.g. "does this episode exist for this anime+player+translation combo?") before broadcasting. Frontend re-mounts player / re-loads source on `room:state_changed`.

**Depends on:** Phase 3 (players are wired to the room).
**Requirements:** WT-STATE-01, WT-STATE-02, WT-STATE-03, WT-STATE-04, WT-STATE-05
**SPEC:** `phases/04-state-switching/04-SPEC.md`
**Touches:**
- `services/watch-together/internal/handler/ws_inbound.go` (add 3 new message handlers)
- `services/watch-together/internal/service/state.go` (catalog HTTP client for validation; new file)
- `services/catalog/...` — verify existing `/internal/anime/{id}/episodes?...` endpoint covers the validation needs of WT-STATE-02; if not, extend in this phase (one new param or endpoint).
- `frontend/web/src/views/WatchTogetherView.vue` (subscribe to `room.onStateChanged`, swap player/episode/translation)
- `frontend/web/src/components/player/*` (existing episode/player/translation switchers gain "in a room?" prop that re-routes the emit through `room.emitChange*` instead of local state mutation)

**Success criteria:**
1. Host clicks next-episode → both browsers' players switch and start the new episode paused at 0.
2. Host switches Kodik → AniLib → both browsers swap the active player; the new player resumes paused at 0.
3. Host switches translation (e.g. AniLibria → AniRise on AniLib) → both browsers reload the source.
4. Trying to switch to an episode that doesn't exist for the current combo → sender sees `EPISODE_UNAVAILABLE` error inline; other members see nothing; room state unchanged.
5. After a switch, drift correction re-stabilizes within 5s of both players starting playback.

### Phase 5: Polish + Production-Ship

**Goal:** Production-grade UX. Reaction burst animations, reconnect grace period, mobile bottom-sheet layout, i18n complete, capacity UX, room-expired redirect, auth-expired handling, Grafana panel.

**Note (carried from Phase 3 close-out):** The Kodik canary spec is **already shipped** at [`frontend/web/e2e/kodik-rpc-probe.spec.ts`](../../frontend/web/e2e/kodik-rpc-probe.spec.ts) (354 lines, `[canary]`-tagged, Plan 03.4). Phase 5 does NOT need to author the spec — it only needs to wire the nightly cron + alerting (most likely a GitHub Actions `schedule:` trigger + Telegram notification using the same `TELEGRAM_ADMIN_CHAT_ID` env var the player ReportButton already uses). Filter via `bunx playwright test --grep canary --reporter=list`.

**Depends on:** Phase 4 (full feature works end-to-end).
**Requirements:** WT-POLISH-01, WT-POLISH-02, WT-POLISH-03, WT-POLISH-04, WT-POLISH-05, WT-POLISH-06, WT-POLISH-07, WT-POLISH-08, WT-NF-05, WT-NF-06, WT-NF-07
**SPEC:** `phases/05-polish/05-SPEC.md`
**Touches:**
- `frontend/web/src/components/watch-together/ReactionBurstOverlay.vue` (animations)
- `frontend/web/src/components/watch-together/RoomSidebar.vue` (mobile bottom-sheet layout)
- `frontend/web/src/composables/useWatchTogetherRoom.ts` (auth-expired flow + room-expired redirect)
- `services/watch-together/internal/service/grace.go` (5min grace timer; new file)
- `services/watch-together/internal/metrics/metrics.go` (new — register counters/gauges per WT-NF-06)
- `infra/grafana/dashboards/watch-together.json` (new dashboard panel)
- `CLAUDE.md` (new "Watch Together" section)
- `frontend/web/src/locales/{en,ru}.json` (final i18n strings)

**Success criteria:**
1. Reaction bursts animate cleanly; sending 10 reactions in rapid succession doesn't pile up artifacts.
2. Last member disconnects → room state remains queryable for 5 min; reconnecting within that window restores full state. After 5 min, room is gone.
3. Mobile (< 1024px viewport): sidebar collapses to bottom sheet with two tabs; player stays at top.
4. Joining an at-capacity room → 10/10 page with a clear message and a return-to-anime button.
5. Visiting an expired room URL → redirect to anime watch page with "Room ended" toast.
6. JWT expires mid-session → prompt re-login; on return, rejoin the same room.
7. i18n: room view renders cleanly in both en and ru with zero raw key strings (smoke-verified in browser).
8. Grafana dashboard panel shows live data after a few test rooms run; all WT-NF-06 metrics emit.
9. `CLAUDE.md` updated; new service appears in both tables; design-doc link discoverable.
