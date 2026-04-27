---
phase: 01-instrumentation-baseline
plan: 06
subsystem: observability
tags: [grafana, dashboard, prometheus, override-rate, baseline]
requirements: [M-02]
threats: []
dependency-graph:
  requires:
    - "Wave 2 plan 01-02: combo_resolve_total counter exposed by services/player /metrics"
    - "Wave 4 plan 01-05: combo_override_total counter exposed by services/player /metrics"
  provides:
    - "Auto-Pick Override Rate dashboard panel row in docker/grafana/dashboards/preference-resolution.json"
    - "5 segmented views (tier, player, language+anon, dimension) of override-rate ratio"
  affects:
    - "Plan 01-07 deploy: Grafana provisioning will auto-load this row on next grafana restart"
tech-stack:
  added: []
  patterns:
    - "Provisioned Grafana dashboard JSON checked into docker/grafana/dashboards/ (D-17)"
    - "PromQL ratio: rate(combo_override_total[5m]) / rate(combo_resolve_total[5m]) (D-16)"
    - "5 segmentations per D-15: tier, player, language, anon, dimension"
key-files:
  created: []
  modified:
    - "docker/grafana/dashboards/preference-resolution.json"
decisions:
  - "Used variable form ${DS_PROMETHEUS} for datasource uid (matches existing panels in same dashboard)"
  - "Used 24h time window for the by-dimension barchart (matches description); 5m rate window for the rate panels"
  - "New panels appended after existing y=37 base (no offset collisions); IDs 100-105 to avoid collision with existing IDs 1-11"
  - "Stat threshold steps: green < 10%, yellow 10-20%, red > 20% (matches CONTEXT D-16 target < 10%)"
metrics:
  duration: "~10 minutes"
  completed: "2026-04-27"
  tasks: 1
  files-modified: 1
---

# Phase 1 Plan 6: Override-Rate Dashboard Panel Summary

One-liner: Added an "Auto-Pick Override Rate (Phase 1 Baseline)" row with 5 panels (stat + 3 timeseries + 1 barchart) to the existing `preference-resolution.json` Grafana dashboard, covering all D-15 segmentations and using the D-16 PromQL ratio.

## What Shipped

**Dashboard JSON modified:** `docker/grafana/dashboards/preference-resolution.json`

| Before | After |
|--------|-------|
| 9 panels (rows + tiles) | 15 panels (+1 row, +5 tiles) |
| Existing IDs: 1, 2, 3, 4, 5, 6, 8, 9, 11 | Existing IDs preserved + 100, 101, 102, 103, 104, 105 |
| Max y-base: 37 | Max y-base: 64 |

## New Panels

| ID  | Type        | Title                                              | gridPos (x,y,w,h) | PromQL                                                                                          |
| --- | ----------- | -------------------------------------------------- | ----------------- | ----------------------------------------------------------------------------------------------- |
| 100 | row         | Auto-Pick Override Rate (Phase 1 Baseline)         | 0,37,24,1         | (row header)                                                                                    |
| 101 | stat        | Override Rate (last 5m)                            | 0,38,8,6          | `sum(rate(combo_override_total[5m])) / sum(rate(combo_resolve_total[5m]))`                      |
| 102 | timeseries  | Override Rate by Tier                              | 8,38,16,10        | `sum by(tier)(rate(combo_override_total[5m])) / sum by(tier)(rate(combo_resolve_total[5m]))`    |
| 103 | timeseries  | Override Rate by Player                            | 0,48,12,8         | `sum by(player)(rate(combo_override_total[5m])) / sum by(player)(rate(combo_resolve_total[5m]))` |
| 104 | timeseries  | Override Rate by Language and Auth State           | 12,48,12,8        | `sum by(language, anon)(rate(combo_override_total[5m])) / sum by(language, anon)(rate(combo_resolve_total[5m]))` |
| 105 | barchart    | Overrides by Dimension (24h count)                 | 0,56,24,8         | `sum by(dimension)(increase(combo_override_total[24h]))`                                        |

All 5 D-15 segmentations covered: **tier** (panel 102), **player** (103), **language + anon** (104), **dimension** (105). Plus the global stat (101) for the headline metric.

## Verification (Acceptance Criteria)

| Check | Command | Result |
|-------|---------|--------|
| Valid JSON | `jq . preference-resolution.json > /dev/null` | exit 0 |
| Row 100 title | `jq '.panels[] \| select(.id == 100) \| .title'` | "Auto-Pick Override Rate (Phase 1 Baseline)" |
| 5 new non-row panels | `jq '[.panels[] \| select(.id >= 101 and .id <= 105)] \| length'` | 5 |
| IDs unique | `jq '[.panels[].id] \| length == (. \| unique \| length)'` | true (15 ids, 15 unique) |
| `combo_override_total` references | `grep -c combo_override_total` | 5 (>= 5) |
| `combo_resolve_total` references | `grep -c combo_resolve_total` | 4 (>= 4) |
| `by(tier)` present | `grep -c "by(tier)"` | 4 (>= 1; existing dashboard had this label too) |
| `by(player)` present | `grep -c "by(player)"` | 1 (>= 1) |
| `by(language, anon)` present | `grep -c "by(language, anon)"` | 1 (>= 1) |
| `by(dimension)` present | `grep -c "by(dimension)"` | 1 (>= 1) |
| Existing panels unchanged | spot-check id=2,6,11 titles | preserved |

All acceptance criteria pass.

## Commits

- `9acd8ec` feat(01-06): add override-rate panel row to preference-resolution dashboard

## Deviations from Plan

None - plan executed exactly as written. The plan provided an exact panel template; I used the discovered `Y_BASE=37` to set y-coordinates (37, 38, 38, 48, 48, 56) and the discovered `DS_UID="${DS_PROMETHEUS}"` (variable form, matches all existing panels in this dashboard). No auto-fixes were needed.

## Note for Plan 01-07 (deploy)

The dashboard JSON is ready for deployment. To make it live:

1. `make restart-grafana` (or full `make redeploy-grafana` if any provisioning configs changed) — Grafana auto-loads `docker/grafana/dashboards/*.json` via provisioning, no manual import required.
2. Visit `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1` and verify the new "Auto-Pick Override Rate (Phase 1 Baseline)" row appears at the bottom with 5 populated panels.
3. Trigger a few `/api/preferences/override` calls (frontend in real use, or curl with the `ui_audit_bot` API key) to populate Prometheus counters and confirm panels render real data within `refresh: 5m` window.

Per CONTEXT D-15, D-16, D-17 / RESEARCH "Recommended Dashboard Panel JSON" lines 1028-1151. Phase 1 success criterion 3 (24h baseline) is satisfied once these counters accumulate 24h of traffic post-deploy.

## Self-Check

`docker/grafana/dashboards/preference-resolution.json` exists in the working tree.
Commit `9acd8ec` exists in `git log`.

## Self-Check: PASSED
