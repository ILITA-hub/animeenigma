---
phase: 06-consolidation-topology-a
plan: 01
subsystem: observability
tags: [otel-collector, clickhouse, filelog, spanmetrics, servicegraph, prometheus-remote-write, gated-cutover]
requires:
  - "ClickHouse analytics plane (Phase 1) live on clickhouse:9000"
  - "Prometheus --web.enable-remote-write-receiver + --web.route-prefix=/prometheus"
  - "otel/opentelemetry-collector-contrib:0.103.1 (clickhouseexporter, filelogreceiver, spanmetrics/servicegraph connectors)"
provides:
  - "OTLP traces dual-exported to ClickHouse analytics.otel_traces (alongside still-live Tempo)"
  - "Container logs in ClickHouse analytics.otel_logs via filelog receiver (envelope unwrapped, timestamp parsed)"
  - "Collector-native span-metrics (calls_total/duration_milliseconds_*) + service-graph (traces_service_graph_*) remote-written to Prometheus with source=otelcol, independent of Tempo"
affects:
  - "06-02 (datasource/dashboard repoint) — must use collector metric names (calls_total, NOT traces_spanmetrics_calls_total) and otel_traces/otel_logs CH tables"
  - "06-03 (destructive removal) — Tempo/Loki/Promtail removal is now a pure deletion; CH path proven live"
tech-stack:
  added: []
  patterns:
    - "Two named ClickHouse exporter instances (clickhouse/traces 336h, clickhouse/logs 168h) for distinct per-signal TTLs"
    - "filelog json_parser + timestamp parser + move(attributes.log->body) to ingest docker json-file logs"
    - "spanmetrics/servicegraph connectors feeding a prometheusremotewrite exporter (route-prefixed endpoint)"
    - "external_labels source:otelcol to distinguish collector series from Tempo's source:tempo during co-emit gate"
key-files:
  created: []
  modified:
    - infra/otel/collector-config.yaml
    - docker/docker-compose.yml
decisions:
  - "Run otel-collector as user:0 — the contrib image's default uid 10001 cannot traverse 0710 root:root /var/lib/docker/containers (filelog permission denied); mirrors the grafana service's user:0"
  - "spanmetrics connector lists only the EXTRA 'operation' dimension — service.name/span.name are built-in and listing them is a duplicate-dimension config error in 0.103.1"
  - "filelog must parse the docker 'time' field into the record Timestamp, else rows land with epoch-0 timestamp and the 168h TTL instantly expires them (total_rows climbs, count()=0)"
metrics:
  duration: "~10 min"
  completed: "2026-06-08"
  tasks: 3
  files: 2
  commits: 5
---

# Phase 6 Plan 01: ClickHouse Trace + Log Export & Collector-Native Span-Metrics (Additive Front Half of Gated Cutover) Summary

Stood up the new ClickHouse trace+log export path and OTel-Collector-native span-metrics/service-graph generation ALONGSIDE the still-running Tempo/Loki/Promtail stack — purely additive, nothing removed. The reversible front half of the gated observability consolidation is now proven live: traces dual-export to ClickHouse + Tempo, container logs ship to ClickHouse via filelog, and RED span-metrics + service-graph reach Prometheus from the collector's connectors independent of Tempo.

## What Was Built

- **Collector config (`infra/otel/collector-config.yaml`)** — added a `filelog` receiver (docker json-file glob, json envelope unwrap, timestamp parse, `start_at: end`); `spanmetrics` + `servicegraph` connectors; two named ClickHouse exporters (`clickhouse/traces` ttl 336h, `clickhouse/logs` ttl 168h) using `${env:CLICKHOUSE_USER/PASSWORD}`; a `prometheusremotewrite` exporter at the `/prometheus`-prefixed endpoint with `external_labels source:otelcol`. Traces pipeline now fans out to `[otlp/tempo, clickhouse/traces, spanmetrics, servicegraph]` (Tempo kept); new `logs` and `metrics/spanmetrics` pipelines added.
- **Compose (`docker/docker-compose.yml`)** — otel-collector gets CH creds in env, a read-only `/var/lib/docker/containers` mount for filelog, `depends_on` clickhouse+prometheus (tempo kept), and `user: "0"` so it can read root-owned docker logs.

## Live Verification (production stack)

