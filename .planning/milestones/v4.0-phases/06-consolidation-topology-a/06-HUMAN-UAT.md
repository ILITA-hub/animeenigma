---
status: partial
phase: 06-consolidation-topology-a
source: [06-VERIFICATION.md]
started: 2026-06-08T02:10:58Z
updated: 2026-06-08T02:10:58Z
---

## Current Test

[awaiting human testing — user deferred at autonomous human-needed gate 2026-06-08]

## Tests

### 1. Grafana backend-tracing dashboard visual render
expected: Opening the backend-tracing dashboard (animeenigma.ru/admin/grafana/) shows the "Recent slow traces (>1s)" panel rendering rows from ClickHouse (datasource type grafana-clickhouse-datasource, queryType traces, no traceqlSearch). Underlying data confirmed: analytics.otel_traces actively growing, datasource /health OK, panel query resolves via Grafana /api/ds/query.
result: [pending]

### 2. Logs→traces click-through correlation
expected: In Grafana Explore (ClickHouse datasource, logs mode), clicking a log row with a non-empty TraceId navigates to the matching trace. Underlying data confirmed: analytics.otel_logs has 188+ recent rows with non-empty TraceId; INNER JOIN otel_logs↔otel_traces on TraceId resolves to real traces (filelog trace_parser fix verified live).
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps
