---
phase: 09-download-triggers
plan: 03
subsystem: player
tags: [autocache, fire-and-forget, producer, http-internal, docker-network, logic-b, next-ep, jp-audio, heartbeat, trig-02]
requires:
  - phase: 09-01
    provides: "Demand handler honors validated wire reason (next_ep/ongoing/backfill); autocache_demand_reason enum accepts 'next_ep'; /internal/library/autocache/demand Docker-network-only route"
  - phase: 08-02
    provides: "POST /internal/library/autocache/demand {mal_id, episode, reason} endpoint shape"
provides:
  - "DemandProducer — fire-and-forget player→library autocache demand producer (buffered chan cap 256 + single worker, 3s timeout, drop-on-full WARN, nil/!enabled no-op, Start/Stop lifecycle)"
  - "ProgressRepository.LogicBContext — one query → (shikimori_id, episodes_aired, watching) keyed by (user_id, anime_id); not-watching/absent → watching=false, nil err"
  - "isJPAudio(player, language) — Player∈{ae,raw} OR Language=='ja' JP-audio gate"
  - "UpdateProgress Logic-B fire point — Want(shikimoriID, N+1, next_ep) on JP-audio + watching + N+1<=episodes_aired, best-effort (never blocks/fails the heartbeat)"
  - "AutocacheConfig{LibraryURL via LIBRARY_SERVICE_URL, DemandEnabled via AUTOCACHE_DEMAND_ENABLED}"
affects:
  - "09-02 library Planner — consumes the next_ep demands this producer fires (drains autocache_demand, attributes trigger=B)"
tech-stack:
  added: []
  patterns:
    - "RecsHintProducer-clone fire-and-forget producer: buffered channel + single worker, 3s HTTP timeout, drop-on-full WARN, nil/!enabled no-op, Start/Stop drained on shutdown via main.go srv.Shutdown() ordering"
    - "Narrow seam interfaces (progressUpserter/logicBLookup/demandFirer) over a concrete repo/producer so the heartbeat fire-path is unit-testable without a DB or a live library"
    - "JP-gate short-circuits the DB lookup: non-JP (ru/en) combos cost zero extra queries on the heartbeat hot path"
key-files:
  created:
    - services/player/internal/service/autocache_demand.go
    - services/player/internal/service/autocache_demand_test.go
    - services/player/internal/service/progress_test.go
  modified:
    - services/player/internal/service/progress.go
    - services/player/internal/repo/progress.go
    - services/player/internal/config/config.go
    - services/player/cmd/player-api/main.go
    - services/player/internal/handler/viewer_context_test.go
key-decisions:
  - "Per-heartbeat fire with no in-memory dedup guard — the library autocache_demand composite PK collapses repeated N+1 wants (RESEARCH Open-Q1 'rely on the PK dedup — cheapest')"
  - "JP-audio gate evaluated BEFORE the repo lookup so ru/en heartbeats incur zero extra DB cost"
  - "LogicBContext returns watching=false (not an error) on an absent/non-watching row; the fire-path treats it as a clean no-op"
  - "Seam interfaces hold the same concrete *ProgressRepository / *DemandProducer in production; introduced solely for DB-free unit testing of the fire-path"
  - "Reused the existing compose var LIBRARY_SERVICE_URL (already set on player) rather than adding a new LIBRARY_INTERNAL_URL"
patterns-established:
  - "Pattern 1: a player-side fire-and-forget producer for a Docker-network-only /internal/* sink, cloned verbatim from RecsHintProducer, so the latency of the sink never reaches the user request path"
  - "Pattern 2: best-effort side-effect at the END of a hot-path service method — gated cheapest-first, lookup errors logged WARN and swallowed, return value provably unchanged"
requirements-completed: [TRIG-02]
duration: ~12min
completed: 2026-06-17
---

# Phase 9 Plan 03: Player Logic B — next-episode autocache pull Summary

