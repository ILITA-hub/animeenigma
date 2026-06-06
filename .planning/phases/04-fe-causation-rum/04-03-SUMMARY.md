---
phase: 04-fe-causation-rum
plan: 03
subsystem: frontend-analytics
tags: [causation, trace_id, axios-interceptor, register, AR-FE-01, AR-FE-02]
requires:
  - "frontend/web/src/api/client.ts (existing traceparent+stampTrace interceptor block)"
  - "frontend/web/src/analytics/index.ts (analytics singleton track() emit)"
  - "frontend/web/src/analytics/types.ts (register wire fields added by Plan 04-02)"
  - "frontend/web/src/router/index.ts (default router singleton export)"
provides:
  - "source='fe' call register row emitted per axios request, carrying the call's trace_id/route/action/target"
  - "end-to-end click↔call trace-stamp proof (traceContext.spec.ts)"
affects:
  - "services/analytics collect handler (Plan 04-01) — ingests the source='fe' call rows this emits"
  - "FE→BE causation joins — clicks, FE calls, and BE effects now share one trace_id"
tech-stack:
  added: []
  patterns:
    - "Read the route inside an interceptor via router.currentRoute.value (router singleton — useRoute() throws in module scope)"
    - "Reuse the per-call traceId from newTraceparent() for both stampTrace and the FE call row (one mint, no second id)"
    - "Opt-in semantic action from axios config.meta.action so unlabeled fetches stay unlabeled"
    - "Coarse route/operation labels (route.name over fullPath; 'METHOD route') to bound register cardinality"
key-files:
  created: []
  modified:
    - frontend/web/src/api/client.ts
    - frontend/web/src/analytics/__tests__/traceContext.spec.ts
decisions:
  - "The FE call row reuses the SAME traceId minted by newTraceparent() that stampTrace back-fills onto the pending click — closing the FE→BE causation last mile with one trace_id (AR-FE-01/AR-FE-02)"
  - "Route read via router.currentRoute.value (RESEARCH Pitfall 4); prefer route.name over fullPath to bound cardinality (T-04-07)"
  - "Emission wrapped in try/catch and runs AFTER stampTrace — preserving the A4 flush-ordering invariant (no flushMs/maxBatch change)"
  - "traceContext.ts left unchanged (REUSE); Task 2 is an end-to-end proof of existing window-association behavior, not new code"
metrics:
  tasks_completed: 2
  files_created: 0
  files_modified: 2
  tests_added: 2
  completed: 2026-06-06
---

# Phase 4 Plan 3: FE Call Register Row + Causation Stamp Summary

Extended the existing axios request interceptor — the very block that already mints the W3C `traceparent` and calls `stampTrace(traceId)` — to ALSO emit a lightweight `source='fe'` register row carrying the call's `trace_id`, current route, optional semantic `action`, and the API path as `target`. Because the row reuses the SAME `traceId` that `stampTrace` back-fills onto the pending click, the click, the FE call row, and the downstream BE effect rows now all share one `trace_id`, closing the FE→BE causation last mile (AR-FE-01) and giving the click its trace linkage (AR-FE-02).

## What Was Built

### Task 1 — Emit the FE call register row from the interceptor (`feat`, commit `ba6e4499`)
Inside the existing `if (TRACING_ON)` block in `client.ts`, AFTER `stampTrace(traceId)`, added one `analytics.track('fe.call', { source: 'fe', trace_id: traceId, route, action, target, target_kind: 'route', operation })` emit:
- **Imports added:** `analytics` from `@/analytics` (the singleton emit seam) and the `router` default-export singleton from `@/router`.
- **Route read via the singleton:** `router.currentRoute.value` at request time — never `useRoute()`, which throws in interceptor/module scope (RESEARCH Pitfall 4). The coarse `routeLabel` prefers the pattern-like `route.name` over the concrete `route.fullPath` to bound register cardinality (T-04-07).
- **Opt-in action:** `(config as ...).meta?.action` — set only when a caller passes a semantic label on the axios config, so poster/poll fetches stay unlabeled (AR-FE-01 "optional semantic action").
- **target / operation:** `target = config.url` (the API path, tagged `target_kind: 'route'`); `operation` is a coarse `${METHOD} ${routeLabel}` label.
- **Same trace_id:** reuses the `traceId` destructured from the single `newTraceparent()` call — no second mint — which is exactly what links the call row to the stamped click and the downstream BE effects.
- **Never throws into the request path:** the whole emit is wrapped in `try/catch`; `track` is also a no-op before `analytics.init()`.
- **A4 ordering preserved:** the emit happens AFTER `stampTrace` and no `flushMs`/`maxBatch` was shortened, so the in-place click stamp still lands before flush ships the click.

