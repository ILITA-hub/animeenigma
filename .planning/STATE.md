---
gsd_state_version: 1.0
milestone: v4.1
milestone_name: Auto Torrent Population (watch-driven first-party RAW cache)
current_plan: Not started
status: executing
stopped_at: v4.1 roadmap written (ROADMAP.md, REQUIREMENTS.md traceability, STATE.md)
last_updated: "2026-06-17T09:57:49.543Z"
last_activity: 2026-06-17 -- Phase 11 planning complete
progress:
  total_phases: 5
  completed_phases: 4
  total_plans: 15
  completed_plans: 13
  percent: 80
---

# Project State

## Project Reference

**Core value:** A logged-in user hits play on the first-party "ae" provider and the RAW (JP-audio) episode is already there — pre-downloaded by the platform's prediction of what they're about to watch, served from a self-managing ~100 GB pool with zero admin action.

**Current focus:** Phase 11 — observability & prediction

## Current Position

Phase: 11
Plan: —
Status: Ready to execute
Last activity: 2026-06-17 -- Phase 11 planning complete

## Progress

**Milestone phases:** 0 / 5 complete
**Current Plan:** Not started
`[          ]` 0%

## Phase Map (v4.1, Phases 7-11)

| Phase | Name | Requirements | Depends on |
|-------|------|--------------|------------|
| 7 | Pool Foundation, Config & Migration | POOL-01..05 | — |
| 8 | Serving & Fetch Signal | SERVE-01..03 | 7 |
| 9 | Download Triggers | TRIG-01..05 | 7, 8 |
| 10 | Eviction & Budget | EVICT-01..05 | 7, 8, 9 |
| 11 | Observability & Prediction | OBS-01..05 | 7, 8, 9, 10 |

## Session Continuity

**Stopped At:** v4.1 roadmap written (ROADMAP.md, REQUIREMENTS.md traceability, STATE.md)
**Resume File:** None — next action is `/gsd:plan-phase 7`

## Performance Metrics

| Phase | Plan | Duration | Tasks | Files |
| ----- | ---- | -------- | ----- | ----- |
| —     | —    | —        | —     | —     |

## Accumulated Context

### Decisions (carried from design spec)

- **D1**: Build into `services/library` as new `internal/autocache/` — reuse `download_worker`, `encoder_worker`, Jackett/Nyaa tier, `minio.Writer`, `DiskGuard`, `library_jobs`. No new microservice.
- **D2/D3**: RAW-only in v1; one RAW object serves both RAW- and SUB-preferring demand (SUB via client-side overlay); DUB demand ignored.
- **D5/D6/D7**: Unified metered pool (admin + auto, one budget, default 100 GB); admin content is "more fresh" (longer window, evicted only after all auto-Stale); reject when full.
- **Sequencing invariant**: the §3.3 admin-content migration (Phase 7) MUST complete before the evictor (Phase 10) goes live — unmigrated admin content on old paths is invisible to the Accountant and the budget math is wrong (spec §10).
- **Metric ownership**: each phase emits its own counters/gauges (serve hit/miss in P8, downloads-by-trigger in P9, eviction/rejection + byte accounting in P10); Phase 11 builds the Grafana dashboard + daily prediction-table heuristic on top.

### Open todos / risks (from spec §10)

- **Combo enumeration for Logic A** needs an internal way to list "active JP-audio watchers per anime" — likely a new internal catalog/player endpoint; confirm data ownership during Phase 9 planning.
- **`avg_raw_ep_size`** for the prediction table bootstraps from observed downloads; before any downloads exist, fall back to a configured ~1.2 GB constant (Phase 11).
- **Migration audit**: the Phase 7 migration task must audit `services/catalog/internal/service/raw_resolver.go` and `services/catalog/internal/parser/library/client.go` for hardcoded old-prefix assumptions.

### Blockers

None.

## Operator Next Steps

- Plan the first phase with `/gsd:plan-phase 7`

</content>
