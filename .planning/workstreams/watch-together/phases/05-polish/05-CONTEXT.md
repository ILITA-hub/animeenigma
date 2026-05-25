# Phase 5: Polish + Production-Ship - Context

**Gathered:** 2026-05-25
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)
**Workstream:** watch-together (v1.0)
**Position:** Final phase of v1.0 milestone

<domain>
## Phase Boundary

Polish all UX rough edges and ship watch-together to production. After Phase 5, the feature is ready for end users:
- Reaction bursts animate cleanly (no pile-up artifacts)
- 5min grace period after last-member-disconnect (room state queryable; rejoining restores full state; after grace, room truly gone)
- Mobile layout: sidebar collapses to bottom sheet with tabs (chat / members / reactions)
- 10/10 capacity full UX with clear "return to anime" CTA
- Expired room URL → redirect to anime watch page with "Room ended" toast
- JWT-expired mid-session → prompt re-login; on return rejoin the same room
- i18n complete (final keys; smoke-verified in en + ru)
- Grafana panel for the WT-NF-06 metrics
- CLAUDE.md "Watch Together" section finalized; design-doc link discoverable
- Daily Kodik canary CI cron wired (the spec exists from Phase 3; this phase wires the schedule)

**Requirements covered:** WT-POLISH-01..08, WT-NF-05, WT-NF-06, WT-NF-07.

</domain>

<decisions>
## Implementation Decisions (Locked)

### Backend — grace period
- New file: `services/watch-together/internal/service/grace.go`
- Mechanism: When `hub.Unregister` fires and `ConnectionsInRoom(roomID) == 0`, start a 5min timer (configurable via `WATCH_TOGETHER_GRACE_PERIOD`, already in config from 01.1)
- During grace: do NOT refresh TTL (so keys expire naturally if no one rejoins)
- If a new connection arrives for the room during grace: cancel the grace timer, refresh TTL normally
- Implementation: a per-room `time.AfterFunc` stored in a sync.Map keyed by roomID. On rejoin: `timer.Stop()` + cleanup.

### Backend — Prometheus metrics (WT-NF-06 — full set)
Phase 1 shipped baseline. Phase 5 adds the rest:
- `wt_rooms_active` (gauge — current room count)
- `wt_members_per_room` (histogram — observed at member-join)
- `wt_chat_messages_per_room` (histogram — final count per room at expiry)
- `wt_session_duration_seconds` (histogram — observed at room close)
- `wt_grace_recoveries_total` (counter — incremented when a connection arrives during grace and the timer is cancelled)
- `wt_persistent_drift_total{user_role}` (counter — already partially in Phase 1; finalize labels)

### Frontend — reaction burst polish
- Smooth fade-out with translate-y over 3s (CSS-only, already shipped in Phase 2)
- Cap concurrent reactions on-screen to 12 (oldest pruned if 13th arrives) to prevent pile-up
- Stagger random horizontal offset more carefully to avoid clumping at same x-position

### Frontend — mobile bottom-sheet
- At `< lg` breakpoint, the RoomSidebar becomes a bottom sheet
- Bottom sheet has tab bar: Chat / Members / Reactions
- Player takes full width at top, bottom sheet below; sheet is collapsible via drag handle (use existing project pattern or simple swipe)
- Sheet height: collapsed = 80px (just tab bar), expanded = ~40% viewport

### Frontend — capacity full UX
- When the WS upgrade returns `CAPACITY_FULL` close-frame (or HTTP 409): WatchTogetherView shows a dedicated "Room is full (10/10)" page with a return-to-anime button
- Different from generic "room ended" — clearer signal

### Frontend — expired room URL
- When `getRoom(id)` returns 410 OR `room:closed` event arrives: redirect to `/watch/{anime_id}` (or just `/`) with a toast `t('watch_together.room_ended_redirect_toast')`
- Use the lastKnownAnimeId from the session storage (set when WatchTogetherView mounts and snapshot loads)

### Frontend — auth-expired mid-session
- The composable receives an `AUTH_EXPIRED` error frame (or WS closes with auth-related close code)
- Show modal: "Your session has expired. Log in again to rejoin the room."
- On login: redirect back to `/watch/room/{roomId}` (preserve)
- Use existing project's login redirect pattern (search auth store)

