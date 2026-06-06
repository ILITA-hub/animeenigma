---
phase: 04
slug: fe-causation-rum
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-06
---

# Phase 04 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `go test` (analytics backend) + Vitest (frontend `frontend/web`) |
| **Config file** | `frontend/web/vitest.config.ts`; Go uses module-local `go test` |
| **Quick run command** | `cd frontend/web && bunx vitest run src/analytics/` |
| **Full suite command** | `cd services/analytics && go test ./... && cd frontend/web && bunx vitest run src/analytics/` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run the quick run command for the touched layer (Vitest for FE, `go test ./internal/...` for analytics)
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

> Planner fills the concrete per-task rows. Phase requirement → validation mapping (from RESEARCH Validation Architecture):

| Requirement | Validation type | Proof |
|-------------|-----------------|-------|
| AR-FE-01 (axios interceptor → trace_id + route + action) | join query | A single `fe` register row (`source='fe'`) with the active `trace_id` joins to N `source='be'` effects sharing that `trace_id` |
| AR-FE-02 (click auto-capture carries trace_id) | join query | A captured click event's `trace_id` == the downstream BE effect/trace `trace_id` (time-window association in `traceContext.ts`) |
| AR-FE-03 (PerformanceObserver RUM, source=fe_rum/accuracy=approx, never summed) | Go test + query | `fe_rum` rows carry zero bytes; `sum(bytes_in) WHERE source='fe_rum'` == 0; byte aggregations filter `source='be'` |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Vitest already configured (`frontend/web/src/analytics/` has existing specs). New `rum.ts` + `collect.go` mapping get co-located tests.
- Go analytics service already has `go test` infra.

*Existing infrastructure covers all phase requirements — no Wave 0 framework install needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live ClickHouse phase-gate (FE row joins to BE effects on trace_id; RUM rows present + byte-poor) | AR-FE-01/02/03 | Requires running stack + real browser navigation | Mirror 02-04/03-06 closeout: navigate the app, query `events` for `source IN ('fe','fe_rum')`, prove the trace_id join + the `source=be` byte filter |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
