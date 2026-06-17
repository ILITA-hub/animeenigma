---
phase: 10-eviction-budget
plan: 03
subsystem: infra
tags: [go, prometheus, library-autocache, eviction, budget, pre-admit, http-507, planner, handler]

# Dependency graph
requires:
  - phase: 10-eviction-budget
    provides: "Plan-02 autocache.Evictor: EnsureRoom(ctx, estBytes) (admitted, err) budget gate + Start/Stop sweep lifecycle; NewEvictor(config, pool, objects, libMetrics, log)"
  - phase: 09-download-triggers
    provides: "Plan-02 autocache.Planner plan() enqueue path (selectRAW → jobs.Create) + searchBackoff machinery (markSearched/inBackoff/demandKey); Plan-01 LibraryMetrics IncRejectedTotal/IncDownloadsTotal"
provides:
  - "Planner pre-admit budget gate: EnsureRoom(estBytes) BEFORE jobs.Create; reject → IncRejectedTotal(budget_full) + IncDownloadsTotal(trigger,rejected) + LEAVE demand + arm searchBackoff (no hot-loop); fail-open on read error"
  - "budgetEvictor seam on Planner (nil-allowed) + avgRawEpSize const (~1.2 GiB) pre-admit estimate fallback"
  - "JobsHandler EvictorAPI seam (nil-allowed) + second pre-admit gate in Create AFTER DiskGuard (both must pass, EVICT-05); reject → 507 + rejected_total{budget_full}; fail-open on read error"
  - "main.go: single Evictor constructed once, Started for the sweep, injected NON-NIL into BOTH NewJobsHandlerWithLink and NewPlanner, Stopped on graceful shutdown"
affects: [11-grafana-panels]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Two pre-admit consumers (admin upload handler + autocache Planner) gate the SAME Evictor.EnsureRoom via a per-package local seam each (EvictorAPI / budgetEvictor) — main.go owns the single concrete instance, the packages import no Evictor"
    - "Fail-open budget gate symmetric across both call sites: a transient EnsureRoom read error never 507s an admin upload nor silently drops an autocache download (mirrors the existing disk-guard fail-open)"
    - "Reject is asymmetric by path: admin → HTTP 507 (writeInsufficientStorage); Planner → LEAVE the demand row + arm the existing searchBackoff (markSearched) so a permanently-too-big pool isn't re-searched/re-rejected every tick"

key-files:
  created: []
  modified:
    - services/library/internal/autocache/planner.go
    - services/library/internal/autocache/planner_test.go
    - services/library/internal/handler/jobs.go
    - services/library/internal/handler/jobs_test.go
    - services/library/cmd/library-api/main.go

key-decisions:
  - "Hoisted autocacheConfigRepo construction ABOVE NewJobsHandlerWithLink (it was constructed ~20 lines lower) so the single Evictor can be built from it BEFORE either consumer (JobsHandler + Planner) is constructed — the plan-checker-flagged wiring foot-gun. Both consumers receive the identical non-nil *autocache.Evictor."
  - "avgRawEpSize is a Phase-10 const (1288490188 ≈ 1.2 GiB), NOT a config column — no migration this phase (per CONTEXT). Used only when a selected release reports SizeBytes <= 0."
  - "The Planner reject path returns searched=true (markSearched) so the per-sweep fan-out cap counts it AND the searchBackoff suppresses re-search — a too-big pool doesn't hot-loop the searcher (T-10-08)."
  - "Both gates fail OPEN on an EnsureRoom error (log warn, proceed) so a transient config/DB read blip never 500s/507s an admin upload nor drops an autocache download (T-10-10)."
  - "Legacy NewJobsHandler (no Link/Retry) keeps its evictor nil — only NewJobsHandlerWithLink (the constructor main.go uses) gains the EvictorAPI param, matching the existing diskGuard-everywhere / mover-WithLink-only split."

