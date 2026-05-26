# Phase 3: Player Sync — All 5 - Context

**Gathered:** 2026-05-25
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)
**Workstream:** watch-together (v1.0)
**Risk:** HIGHEST in v1.0 (Kodik adapter via undocumented RPC)

<domain>
## Phase Boundary

Wire synchronized playback across all 5 players. After Phase 3, two browsers in the same room playing the same anime stay in lockstep — play, pause, seek mirror within 500ms; drift is corrected; sender-attribution toasts narrate who did what; connection-status overlay during disconnects; daily Playwright canary for the Kodik RPC catches bundle changes.

**Requirements covered:** WT-SYNC-01..10.

### Players to wire (5)
- HTML5 `<video>` based (use a generic bridge composable):
  - AnimeLib (MP4)
  - OurEnglish (HLS or MP4 via backend)
  - Hanime (HLS)
  - Raw (HLS or MP4)
- Iframe-based (postMessage RPC):
  - Kodik (`kodik_player_api` undocumented RPC, see `reference_kodik_inbound_postmessage_api.md` memory)

</domain>

<decisions>
## Implementation Decisions (Locked)

### Generic HTML5 bridge — `usePlayerSyncBridge.ts`
A composable that takes a `videoRef: Ref<HTMLVideoElement | null>` and a `room: WatchTogetherRoomHandle` and:
- Listens to native `@play`, `@pause`, `@seeked` events on the video → emits `room.emitPlay(time)`, `room.emitPause(time)`, `room.emitSeek(time)`
- Periodically (e.g., every 2s during play) emits `room.emitTimeTick(currentTime)` for drift detection
- Subscribes `room.onPlaybackEvent(handler)`:
  - On inbound `playback:play`: `video.play()` + `video.currentTime = event.time` (if drift > 1s)
  - On inbound `playback:pause`: `video.pause()` + `video.currentTime = event.time`
  - On inbound `playback:seek`: `video.currentTime = event.time`
- Subscribes `room.onCorrection(handler)`:
  - On `playback:correction`: if severity="soft", smooth playback rate adjustment (0.95-1.05 for a few seconds); if severity="hard", direct `video.currentTime = event.time`

### Re-emission guard
When applying an inbound event, set a flag (`applyingRemote = true`) and a "last applied" timestamp. The local `@play|@pause|@seeked` listeners check this flag and skip emitting if true OR if the event's currentTime matches `lastAppliedTime` within ±0.5s. Reset the flag after the natural event fires.

### Wiring `usePlayerSyncBridge` into HTML5 players
Each of AnimeLibPlayer.vue, OurEnglishPlayer.vue, HanimePlayer.vue, RawPlayer.vue:
- When `props.room` is set (non-null), call `usePlayerSyncBridge(videoRef, props.room)` in setup
- When `props.room` is null/undefined: no behavior change — existing single-user playback works exactly as today (zero regression)

### Kodik adapter — `KodikPlayer.vue`
Extend `handleKodikMessage` at line 280 to ALSO consume:
- `kodik_player_play` → emit `room.emitPlay(currentTime)`
- `kodik_player_seek` → emit `room.emitSeek(seekedToTime)`
- `kodik_player_video_started` → no emit, but useful for diagnostic
- `kodik_player_video_ended` → no emit
- `kodik_player_time` → reply to boot-time `get_time` probe (see Boot Probe below)

Add an outbound `postCommand(method, payload)` helper:
```ts
function postCommand(method: string, payload?: Record<string, unknown>) {
  iframe.value?.contentWindow?.postMessage(
    { key: 'kodik_player_api', value: { method, ...payload } },
    '*'
  )
}
```

When `props.room` is set:
- Subscribe `room.onPlaybackEvent`:
  - `playback:play` → `postCommand('play')`
  - `playback:pause` → `postCommand('pause')`
  - `playback:seek` with `time` → `postCommand('seek', {seconds: time})`
- Outbound events from Kodik → call `room.emitPlay/Pause/Seek` (with re-emission guard)

### Kodik boot-time smoke probe (highest risk)
On `KodikPlayer.vue` mount in a room context:
1. After iframe `load` event, send `postCommand('get_time')`
2. Start a 2s timer waiting for inbound `kodik_player_time` reply
3. If reply arrives: `kodikSyncAvailable = true`
4. If 2s elapses with no reply: `kodikSyncAvailable = false`, show banner `t('watch_together.kodik_sync_unavailable')`, DISABLE outbound sync from this client (incoming `kodik_player_time_update`/`pause` are still consumed for the local UI updates)

