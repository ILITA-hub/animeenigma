# Phase 5 — Plan 01 — Summary

**Completed (code):** 2026-05-03
**Status:** ✓ Implementation complete; ⏳ Wave 2 batch deploy + production verification pending

## One-liner

Added the three top-priority gap-fill columns/events from the Phase 2 audit
(`docs/analytics-audit-2026-04-28.md`):

- **G-02 — rewatch detection**: new `watch_progress.watch_count` column.
  `MarkCompleted` increments only when the existing row was already
  `completed=true`, so a fresh first-time finish leaves it at 1 and a rewatch
  bumps it to 2, 3, …
- **G-04-lite — session correlation**: new `watch_history.session_id` column +
  optional `session_id` field on the heartbeat / mark-watched payloads.
  Frontend generates one UUID per playback session (rotates on episode
  change) so Tier 2 can fold heartbeats from the same session together
  without double-counting binge-watch behavior.
- **G-01 — drop-off beacon**: new `watch_progress.dropped_off_at` column +
  new `POST /api/users/progress/{animeId}/dropoff` endpoint. Frontend wires
  `navigator.sendBeacon` on `pagehide` / `visibilitychange:hidden` for the 3
  HTML5-video players (Kodik can't fire it — iframe). G-03 (trajectory) and
  G-05 (intro/outro) deferred per Phase 2 candidate-lock.

Additive-only schema change (per project compatibility constraint). All four
players continue to work unchanged for users without the new fields — empty
session_id and null dropped_off_at are valid.

## What changed

| Layer | File | Change |
|---|---|---|
| Domain | `services/player/internal/domain/watch.go` | `WatchProgress` += `WatchCount`, `DroppedOffAt`; `WatchHistory` += `SessionID`; `UpdateProgressRequest` + `MarkEpisodeWatchedRequest` += `SessionID`; new `DropOffRequest` type |
| Backend repo | `services/player/internal/repo/progress.go` | `MarkCompleted` rewatch-detection via SQL CASE expression; new `MarkDropOff(ctx, userID, animeID, ep, secs)` method |
| Backend service | `services/player/internal/service/progress.go` | New `MarkDropOff(ctx, userID, animeID, *DropOffRequest)` |
| Backend service | `services/player/internal/service/list.go` | `WatchHistory` row populates `SessionID: req.SessionID` |
| Backend handler | `services/player/internal/handler/progress.go` | New `MarkDropOff(w, r)` — beacon-tolerant body parsing, errors logged not surfaced |
| Backend transport | `services/player/internal/transport/router.go` | New route `POST /progress/{animeId}/dropoff` (under existing `/users` auth scope) |
| Frontend composable | `frontend/web/src/composables/useWatchSession.ts` | NEW — generates session UUID, exposes `sendDropOffBeacon` + `registerBeaconHooks` (sendBeacon w/ keepalive-fetch fallback) |
| Frontend API | `frontend/web/src/api/client.ts` | `markEpisodeWatched(..., sessionId?)` accepts optional session_id; `getProgress(animeId)` added (also used by Phase 4) |
| Frontend players | `KodikPlayer.vue`, `AnimeLibPlayer.vue`, `HiAnimePlayer.vue`, `ConsumetPlayer.vue` | Each calls `useWatchSession`, passes `sessionId.value` into heartbeat + completion mark; the 3 HTML5 players register beacon hooks (Kodik skipped — iframe has no time) |
| Tests | `services/player/internal/repo/progress_test.go` | +5 tests: WatchCount on first completion / heartbeat-flip / rewatch + DropOff create-row + DropOff preserve-completed |
| Tests | `services/player/internal/service/list_mark_completed_test.go` | +1 test: SessionID round-trips into watch_history.session_id |

## Test results

```
ok  github.com/ILITA-hub/animeenigma/services/player/internal/handler   0.019s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/repo      0.034s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service   0.021s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/transport 0.007s
```

Frontend `bunx tsc --noEmit` clean. ESLint: 0 errors, 3 pre-existing warnings
in untouched files (AnimeContextMenu.vue, Profile.vue).

## Success criteria status

| SC | Status | Evidence |
|---|---|---|
| 1. New columns added via AutoMigrate; populate on prod traffic | ✓ Code complete; ⏳ verifies on first deploy | Domain struct tags drive AutoMigrate |
| 2. session-start vs session-resume distinguishable | ✓ Code complete | Frontend rotates session_id on episode change; backend persists into watch_history.session_id |
| 3. Each new column has a one-line consumer note | ✓ | Inline comments on each struct field reference Phase 6 / future recs / debugging |
| 4. Additive-only — no drop or rename | ✓ | All changes are new columns or new fields on request types |

## What's next

1. **Right now:** Commit Phase 5 (single commit: backend + tests + frontend).
2. **Wave 2 batch deploy:** Phase 4 must commit too, then `/animeenigma-after-update` redeploys `player` + `web`.
3. **Production verification:** Inspect first traffic for non-empty `session_id` rows in watch_history; spot-check `dropped_off_at` after a manual close-mid-episode test.
4. **Phase 6 dependency:** `watch_count`, `session_id`, and `dropped_off_at` are inputs to Tier 2 weighted aggregation — Phase 6 query will reference them.