patterns-established:
  - "Per-consumer EnsureRoom seam: budgetEvictor (autocache pkg) + EvictorAPI (handler pkg) are byte-identical single-method interfaces; main.go's concrete *Evictor satisfies both, so neither package imports the other's wiring."
  - "EVICT-05 ordering in Create: DiskGuard (physical) first, then the budget gate (logical) — asserted by a dedicated test that the budget EnsureRoom is NOT called when DiskGuard rejects first."

requirements-completed: [EVICT-04, EVICT-05]

# Metrics
duration: ~10min
completed: 2026-06-17
---

# Phase 10 Plan 03: Pre-Admit Budget Gates Summary

**Both download paths — the autocache Planner's enqueue (`plan()` before `jobs.Create`) and the admin upload handler (`JobsHandler.Create` after `DiskGuard.Allow`) — now gate the SAME `Evictor.EnsureRoom(estimate)` layered ON TOP of the physical DiskGuard (EVICT-05); an unfittable download is rejected (admin → HTTP 507 + `rejected_total{budget_full}`; Planner → LEAVES the demand row + arms the `searchBackoff` so it never hot-loops), both gates fail open on a budget-read blip, and main.go owns the single Evictor instance: constructed once, swept on a ticker, injected non-nil into both consumers, stopped on shutdown.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-06-17T11:14Z
- **Completed:** 2026-06-17T11:20Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments
- **Planner pre-admit gate (Task 1):** a `budgetEvictor` seam (nil-allowed) + `evictor` field + `NewPlanner` param; in `plan()`, after `selectRAW` wins and BEFORE `jobs.Create`, the Planner computes `estBytes = rel.SizeBytes` (falling back to the `avgRawEpSize` ~1.2 GiB const when the release reports no size) and calls `EnsureRoom`. On reject it increments `rejected_total{budget_full}` + `downloads_total{trigger,rejected}`, LEAVES the demand row, and arms the existing `searchBackoff` (markSearched) so a too-big pool isn't re-searched every tick (T-10-08). Fail-open on a read error (mirrors the disk-guard fail-open).
- **Admin upload gate (Task 2):** an `EvictorAPI` seam (nil-allowed) threaded through `NewJobsHandlerWithLink`; a second gate in `Create` immediately AFTER the DiskGuard block — both must pass (EVICT-05). Reject → `IncRejectedTotal("budget_full")` + `writeInsufficientStorage` (HTTP 507); a budget-read error fails open (proceeds to 201).
- **main.go wiring (Task 3):** hoisted `autocacheConfigRepo` above the jobs-handler construction, built the SINGLE `autocache.NewEvictor(autocacheConfigRepo, episodeRepo, writer, libMetrics, log)` once, `evictor.Start(rootCtx)` for the sweep, injected the same non-nil evictor into BOTH `NewJobsHandlerWithLink` AND `NewPlanner`, and wired `evictor.Stop()` into graceful shutdown after `planner.Stop()`.
- **Tests:** 4 new Planner cases (budget-admitted enqueues + estimate=release size; budget-rejected leaves demand + metric + backoff armed; budget-error fails open; SizeBytes<=0 → avgRawEpSize fallback) and 4 new handler cases (both-gates-pass 201 + estimate=body.SizeBytes; budget-full 507 + metric; budget-error fails open 201; disk-full short-circuits BEFORE the budget gate). All existing Planner/handler cases preserved by threading a nil (Planner) or admit-by-default (handler) evictor through the shared helpers.

## Task Commits

Each task was committed atomically:

1. **Task 1: Planner pre-admit budget gate + budgetEvictor seam** - `3a889627` (feat)
2. **Task 2: Admin upload pre-admit gate — EvictorAPI seam in JobsHandler.Create** - `c8cc8939` (feat)
3. **Task 3: main.go — construct + Start Evictor + inject into Planner & JobsHandler + Stop** - `7f629551` (feat)

_Note: TDD tasks (1, 2) landed as single feat commits — implementation + co-located handwritten-fake tests together, matching the package's existing test style (Plans 01/02)._

