---
phase: 06-consolidation-topology-a
reviewed: 2026-06-08T00:00:00Z
depth: deep
files_reviewed: 4
files_reviewed_list:
  - infra/otel/collector-config.yaml
  - docker/docker-compose.yml
  - docker/grafana/provisioning/datasources/datasources.yml
  - infra/grafana/dashboards/backend-tracing.json
findings:
  critical: 0
  warning: 3
  info: 2
  total: 5
status: resolved
resolution:
  resolved_at: 2026-06-08
  warnings_fixed: [WR-01, WR-02, WR-03]
  info_skipped: [IN-01, IN-02]
  notes: >
    All 3 Warnings applied + verified on the live production stack.
    WR-01 (orphaned loki_data/tempo_data volume declarations) removed;
    compose config parses clean, no warnings, underlying volumes already
    reclaimed in 06-03. WR-02 (memory_limiter+batch on metrics/spanmetrics)
    and WR-03 (order-independent trace_id/span_id regex) applied to
    collector-config.yaml, validated via `otelcol validate`, recreated via
    --force-recreate. Post-recreate live re-verification: collector Up,
    otel_traces/otel_logs still ingesting, calls_total{source=otelcol}
    span-metrics present in Prometheus (46 series), otel_logs.TraceId
    correlation > 0 (34 in last 5m). IN-01 (cosmetic dashboard description)
    and IN-02 (renaming live span-metric names — would risk breaking working
    dashboards) deliberately SKIPPED per scope decision.
---

# Phase 06: Code Review Report

**Reviewed:** 2026-06-08
**Depth:** deep
**Files Reviewed:** 4
**Status:** issues_found (3 Warnings, 2 Info; 0 Critical)

## Summary

Phase 6 executes a live-production observability consolidation: Tempo, Loki, and Promtail are retired and their functions absorbed by ClickHouse (traces + logs via the OTel Collector) and Prometheus (span-metrics via collector connectors). The implementation was done in three gated sub-plans, each verified on the live stack before the next destructive step.

**Security posture is sound.** ClickHouse credentials are injected via `${CLICKHOUSE_PASSWORD:-changeme}` compose env — consistent with the pre-existing pattern for every other CH-connecting service, documented in `.env.example`. No plaintext secret is committed. No new host port is opened. The filelog mount uses the host log directory read-only (not docker.sock). The `user:"0"` escalation on the collector is the minimum needed to traverse root-owned Docker log directories, mirrors the existing Grafana service precedent, and is constrained to a read-only mount.

**Correctness is good overall.** No dangling `depends_on` references, no Tempo/Loki/TraceQL bindings left in the active dashboard, the `deleteDatasources` prune block correctly purges provisioned read-only datasources. The three issues below are genuine bugs or latent reliability risks — none are critical because all were either verified live or their failure mode is graceful degradation rather than an outage.

---

## Warnings

### WR-01: `loki_data` and `tempo_data` volumes still declared — compose warns on every `up` and manual cleanup is blocked

**File:** `docker/docker-compose.yml:949,957`
**Issue:** The named volumes `loki_data:` and `tempo_data:` remain in the `volumes:` section even though no service mounts them. Docker Compose does not error on orphaned volume declarations, but every `docker compose up` will emit a warning about unused volumes, which trains operators to ignore warnings. More importantly, `docker volume rm animeenigma_loki_data animeenigma_tempo_data` (the recommended post-cutover cleanup in the 06-03 SUMMARY) cannot succeed while the volumes are declared in the compose file and compose considers them "managed" — the rm will succeed on the OS but the next `up` will silently recreate them as empty volumes if the declarations remain.
**Fix:** Remove the two orphaned lines from the `volumes:` block:
```yaml
# Delete these two lines:
  loki_data:
  tempo_data:
```

---

### WR-02: `metrics/spanmetrics` pipeline has no `memory_limiter` processor — OOM backpressure gap

**File:** `infra/otel/collector-config.yaml:149-151`
**Issue:** The `metrics/spanmetrics` pipeline receives output from the `spanmetrics` and `servicegraph` connectors and pushes it via `prometheusremotewrite`. Unlike the `traces` and `logs` pipelines, it has no processors at all — specifically no `memory_limiter`. The OTel Collector's memory limiter works by signalling backpressure to the pipelines connected to it; a pipeline without it is invisible to the limiter's budget accounting. Under a memory spike (e.g., a burst of spans when the collector is near the 75% heap limit), the metrics pipeline will not throttle — the connectors will keep materialising metric points and the remote-write exporter will keep batching them, contributing to the very OOM condition the limiter is trying to prevent. Current span-metrics cardinality is small (8–11 series), so the blast radius is low, but the gap widens if service count grows.
**Fix:**
```yaml
    metrics/spanmetrics:
      receivers: [spanmetrics, servicegraph]
      processors: [memory_limiter, batch]   # add memory_limiter + batch
      exporters: [prometheusremotewrite]
```
The `memory_limiter` processor is already defined in `processors:` — no new stanza needed, just reference it.

