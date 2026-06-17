---
phase: 10-eviction-budget
plan: 02
subsystem: infra
tags: [go, prometheus, library-autocache, eviction, budget, ticker, mutex]

# Dependency graph
requires:
  - phase: 10-eviction-budget
    provides: "Plan-01 primitives: EpisodeRepository.SumPoolBytes / ListStaleEvictionCandidates(4-tier order) / DeleteByID, minio.Writer.DeletePrefix, LibraryMetrics evicted/rejected counters + bytes_used/budget_bytes/episodes gauges"
  - phase: 09-download-triggers
    provides: "autocache.Planner Start/Stop/loop/runOnce/sleep ticker skeleton + configGetter/plannerLogger local-interface-seam idiom + minSweepInterval const"
provides:
  - "autocache.Evictor: EnsureRoom (budget arithmetic + ordered Stale eviction → admitted bool), pure Classify(ep,cfg,now)→Fresh|Stale, evictOne (object-then-row), Start/Stop/loop/runOnce/Sweep lifecycle, Accountant gauge refresh"
  - "Two new local-interface seams (poolAccountant, objectDeleter) the Evictor composes; configGetter/plannerLogger REUSED from planner.go (no redeclare)"
  - "ensureRoomLocked: one factored reclaim core shared by EnsureRoom (pre-admit) and Sweep, serialized by ONE e.mu so freed headroom is never double-spent"
  - "EpisodeRepository.ListPool — aeProvider/ pool list the Accountant Classify-buckets per (source,freshness)"
  - "LibraryMetrics GetBytesUsed/GetBudgetBytes/GetEpisodes ForTest gauge seams"
affects: [10-03-pre-admit, 11-grafana-panels]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Factored lock-free reclaim core (ensureRoomLocked) shared by a locking public method (EnsureRoom) and an already-locked caller (Sweep) — one mutex, no double-spend, no re-entrant lock"
    - "Accountant gauge publishing: list the pool once + pure-Go Classify-bucket per (source,freshness), Set ALL four buckets incl zeros so an emptied bucket resets"
    - "Evictor mirrors Planner ticker lifecycle verbatim (Start/Stop/loop/runOnce/sleep) sharing the package configGetter/plannerLogger seams + minSweepInterval const"

key-files:
  created:
    - services/library/internal/autocache/evictor.go
    - services/library/internal/autocache/evictor_test.go
  modified:
    - services/library/internal/repo/episode.go
    - services/library/internal/metrics/library_metrics.go

key-decisions:
  - "ensureRoomLocked factored out so Sweep's proactive reclaim and EnsureRoom's pre-admit share ONE lock + ONE eviction loop — the mutex is held across the entire SumPoolBytes→ListCandidates→evict cycle in both paths (T-10-04 no double-spend)."
  - "Sweep's proactive reclaim reuses EnsureRoom semantics with estBytes=0 (\"is the pool currently within budget? evict Stale until it is\") — keeps the budget math in one place."
  - "Accountant lists the full pool (new ListPool repo method) and Classify-buckets in Go rather than adding per-(source,freshness) SQL — the freshness rule stays in the single pure-Go Classify helper that the unit tests pin, avoiding a second copy of the windows in SQL."
  - "publishAccountantGauges Sets all four (source × freshness) buckets including the zero buckets so a bucket that emptied since the last sweep resets to 0 instead of retaining a stale gauge value."
  - "Unknown EpisodeSource defaults to the admin freshness window (longest, safest) in both Classify and the Accountant bucketing — an unexpected enum value is never silently dropped from the gauges."

patterns-established:
  - "Re-entrant-safe mutex discipline: public EnsureRoom does mu.Lock()+ensureRoomLocked; Sweep holds mu and calls ensureRoomLocked directly — never a double-lock, one serialization point for pre-admit vs sweep."
  - "Evictor unit idiom (mirrors planner_test): handwritten fakeAccountant/fakeDeleter/fakeConfig seam fakes + a fresh prometheus.NewRegistry() per case; runOnce/Sweep/EnsureRoom driven directly, no ticker, no Postgres/MinIO."

requirements-completed: [EVICT-01, EVICT-02, EVICT-03, EVICT-04]

# Metrics
duration: ~10min
completed: 2026-06-17
---

# Phase 10 Plan 02: Evictor Summary