## Files Created/Modified
- `services/library/internal/autocache/planner.go` (modified) - `budgetEvictor` seam, `avgRawEpSize` const, `evictor` field + `NewPlanner` param, the pre-admit gate (step 5) before the enqueue with fail-open + reject-and-backoff.
- `services/library/internal/autocache/planner_test.go` (modified) - `fakeEvictor` scriptable seam fake, `newPlannerWithEvictor` helper, 4 new gate cases; existing cases thread a nil evictor (gate skipped).
- `services/library/internal/handler/jobs.go` (modified) - `EvictorAPI` seam, `evictor` field, `NewJobsHandlerWithLink` param, the second pre-admit gate in `Create` after the DiskGuard block (507 on reject, fail-open on error).
- `services/library/internal/handler/jobs_test.go` (modified) - `fakeBudgetGuard` stub, `newBudgetHandler` helper, 4 new cases; `newLinkHandler` threads an admit-by-default budget guard so the Link/Retry cases stay green.
- `services/library/cmd/library-api/main.go` (modified) - hoisted `autocacheConfigRepo`; constructed + Started the single Evictor; injected into both consumers; `evictor.Stop()` in shutdown.

## Decisions Made
See `key-decisions` in frontmatter. The load-bearing one: `autocacheConfigRepo` was hoisted above `NewJobsHandlerWithLink` so the single Evictor is built before BOTH consumers, and the identical non-nil `*autocache.Evictor` is injected into the JobsHandler AND the Planner — verified by the whole-service `go build` (the changed `NewJobsHandlerWithLink`/`NewPlanner` signatures only compile when both call sites pass the constructed evictor) plus the `grep` of all three injection sites in main.go.

## Deviations from Plan

None - plan executed exactly as written.

The only minor friction: `git add` on `services/library/cmd/library-api/main.go` required `-f` because the sibling `library-api` binary path is gitignored (the directory name matches a gitignore pattern); the tracked `main.go` source file itself committed normally. Not a code deviation.

## Issues Encountered
One test-authoring miss, fixed before the Task-1 commit: the first draft of `TestPlannerBudgetFallbackEstimate` used a release without an allowlisted RAW uploader, so `selectRAW` filtered it out and `EnsureRoom` was never reached (the gate sits after `selectRAW`). Fixed by giving the zero-size release the `Ohys-Raws` uploader (matching the existing `winningRelease()` fixture) so it survives `isRAW` and exercises the fallback estimate. All tests green after the fix.

## User Setup Required
None - no external service configuration required. No new env var, no go.mod dependency, no migration (`avg_raw_ep_size` is a const this phase per CONTEXT). The Evictor reads config + freshness windows live each call, so an admin PATCH to the budget applies with no redeploy.

## Next Phase Readiness
- **Phase 11 (Grafana):** every download now flows through `EnsureRoom`, so `library_autocache_rejected_total{reason="budget_full"}` increments on a budget reject (admin 507 or Planner backoff), and the Evictor sweep (started in main.go) emits the live `bytes_used{source,freshness}` / `budget_bytes` / `episodes{source,freshness}` gauges + `evicted_total{source}` for the panels.
- `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` all clean; `go.mod` unchanged; no migration.

## Self-Check: PASSED

- Files: planner.go / jobs.go / main.go (+ both _test.go) all present and modified on disk.
- Commits: `3a889627`, `c8cc8939`, `7f629551` all in git history on branch `worktree-agent-a614217050a436abc`.
- Verification: `go build ./...`, `go vet ./...`, `go test ./... -count=1` all green (autocache + handler + repo + minio + metrics + service); acceptance greps pass (`budgetEvictor`/`EnsureRoom`/`IncRejectedTotal`/`avgRawEpSize` in planner.go; `EvictorAPI`/`EnsureRoom`/`IncRejectedTotal("budget_full")` in jobs.go; `autocache.NewEvictor`/`evictor.Start(rootCtx)`/`evictor.Stop()` in main.go); `git diff -- services/library/go.mod` empty; no file deletions across the 3 commits.

---
*Phase: 10-eviction-budget*
*Completed: 2026-06-17*