---

### WR-03: `regex_parser` captures `span_id` only when both `trace_id` AND `span_id` are adjacent in the log line — a `WithContext` call after additional `With()` fields breaks correlation silently

**File:** `infra/otel/collector-config.yaml:55`
**Issue:** The regex `"trace_id":\s*"(?P<trace_id>[0-9a-f]{32})",\s*"span_id":\s*"(?P<span_id>[0-9a-f]{16})"` requires `trace_id` and `span_id` to appear **adjacent** in the JSON field list — only `\s*` (zero or more spaces) can separate them. The `libs/logger`'s `WithContext()` always adds both fields together in order, so for a plain `WithContext()` call this is correct. However, if any service calls `log.WithContext(ctx).With("extra_field", val)` or — more commonly — if handler middleware injects structured fields via `With()` *before* `WithContext()` is called, the resulting log line may have other fields interspersed between `trace_id` and `span_id`, breaking the regex match. The failure mode is silent (no error logged, `TraceId` stays empty for that line), so correlation simply doesn't work for those log lines without any warning to operators.

Current `libs/logger.WithContext` appends `trace_id` then `span_id` as the last two fields via `l.base.Sugar().With(...)`, which means they are always adjacent at the end of the `With()` chain — making this a *low-probability* failure today. But it is a latent reliability bug.
**Fix:** Use two separate single-capture regexes or a non-greedy lookahead that does not require adjacency:
```yaml
        regex: '.*"trace_id":\s*"(?P<trace_id>[0-9a-f]{32})".*"span_id":\s*"(?P<span_id>[0-9a-f]{16})"'
```
Or alternatively extract `trace_id` and `span_id` in separate `regex_parser` operators (the second guarded by `attributes.trace_id != nil`) to be completely order-independent.

---

## Info

### IN-01: `backend-tracing.json` description still references the Tempo safety-net disclaimer from 06-02

**File:** `infra/grafana/dashboards/backend-tracing.json:7`
**Issue:** The dashboard description contains: *"Tempo stays live as the safety net until 06-03"*. Tempo was removed in 06-03 and the dashboard description was not updated. This is stale documentation that will confuse anyone reading it — the dashboard has been the sole trace surface for ClickHouse since 06-03 completed.
**Fix:** Update the description to remove the interim-gate language, e.g.:
```
"description": "ClickHouse trace explorer (distributed tracing, OTel → OTel Collector → ClickHouse analytics.otel_traces). Repointed to ClickHouse from Tempo in Phase 6 (2026-06-08). ..."
```

---

### IN-02: Span-metrics connector emits `calls_total` / `duration_milliseconds_*` — not the Tempo-compatible `traces_spanmetrics_*` names — with no namespace configured; future dashboards may be built against a name that differs from all documentation

**File:** `infra/otel/collector-config.yaml:95-104`
**Issue:** The `spanmetrics` connector's default metric namespace produces `calls_total` and `duration_milliseconds_bucket/count/sum`. Tempo's `metrics_generator` produced `traces_spanmetrics_calls_total`. The 06-01 SUMMARY flags this carry-forward and the 06-03 SUMMARY confirms it (live series `calls_total{source=otelcol}` is the active writer; `traces_spanmetrics_calls_total` is decaying). No existing dashboard queries either name (confirmed by grep), so there is no current breakage. The risk is that the next person building a RED dashboard will encounter two naming conventions in the docs/codebase — Tempo docs say `traces_spanmetrics_calls_total`, collector connector defaults say `calls_total` — and pick the wrong one silently.
**Fix:** Pin the namespace explicitly so that newly emitted metrics have the canonical collector name documented:
```yaml
  spanmetrics:
    namespace: traces.spanmetrics     # emits traces_spanmetrics_calls_total (Tempo-compatible)
    dimensions:
      - name: operation
```
OR leave the default and add a comment to `collector-config.yaml` noting the metric names (`calls_total`, `duration_milliseconds_*`) so the next dashboard author knows what to query. Either approach is acceptable; the namespace option restores Tempo-compatible naming at zero cost.

---

_Reviewed: 2026-06-08_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
