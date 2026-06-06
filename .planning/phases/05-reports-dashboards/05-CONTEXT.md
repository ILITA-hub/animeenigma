# Phase 5: Reports & Dashboards - Context

**Gathered:** 2026-06-06
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)

<domain>
## Phase Boundary

An admin can answer "what is the platform doing now/today, by operation and by external dependency, with anomalies surfaced" through pivotable Grafana reports built on the populated Activity Register (ClickHouse `events`).

Depends on Phases 2, 3, 4 (egress + DB/cache + FE effect data populated).
Requirements: AR-REPORT-01 (wide-event pivot dashboard with template vars to group/filter by ANY dimension), AR-REPORT-02 ("from → choke-point → effects" report: origin → operation → per-target requests+bytes), AR-REPORT-03 (volume anomaly flagging vs baseline), AR-REPORT-04 (awareness overview: current top operations + top external deps + active anomalies).
</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices at Claude's discretion — discuss skipped. Use ROADMAP success criteria + codebase conventions.

Key facts:
- This is GRAFANA dashboard authoring (JSON provisioning under docker/grafana/provisioning/dashboards/ + datasources), NOT Vue/web frontend. The frontend design-system/UI-SPEC machinery does NOT apply.
- Data source: ClickHouse `analytics.events` (columns: timestamp, trace_id, source, origin, operation, effect_kind, target, target_kind, anime_id, duration_ms, row_count, and byte columns). A ClickHouse Grafana datasource must exist or be provisioned (grafana-clickhouse-datasource plugin) — research must confirm whether it's already wired (Phase 1/2 may have added it) or needs provisioning.
- Anomaly flagging: prefer ClickHouse-query-driven baseline comparison (e.g. count vs trailing avg/stddev) rendered as a panel + optional Grafana alert rule, over heavy ML. Keep it query-based and explainable.
- Byte aggregations MUST filter source='be' (authoritative) — never sum fe_rum approximate rows (AR-FE-03 discipline carries into the reports).
- Existing Grafana already provisioned (Prometheus + Tempo datasources from Phase 3). Reuse the provisioning pattern.
</decisions>

<code_context>
## Existing Code Insights

Research will map: existing docker/grafana/provisioning/{dashboards,datasources} layout, whether a ClickHouse datasource is provisioned, the events schema columns available for pivoting, and the Grafana version (template-var + transformations capability).
</code_context>

<specifics>
## Specific Ideas

Pivot dashboard uses Grafana template variables bound to ClickHouse `SELECT DISTINCT <dim>` queries so switching a var regroups live. The "from→choke-point→effects" report is origin→operation→target with requests+bytes measures. Awareness overview = single dashboard: top operations now/today + top external dependencies + active anomalies.
</specifics>

<deferred>
## Deferred Ideas
None — discuss skipped. (Pyroscope cost-by-function profiling AR-V2-02 is a separately-deferred backlog item, out of scope here.)
</deferred>
