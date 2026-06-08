---
phase: 06-consolidation-topology-a
verified: 2026-06-08T02:10:00Z
status: human_needed
score: 5/5
overrides_applied: 0
human_verification:
  - test: "Visual render of backend-tracing dashboard in Grafana"
    expected: "The 'Recent slow traces (>1s)' panel renders trace rows/spans from ClickHouse (not Tempo); trace click-through to span detail works"
    why_human: "DS-NF-06 (standing rule): Tailwind/jsdom cannot catch cascade bugs; Grafana panel rendering must be confirmed in a real browser. The executor confirmed the human gate was approved by the user at the orchestrator level before 06-03 ran, but that human confirmation is not recorded as a programmatic assertion in this report."
  - test: "Visual render of ClickHouse log view in Grafana"
    expected: "Recent logs display with non-empty TraceId values; clicking trace_id in a log row navigates to the matching trace in the ClickHouse trace view"
    why_human: "Logs-to-traces click-through correlation is a Grafana UI behavior that cannot be verified by grep or curl — requires a human to open the Grafana log view, confirm logs render with TraceId, and confirm the link resolves."
---

# Phase 6: Consolidation Topology A — Verification Report

**Phase Goal:** ClickHouse becomes the single event/trace/log plane — Tempo and Loki are retired and their views repointed — while Prometheus + Grafana stay as the metrics + alerting + rendering layer.
**Verified:** 2026-06-08T02:10:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Tempo and tempo-init containers are not running | VERIFIED | `docker ps` output contains no `tempo` or `tempo-init` container. Confirmed live. |
| 2 | Loki and Promtail containers are not running | VERIFIED | `docker ps` output contains no `loki` or `promtail` container. Confirmed live. |
| 3 | ClickHouse `analytics.otel_traces` receives live traces | VERIFIED | `SELECT count() FROM analytics.otel_traces WHERE Timestamp > now() - INTERVAL 10 MINUTE` → **2337** rows. Total cumulative: 4920. Actively growing. |
| 4 | ClickHouse `analytics.otel_logs` receives live logs with populated TraceId | VERIFIED | `SELECT count() FROM analytics.otel_logs WHERE TraceId != '' AND Timestamp > now() - INTERVAL 10 MINUTE` → **188** rows. Recent 10 min total: 5485. TraceId promotion via filelog trace_parser is working. |
| 5 | Prometheus + Grafana alerting/metrics layer is intact (AR-CONS-03) | VERIFIED | Grafana unified alerting: 15 rules, 0 in error state (all `health=ok`). Span-metrics from OTel connector: `calls_total{source="otelcol"}` → 33 series actively advancing; `traces_service_graph_request_total{source="otelcol"}` → 16 series. `rules.yml` untouched (git diff clean). |

**Score:** 5/5 truths verified (automated)

### Deferred Items

