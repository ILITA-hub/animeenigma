---
phase: 03-db-cache-effects-auto-operation-discovery
plan: 02
subsystem: observability
tags: [tempo, prometheus, grafana, metrics-generator, span-metrics, service-graphs, RED, config]
requires:
  - "Tempo 2.4.1 single-binary already deployed (docker-compose tempo service)"
  - "Prometheus v2.50.1 with /prometheus route-prefix"
  - "Grafana Tempo datasource (uid aenigma-tempo) + Prometheus datasource (uid PBFA97CFB590B2093)"
provides:
  - "Tempo metrics_generator: span-metrics + service-graphs processors enabled, remote_write to Prometheus"
  - "Prometheus remote-write-receiver enabled at /prometheus/api/v1/write"
  - "Grafana Tempo datasource wired to Prometheus for serviceMap + tracesToMetrics"
affects:
  - "Phase 6 (D-14) will retire Tempo and regenerate these RED metrics via the OTel Collector spanmetrics connector"
tech-stack:
  added: []
  patterns:
    - "Config-only observability: per-operation RED + service graph from existing traces, zero per-span application code"
key-files:
  created: []
  modified:
    - infra/tempo/tempo.yaml
    - docker/docker-compose.yml
    - docker/grafana/provisioning/datasources/datasources.yml
decisions:
  - "remote_write URL carries the /prometheus route-prefix (http://prometheus:9090/prometheus/api/v1/write) to match --web.route-prefix"
  - "operation dimension included in span_metrics to surface the per-op RED metric (bounded label per T-03-05)"
  - "serviceMap + tracesToMetrics both bind to the existing Prometheus uid PBFA97CFB590B2093 (never rename — orphaned 14 alert rules before)"
metrics:
  duration: ~6m
  completed: 2026-06-05
---

# Phase 3 Plan 02: Tempo Span-Metrics + Service-Graphs Generator Summary

Enabled Tempo's `metrics_generator` (span-metrics + service-graphs) with remote_write to Prometheus and wired the Grafana Tempo datasource to Prometheus for the service graph + per-operation RED metrics — config-only across 4 infra files, no application code touched (AR-EFFECT-04 / D-12).

## What Was Built

### Task 1 — Tempo metrics_generator + Prometheus remote-write-receiver (commit `bec6d610`)
- Added a top-level `metrics_generator:` block to `infra/tempo/tempo.yaml`:
  - `registry.external_labels.source: tempo`
  - `storage.path: /var/tempo/generator/wal` (on the existing writable `tempo_data` volume)
  - `storage.remote_write` → `http://prometheus:9090/prometheus/api/v1/write` with `send_exemplars: true`
  - `traces_storage.path: /var/tempo/generator/traces`
  - `processor.span_metrics.dimensions: [service.name, span.name, operation]` (the `operation` dimension surfaces the per-op RED metric)
  - `processor.service_graphs.dimensions: [service.name]`
- Added the **per-tenant enable** `overrides.defaults.metrics_generator.processors: [span-metrics, service-graphs]` — without it the generator silently no-ops (RESEARCH Pitfall 6).
- Added `--web.enable-remote-write-receiver` to the Prometheus container args in `docker/docker-compose.yml` (it was **absent** — verified, so it had to be added so `/prometheus/api/v1/write` accepts the push). The existing `--web.external-url` / `--web.route-prefix=/prometheus` args were left intact.

### Task 2 — Grafana Tempo → Prometheus wiring (commit `c3d27f78`)
- On the Tempo datasource `jsonData` in `docker/grafana/provisioning/datasources/datasources.yml`:
  - `serviceMap.datasourceUid: PBFA97CFB590B2093` (existing Prometheus uid) so the service graph resolves `traces_service_graph_*`.
  - `tracesToMetrics` block pointing at the same Prometheus uid (with a `traces_spanmetrics_calls_total` RED-rate query) so per-operation RED panels resolve.
  - Kept `nodeGraph.enabled: true`; left the Prometheus/Loki/PostgreSQL/ClickHouse datasource entries untouched.
  - Used `$$`-escaped Grafana macros (`$$__tags`, `$$__rate_interval`) because file-provisioning interpolates bare `${}`/`$` (same escaping pattern as the existing Loki `$${__value.raw}`).

## Verification

All plan acceptance criteria passed at execution time:

| Check | Result |
|-------|--------|
| `python3 ... tempo metrics_generator OK` | PASS |
| `grep -c 'metrics_generator' infra/tempo/tempo.yaml` >= 2 | 2 |
| `grep -c 'span-metrics' infra/tempo/tempo.yaml` >= 1 | 1 |
| `grep -c 'enable-remote-write-receiver' docker/docker-compose.yml` >= 1 | 1 |
| remote_write URL ends with `/prometheus/api/v1/write` | PASS |
| `python3 ... grafana tempo->prom wiring OK` | PASS |
| `grep -c 'serviceMap' datasources.yml` >= 1 | 2 |
| `grep -c 'tracesToMetrics' datasources.yml` >= 1 | 2 |
| serviceMap/tracesToMetrics uid == Prometheus uid (PBFA97CFB590B2093) | PASS |
| nodeGraph.enabled still true | PASS |

Live confirmation of `traces_spanmetrics_*` in Prometheus + the rendered service graph in Grafana is the **phase-gate checkpoint in plan 06** (not exercised here — this plan ships config only and never starts the stack).

## Deviations from Plan

None — plan executed exactly as written. Both files the plan flagged as "verify, don't assume" matched the RESEARCH expectations:
- The Prometheus route-prefix is `/prometheus`, so the assumed remote_write URL was correct.
- `--web.enable-remote-write-receiver` was confirmed **absent** and was added (the plan anticipated this — "add it if absent").
- The Tempo container already mounts `tempo_data:/var/tempo`, so the generator WAL/traces paths are writable with no compose volume change.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, or trust-boundary surface beyond what the plan's threat register (T-03-04 accept, T-03-05 mitigate) already covers. The `operation` span-metrics dimension is the bounded service-frame label called out in T-03-05, not raw URLs.

## TDD Gate Compliance

N/A — config-only plan, no `tdd="true"` tasks.

## Self-Check: PASSED

- All 3 modified files present on disk.
- SUMMARY.md present on disk.
- All commits exist in git log: `bec6d610` (Task 1), `c3d27f78` (Task 2), `b61c0fde` (SUMMARY).
- Working tree clean.
