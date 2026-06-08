---
phase: 6
slug: consolidation-topology-a
status: planned
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-08
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> NOTE: This is an infrastructure/observability config phase (OTel Collector +
> docker-compose + Grafana provisioning). "Tests" here are operational verification
> commands (config validation, health probes, live query checks against the
> production stack) rather than unit tests. There is no application code under test.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Operational verification (no unit-test framework — infra/config phase) |
| **Config file** | `docker/docker-compose.yml`, OTel Collector config, Grafana provisioning |
| **Quick run command** | `docker run --rm -v <otel-config>:/c otel/opentelemetry-collector-contrib:0.103.1 validate --config /c` |
| **Full suite command** | `make health` + Grafana datasource health + live ClickHouse trace/log query + Prometheus alert/metric render checks |
| **Estimated runtime** | ~120 seconds |

---

## Sampling Rate

- **After every task commit:** `otelcol validate` (config tasks) or `docker compose config` (compose tasks)
- **After every plan wave:** `make health` + affected Grafana datasource `/health` probe
- **Before final verification:** Full operational suite must be green (traces+logs render from ClickHouse; Prometheus alerts still firing; metric dashboards still render); Tempo/Loki/Promtail removed.
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 06-01-T1 | 06-01 | 1 | AR-CONS-01/02/03 | T-06-01 | collector config adds CH traces+logs exporters, filelog, spanmetrics/servicegraph connectors (Tempo kept) | operational | `grep` confirms exporters/receiver/connectors present + otlp/tempo retained | ✅ | ⬜ pending |
| 06-01-T2 | 06-01 | 1 | AR-CONS-01/02 | T-06-02 | otel-collector gets CH creds env + docker-log mount + depends_on | operational | `docker compose config` exits 0 | ✅ | ⬜ pending |
| 06-01-T3 | 06-01 | 1 | AR-CONS-01/02/03 | T-06-01 | traces+logs land in CH; span-metrics reach Prometheus; config validates | operational | `otelcol validate` 0; `SELECT count() FROM otel_traces/otel_logs`>0; `traces_spanmetrics_calls_total`>0 | ✅ | ⬜ pending |
| 06-02-T1 | 06-02 | 2 | AR-CONS-01/02 | T-06-06 | CH trace+log datasource (OTel mode) provisioned, fresh/extended uid | operational | datasource health OK; `grep otel_traces/otel_logs` present | ✅ | ⬜ pending |
| 06-02-T2 | 06-02 | 2 | AR-CONS-01 | T-06-06 | backend-tracing.json repointed to CH, no TraceQL | operational | `grep` CH datasource + no traceqlSearch + valid JSON | ✅ | ⬜ pending |
| 06-02-T3 | 06-02 | 2 | AR-CONS-01/02 | — | repointed trace+log views render from CH (visual DS-NF-06) | operational+manual | live CH query has rows + manual browser render | ✅ | ⬜ pending |
| 06-03-T1 | 06-03 | 3 | AR-CONS-01/02 | T-06-08 | human render gate BEFORE destructive removal | manual | human approves CH render + span-metrics present | ✅ | ⬜ pending |
| 06-03-T2 | 06-03 | 3 | AR-CONS-01/02 | T-06-09 | drop otlp/tempo; delete tempo/loki/promtail services + depends_on | operational | no otlp/tempo; no loki/promtail/tempo services; `docker compose config` 0 | ✅ | ⬜ pending |
| 06-03-T3 | 06-03 | 3 | AR-CONS-01/02 | T-06-06 | remove Tempo+Loki datasources + orphaned configs | operational | no aenigma-tempo/aenigma-loki; tempo/loki/promtail config files deleted | ✅ | ⬜ pending |
| 06-03-T4 | 06-03 | 3 | AR-CONS-01/02/03 | T-06-12 | cutover applied; final AR-CONS-01/02/03 verification | operational+manual | no tempo/loki/promtail containers; no alert rule in `error`; metric dashboards render | ✅ | ⬜ pending |

*Final per-task map is filled by the planner. Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Operational verification only — no test files or framework to install. The OTel
  contrib collector image, ClickHouse, the `grafana-clickhouse-datasource` plugin,
  and the `aenigma-clickhouse` datasource already exist from Phase 1.
- Pre-cutover guard: `otelcol validate` the new collector config BEFORE recreate.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Backend-tracing dashboard visually renders trace search/spans from ClickHouse | AR-CONS-01 | Visual render against repointed datasource; TraceQL→SQL rewrite needs eyeball | Open `animeenigma.ru/admin/grafana/` backend-tracing dashboard, run a trace search, confirm spans display |
| Log views visually render from ClickHouse | AR-CONS-02 | Visual render of repointed log panels | Open log dashboard in Grafana, confirm recent logs display with trace_id linking |
| Existing alerts still firing post-cutover | AR-CONS-03 | Confirms metrics plane untouched | `/prometheus/admin/...` rules page shows rules in firing/inactive (not error) state |

*Some operational checks can be scripted via curl; visual render checks are manual per DS-NF-06 standing rule.*

---

## Validation Sign-Off

- [ ] All tasks have an operational verify command or are flagged manual-only
- [ ] Sampling continuity: no 3 consecutive tasks without a verify command
- [ ] Gated cutover: new CH trace/log path proven to render BEFORE Tempo/Loki removed
- [ ] span-metrics → Prometheus path proven to survive Tempo removal (AR-CONS-03)
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter once planner fills the map

**Approval:** planner-filled 2026-06-08
