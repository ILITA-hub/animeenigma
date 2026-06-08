# Phase 6: Consolidation → Topology A - Context

**Gathered:** 2026-06-08
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss), enriched with known codebase landmines

<domain>
## Phase Boundary

ClickHouse becomes the single event/trace/log plane — Tempo and Loki are retired and
their views repointed — while Prometheus + Grafana stay as the metrics + alerting +
rendering layer.

Success criteria (what must be TRUE):
1. The OTel Collector exports traces to ClickHouse and **Tempo is retired** — its datasource
   and container are removed and the backend-tracing dashboard is repointed to ClickHouse and
   still renders. (AR-CONS-01)
2. Logs are shipped to ClickHouse and **Loki is retired** — its datasource and container are
   removed and the log views are repointed to ClickHouse and still render. (AR-CONS-02)
3. Prometheus + Grafana remain unchanged as the metrics + alerting + rendering layer — verified
   by existing alerts still firing and existing metric dashboards still rendering. (AR-CONS-03)

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
Implementation choices are at Claude's discretion — discuss phase was skipped per user setting.
Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Hard constraints (non-negotiable)
- **This server IS production** — `make redeploy` deploys live. Treat every step as a live change.
- Consolidation must be **gated/reversible until the final cutover** (per the phase CDI note). Stand
  up the new ClickHouse trace/log path and prove it renders BEFORE removing Tempo/Loki containers
  and datasources. Do not delete the old stores until the repointed views are verified.

</decisions>

<code_context>
## Existing Code Insights & Known Landmines

- **CRITICAL — Tempo hosts Phase 3 span-metrics.** Retiring Tempo naively will break Phase 3's
  span-metrics → Prometheus pipeline (the span-metrics connector lives in the OTel Collector and
  historically exported via Tempo's metrics-generator path). Before removing Tempo, confirm where
  span-metrics are generated/exported and ensure that pipeline survives (route span-metrics
  generation in the OTel Collector directly to Prometheus remote-write, independent of Tempo).
  AR-CONS-03 (Prometheus/Grafana unchanged, alerts still firing) explicitly depends on this.
- Prometheus `command:`/env/compose changes need a `docker compose up -d --no-deps prometheus`
  **recreate** — `make restart-prometheus` keeps old args. Mounted-file config (e.g. tempo.yaml,
  loki config, datasource provisioning) changes only need a restart.
- Prometheus remote-write receiver requires `--web.enable-remote-write-receiver`; verify it is
  enabled if span-metrics or anything remote-writes to Prometheus.
- Admin/observability URLs in production use path routing `animeenigma.ru/admin/<tool>/...`
  (e.g. Prometheus under `/prometheus` route-prefix), NOT the `admin.animeenigma.ru/<tool>`
  subdomain form in CLAUDE.md. The Prometheus API is under `/prometheus/api/v1/...`.
- The ClickHouse plane + OTel Collector were established in Phases 1–5; the `EventStore` interface
  is the swap seam. Phase 5 dashboards are built on the populated register and must keep rendering.
- Grafana keeps a single instance across Prometheus/Loki/Tempo today; datasource provisioning is
  file-based (provisioning dir) — removing a datasource = remove its provisioning yaml + restart.

</code_context>

<specifics>
## Specific Ideas

- Use the OTel Collector's ClickHouse exporter for traces (and logs), keeping the collector as the
  single ingestion choke point already used by the register.
- Repoint the backend-tracing dashboard and log views to a ClickHouse Grafana datasource rather
  than rewriting them — minimize dashboard churn.
- Sequence: (1) add ClickHouse trace/log export alongside existing Tempo/Loki, (2) add ClickHouse
  Grafana datasource, (3) repoint dashboards + verify they render against ClickHouse, (4) prove
  span-metrics → Prometheus still flows, (5) THEN retire Tempo + Loki (containers + datasources),
  (6) final verification of alerts + metric dashboards.

</specifics>

<deferred>
## Deferred Ideas

- AR-V2-01: `AggregatingMergeTree` pre-aggregated rollups (1С-style accumulation registers).
- AR-V2-02: Pyroscope continuous profiling integration.
Both are explicitly v2/Deferred — out of scope for this phase.

</deferred>