### Drift correction UX
- Sender-attribution toasts (e.g., "Alice paused", "Alice seeked to 5:00") use `event.by_user_id` to look up username from `room.members`
- Show toasts on inbound `playback:play|pause|seek` events when `by_user_id !== room.currentUserId`
- Use the existing project toast composable
- Connection-status overlay: full-screen translucent banner during `room.connectionStatus === 'reconnecting'` with `t('watch_together.reconnecting_indicator')`; clears on reconnect

### Playwright canary — `e2e/kodik-rpc-probe.spec.ts`
Daily CI regression test:
1. Open a known Kodik-bearing anime
2. Wait for player iframe load
3. Send `get_time` via parent postMessage
4. Assert `kodik_player_time` reply arrives within 5s
5. Send `play`, expect outbound `kodik_player_play` event
6. Send `pause`, expect outbound `kodik_player_pause` event
7. Send `seek(60)`, expect outbound `kodik_player_seek` event with `value=60` (approximate)

If this canary fails: alert in the daily CI report. The RPC has been removed/changed by Kodik.

### Two-browser sync e2e — `e2e/watch-together-sync.spec.ts`
One spec, parameterized over each of the 5 players. For each: spawn 2 browser contexts, invite, send play → assert second context plays within 500ms, send pause → assert second pauses, send seek → assert second seeks.

### Metrics (frontend telemetry placeholder)
- Phase 5 will wire prometheus-from-browser; Phase 3 ships console.debug logs only

### Claude's Discretion
- Toast library choice (use the project's existing one)
- Specific debounce intervals for time_tick (suggest 2s default, configurable)
- Smooth-correction interpolation curve (linear ramp vs ease) — your call
- File-level organization (one big bridge composable vs split helpers)

</decisions>

<canonical_refs>
## Canonical References

### Source design + requirements
- `docs/superpowers/specs/2026-05-25-watch-together-design.md` (message protocol)
- `.planning/workstreams/watch-together/REQUIREMENTS.md` (WT-SYNC-01..10)
- `.planning/workstreams/watch-together/phases/02-frontend-shell/02-PHASE-SUMMARY.md` (what Phase 2 delivered)

### Memory (CRITICAL for Kodik adapter)
- `/root/.claude/projects/-data-animeenigma/memory/reference_kodik_inbound_postmessage_api.md` — Full RPC documentation, method table, security posture, anchor lines

### Frontend exemplars
- `frontend/web/src/composables/useWatchTogetherRoom.ts` — Room handle interface; what bridge composable consumes
- `frontend/web/src/components/player/KodikPlayer.vue:280` — Existing `handleKodikMessage` handler (extend, don't replace)
- `frontend/web/src/components/player/AnimeLibPlayer.vue` — HTML5 `<video>` reference player (apply bridge)
- `frontend/web/src/components/player/OurEnglishPlayer.vue` — Same
- `frontend/web/src/components/player/HanimePlayer.vue` — Same
- `frontend/web/src/components/player/RawPlayer.vue` — Same

### Backend (already exposes the sync protocol)
- `services/watch-together/internal/handler/inbound_router.go` (or service/inbound.go) — handles playback:play/pause/seek/time_tick + drift correction
- `services/watch-together/internal/service/sync.go` — DriftEngine

### Backend metrics (Phase 1 baseline)
- `wt_drift_corrections_total{severity}` — already emitted in Phase 1; Phase 5 wires Grafana panel

</canonical_refs>

<specifics>
## Specific Ideas

### Drift correction subtle UX
- Soft correction (1.5s < drift < 5s): adjust playback rate to 0.97 or 1.03 for ~5s until drift closes. User barely notices.
- Hard correction (drift > 5s): direct `currentTime` jump. Show small toast "Re-syncing with room"

### Member username lookup for toasts
`room.members` is a `Member[]` with `userId` + `username`. Build a `Map<userId, username>` for O(1) lookup. Update when `member:joined` / `member:left` events fire.

### Connection-status overlay
Mount in `WatchTogetherView.vue` (NOT each player) — a single overlay on the whole view that responds to `room.connectionStatus`.

</specifics>

<deferred>
## Deferred Ideas

- Episode/player/translation switching propagation (Phase 4)
- Reaction burst polish (Phase 5)
- Mobile bottom-sheet (Phase 5)
- Grafana frontend metrics panel (Phase 5)

</deferred>

---

*Phase: 03-player-sync*
*Context auto-generated: 2026-05-25 via workflow.skip_discuss*
