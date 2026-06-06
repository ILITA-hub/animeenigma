---
phase: 05-reports-dashboards
plan: 01
subsystem: observability
tags: [grafana, clickhouse, dashboard-json, activity-register, pivot, flow]
provides:
  - "AR-REPORT-01 wide-event pivot dashboard (live $group_by regroup + self-discovering filter dropdowns)"
  - "AR-REPORT-02 from->choke-point->effects flow report (origin->operation->target requests + bytes)"
  - "Two file-provisioner-loadable Grafana dashboards bound to aenigma-clickhouse"
affects: [05-02-anomaly-overview, 05-03-live-render-gate]
tech-stack:
  added: []
  patterns:
    - "ClickHouse SQL query template variables (SELECT DISTINCT) driving GROUP BY ${var}"
    - "$__conditionalAll + :singlequote multi-value filter macros"
    - "custom fixed-allowlist var for raw-column SQL interpolation (SQL-injection guard)"
    - "sumIf(..., source='be') byte discipline (AR-FE-03)"
    - "Grafana groupBy row-grouping transform for drill shape"
key-files:
  created:
    - infra/grafana/dashboards/activity-register-pivot.json
    - infra/grafana/dashboards/activity-register-flow.json
  modified: []
key-decisions:
  - "Sankey/node-graph visual flow DEFERRED for AR-REPORT-02 — needs a community panel plugin (GF_INSTALL_PLUGINS + container recreate, a real Wave-0 cost); the table + groupBy row-grouping transform fully satisfies the requirement. Noted in the flow dashboard description AND here, not a silent scope reduction."
  - "$group_by is a custom fixed-allowlist var (not query/text) because it is interpolated as a raw unquoted SQL column fragment — SQL-injection guard (T-05-02)."
  - "Pivot $target dropdown scoped by $effect_kind + $__timeFilter + LIMIT 200 to keep the high-cardinality dropdown usable (T-05-03)."
duration: 6min
completed: 2026-06-06
---

# Phase 5 Plan 01: Activity Register Reports (Pivot + Flow) Summary

**Authored the two table-driven Activity Register reports as pure declarative Grafana JSON over the already-provisioned aenigma-clickhouse datasource: a wide-event PIVOT dashboard whose $group_by template var regroups the table by ANY dimension live, and a from->choke-point->effects FLOW report rendering origin->operation->per-target requests + bytes.**

## Performance
- **Duration:** ~6 min
- **Tasks:** 2 / 2 completed
- **Files modified:** 2 created (0 modified)

## Accomplishments
- **AR-REPORT-01 pivot** — `activity-register-pivot.json`: a custom `$group_by` allowlist var (origin/operation/effect_kind/target_kind/target/source) drives a `GROUP BY ${group_by}` table; three `SELECT DISTINCT` dropdowns ($effect_kind/$origin/$target) self-discover live values; the byte measure uses `sumIf(bytes_out + bytes_in, source = 'be')`; a "Requests over time" timeseries panel shares the same `$__conditionalAll` filter guards.
- **AR-REPORT-02 flow** — `activity-register-flow.json`: an `origin, operation, target` table with `sum(requests)` + `sum(bytes_out + bytes_in)` filtered `effect_kind = 'egress' AND source = 'be'`; a Grafana `groupBy` row-grouping transform renders the from->choke-point->effects drill; a scoping `$origin` dropdown.
- Both dashboards bind every panel + var to datasource uid `aenigma-clickhouse`, carry unique uids (`activity-register-pivot`, `activity-register-flow` — no collision with `product-analytics`), and parse cleanly with `jq empty`.

## Task Commits
1. **Task 1: Author the wide-event pivot dashboard (AR-REPORT-01)** — `d1647257`
2. **Task 2: Author the from->choke-point->effects flow report (AR-REPORT-02)** — `9bc0706d`

## Files Created/Modified
- `infra/grafana/dashboards/activity-register-pivot.json` — AR-REPORT-01 pivot: custom `$group_by` var + 3 self-discovering filter vars driving a `GROUP BY ${group_by}` table (byte measure `source='be'`-filtered) + a requests-over-time timeseries.
- `infra/grafana/dashboards/activity-register-flow.json` — AR-REPORT-02 flow: `GROUP BY origin, operation, target` table with requests + bytes (`effect_kind='egress' AND source='be'`) + a `groupBy` row-grouping transform.

## Decisions & Deviations
- **Sankey deferral (explicit, not silent):** A literal Sankey / node-graph visual flow for AR-REPORT-02 is intentionally NOT shipped. It requires a community panel plugin — a `GF_INSTALL_PLUGINS` addition plus a Grafana container recreate, i.e. a real Wave-0 cost. The table + Grafana `groupBy` row-grouping transform fully satisfies AR-REPORT-02; Sankey is a noted future enhancement (research Open Question 1). This deferral is documented in the flow dashboard's `.description` field AND here.
- **$group_by as a custom allowlist var:** interpolated as a raw unquoted SQL column fragment, so it is a fixed-allowlist `custom` var (never query/text) — the SQL-injection guard called out in the threat model (T-05-02).
- **No auto-fixed bugs / Rule 1-3 deviations** — the plan executed exactly as written.
- **No STATE.md / ROADMAP.md modifications** — owned by the orchestrator after the wave merges (per execution constraints).
- **No Grafana restart/redeploy** — that is the 05-03 live-render gate; this plan is structural JSON authoring only.

## Known Stubs
None. Both dashboards are complete, query real ClickHouse columns, and drive all dropdowns from live `SELECT DISTINCT` (no hardcoded enums, no empty placeholders).

## Next Phase Readiness
- Plan 05-02 (anomaly + awareness overview) and 05-03 (the non-autonomous live-render gate: `make restart-grafana`, switch `$group_by`, watch the table regroup, read a real origin's per-target breakdown) can proceed.
- Both files land in `infra/grafana/dashboards/` (mounted → `/var/lib/grafana/dashboards-infra`), so they auto-load on the next `make restart-grafana` without a rebuild.

## Self-Check: PASSED
