---
phase: 11-observability-prediction
plan: 02
subsystem: infra
tags: [grafana, prometheus, observability, dashboard, promql, autocache]

# Dependency graph
requires:
  - phase: 11-01
    provides: "library_autocache_predicted_bytes{component=ongoing|nextep} scheduler gauge — the metric OBS-05's prediction table charts against library_autocache_budget_bytes"
  - phase: 10
    provides: "library_autocache_bytes_used / _budget_bytes / _episodes / _evicted_total / _rejected_total gauges+counters the Autocache Pool panels chart"
  - phase: 09-download-triggers
    provides: "library_autocache_downloads_total{trigger,result} counter for OBS-04"
  - phase: 08
    provides: "library_autocache_serve_total{result} counter for the OBS-02 hit-rate ratio"
provides:
  - "6 appended Autocache Pool panels (ids 8..13) on infra/grafana/dashboards/library.json covering OBS-01..05"
  - "OBS-05 hand-authored Prometheus instant-query TABLE panel (predicted_bytes{component} + sum() total + budget) — the first Prometheus-datasource table in the repo"
  - "table panel type registered in the dashboard __requires"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Prometheus instant-query Grafana table panel (format:table + instant:true) — no prior analog in repo (all existing tables are ClickHouse rawSql)"
    - "Clone-the-self-analog dashboard extension: reuse existing barchart/stat panel envelopes, swap expr/unit/title/id/gridPos, append after the last panel"

key-files:
  created:
    - .planning/phases/11-observability-prediction/11-02-SUMMARY.md
  modified:
    - infra/grafana/dashboards/library.json

key-decisions:
  - "OBS-04 charts sum by (trigger, result) (NOT trigger-only) with legendFormat {{trigger}}/{{result}} — preserves the result dimension the requirement+plan-checker expect"
  - "OBS-05 ships the COARSE v1 table (ongoing/nextep/total + budget) — no per-anime rows (high-cardinality, deferred to v2 per CONTEXT)"
  - "OBS-02 hit-rate stat thresholds red→yellow→green at percentunit 0.5/0.8 (a 50%/80% cache-hit health band)"
  - "OBS-05 uses the minimal component-rows+total+budget instant targets; the optional merge/organize transformation column-join was skipped (polish only, not required for 'compared against budget_bytes')"

patterns-established:
  - "Pattern: Prometheus instant-query table panel (format:table, instant:true) for a small fixed-cardinality gauge family, distinct from the repo's ClickHouse rawSql tables"

requirements-completed: [OBS-01, OBS-02, OBS-03, OBS-04, OBS-05]

# Metrics
duration: 4min
completed: 2026-06-17
---

# Phase 11 Plan 02: Grafana Autocache Pool Panels Summary

**Appended 6 Autocache Pool panels (ids 8..13) to the library Grafana dashboard — stacked bytes_used vs budget, episodes, preload hit-rate %, eviction/rejection (24h), downloads by trigger/result (24h), and a hand-authored Prometheus instant-query prediction table joining predicted_bytes{component} + total against budget_bytes.**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-06-17T12:06:00Z
- **Completed:** 2026-06-17T12:08:00Z
- **Tasks:** 2
- **Files modified:** 1 (dashboard JSON)

## Accomplishments
- **OBS-01** (id:8 stacked barchart, `bytes`) — `library_autocache_bytes_used` `{{source}}/{{freshness}}` stacked vs a `budget` series from `library_autocache_budget_bytes`; (id:9 barchart, `short`) — `library_autocache_episodes` by source/freshness.
- **OBS-02** (id:10 stat, `percentunit`, red→yellow→green) — preload hit-rate `sum(rate(serve_total{result="hit"}[1h])) / sum(rate(serve_total[1h]))`.
- **OBS-03** (id:11 barchart, `short`) — `sum by (source) (increase(evicted_total[24h]))` + `sum by (reason) (increase(rejected_total[24h]))`.
- **OBS-04** (id:12 barchart, `short`) — `sum by (trigger, result) (increase(downloads_total[24h]))` legend `{{trigger}}/{{result}}` (keeps the `result` dimension).
- **OBS-05** (id:13 TABLE, `bytes`) — three instant `format:table` targets: `library_autocache_predicted_bytes` (component rows) + `sum(library_autocache_predicted_bytes)` (total) + `library_autocache_budget_bytes` (budget).
- Registered `{ type:"panel", id:"table" }` in `__requires` (the only newly-introduced panel type).
- Existing panels 1..7 left byte-for-byte intact (verified by JSON diff against the pre-edit state); all 13 ids unique; gridPos non-overlapping (new rows y=24/32/40/48).

