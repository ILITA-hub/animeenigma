---
phase: 06-consolidation-topology-a
plan: 03
subsystem: observability
tags: [tempo-removal, loki-removal, promtail-removal, otel-collector, clickhouse, span-metrics, gated-cutover, topology-a]
requires:
  - "06-01: ClickHouse trace+log export + collector-native span-metrics/service-graph proven live alongside Tempo"
  - "06-02: backend-tracing dashboard + log views repointed to ClickHouse and proven to render; otel_logs.TraceId fix landed"
  - "Human render-confirmation gate (Task 1) satisfied at the orchestrator level — user visually/operationally confirmed CH trace+log views render and approved the irreversible removal"
provides:
  - "Topology A: ClickHouse is the SINGLE event/trace/log plane; Prometheus + Grafana remain the unchanged metrics/alerting/rendering layer"
  - "Collector traces pipeline is Tempo-free ([clickhouse/traces, spanmetrics, servicegraph]); the spanmetrics/servicegraph connectors are the SOLE span-metrics/service-graph writer to Prometheus"
  - "Tempo, tempo-init, Loki, Promtail removed (containers + compose services + datasources + config files + depends_on edges); orphaned data volumes reclaimed"
affects:
  - "Milestone v4.0 Activity Register — Phase 6 (the last phase) is complete; observability consolidation done"
tech-stack:
  added: []
  patterns:
    - "deleteDatasources provisioning block to prune already-provisioned read-only (editable:false) datasources — removing the block alone does NOT delete them from Grafana's DB (the API refuses)"
    - "Distinguish a live span-metrics writer from a decaying one via timestamp() of the last real sample, not instant-query eval time (which always reports 'now')"
key-files:
  created: []
  modified:
    - infra/otel/collector-config.yaml
    - docker/docker-compose.yml
    - docker/grafana/provisioning/datasources/datasources.yml
  deleted:
    - infra/tempo/tempo.yaml
    - docker/loki/loki-config.yml
    - docker/promtail/config.yml
decisions:
  - "Used deleteDatasources (not just block removal) to prune the editable:false Tempo+Loki datasources — Grafana's API returns 'Cannot delete read-only data source' and a removed-from-file provisioned datasource persists in the DB until pruned"
  - "MinIO tempo bucket cleanup DEFERRED (mc not trivially available in the minio image); the data volumes (docker_tempo_data, docker_loki_data) WERE reclaimed — the bulk of the space"
  - "Validated AR-CONS-03 alert survival against Grafana unified alerting (15 rules, all health=ok), not Prometheus rule files (none exist — groups:[]); RESEARCH Pitfall 1 confirmed: no alert/dashboard consumes Tempo-generated metrics"
metrics:
  duration: "~8 min"
  completed: "2026-06-08"
  tasks: 4
  files: 6
  commits: 3
---

# Phase 6 Plan 03: Destructive Cutover — Retire Tempo/Loki/Promtail, Complete Topology A Summary

Executed the irreversible back half of the gated observability consolidation: removed Tempo, tempo-init, Loki, and Promtail (containers, compose services, Grafana datasources, orphaned config files, and every `depends_on` edge), dropped the collector's Tempo trace exporter so ClickHouse is the sole trace store and the collector's spanmetrics/servicegraph connectors are the sole span-metrics source, and ran the full AR-CONS-01/02/03 final verification — all green on the live production stack. Topology A is complete: ClickHouse is the single event/trace/log plane; Prometheus + Grafana remain the untouched, verified metrics/alerting/rendering layer.

## Human Gate (Task 1) — Satisfied at the Orchestrator Level

The plan's Task 1 is a blocking `checkpoint:human-verify`. Per the orchestrator's execution context, that gate was ALREADY executed before this executor ran: the user visually/operationally confirmed the ClickHouse trace + log views render and span-metrics are present in Prometheus, and explicitly **approved** proceeding with the irreversible Tempo/Loki/Promtail removal. The trace↔log correlation gap the user asked to fix first (empty `otel_logs.TraceId`) was also fixed and verified live in a 06-02 follow-up (commit `3eb9622b`; `otel_logs.TraceId` now populated, INNER JOIN to `otel_traces` resolves). Therefore Task 1 was treated as PASSED/approved and the executor proceeded directly with the destructive tasks (2/3/4).

## What Was Done