### Frontend — i18n final pass
- Smoke-verify every t() call in the watch-together feature renders translations (NOT raw key strings)
- Add any missing keys discovered during polish
- Both en + ru parity test green

### Grafana dashboard
- New file: `infra/grafana/dashboards/watch-together.json`
- Panels:
  - Active rooms (gauge)
  - Members per room (histogram heatmap)
  - WS connections active (gauge)
  - Chat messages per room (histogram)
  - Session duration (histogram)
  - Drift corrections (counter, by severity)
  - Grace recoveries (counter)
  - HTTP request latency p50/p95/p99
- Auto-loaded via existing Grafana provisioning

### CLAUDE.md — finalize Watch Together section
- Phase 1 added a stub. Phase 5 fills it out:
  - Architecture overview (Redis-only, no Postgres; 5min grace; capacity 10)
  - REST + WS surface summary
  - Link to design doc
  - Link to v1.0 phase summaries

### Daily Kodik canary CI cron
- Add a GitHub Actions workflow or equivalent CI hook that runs `bunx playwright test e2e/kodik-rpc-probe.spec.ts` daily
- On failure: alert (Slack webhook? Telegram? existing project pattern)
- The exact CI infrastructure depends on what the project already uses — read `.github/workflows/` and adapt

### Claude's Discretion
- Toast library / modal library (use existing project utilities)
- Bottom-sheet implementation (existing component if there is one; else simple CSS)
- Auth-expired modal implementation
- Grafana panel layout/colors

</decisions>

<canonical_refs>
## Canonical References

### Prior phase outputs (all 4)
- `.planning/workstreams/watch-together/phases/01-backend-foundation/01-PHASE-SUMMARY.md`
- `.planning/workstreams/watch-together/phases/02-frontend-shell/02-PHASE-SUMMARY.md`
- `.planning/workstreams/watch-together/phases/03-player-sync/03-PHASE-SUMMARY.md`
- `.planning/workstreams/watch-together/phases/04-state-switching/04-PHASE-SUMMARY.md`

### Backend
- `services/watch-together/internal/hub/hub.go` (Unregister hook — extend with grace timer trigger)
- `services/watch-together/internal/service/` (where grace.go lands)
- `services/watch-together/internal/repo/redis_repo.go` (CountMembers; TTL ops)
- `services/watch-together/cmd/watch-together-api/main.go` (DI wiring for grace service)
- `libs/metrics/` (Prometheus registry pattern)

### Frontend
- `frontend/web/src/components/watch-together/ReactionBurstOverlay.vue` (cap concurrent reactions)
- `frontend/web/src/components/watch-together/RoomSidebar.vue` (mobile bottom-sheet refactor)
- `frontend/web/src/views/WatchTogetherView.vue` (capacity-full + expired + auth-expired UX)
- `frontend/web/src/composables/useWatchTogetherRoom.ts` (auth-expired error handling)
- `frontend/web/src/locales/en.json` + `ru.json` (final keys)

### Infra
- `infra/grafana/dashboards/` (where new dashboard JSON lands)
- `.github/workflows/` (CI cron addition for Kodik canary)
- `CLAUDE.md` (Watch Together section finalization)

</canonical_refs>

<specifics>
## Specific Ideas

### Grace period test scenario
1. Two members in room. Both disconnect.
2. Grace starts (5min timer)
3. Before 5min: one reconnects. Grace timer cancelled. Room state intact.
4. After 5min: timer fires. Room expires (no TTL refresh during grace). GET /rooms/{id} returns 410.

### Capacity full UX
The backend already returns CAPACITY_FULL on WS upgrade. Frontend needs the dedicated page.

### Daily canary
The spec ships in Phase 3 (`frontend/web/e2e/kodik-rpc-probe.spec.ts`). Phase 5 just wires the CI cron + alert. Document expected alert mechanism.

</specifics>

<deferred>
## Deferred Ideas (v1.1+)

- Per-user player (each member watches in their own language) — v1.1
- Persistent named rooms — v1.2
- Voice piggyback — v1.3
- Strict catalog validation for ourenglish/hanime/raw (currently permissive) — v1.1
- Optimistic state changes with rollback — future polish

</deferred>

---

*Phase: 05-polish*
*Context auto-generated: 2026-05-25 via workflow.skip_discuss*