**`autocache.Evictor` — the budget gate (EnsureRoom: ordered Stale eviction until an incoming estimate fits, else admitted=false), the source-specific Fresh/Stale rule (pure Classify), object-then-row eviction, and a config-gated periodic Sweep that proactively reclaims newly-Stale rows and publishes the bytes_used/budget_bytes/episodes Accountant gauges — all serialized by ONE mutex so pre-admit and sweep never double-spend freed headroom.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-06-17T11:04Z
- **Completed:** 2026-06-17T11:11Z
- **Tasks:** 2
- **Files modified:** 4 (2 created, 2 modified)

## Accomplishments
- `autocache.Evictor` composes the Plan-01 repo/minio/metrics primitives via two new local-interface seams (`poolAccountant`, `objectDeleter`), reusing the package's existing `configGetter`/`plannerLogger` seams with no redeclaration — the package still imports no repo/service/minio package.
- `EnsureRoom(estBytes)` holds `e.mu` across the whole read→evict cycle, evicts Stale rows in the SQL-locked 4-tier order (via `ListStaleEvictionCandidates`) until the estimate fits, and returns `admitted=false` on queue exhaustion (EVICT-04). `evictOne` deletes MinIO objects FIRST and the row only on success; an object-delete failure skips the row (no orphaned serving pointer), a row-delete failure isn't counted as a reclaim.
- Pure `Classify(ep,cfg,now)→Fresh|Stale` implements the source-specific windows (autocache: <auto_fresh_download_days OR <auto_fresh_fetch_days; admin: <admin_fresh_days for both) including NULL downloaded_at (classify by last_fetch_at only) and NULL last_fetch_at (never fetched).
- A config-gated `Sweep` (Start/Stop/loop/runOnce/sleep mirroring the Planner) reclaims over-budget Stale via the shared `ensureRoomLocked(ctx,0)` core, then refreshes `bytes_used{source,freshness}` / `budget_bytes` / `episodes{source,freshness}` by listing the pool once and Classify-bucketing — so Phase 11 has live series even when no download flows. `!cfg.Enabled` halts eviction too (POOL-05 / T-10-06).
- 17 handwritten-fake unit tests (race-clean): Classify across both sources × NULL/in-window/out-of-window; EnsureRoom fits-no-evict / evict-in-tier-order / early-break / queue-exhausted-reject / object-delete-fail-skip / row-delete-fail-skip; runOnce disabled-no-op + cadence-floor; Sweep reclaim + gauge publish (incl zeroed empty buckets) + within-budget-still-publishes; Start→Stop clean exit.

## Task Commits

Each task was committed atomically:

1. **Task 1: Evictor struct + seams + Classify + EnsureRoom + evictOne** - `7f69ec07` (feat)
2. **Task 2: Evictor lifecycle (Start/Stop/loop/runOnce/Sweep) + Accountant gauge refresh** - `a7a55959` (feat)

