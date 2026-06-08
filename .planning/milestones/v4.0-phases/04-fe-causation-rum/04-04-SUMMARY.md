---
plan: 04-04
phase: 04
status: deferred-human-verify
---

# 04-04 — Live ClickHouse Phase-Gate (FE Causation + RUM)

## Self-Check: DEFERRED (human-verify checkpoint)

**Task 1 — automated pre-gate: COMPLETE.**
- `cd services/analytics && go test ./... -count=1` → green.
- `cd frontend/web && bunx vitest run src/analytics/` → 40/40 green; analytics Phase-4 files `bunx tsc --noEmit` clean.
- `make redeploy-analytics` → done; analytics `/health` 200. The `collect.go` FE register-field mapping + `source` whitelist ({fe,fe_rum}) + byte-poverty are LIVE on the backend.

**Task 2 — live browser human-verify gate: DEFERRED (2026-06-06).**
Blocked by an EXTERNAL parallel session's uncommitted, type-error-broken changes in
`frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts` (unified-player
workstream — NOT Phase 4). These break the project-wide `bunx tsc --noEmit`, which is a
prerequisite of `make redeploy-web`. Phase 4's own files are tsc-clean. Per the
path-scoped / no-cross-workstream-interference rule, the orchestrator did not touch or
ship that work.

The FE half (axios interceptor `source='fe'` call row, click trace-stamp, `rum.ts`
PerformanceObserver beacon) only runs in the browser, so the real FE→BE `trace_id`
sharing proof requires `web` redeployed. User (autonomous run) chose to DEFER this gate
and continue to Phases 5/6.

**To complete later:** once the parallel `useProviderResolver.ts` work is committed/fixed
so the web type-check passes → `make redeploy-web`, navigate the live app, then run the
three AR-FE ClickHouse proofs from `04-04-PLAN.md` (trace_id join, click→effect join,
`sum(bytes_in) WHERE source='fe_rum'==0`). Resume signal: "approved".

## What IS proven now
- AR-FE-01 / AR-FE-03 backend mapping: live analytics ingest maps the FE register fields, whitelists `source`, flags `fe_rum`→`accuracy='approximate'`, forces byte-poverty (unit + integration tests green; deployed).
- AR-FE-01 / AR-FE-02 / AR-FE-03 FE emit logic: 40/40 frontend unit tests green (interceptor emit, click stamp, RUM aggregation/byte-poverty).
- Outstanding: live end-to-end FE→BE `trace_id` join from a real browser (deferred).