No truths deferred to later phases. Phase 6 is the final phase of the v4.0 milestone.

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `infra/otel/collector-config.yaml` | Tempo-free traces pipeline; clickhouse/traces + clickhouse/logs exporters; filelog receiver; spanmetrics + servicegraph connectors; prometheusremotewrite exporter | VERIFIED | `otlp/tempo` exporter absent. Traces pipeline exporters: `[clickhouse/traces, spanmetrics, servicegraph]`. filelog receiver with json_parser + timestamp parser + move + regex_parser + trace_parser operators. prometheusremotewrite endpoint `http://prometheus:9090/prometheus/api/v1/write` (correct route-prefix). |
| `docker/docker-compose.yml` | loki/promtail/tempo/tempo-init services deleted; grafana depends_on:loki removed; otel-collector depends_on:tempo removed | VERIFIED | Python YAML parse confirms: `tempo`, `tempo-init`, `loki`, `promtail` not present in services. otel-collector depends_on: `[clickhouse, prometheus]` (tempo absent). grafana depends_on: `[prometheus]` (loki absent). |
| `docker/grafana/provisioning/datasources/datasources.yml` | Tempo + Loki datasource blocks removed; deleteDatasources prune block present; ClickHouse/PostgreSQL/Prometheus retained | VERIFIED | `aenigma-tempo` and `aenigma-loki` UIDs absent from `datasources:` block. `deleteDatasources:` block present targeting Tempo + Loki (orgId 1). Active datasources per Grafana API: `['ClickHouse', 'PostgreSQL', 'Prometheus']`. |
| `infra/grafana/dashboards/backend-tracing.json` | datasource type `grafana-clickhouse-datasource` uid `aenigma-clickhouse`; no `traceqlSearch`; no `aenigma-tempo` | VERIFIED | Panel: `ds_type=grafana-clickhouse-datasource`, `ds_uid=aenigma-clickhouse`, `queryType=traces`. `has aenigma-tempo: False`. `has traceqlSearch: False`. Dashboard uid `backend-tracing` preserved. |
| `infra/tempo/tempo.yaml` | Deleted | VERIFIED | File does not exist. |
| `docker/loki/loki-config.yml` | Deleted | VERIFIED | File does not exist. |
| `docker/promtail/config.yml` | Deleted | VERIFIED | File does not exist. |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `collector-config.yaml` traces pipeline | `clickhouse:9000 analytics.otel_traces` | clickhouse/traces exporter | WIRED | `tcp://clickhouse:9000?dial_timeout=10s` present; `traces_table_name: otel_traces`; live count 4920. |
| `collector-config.yaml` logs pipeline | `clickhouse:9000 analytics.otel_logs` | clickhouse/logs exporter | WIRED | `logs_table_name: otel_logs`; TTL 168h; 5485 recent rows, 188 with non-empty TraceId. |
| `collector-config.yaml` metrics/spanmetrics pipeline | `prometheus:9090/prometheus/api/v1/write` | prometheusremotewrite exporter | WIRED | Endpoint contains `/prometheus/api/v1/write` (correct route-prefix). Live: 33 `calls_total{source=otelcol}` series + 16 service_graph series. |
| `datasources.yml` ClickHouse datasource | `analytics.otel_traces` + `analytics.otel_logs` | grafana-clickhouse-datasource in OTel mode | WIRED | `traces.defaultTable: otel_traces`, `logs.defaultTable: otel_logs`, `otelEnabled: true`. Grafana API datasource health: `{"status":"OK","message":"Data source is working"}`. |
| `backend-tracing.json` panel | `aenigma-clickhouse` | grafana-clickhouse-datasource | WIRED | Panel datasource bound to `aenigma-clickhouse`; no Tempo binding. |
| `grafana depends_on` | (removed loki) | edge deleted | WIRED | `depends_on: loki` removed; compose validates; Grafana started cleanly. |

---

## Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| `backend-tracing.json` slow-traces panel | `otel_traces` rows via rawSql | `analytics.otel_traces` CH table | Yes — 4920 cumulative traces, actively growing from live OTLP ingest | FLOWING |
| `otel_logs` (datasource config) | `otel_logs` rows | `analytics.otel_logs` CH table | Yes — 5485 recent rows; 188 with non-empty TraceId (trace_parser working) | FLOWING |
| Prometheus span-metrics | `calls_total{source=otelcol}` | `metrics/spanmetrics` pipeline → prometheusremotewrite | Yes — 33 series, timestamps actively advancing | FLOWING |

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| ClickHouse traces ingestion (last 10 min) | `SELECT count() FROM analytics.otel_traces WHERE Timestamp > now() - INTERVAL 10 MINUTE` | 2337 | PASS |
| ClickHouse logs with TraceId (last 10 min) | `SELECT count() FROM analytics.otel_logs WHERE TraceId != '' AND Timestamp > now() - INTERVAL 10 MINUTE` | 188 | PASS |
| Collector span-metrics in Prometheus | `calls_total{source="otelcol"}` | 33 series | PASS |
| Service-graph metrics in Prometheus | `traces_service_graph_request_total{source="otelcol"}` | 16 series | PASS |
| Grafana alert rules — no error state | Grafana `/api/prometheus/grafana/api/v1/rules` | 15 rules, 0 errors, all health=ok | PASS |
| Active Grafana datasources | Grafana `/api/datasources` | `['ClickHouse', 'PostgreSQL', 'Prometheus']` — no Tempo, no Loki | PASS |
| No retired containers running | `docker ps` | No tempo/tempo-init/loki/promtail containers | PASS |
| otel-collector running | `docker inspect animeenigma-otel-collector` | Status: running | PASS |
| Collector logs — no fatal errors | `docker logs --tail 20` | Only pre-existing benign 0.0.0.0 DoS warning — no fatal/panic | PASS |

---

## Probe Execution