### Task 2 — Tempo-free collector + remove retired compose services (commit `42b328a9`)
- **`infra/otel/collector-config.yaml`** — removed the `otlp/tempo` exporter definition and its entry from the traces pipeline exporters list. The traces pipeline is now `[clickhouse/traces, spanmetrics, servicegraph]`; ClickHouse is the sole trace store and the connectors are the sole span-metrics/service-graph source. Header + inline comments updated to reflect the completed cutover.
- **`docker/docker-compose.yml`** — deleted the `loki`, `promtail`, `tempo-init`, and `tempo` service blocks; removed `grafana` `depends_on: loki` (RESEARCH Pitfall 6 — a dangling edge blocks compose startup) and `otel-collector` `depends_on: tempo`; updated the Prometheus remote-write comment (collector is now the sole writer).

### Task 3 — Remove Tempo+Loki datasources + delete orphaned configs (commit `ba982eb2`)
- **`datasources.yml`** — deleted the Tempo datasource (`aenigma-tempo`) + its `serviceMap`/`tracesToMetrics` convenience config (the ONLY consumer of `traces_spanmetrics_*` / `traces_service_graph_*` metric names), and the Loki datasource (`aenigma-loki`) + its `derivedFields → aenigma-tempo` correlation. Retained Prometheus, PostgreSQL, and ClickHouse (events+traces+logs).
- `git rm infra/tempo/tempo.yaml docker/loki/loki-config.yml docker/promtail/config.yml` (no longer mounted by any service).

### Task 4 — Apply cutover + final verification (deleteDatasources fix: commit `128b63de`)
- Pre-recreate guard: `otelcol validate` on the Tempo-free config → **exit 0** (RESEARCH Pitfall 5).
- Recreated the collector (`up -d --no-deps --force-recreate otel-collector`) → Up, no errors.
- Removed the retired containers (`up -d --remove-orphans` pruned tempo/tempo-init/loki/promtail) and restarted Grafana.
- **Deviation (Rule 3):** removing the datasource blocks did NOT prune the already-provisioned `editable:false` Tempo+Loki datasources from Grafana's DB (the API returns `Cannot delete read-only data source`). Added a `deleteDatasources` provisioning block; after restart only ClickHouse / PostgreSQL / Prometheus remain.
- Optional cleanup: reclaimed orphaned `docker_tempo_data` + `docker_loki_data` volumes. MinIO `tempo` bucket cleanup deferred (mc not trivially available; harmless orphan).

## Final Verification (live production stack)

| Req | Check | Result |
|-----|-------|--------|
| **AR-CONS-01** | `docker ps` has tempo/tempo-init | **NONE** ✓ |
| AR-CONS-01 | `SELECT count() FROM analytics.otel_traces` | **2506** (climbing past the 06-02 baseline of 1192 → fresh ingest after Tempo removal) ✓ |
| AR-CONS-01 | datasources.yml / Grafana has `aenigma-tempo` | **NO** (pruned via deleteDatasources) ✓ |
| AR-CONS-01 | backend-tracing dashboard binding | grafana-clickhouse-datasource, **no** aenigma-tempo, **no** traceqlSearch; 3 slow-trace (>1s) rows to render ✓ |
| **AR-CONS-02** | `docker ps` has loki/promtail | **NONE** ✓ |
| AR-CONS-02 | `otel_logs` last 5 min | **2497** rows (18 with non-empty TraceId) ✓ |
| AR-CONS-02 | logs↔traces `INNER JOIN ON TraceId` (30 min) | **58** rows resolve → correlation works ✓ |
| AR-CONS-02 | ClickHouse datasource `/health` | `{"status":"OK","message":"Data source is working"}` ✓ |
| **AR-CONS-03** | Grafana unified alert rules | **15 rules, all health=ok, NONE in error** ✓ |
| AR-CONS-03 | Prometheus rule files in error | none (`groups:[]` — alerts are Grafana-managed) ✓ |
| AR-CONS-03 | span-metrics live: `calls_total{source=otelcol}` | **8 series, actively advancing** (last sample ts climbing) — collector connector is the SOLE writer ✓ |
| AR-CONS-03 | stale Tempo `traces_spanmetrics_calls_total` | frozen at the last pre-removal sample, series count decaying (13→9) as expected ✓ |
| AR-CONS-03 | `traces_service_graph_request_total{source=otelcol}` | 7 series (collector servicegraph) ✓ |
| AR-CONS-03 | metric dashboards present | animeenigma-services, playback-health, product-analytics, activity-register-{overview,flow,pivot}, + all others ✓ |
| AR-CONS-03 | Prometheus datasource executes a panel query (`sum(up)` via Grafana `/api/ds/query`) | 1 frame returned ✓ |
| AR-CONS-03 | rules.yml / metric dashboards | **untouched** ✓ |

