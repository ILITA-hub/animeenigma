---
phase: 06-consolidation-topology-a
plan: 02
subsystem: observability
tags: [grafana, clickhouse, otel, trace-view, log-view, datasource-repoint, gated-cutover]
requires:
  - "06-01: analytics.otel_traces + analytics.otel_logs populated by the OTel Collector clickhouseexporter"
  - "grafana-clickhouse-datasource plugin v4.17.0 (OTel trace builder + trace-ID search + logs<->traces linking)"
  - "Existing aenigma-clickhouse Grafana datasource (Phase 1) + CH creds in the grafana container env"
provides:
  - "ClickHouse trace + log views in Grafana (OTel mode) on the existing aenigma-clickhouse datasource — events + traces + logs on ONE datasource"
  - "backend-tracing dashboard repointed off Tempo/TraceQL onto the ClickHouse OTel trace query (slow traces >1s)"
  - "Native logs<->traces correlation config (traceToLogs/logsToTrace on trace_id), replacing the Loki derivedField->Tempo link"
affects:
  - "06-03 (destructive removal): Tempo + Loki datasources/containers can now be deleted — the CH-backed trace/log surface is proven to render operationally; the manual DS-NF-06 visual render gate is the human checkpoint that precedes deletion"
tech-stack:
  added: []
  patterns:
    - "Extend the existing CH datasource IN PLACE (traces/logs blocks) instead of a new uid — eliminates the orphaned-uid/14-alert hazard entirely (no new uid created)"
    - "OTel trace builder query (Duration > 1e9 ns over otel_traces, root spans) as the TraceQL '{ duration > 1s }' equivalent"
    - "logsToTrace/traceToLogs both resolve to SELF (same CH datasource) joined on trace_id"
key-files:
  created: []
  modified:
    - docker/grafana/provisioning/datasources/datasources.yml
    - infra/grafana/dashboards/backend-tracing.json
decisions:
  - "Extended aenigma-clickhouse in place rather than adding aenigma-clickhouse-traces/-logs — lowest churn, zero new uid, and it makes events+traces+logs queryable from one datasource (the native trace_id correlation needs both signals on one source anyway)"
  - "Slow-trace panel uses rawSql (SELECT root spans WHERE Duration > 1e9 ns ORDER BY Timestamp DESC LIMIT 50) with builderOptions kept in OTel mode so the trace builder UI stays editable"
  - "Kept Tempo + Loki datasources AND containers live — this is the repoint half of a gated cutover; destructive removal is 06-03"
metrics:
  duration: "~4 min"
  completed: "2026-06-08"
  tasks: 3
  files: 2
  commits: 2
---

# Phase 6 Plan 02: Repoint Grafana Trace + Log Views to ClickHouse (Gated Cutover Back Half) Summary

Repointed Grafana's trace + log surface from Tempo/Loki onto ClickHouse and proved it renders operationally — while Tempo and Loki stay fully live as the safety net. The existing `aenigma-clickhouse` datasource was extended in place (no new uid) to carry traces (`analytics.otel_traces`) and logs (`analytics.otel_logs`) in OTel mode with native logs↔traces correlation on `trace_id`, and `backend-tracing.json` was moved off TraceQL onto the CH plugin's OTel trace query. The new surface is now the live trace/log view BEFORE 06-03 deletes the old stores.

## What Was Built

- **`docker/grafana/provisioning/datasources/datasources.yml`** — extended the existing `aenigma-clickhouse` datasource (grafana-clickhouse-datasource) IN PLACE with:
  - a `traces` block → `analytics.otel_traces`, `otelEnabled: true`, `otelVersion: latest`, OTel column mapping (Duration[ns]/TraceId/SpanId/SpanName/ServiceName/Timestamp)
  - a `logs` block → `analytics.otel_logs`, `otelEnabled: true`, Timestamp/SeverityText/Body mapping
  - `traceToLogs` + `logsToTrace` both pointing at the SAME datasource (`aenigma-clickhouse`) — native trace_id correlation replacing the Loki `derivedFields → aenigma-tempo` link
  - Tempo + Loki datasource blocks left untouched (gate intact). Creds stay bare `${CLICKHOUSE_USER}`/`${CLICKHOUSE_PASSWORD}`; `editable: false`.
- **`infra/grafana/dashboards/backend-tracing.json`** — repointed the single `traces` panel:
  - datasource `{type: tempo, uid: aenigma-tempo}` → `{type: grafana-clickhouse-datasource, uid: aenigma-clickhouse}`
  - dropped `queryType: traceqlSearch` / `query: "{ duration > 1s }"`; replaced with the CH plugin OTel trace query (rawSql: root spans `WHERE Duration > 1000000000` ns, `ORDER BY Timestamp DESC LIMIT 50`) + `builderOptions` in OTel mode
  - description rewritten to honestly note the Tempo→ClickHouse move and the TraceQL→builder/SQL parity gap; `uid backend-tracing` preserved

## Live Verification (production stack)