No probe scripts declared for this phase. Verification was operational (compose config validation, live CH queries, Grafana API checks). `otelcol validate` was run as a pre-recreate guard during execution (exits 0, documented in 06-03-SUMMARY).

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| AR-CONS-01 | 06-01, 06-02, 06-03 | OTel Collector exports traces to ClickHouse; Tempo retired (datasource + container removed; backend-tracing dashboard repointed) | SATISFIED | No tempo/tempo-init containers; `analytics.otel_traces` count=4920 and growing; `aenigma-tempo` datasource absent from Grafana; `backend-tracing.json` bound to `aenigma-clickhouse` with `queryType=traces`. |
| AR-CONS-02 | 06-01, 06-02, 06-03 | Logs shipped to ClickHouse; Loki retired (datasource + container removed; log views repointed) | SATISFIED | No loki/promtail containers; `analytics.otel_logs` recent count=5485; 188 rows with non-empty TraceId (log↔trace correlation working); `aenigma-loki` datasource absent from Grafana; `datasources.yml` ClickHouse source has OTel log config. |
| AR-CONS-03 | 06-01, 06-02, 06-03 | Prometheus + Grafana remain the metrics + alerting + rendering layer (unchanged) | SATISFIED | 15 Grafana alert rules, 0 in error state; `calls_total{source=otelcol}` 33 series + service_graph 16 series (collector connector = sole span-metrics writer); `rules.yml` git diff clean (untouched); Prometheus/ClickHouse/PostgreSQL datasources intact. |

All three requirement IDs from PLAN frontmatter are accounted for and satisfied. REQUIREMENTS.md marks all three as `[x]` (complete).

---

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `infra/otel/collector-config.yaml` | multiple | Inline comments labeled "Rule 1 bug, caught live" and pitfall/research references | Info | Documentation of real bugs caught during execution; not implementation stubs. No `TBD`/`FIXME`/`XXX` debt markers. |

No blocker anti-patterns. No `TBD`, `FIXME`, or `XXX` debt markers found in any phase-modified file.

**Deferred items (documented in `deferred-items.md`, out-of-scope):**
- `animepahe-resolver` container unhealthy — pre-existing since 2026-05-19, unrelated to this phase.
- `meilisearch` / `notifications` containers unhealthy — pre-existing, untouched.
- MinIO `tempo` bucket — harmless orphan (Tempo no longer writes to it); `docker_tempo_data` and `docker_loki_data` volumes were reclaimed.

---

## Human Verification Required

### 1. Grafana backend-tracing dashboard visual render

**Test:** Open `https://animeenigma.ru/admin/grafana` (or the prod Grafana URL), navigate to the `backend-tracing` dashboard, and confirm the "Recent slow traces (>1s)" panel renders rows from ClickHouse (not a datasource-error state). Click a trace row to confirm the trace detail/span view opens.

**Expected:** At least one slow-trace row visible; no "datasource not found" or "query failed" banner; span detail renders for the clicked trace.

**Why human:** DS-NF-06 standing rule — Grafana panel rendering must be confirmed in a real browser. The automated check confirmed the panel datasource binding and that the rawSql query returns 1 frame via `/api/ds/query`, but visual rendering of the Grafana panel cannot be verified programmatically.

**Note:** Per the 06-03-SUMMARY, the user confirmed this visual gate at the orchestrator level before the destructive 06-03 tasks ran. This item is for formal closure in this verification report.

### 2. ClickHouse logs↔traces click-through correlation

**Test:** Open the Grafana Explore view with the ClickHouse datasource in Logs mode. Confirm recent logs appear. Click a log row that has a TraceId value. Confirm the "Related traces" link opens and shows the matching trace.

**Expected:** Log row with non-empty TraceId resolves to a trace in the ClickHouse trace view; no "data source not found" error on the link.

**Why human:** The `traceToLogs` / `logsToTrace` configuration is verified to be present in datasources.yml and the live ClickHouse datasource has 188 rows with non-empty TraceId in the last 10 minutes, but the click-through navigation is a Grafana UI behavior requiring a browser.

---

## Gaps Summary

No automated gaps found. All 5 observable truths are VERIFIED against the live stack. All 3 requirement IDs (AR-CONS-01, AR-CONS-02, AR-CONS-03) are satisfied with codebase and live-query evidence.

The two human verification items above are the only open items. Both concern Grafana visual rendering (DS-NF-06). Per the 06-03-SUMMARY, the user approved the visual render gate at the orchestrator level before 06-03 destructive tasks ran — these items are for formal documentation closure, not a blocking gap.

---

_Verified: 2026-06-08T02:10:00Z_
_Verifier: Claude (gsd-verifier)_