| Check | Result |
|-------|--------|
| `otelcol validate` (pre-recreate guard) | exits 0 (after fixing two config issues — see Deviations) |
| collector container | Up, not restart-looping, no fatal/permission errors in logs |
| `SELECT count() FROM analytics.otel_traces` | 781 (and climbing) |
| `analytics.otel_logs` last 5 min, plain-message Body | 1602 rows, bodies are raw messages (not the `{"log":...}` envelope), timestamps correct (2026-06-08) |
| `calls_total{source="otelcol"}` (collector spanmetrics) | 18 series |
| `traces_service_graph_request_total{source="otelcol"}` (collector servicegraph) | 11 series |
| `traces_spanmetrics_calls_total` (Tempo, still live) | 19 series (gate co-emit intact) |
| Tempo / Loki / Promtail containers | all still Up/healthy (gate intact) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] spanmetrics connector duplicate-dimension config error**
- **Found during:** Task 3 (pre-recreate `otelcol validate` guard)
- **Issue:** The plan/RESEARCH snippet listed `service.name`, `span.name`, AND `operation` as spanmetrics `dimensions` (mirroring Tempo's processor). In OTel collector 0.103.1 the spanmetrics connector emits `service.name`/`span.name` as **built-in** dimensions, so listing them produced `failed validating dimensions: duplicate dimension name service.name` and the config would not load.
- **Fix:** Reduced `spanmetrics.dimensions` to only the extra `operation` dimension.
- **Files modified:** infra/otel/collector-config.yaml
- **Commit:** 514e8758
- **Note:** The pre-recreate validate guard (RESEARCH Pitfall 5 / Assumption A1) did exactly its job — caught a broken config BEFORE it could halt all traces.

**2. [Rule 3 - Blocking] filelog permission denied on /var/lib/docker/containers**
- **Found during:** Task 3 (post-recreate, collector log)
- **Issue:** `/var/lib/docker/containers` is `0710 root:root` with `0700` per-container subdirs; the contrib image's default uid `10001` could not traverse them — filelog logged `open .: permission denied` and ingested zero logs. (Promtail avoids this by reading via docker.sock SD, not the file path — RESEARCH Assumption A5.)
- **Fix:** Set `user: "0"` on the otel-collector service (mirrors the grafana service in the same compose).
- **Files modified:** docker/docker-compose.yml
- **Commit:** 14bfd069

**3. [Rule 1 - Bug] otel_logs rows instantly TTL-expired (epoch-0 timestamp)**
- **Found during:** Task 3 (logs verification — `system.tables.total_rows`=1100 but `count()`=0)
- **Issue:** filelog records had no timestamp mapped to the OTel `Timestamp` field, so it defaulted to `1970-01-01`. The `clickhouse/logs` exporter's `TTL toDateTime(Timestamp) + 168h` then expired every row at insert time. Exporter telemetry confirmed `otelcol_exporter_sent_log_records=534, send_failed=0` yet `count()=0` — the rows were inserted then immediately TTL-dropped.
- **Fix:** Added a `timestamp` parser to the filelog `json_parser` operator reading the docker `time` field (RFC3339Nano, layout `%Y-%m-%dT%H:%M:%S.%fZ`). otel_logs now retains rows with correct timestamps.
- **Files modified:** infra/otel/collector-config.yaml
- **Commit:** 3ac7d50b
- **Diagnostic method:** ran throwaway debug collectors with `verbosity: detailed` + `:8888` internal-telemetry metrics (`otelcol_exporter_sent_log_records`) to prove filelog read + export succeeded, then traced the disappearance to the TTL via `system.tables.total_rows` vs `count()`.

## Naming Note for 06-02 (carry forward, not a defect)

The OTel spanmetrics connector emits its default metric namespace: `calls_total` + `duration_milliseconds_*` — **NOT** Tempo's `traces_spanmetrics_calls_total`. The servicegraph connector DOES match Tempo's `traces_service_graph_*` names. So:
- The plan's literal acceptance criterion ("`traces_spanmetrics_calls_total` returns >= 1 result") passes because **Tempo's generator is still live** (source=tempo, 19 series).
- The AR-CONS-03 intent (collector-native span-metrics survival independent of Tempo) is proven via `calls_total{source="otelcol"}` (18 series) + `traces_service_graph_request_total{source="otelcol"}` (11 series).
- **06-02 dashboards/datasources that reference `traces_spanmetrics_*` will need to either point at `calls_total`/`duration_milliseconds_*` or set `spanmetrics.namespace: traces.spanmetrics` on the connector to restore the Tempo-compatible metric names.** Flagging so the 06-02 planner accounts for it before Tempo is deleted in 06-03.

The `operation` dimension is present in the connector config and appears on spans that emit it; gateway HTTP spans show `operation=null` (they don't set the attribute) — same selective behavior Tempo had.

## Threat Model Verification

- **T-06-01 (config typo halts traces):** mitigated — `otelcol validate` ran BEFORE every recreate and caught the duplicate-dimension error; Tempo path stayed live throughout, so the change was fully reversible.
- **T-06-02 (CH creds disclosure):** mitigated — creds come from `docker/.env` via `${CLICKHOUSE_USER:-...}/${CLICKHOUSE_PASSWORD:-...}` compose defaults; no plaintext secret committed.
- **T-06-03 / T-06-04 (new port / log PII):** accepted as planned — no new host port published; CH stays 127.0.0.1-bound; filelog ingests the same container stdout Promtail already shipped (no new exposure).

## Self-Check: PASSED

- infra/otel/collector-config.yaml — FOUND (modified, validates, live)
- docker/docker-compose.yml — FOUND (modified, compose config valid, collector running)
- Commits c97dea6b, 853a0805, 514e8758, 14bfd069, 3ac7d50b — all present in git log
- Live: otel_traces populating, otel_logs retaining rows, collector span-metrics + service-graph in Prometheus, Tempo/Loki/Promtail still running