| Check | Result |
|-------|--------|
| Grafana restart (mounted-file provisioning reload) | Up healthy; no datasource/dashboard provisioning errors in log since restart |
| CH datasource `/health` (`aenigma-clickhouse`) | `{"status":"OK","message":"Data source is working"}` |
| Datasource jsonData via API | `traces`, `logs`, `traceToLogs`, `logsToTrace` all present |
| `SELECT count() FROM analytics.otel_traces` | 1192 (climbing) |
| `otel_traces` slow (>1s, Duration > 1e9 ns) | 1 row (the slow-trace panel has data) |
| `otel_logs` last 10 min | 4970 rows (log view has data to render) |
| Slow-trace SQL driven through Grafana `/api/ds/query` against `aenigma-clickhouse` | 1 frame, 1 row (traceID `89a74ed1…`) — datasource executes the panel query end-to-end |
| `backend-tracing` panel datasource via Grafana dashboard API | resolves to `grafana-clickhouse-datasource` / `aenigma-clickhouse`, queryType `traces`, NO traceqlSearch |
| backend-tracing.json | valid JSON; no `"type": "tempo"` / `aenigma-tempo` binding (the word "Tempo" survives only in the honest description note) |
| Grafana-managed alert rules | 15 rules loaded, none orphaned (in-place extend created NO new uid → the 14-alert hazard never triggered) |
| Tempo + Loki containers | both still Up/healthy (gate intact) |
| Tempo + Loki datasource blocks | still present in datasources.yml (gate intact) |

## Feature-Parity Gaps (documented honestly)

| Capability Tempo/Loki had | ClickHouse replacement | Note |
|---------------------------|------------------------|------|
| TraceQL `{ duration > 1s }` | CH OTel trace query: SQL `WHERE Duration > 1e9` ns over `otel_traces` | No TraceQL syntax; the slow-trace use case is preserved via rawSql/builder. Builder kept in OTel mode so the panel stays editable. |
| Loki LogQL + label browser | CH OTel log view / SQL over `otel_logs` | No LogQL; SQL filtering instead. Ad-hoc log exploration UX differs. |
| Tempo trace→logs (`tracesToLogsV2`→Loki) | CH `traceToLogs`/`logsToTrace` (self, joined on trace_id) | Correlation config is provisioned natively. **Caveat below.** |

### Known caveat carried to 06-03 (NOT a 06-02 defect)

The native logs↔traces link is **configured correctly** on the datasource, but the `TraceId` column in `analytics.otel_logs` is currently **empty** (0 rows with a non-empty `TraceId`). The filelog receiver (06-01) ingests container stdout as a plain `Body` string; the app's `trace_id` is embedded inside the JSON log line (`libs/logger/logger.go:92`) and is NOT promoted into the OTel `TraceId` column. So the link target exists and renders log/trace views independently, but the *click-through correlation* won't resolve until filelog parses the inner JSON and maps `trace_id → TraceId`. This is an upstream **filelog-config** gap (06-01 receiver), not a datasource/dashboard gap — flagged here so 06-03 (and any follow-up) can decide whether to enhance the filelog operator before/after Tempo+Loki removal. The Loki link being replaced had the same practical limitation framing. Logs render; traces render; trace→logs config is in place.

## Deviations from Plan

None — the plan's preferred lowest-churn path (extend `aenigma-clickhouse` in place) was viable, so no fresh-uid datasource was needed. Tasks executed exactly as written. The one item to surface is the carried-forward caveat above (otel_logs `TraceId` population), which is a 06-01 filelog observation, not a deviation in this plan's work.

## Manual Visual Gate (DS-NF-06) — deferred to 06-03 human checkpoint

Per the plan + VALIDATION "Manual-Only Verifications", the in-browser eyeball of the rendered trace panel and log view (open `https://animeenigma.ru/admin/grafana` → backend-tracing, run the slow-trace query, confirm spans display from ClickHouse; open a CH log view and confirm recent logs display) is the manual render gate. It is the human checkpoint that precedes destructive removal in 06-03 and is intentionally NOT blocked on here. Operational proof (datasource health OK + the panel query returning the slow trace through Grafana's own `/api/ds/query`) is captured above as the executor-side evidence.

## Threat Model Verification

- **T-06-06 (orphaned-uid breaks 14 alerts):** mitigated — extended `aenigma-clickhouse` IN PLACE, created NO new uid, reused NONE of `PBFA97CFB590B2093`/`aenigma-postgres`/`aenigma-tempo`/`aenigma-loki`. Post-change: 15 Grafana alert rules all loaded, datasource health OK.
- **T-06-07 (CH creds disclosure):** mitigated — creds stay bare `${CLICKHOUSE_USER}`/`${CLICKHOUSE_PASSWORD}` from the grafana container env; no plaintext secret committed.
- **T-06-05 (datasource as auth bypass):** accepted as planned — no Grafana auth-proxy change; datasource stays `editable: false` (read-only).
- **T-06-SC (package installs):** accepted — provisioning + dashboard JSON only; zero package installs.

## Self-Check: PASSED

- `docker/grafana/provisioning/datasources/datasources.yml` — FOUND (extended, contains `otel`/`otel_traces`/`otel_logs`, Loki+Tempo blocks intact)
- `infra/grafana/dashboards/backend-tracing.json` — FOUND (valid JSON, CH datasource, no traceqlSearch/tempo binding)
- Commit `1703f5db` (datasource) — present in git log
- Commit `05df4ffe` (dashboard) — present in git log
- Live: CH datasource health OK; otel_traces=1192, otel_logs(10m)=4970; slow-trace query returns the trace through Grafana; Tempo/Loki containers + datasources still live (gate intact)