## Task Commits

Each task was committed atomically:

1. **Task 1: Append OBS-01..04 panels (ids 8..12)** - `0aebaa6c` (feat) — storage/episodes barcharts, hit-rate stat, evicted+rejected barchart, downloads-by-trigger/result barchart.
2. **Task 2: Append OBS-05 prediction TABLE (id:13) + register table type** - `e8cc5ae7` (feat) — instant-query prediction table + `__requires` table entry.

_Plan metadata commit follows this SUMMARY._

## Files Created/Modified
- `infra/grafana/dashboards/library.json` - Appended 6 panel objects (ids 8..13) to the `panels` array after panel id:7, reusing the `${DS_PROMETHEUS}` templated datasource at panel + target level; added the `table` entry to `__requires`.

## Decisions Made
- **OBS-04 result dimension:** Charted `sum by (trigger, result)` with legend `{{trigger}}/{{result}}` rather than the plan body's trigger-only `sum by (trigger)`. The requirement (OBS-04) and the plan-checker both expect the `result` label to be preserved, and the underlying counter is `downloads_total{trigger,result}`. Documented as a deviation below.
- **Coarse OBS-05 table:** Shipped the v1 ongoing/nextep/total + budget table; no per-anime rows (CONTEXT explicitly prefers coarse v1 to avoid high-cardinality gauge labels; per-anime deferred to v2).
- **Skipped the optional transformation column-join** on OBS-05 — the component-rows + total + budget instant targets satisfy "compared against budget_bytes" without a `merge`/`organize` transform (PATTERNS marks the column-join as polish-only).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] OBS-04 must keep the `result` dimension**
- **Found during:** Task 1 (OBS-04 downloads panel)
- **Issue:** The plan body specified `sum by (trigger) (increase(library_autocache_downloads_total[24h]))` (trigger-only), but the OBS-04 requirement and the source counter expose `{trigger,result}`; dropping `result` loses the success/error breakdown the requirement and the plan-checker expect.
- **Fix:** Used `sum by (trigger, result) (increase(library_autocache_downloads_total[24h]))` with `legendFormat "{{trigger}}/{{result}}"`.
- **Files modified:** infra/grafana/dashboards/library.json (panel id:12)
- **Verification:** `jq` confirms panel id:12 expr contains `result`; JSON parses; ids 1..13 unique and non-overlapping.
- **Committed in:** `0aebaa6c` (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 missing-critical dimension)
**Impact on plan:** The single deviation aligns OBS-04 with its requirement's `result` dimension and the explicit scope instruction. No scope creep — still 6 panels, ids 8..13, dashboard-only.

## Issues Encountered
None. Both Task gates and the final full-dashboard verification (`jq` parse, 13 unique ids, panels 1..7 byte-identical, no gridPos overlap, datasource uid templated, OBS-05 instant/format=table, `table` in `__requires`, zero per-anime cardinality) passed on the first run after each edit.

## User Setup Required
None - file-provisioned Grafana dashboard, no env vars or external config. The panels render against the existing internal Prometheus datasource (`${DS_PROMETHEUS}` → `PBFA97CFB590B2093`) on dashboard reload/provisioning; `library_autocache_predicted_bytes` is already emitted by the scheduler (Plan 11-01).

## Next Phase Readiness
- Phase 11 (final phase of v4.1) observability surface is complete: all OBS-01..05 requirements have a dedicated panel on the library dashboard.
- No blockers. A live Grafana render smoke is opt-in only (owner request) per CONTEXT — not run here.

---
*Phase: 11-observability-prediction*
*Completed: 2026-06-17*

## Self-Check: PASSED

`infra/grafana/dashboards/library.json` and `11-02-SUMMARY.md` exist on disk, the dashboard contains `library_autocache_predicted_bytes`, and both task commits (`0aebaa6c`, `e8cc5ae7`) are present in the git log. `jq . infra/grafana/dashboards/library.json` parses cleanly; panels 1..7 are byte-identical to the pre-edit state; all 13 ids are unique and non-overlapping.
