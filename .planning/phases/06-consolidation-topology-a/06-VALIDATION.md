---
phase: 6
slug: consolidation-topology-a
status: draft
nyquist_compliant: false
wave_0_complete: false
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
| 06-XX-XX | TBD | 1 | AR-CONS-01 | — | traces exported to ClickHouse `otel_traces` | operational | live ClickHouse query returns rows for recent trace window | ❌ W0 | ⬜ pending |
| 06-XX-XX | TBD | 1 | AR-CONS-02 | — | logs shipped to ClickHouse `otel_logs` (Promtail retired) | operational | live ClickHouse query returns recent log rows | ❌ W0 | ⬜ pending |
| 06-XX-XX | TBD | 2 | AR-CONS-01 | — | backend-tracing dashboard repointed to CH, renders | operational | Grafana datasource `/health` 200 + dashboard panel returns data | ❌ W0 | ⬜ pending |
| 06-XX-XX | TBD | 2 | AR-CONS-02 | — | log views repointed to CH, render | operational | Grafana CH log query returns rows | ❌ W0 | ⬜ pending |
| 06-XX-XX | TBD | 3 | AR-CONS-03 | — | Prometheus alerts still fire, metric dashboards render | operational | `/prometheus/api/v1/rules` shows active rules; metric panels return data | ❌ W0 | ⬜ pending |
| 06-XX-XX | TBD | 3 | AR-CONS-01/02 | — | Tempo + Loki containers + datasources removed | operational | `docker compose ps` shows no tempo/loki; Grafana has no tempo/loki datasource | ❌ W0 | ⬜ pending |

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

**Approval:** pending
