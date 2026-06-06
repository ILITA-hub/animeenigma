---
phase: 05-reports-dashboards
plan: 02
subsystem: observability
tags: [grafana, clickhouse, dashboards, alerting, activity-register, anomaly-detection]
requires:
  - aenigma-clickhouse datasource (provisioned, Phase 1 01-03)
  - grafana-clickhouse-datasource plugin (installed, Phase 1)
  - analytics.events ClickHouse table (Phases 2-4)
provides:
  - AR-REPORT-04 awareness overview dashboard (top ops + top deps + active anomalies)
  - AR-REPORT-03 explainable avg+Nσ volume-anomaly panel + provisioned alert rule
affects:
  - infra/grafana/dashboards/ (new dashboard JSON, file-provisioned)
  - docker/grafana/provisioning/alerting/rules.yml (one new alert rule)
tech-stack:
  added: []
  patterns:
    - "ClickHouse windowed avg+Nσ baseline as explainable (non-ML) anomaly flagging"
    - "Dashboard custom vars ($sigma/$window) tune anomaly sensitivity without SQL edits"
    - "3-refId A=query/B=reduce/C=threshold provisioned alert on a ClickHouse datasource"
key-files:
  created:
    - infra/grafana/dashboards/activity-register-overview.json
  modified:
    - docker/grafana/provisioning/alerting/rules.yml
decisions:
  - "Anomaly alert hardcodes σ=3 / window=24h (alert rules can't read dashboard template vars); the dashboard panel keeps the tunable $sigma/$window"
  - "Anomaly rule appended to the existing 'AnimeEnigma Alerts' group (not a new group/file) so Grafana's single-file provider loads it without a second provider entry"
  - "Added a 'Active anomaly count' stat panel on top of the 3 required tables for an at-a-glance number"
metrics:
  duration: ~6m
  tasks: 2
  files_created: 1
  files_modified: 1
  completed: 2026-06-06
---

# Phase 5 Plan 02: Awareness Overview Dashboard + Volume-Anomaly Alert Summary

Authored the AR-REPORT-04 awareness overview dashboard carrying the AR-REPORT-03 explainable avg+Nσ volume-anomaly panel, and appended the matching provisioned alert rule on the ClickHouse datasource — pure declarative Grafana JSON + YAML over `aenigma-clickhouse`, no Go/Vue.

## What Was Built

### Task 1 — `infra/grafana/dashboards/activity-register-overview.json` (commit 3a8025af)
A new dashboard (uid `activity-register-overview`, tags `activity-register`/`awareness`, schemaVersion 39, `now-24h`→`now`, refresh 30s) with four panels in one view:
1. **Active anomaly count** (stat, red ≥1) — counts `is_anomaly` rows from the baseline subquery.
2. **Top operations (now/today)** (table) — `GROUP BY operation ORDER BY sum(requests) DESC LIMIT 10`.
3. **Top external dependencies** (table) — `effect_kind = 'egress' AND source = 'be' GROUP BY target ORDER BY bytes DESC LIMIT 10`, `bytes` field unit set to `bytes`.
4. **Active volume anomalies** (table) — the Pattern-3 avg+Nσ baseline query, `is_anomaly` colored via threshold + value-mapping (1=ANOMALY/red, 0=ok/green), sorted anomalies-first.

Two custom tuning vars — `sigma` (default 3) and `window` (default 24) — interpolate into the anomaly rawSql (`INTERVAL $window HOUR`, `baseline_avg + $sigma * baseline_sd`) so the admin tunes noise without editing SQL. All panels bind `aenigma-clickhouse`; no hardcoded effect_kind/target enums.

### Task 2 — `docker/grafana/provisioning/alerting/rules.yml` (commit 4bf0200c)
Appended exactly one rule (`activity-register-volume-anomaly`) to the existing "AnimeEnigma Alerts" group, mirroring the 3-refId shape: refId A = ClickHouse rawSql counting `is_anomaly` rows over a fixed σ=3 / 24h baseline (filters `source='be' AND effect_kind='egress'`), B = reduce(last), C = threshold(gt 0). `for: 5m`, labels `severity: warning` / `component: activity-register`. refId A points at `aenigma-clickhouse` (NOT the Prometheus uid `PBFA97CFB590B2093`). Existing 14 rules + the Scraper Self-Healing group untouched (rule count 14→15, PBFA refs unchanged at 14).

## Verification

Task 1:
- `jq empty` exits 0; uid == `activity-register-overview`; 3 table panels.
- top-deps rawSql contains `effect_kind = 'egress'` AND `source = 'be'`.
- anomaly rawSql contains `is_anomaly`, `INTERVAL $window HOUR`, `$sigma`.
- `sigma` + `window` templating vars present; `aenigma-clickhouse` bound.

Task 2:
- `python3 -c "import yaml; yaml.safe_load(...)"` exits 0.
- `grep -c 'uid: activity-register-volume-anomaly'` == 1.
- rule count delta +1 (14→15); `datasourceUid: aenigma-clickhouse` present; `is_anomaly` + `source = 'be'` + `effect_kind = 'egress'` present; Prometheus-uid count unchanged at 14.

The live gate (inject a synthetic spike, open Grafana, watch the panel flag + alert transition; do NOT restart Grafana in this wave) is the non-autonomous plan 05-03 gate — out of scope here.

## Deviations from Plan

None — plan executed exactly as written. (Optional "Active anomaly count" stat panel was included as the plan explicitly permitted.)

## Threat Surface

No new security surface. T-05-04/05/06 mitigations satisfied: byte/anomaly measures filter `source='be' AND effect_kind='egress'`; σ/window are tunable + `for: 5m` debounces false positives; alert refId A explicitly uses `aenigma-clickhouse`.

## Known Stubs

None.

## Self-Check: PASSED
- FOUND: infra/grafana/dashboards/activity-register-overview.json
- FOUND: docker/grafana/provisioning/alerting/rules.yml (modified)
- FOUND commit: 3a8025af (Task 1)
- FOUND commit: 4bf0200c (Task 2)