**A fire-and-forget `DemandProducer` (RecsHintProducer clone) wired into `UpdateProgress`: when an active JP-audio watcher (`Player∈{ae,raw}` OR `lang=ja`) sends a progress heartbeat for episode N of a `watching` anime, the player fires a `next_ep` demand for N+1 (if aired) to the library autocache endpoint — drop-on-full, non-blocking, and provably unable to block or fail the heartbeat.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-06-17T09:54:00Z
- **Completed:** 2026-06-17T10:04:00Z
- **Tasks:** 3 (2 TDD)
- **Files modified:** 8 (3 created, 5 modified)

## Accomplishments
- `DemandProducer` — a verbatim clone of the proven `RecsHintProducer` fire-and-forget pattern (buffered channel cap 256 + single worker, 3s HTTP timeout, drop-on-full WARN, nil/!enabled no-op, Start/Stop lifecycle) that POSTs `{mal_id, episode, reason}` to `/internal/library/autocache/demand`. Sends `reason="next_ep"` on the wire — honored as-is by the Plan-09-01 demand handler.
- Logic-B fire point in `UpdateProgress`: `repo.LogicBContext` resolves `(shikimori_id, episodes_aired, watching)` in one query, and the service fires `Want(shikimoriID, N+1, "next_ep")` ONLY when the combo is JP-audio AND the anime is `status=watching` AND `shikimori_id` is non-empty AND `N+1 <= episodes_aired`. Every other branch (ru/en combo, non-watching list status, over-aired N+1, empty shikimori_id, lookup error) fires nothing.
- The fire is best-effort: a lookup error or a slow/down library logs WARN and leaves `UpdateProgress`'s result unchanged — the heartbeat never regresses (proven by `TestUpdateProgress_LookupErrorDoesNotFailHeartbeat`).
- Config + boot DI: `AutocacheConfig` reads `LIBRARY_SERVICE_URL` (default `http://library:8089`) + `AUTOCACHE_DEMAND_ENABLED` (default on); `main.go` constructs the producer, `Start()`/`defer Stop()` beside `recsHintProducer`, and threads it into `NewProgressService`.

## Task Commits

Each task was committed atomically (TDD tasks split test → feat):

1. **Task 1: DemandProducer (fire-and-forget player→library)** - `c4f3db5b` (test, RED) → `dda87592` (feat, GREEN)
2. **Task 2: Logic-B lookup + UpdateProgress fire point** - `425dd98c` (test, RED) → `204d49e8` (feat, GREEN)
3. **Task 3: Config + main.go DI** - `ee243c47` (feat)

**Plan metadata:** committed in this SUMMARY commit (docs).

## Files Created/Modified
- `services/player/internal/service/autocache_demand.go` - `DemandProducer` + `demandMsg` + `demandChanCap`; clones recs_hint.go incl. the shutdown-ordering comment
- `services/player/internal/service/autocache_demand_test.go` - httptest body capture (`reason=next_ep`/mal_id/episode), drop-on-full, nil/!enabled no-op, non-2xx-does-not-error
- `services/player/internal/repo/progress.go` - `LogicBContext(ctx, userID, animeID) → (shikimoriID, episodesAired, watching, err)` (anime_list × animes JOIN, watching-only)
- `services/player/internal/service/progress.go` - `isJPAudio`, seam interfaces, `maybeFireNextEpDemand` fire point at the end of `UpdateProgress`; defensive nil-guard on `prefService`
- `services/player/internal/service/progress_test.go` - fire + each no-fire branch, lookup-error-doesn't-fail-heartbeat, nil-demand-safe, isJPAudio table
- `services/player/internal/config/config.go` - `AutocacheConfig` block + Load wiring
- `services/player/cmd/player-api/main.go` - construct/Start/Stop `demandProducer`, thread into `NewProgressService`
- `services/player/internal/handler/viewer_context_test.go` - updated `NewProgressService` call site (nil demand)