### Task 2 — Assert click↔call trace stamp end-to-end (`test`, commit `35f5bae6`)
Extended `traceContext.spec.ts` with an `AR-FE-02` describe block proving the call's `trace_id` is back-filled onto the click that triggered it:
- **Positive:** a click registered at `t0`, then `stampTrace(CALL_TRACE_ID, 1500, t0+800)` (in-window) → `click.trace_id === CALL_TRACE_ID`.
- **Negative:** the same click, then `stampTrace(CALL_TRACE_ID, 1500, t0+2000)` (past the 1500ms window) → `click.trace_id` stays `undefined`.
- `traceContext.ts` was left unchanged (REUSE per the plan); the test is an end-to-end proof of the existing window-association behavior.

## Verification

- `bunx vitest run src/analytics/__tests__/traceContext.spec.ts` → 5/5 pass.
- `bunx vitest run src/analytics/` (full suite) → 40/40 pass across 8 files (no regressions; +2 over Plan 02's 38).
- `bunx tsc --noEmit` → clean (exit 0).
- `grep -n "router.currentRoute" src/api/client.ts` → present (router singleton, not useRoute).
- `grep -nE "track\(.*fe|source: 'fe'" src/api/client.ts` → FE call row emit present.
- `grep -c "newTraceparent()" src/api/client.ts` → 1 (no second trace_id mint).

## Threat Model Compliance

| Threat ID | Disposition | How honored |
|-----------|-------------|-------------|
| T-04-07 (DoS — route/operation cardinality) | mitigate | `routeLabel` prefers pattern-like `route.name` over `fullPath`; `operation` is the coarse `METHOD route` label. Backend `collect.go` length-caps the strings (Plan 01). |
| T-04-08 (Info Disclosure — trace_id correlation) | accept | `trace_id` is per-call ephemeral (minted fresh per axios request via `newTraceparent()`), not a stable user id; not persisted/reused across calls. |
| T-04-SC (Tampering — installs) | accept | No packages installed this phase. |

## Deviations from Plan

None — plan executed exactly as written. Both edits landed in the planned files (`client.ts` interceptor block; `traceContext.spec.ts`), reused the existing `traceId` mint and `stampTrace` call, and preserved the A4 flush-ordering invariant.

**Note (environment, not a deviation):** `node_modules` was absent in the fresh worktree, so `bun install` was run once to enable `tsc`/`vitest`. No dependency was added or changed; `package.json`/lockfile were not committed.

## TDD Gate Compliance

Task 2 carries `tdd="true"`, but the behavior under test (`traceContext.ts` window-association) is explicitly REUSE-unchanged per the plan (`traceContext.ts` is an upstream analog, not an edit target). The new spec is therefore an end-to-end PROOF of already-correct behavior, not a RED-then-GREEN cycle against new source. No `feat`/`refactor` commit follows the `test` commit because no production code changed for Task 2 — the production change (the FE call row) lives in Task 1's `feat` commit `ba6e4499`. The MVP+TDD behavior-adding gate does not fire here: Task 2's `<files>` is test-only (no non-test source file), so it is exempt.

## Known Stubs

None. The interceptor emits a fully-populated `source='fe'` row through the real `analytics.track` → `Transport` beacon path on every traced axios request.

## Self-Check: PASSED

- `frontend/web/src/api/client.ts` — FOUND (modified, FE call row emit present)
- `frontend/web/src/analytics/__tests__/traceContext.spec.ts` — FOUND (modified, AR-FE-02 block present)
- Commit `ba6e4499` — FOUND
- Commit `35f5bae6` — FOUND