The decisive AR-CONS-03 evidence: the collector's `calls_total{source=otelcol}` last real sample timestamp keeps advancing while Tempo's `traces_spanmetrics_calls_total` is frozen and its series decay — proving span-metrics still flow to Prometheus from the collector connector AFTER Tempo is gone (RESEARCH Pitfall 1 / 06-01 carry-forward: the live span-metric source is `calls_total`, not the Tempo-named series).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Provisioned read-only Tempo+Loki datasources not pruned by block removal**
- **Found during:** Task 4 (post-restart datasource verification)
- **Issue:** After deleting the Tempo+Loki blocks from `datasources.yml` and restarting Grafana, both datasources STILL appeared in the Grafana API. Grafana provisioning does not delete a removed-from-file datasource from its DB, and the `DELETE /api/datasources` call returns `Cannot delete read-only data source` because they are `editable: false`. The cutover would have left dangling Tempo/Loki datasources.
- **Fix:** Added a `deleteDatasources:` provisioning block (Tempo + Loki, orgId 1) — the supported mechanism to prune provisioned datasources on boot. After restart, only ClickHouse / PostgreSQL / Prometheus remain.
- **Files modified:** docker/grafana/provisioning/datasources/datasources.yml
- **Commit:** 128b63de

### Notes (not deviations)

- The plan's automated verify for Task 4 checked `traces_spanmetrics_calls_total` presence; that name belongs to Tempo's (now-removed) generator and is decaying. The AR-CONS-03 *intent* (span-metrics survive Tempo's removal) is proven via the live, advancing `calls_total{source=otelcol}` connector series, exactly as flagged in the 06-01 carry-forward note.
- AR-CONS-03 alert survival was validated against Grafana unified alerting (15 rules, all `ok`) because Prometheus has no native rule files (`/prometheus/api/v1/rules` → `groups:[]`).

## Out-of-Scope / Deferred (logged in deferred-items.md)

- **`animepahe-resolver` Up 2 weeks (unhealthy)** — pre-existing since 2026-05-19, unrelated to this phase. A `docker compose up -d` refused to start a sibling that depends on it, but disturbed no running service. Not in scope.
- **`meilisearch` / `notifications` unhealthy** — pre-existing, untouched.
- **MinIO `tempo` bucket** — DEFERRED (mc not trivially available in the minio image); harmless orphan. The bulk reclaimable space (tempo/loki data volumes) was reclaimed.

## Threat Model Verification

- **T-06-08 (config typo halts traces):** mitigated — `otelcol validate` exited 0 BEFORE the recreate; collector came up clean, otel_traces kept climbing.
- **T-06-09 (grafana fails on dangling depends_on:loki):** mitigated — `depends_on: loki` removed in the same change as the Loki service deletion; `docker compose config` validated; Grafana restarted cleanly.
- **T-06-10 (ClickHouse single SPOF):** accepted as planned — Prometheus alerting stays independent of CH; clickhouse-backup sidecar present.
- **T-06-11 (no new exposed CH port):** verified — no port changes; CH stays 127.0.0.1-bound; deletions only removed ports.
- **T-06-12 (accidental rules.yml / metric-dashboard edit):** mitigated — rules.yml and metric dashboards untouched; final check confirms all 15 alert rules health=ok and dashboards render.
- **T-06-SC (package installs):** accepted — config/provisioning removal only; zero installs.

## Self-Check: PASSED

- infra/otel/collector-config.yaml — FOUND (Tempo exporter gone, validates, collector live)
- docker/docker-compose.yml — FOUND (loki/promtail/tempo/tempo-init blocks deleted, depends_on edges removed, config valid)
- docker/grafana/provisioning/datasources/datasources.yml — FOUND (Tempo+Loki datasources removed + deleteDatasources prune; CH/PG/Prom retained)
- infra/tempo/tempo.yaml, docker/loki/loki-config.yml, docker/promtail/config.yml — DELETED (confirmed absent)
- Commits 42b328a9, ba982eb2, 128b63de — all present in git log
- Live: no tempo/loki/promtail containers; otel_traces=2506; otel_logs(5m)=2497; collector span-metrics advancing; 15 Grafana alert rules all ok; metric dashboards render