## Decisions Made
See `key-decisions` frontmatter. Headline: per-heartbeat fire with no in-memory guard (the library demand PK absorbs repeats); the JP-audio gate runs BEFORE the repo lookup so ru/en heartbeats cost nothing extra; reused the existing `LIBRARY_SERVICE_URL` compose var.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Nil-guard on `prefService` in `UpdateProgress`**
- **Found during:** Task 2 (service tests)
- **Issue:** `UpdateProgress` called `s.prefService.UpsertAnimePreference` whenever `req.Player != ""`. The new DB-free unit tests construct the service without a `PreferenceService`, so a JP-audio combo (which always sets `Player`) panicked on a nil `*PreferenceService` before reaching the Logic-B fire point. In production `prefService` is always non-nil, but the unguarded deref is a latent nil-panic.
- **Fix:** Guarded the call with `&& s.prefService != nil`. No production behavior change (prefService is always wired in `main.go`); makes the hot-path method robust to a missing collaborator.
- **Files modified:** `services/player/internal/service/progress.go`
- **Verification:** `go test ./internal/service/... -run Progress` green.
- **Committed in:** `204d49e8` (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug — latent nil-deref hardening).
**Impact on plan:** Minimal — a defensive nil-guard required for the planned DB-free fire-path tests; no scope creep, no production behavior change.

## Issues Encountered
- **Out-of-scope pre-existing failure:** `TestMALExportHandler_GetUserExports_Authorized` (`services/player/internal/handler/mal_export_test.go:118`) fails in the sandbox with a 500 `EXTERNAL_API_ERROR "scheduler service"` — the test issues a real network call to the scheduler service, which is unreachable in this isolated environment. NOT caused by 09-03 (Logic B touches service/repo/config/main only; no MAL-export or scheduler-client code). Logged to `.planning/phases/09-download-triggers/deferred-items.md` per the SCOPE BOUNDARY rule. The plan-scoped verification packages (`internal/service`, `internal/repo`) are green.

## Threat Surface
All threat-register `mitigate` dispositions were satisfied by the planned design:
- **T-09-08** (spoofed combo forcing a demand): the demand fires only when the anime is `status=watching` for the authenticated user (server-side `LogicBContext` lookup) AND `N+1 <= episodes_aired` — a spoofed combo on a non-watched / over-aired episode fires nothing.
- **T-09-09** (per-heartbeat flood): drop-on-full (channel cap 256) + the library `autocache_demand` composite-PK dedup collapse repeated N+1 wants to one row.
- **T-09-10** (slow/down library blocking the heartbeat): the async producer returns immediately (3s client timeout on the worker, not the caller); the heartbeat never waits on the POST.
- **T-09-SC** (package installs): none — stdlib `net/http` + the existing logger only; zero new deps.

No new security-relevant surface introduced beyond the planned `<threat_model>`.

## User Setup Required
None - no external service configuration required. `LIBRARY_SERVICE_URL` is already set on the player service in `docker/docker-compose.yml`; `AUTOCACHE_DEMAND_ENABLED` defaults on.

## Next Phase Readiness
- Logic B is live: an active JP-audio watcher starting episode N now fires a `next_ep` demand for N+1 (when aired) into the autocache pipeline. Plan 09-02's library Planner drains it and attributes `trigger=B`.
- Plan 09-04 (scheduler Logic A → `ongoing`) is independent and unblocked.
- No blockers.

## Self-Check: PASSED

- `services/player/internal/service/autocache_demand.go` — FOUND
- `services/player/internal/service/progress.go` (Logic-B fire point) — FOUND
- `services/player/internal/repo/progress.go` (LogicBContext) — FOUND
- `services/player/internal/config/config.go` (AutocacheConfig) — FOUND
- `.planning/phases/09-download-triggers/09-03-SUMMARY.md` — FOUND
- Commits `c4f3db5b`+`dda87592` (T1 RED/GREEN), `425dd98c`+`204d49e8` (T2 RED/GREEN), `ee243c47` (T3) — all present in branch history
- `cd services/player && go build ./... && go vet ./...` — BUILD-OK / VET-OK
- `go test ./internal/service/... ./internal/repo/... -count=1` — TEST-OK (plan-scoped packages green; handler MAL-export network failure is pre-existing + out-of-scope)

---
*Phase: 09-download-triggers*
*Completed: 2026-06-17*