_Note: TDD tasks were committed as single feat commits (implementation + co-located handwritten-fake unit tests landed together, matching the package's existing test style established in Plan 01)._

## Files Created/Modified
- `services/library/internal/autocache/evictor.go` (created) - The Evictor: `poolAccountant`/`objectDeleter` seams, `Freshness` type + consts, pure `Classify`, `Evictor` struct + `NewEvictor`, `EnsureRoom`/`ensureRoomLocked`/`evictOne`, `Start`/`Stop`/`loop`/`runOnce`/`Sweep`/`publishAccountantGauges`/`sleep`.
- `services/library/internal/autocache/evictor_test.go` (created) - 17 tests with `fakeAccountant`/`fakeDeleter` seam fakes + fresh registries per case.
- `services/library/internal/repo/episode.go` (modified) - Added `ListPool(ctx)` — the aeProvider/ pool list the Accountant buckets (Plan-01 provided SumPoolBytes/candidates/DeleteByID but not a full-pool list).
- `services/library/internal/metrics/library_metrics.go` (modified) - Added `GetBytesUsedForTest`/`GetBudgetBytesForTest`/`GetEpisodesForTest` gauge test seams (the file already had the Set methods + collectors from Plan 01; only the read seams were missing).

## Decisions Made
See `key-decisions` in frontmatter. The load-bearing one: `ensureRoomLocked` is factored as a lock-free reclaim core so EnsureRoom (pre-admit, takes the lock) and Sweep (already holds the lock) share ONE mutex and ONE eviction loop — satisfying T-10-04 (no double-spend of freed headroom) without a re-entrant lock and without duplicating the budget arithmetic.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added `EpisodeRepository.ListPool` for the Accountant gauges**
- **Found during:** Task 2 (Sweep / Accountant gauge refresh)
- **Issue:** The Accountant must bucket the pool per (source, freshness) to publish `bytes_used`/`episodes`, but Plan 01 provided only `SumPoolBytes` (total), `ListStaleEvictionCandidates` (Stale-only), and `DeleteByID` — no full-pool list. Without a pool list the Accountant couldn't classify-bucket every row (Fresh rows are absent from the Stale candidate query).
- **Fix:** Added a small pool-scoped `ListPool(ctx)` (`minio_path LIKE 'aeProvider/%'`, `created_at ASC`) mirroring the existing `ListAdminLegacyPath` idiom, and added it to the `poolAccountant` seam. The plan's own Task-2 `<action>` explicitly recommended this exact seam ("RECOMMEND: list the pool once (a `ListPool(ctx) []Episode` seam ...)").
- **Files modified:** services/library/internal/repo/episode.go, services/library/internal/autocache/evictor.go
- **Verification:** `go build ./... && go vet ./... && go test ./internal/repo/... ./internal/autocache/...` all green; the Accountant gauge tests assert the bucketed values.
- **Committed in:** `a7a55959` (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added gauge `GetXForTest` read seams**
- **Found during:** Task 2 (Sweep gauge assertions)
- **Issue:** Plan 01 added the gauge `Set` methods but no `GetXForTest` read seams for `bytes_used`/`budget_bytes`/`episodes`, so the Accountant sweep couldn't be unit-asserted (the package convention is a `GetXForTest` seam returning the raw collector for `testutil.ToFloat64`).
- **Fix:** Added `GetBytesUsedForTest`/`GetBudgetBytesForTest`/`GetEpisodesForTest` matching the file's existing `GetXForTest` convention.
- **Files modified:** services/library/internal/metrics/library_metrics.go
- **Verification:** Sweep tests read the gauges via the new seams; full `go test ./...` green.
- **Committed in:** `a7a55959` (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 missing-critical test seam)
**Impact on plan:** Both are small, in-scope library additions the plan's own Task-2 action text anticipated. No scope creep — no pre-admit wiring (10-03), no Grafana (Phase 11), no Planner/handler/main.go changes.

## Issues Encountered
None — the Plan-01 primitives matched the planned seam shapes exactly, so the Evictor composed cleanly. `gofmt` reformatted the test `ptr` helper alignment (cosmetic, kept).

## User Setup Required
None - no external service configuration required. The Evictor is not yet wired into main.go (that is Plan 03's pre-admit wiring), so this plan ships no runtime behavior change.

## Next Phase Readiness
- **Plan 03 (pre-admit wiring)** can now inject `*autocache.Evictor` into the Planner `plan()` and the admin `JobsHandler.Create` via an `EnsureRoom(ctx, estBytes) (bool, error)` seam, call `evictor.Start(rootCtx)` in main.go for the sweep, and `IncRejectedTotal("budget_full")` at the call site on `!admitted` (EVICT-04 reject signal). The Evictor reads cfg + freshness windows live each call, so an admin PATCH applies with no redeploy.
- **Phase 11 (Grafana)** has the live `library_autocache_bytes_used{source,freshness}` / `_budget_bytes` / `_episodes{source,freshness}` gauges + `_evicted_total{source}` counter emitted by the Sweep once the Evictor is started.
- `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` all clean (also `-race` clean on the autocache package). No new go.mod dependency.

## Self-Check: PASSED

- Files: evictor.go, evictor_test.go present; episode.go + library_metrics.go modified.
- Commits: 7f69ec07, a7a55959 both in git history on branch worktree-agent-a2d3c31b76f3bffd5.
- Verification: `go build ./...`, `go vet ./...`, `go test ./... -count=1` clean; `go test ./internal/autocache/... -race` clean; 17 Evictor tests pass; no go.mod change.

---
*Phase: 10-eviction-budget*
*Completed: 2026-06-17*
