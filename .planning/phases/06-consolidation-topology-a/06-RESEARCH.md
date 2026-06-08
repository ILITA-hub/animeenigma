# Phase 6: Consolidation → Topology A - Research

**Researched:** 2026-06-08
**Domain:** Observability consolidation — OTel Collector ClickHouse exporter (traces + logs), retire Tempo + Loki, preserve Prometheus + Grafana alerting/metrics
**Confidence:** HIGH (all infra files read directly from repo; exporter/datasource capabilities verified against official sources)

## Summary

This phase collapses three observability stores (Tempo for traces, Loki for logs, ClickHouse for events) into one (ClickHouse), while leaving Prometheus + Grafana as the metrics/alerting/rendering layer. The ClickHouse plane, the `grafana-clickhouse-datasource` plugin, and the `aenigma-clickhouse` Grafana datasource **already exist** (built in Phase 1). What's missing is: (1) a ClickHouse **traces** exporter + pipeline in the OTel Collector, (2) a ClickHouse **logs** path (the collector has no log pipeline today — logs flow Promtail→Loki, entirely outside OTel), (3) repointed Grafana datasources/dashboards, and (4) **moving span-metrics generation off Tempo** before Tempo is deleted.

The single biggest landmine — "Tempo hosts Phase 3 span-metrics" — is **less dangerous than feared, but must still be handled**. I verified that `traces_spanmetrics_*` and `traces_service_graph_*` metrics are referenced in **exactly one place**: the Tempo datasource's `tracesToMetrics`/`serviceMap` convenience config in `datasources.yml`. **No alert rule and no dashboard panel queries them.** AR-CONS-03's "alerts still firing" depends on application `/metrics` scrapes (`up`, `scheduler_*`, `provider_health_up`, `playability_canary_*`, `http_request_duration_seconds`, …) — none of which come from Tempo. So retiring Tempo will NOT break any firing alert. However, to honor the ROADMAP/CONTEXT intent and keep the per-operation RED metrics + service graph alive, the OTel Collector's `spanmetrics` + `servicegraph` connectors should regenerate them and remote-write to Prometheus (Prometheus already runs `--web.enable-remote-write-receiver`).

**Primary recommendation:** Execute a 6-step gated cutover entirely via mounted-config edits (no app-code changes): (1) add ClickHouse traces+logs export to the OTel Collector ALONGSIDE Tempo, plus a `filelog` receiver to replace Promtail's log scrape, and add the `spanmetrics`+`servicegraph` connectors that remote-write to Prometheus; (2) add/confirm ClickHouse trace+log datasource config; (3) repoint `backend-tracing.json` + the Loki-derived-field correlation to ClickHouse and verify they render; (4) prove span-metrics still reach Prometheus from the collector; (5) THEN remove Tempo + Loki + Promtail containers, datasources, and the `depends_on` edges; (6) final verification of alerts + every metric dashboard against the production stack.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Trace ingestion (OTLP from services) | OTel Collector | — | Already the single OTLP choke point (`:4317/:4318`); services export here today |
| Trace storage | ClickHouse (`otel_traces`) | — (Tempo retired) | OTel ClickHouse exporter creates/owns this table |
| Span-metrics + service-graph generation | OTel Collector (`spanmetrics`/`servicegraph` connectors) | Prometheus (sink via remote-write) | Moves OFF Tempo's `metrics_generator`; Prometheus receiver already enabled |
| Log collection | OTel Collector (`filelog` receiver) | — (Promtail retired) | Replaces Promtail's docker.sock scrape; keeps one ingestion daemon |
| Log storage | ClickHouse (`otel_logs`) | — (Loki retired) | OTel ClickHouse exporter creates/owns this table |
| Metrics scrape + storage | Prometheus | — | Unchanged — AR-CONS-03 explicitly keeps it |
| Alerting | Prometheus/Grafana unified alerting | — | Unchanged — rules query app `/metrics`, not Tempo |
| Visualization / datasources | Grafana | ClickHouse, Prometheus | Single Grafana; datasource provisioning is file-based |

## Standard Stack

### Core (all ALREADY present in the repo — this phase reconfigures, it does not install new services)
| Component | Version (pinned in repo) | Purpose | Status |
|-----------|--------------------------|---------|--------|
| `otel/opentelemetry-collector-contrib` | `0.103.1` | OTLP ingest, tail-sampling, ClickHouse export, filelog, connectors | Present (`docker-compose.yml:360`) — contrib distro **includes** `clickhouseexporter`, `filelogreceiver`, `spanmetrics`/`servicegraph` connectors [CITED: github.com/open-telemetry/opentelemetry-collector-contrib] |
| `clickhouse/clickhouse-server` | `26.3.12.3` | Unified event/trace/log store | Present (`docker-compose.yml:313`) |
| `grafana/grafana` | `10.3.3` | Dashboards + unified alerting | Present (`docker-compose.yml:374`) |
| `prom/prometheus` | `v2.50.1` | Metrics + remote-write receiver | Present, `--web.enable-remote-write-receiver` already on (`docker-compose.yml:227`) |
| `grafana-clickhouse-datasource` (plugin) | installed via `GF_INSTALL_PLUGINS` | Native OTel trace + log + SQL querying in Grafana | Present (`docker-compose.yml:425`) — Grafana 10.3.3 ships plugin v4.x with the OTel trace builder + trace-ID search [CITED: grafana.com/grafana/plugins/grafana-clickhouse-datasource] |

### Being retired this phase
| Component | Version | Replaced by |
|-----------|---------|-------------|
| `grafana/tempo` | `2.4.1` | ClickHouse `otel_traces` + OTel `spanmetrics`/`servicegraph` connectors |
| `grafana/loki` | `2.9.4` | ClickHouse `otel_logs` |
| `grafana/promtail` | `2.9.4` | OTel Collector `filelog` receiver |
| `tempo-init` (MinIO bucket bootstrap) | minio/mc | removed with Tempo (MinIO `tempo` bucket becomes orphaned — leave or clean up) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| OTel `filelog` receiver for logs | Keep Promtail but point it at ClickHouse | Promtail has **no ClickHouse client** — it only speaks Loki push. Would need a Loki-compatible shim. `filelog`→clickhouseexporter keeps everything in the one collector already present. **Use filelog.** |
| OTel `spanmetrics` connector | Keep Tempo's `metrics_generator` alive headless | Defeats the phase goal (Tempo not retired). Connector is the documented Phase-6 path (tempo.yaml:44-45 literally says so). |
| Self-managed ClickHouse DDL (`create_schema: false`) | Let exporter auto-create tables (`create_schema: true`) | Auto-create is simplest and matches Phase-1's schema-on-boot convention. Default TTL is 0 (forever) — **must set `ttl:` explicitly** to match Tempo's 14d / Loki's 7d. Use exporter `ttl` setting; if finer control needed (partitioning/codecs) switch to `create_schema:false` + hand-written DDL in a v2 follow-up. **Recommend `create_schema:true` + explicit `ttl`.** |

**Installation:** None. No new packages, no new containers. This is a config-and-provisioning phase. (The `grafana-clickhouse-datasource` plugin install already happens at Grafana boot.)

## Package Legitimacy Audit

> No external packages are installed in this phase — all components are already pinned in the repo and were vetted in Phases 1–5. slopcheck N/A.

| Package | Registry | Disposition |
|---------|----------|-------------|
| (none) | — | No installs — config/provisioning-only phase |

## Architecture Patterns

### System Architecture Diagram

**BEFORE (current topology):**
```
services ──OTLP──► otel-collector ──otlp/tempo──► Tempo ──metrics_generator──remote_write──► Prometheus
                                                    │
                                                    └─(S3) MinIO "tempo" bucket
docker stdout ──► Promtail ──push──► Loki ◄──Explore/derivedField(trace_id→Tempo)── Grafana
services /metrics ──scrape──► Prometheus ◄──query── Grafana (dashboards + alerts)
analytics svc ──InsertBatch──► ClickHouse "events" ◄──query── Grafana (product-analytics, activity-register-*)
```

**AFTER (Topology A):**
```
services ──OTLP──► otel-collector ─┬─ clickhouse exporter (traces) ──► ClickHouse "otel_traces"
                                   ├─ clickhouse exporter (logs)   ──► ClickHouse "otel_logs"
                                   ├─ spanmetrics  connector ──remote_write──► Prometheus
                                   └─ servicegraph connector ──remote_write──► Prometheus
docker /var/lib/docker/containers/*.log ──filelog receiver──► (same collector logs pipeline)
services /metrics ──scrape──► Prometheus ◄──query── Grafana (dashboards + alerts UNCHANGED)
analytics svc ──InsertBatch──► ClickHouse "events"   (unchanged)
Grafana ◄──query── ClickHouse (events, otel_traces, otel_logs)   [Tempo + Loki gone]
```

### Component Responsibilities (files to touch)

| File | Change |
|------|--------|
| `infra/otel/collector-config.yaml` | Add `clickhouse` exporter; add `traces` pipeline export to ClickHouse (keep Tempo during gate, drop at cutover); add `logs` pipeline (filelog→batch→clickhouse); add `spanmetrics`+`servicegraph` connectors with a `prometheusremotewrite` exporter to `http://prometheus:9090/prometheus/api/v1/write`; add `filelog` receiver |
| `docker/docker-compose.yml` | otel-collector: mount `/var/lib/docker/containers:ro` + add `clickhouse`/`prometheus` to `depends_on`, drop `tempo` dep at cutover. **Delete** `tempo`, `tempo-init`, `loki`, `promtail` services at cutover. Grafana: **remove** `depends_on: loki` (line 452-453) — otherwise compose won't start once Loki is gone. |
| `docker/grafana/provisioning/datasources/datasources.yml` | Add ClickHouse trace + log datasource config (OTel table mappings); **remove** `Loki` and `Tempo` datasource blocks at cutover; remove the Loki `derivedFields → aenigma-tempo` correlation |
| `infra/grafana/dashboards/backend-tracing.json` | Repoint `traces` panel datasource from `{type:tempo, uid:aenigma-tempo}` to the ClickHouse trace datasource; change `queryType` from `traceqlSearch` to the CH plugin's trace query |
| `infra/tempo/tempo.yaml` | Delete at cutover (no longer mounted) |
| `docker/loki/loki-config.yml`, `docker/promtail/config.yml` | Delete at cutover |
| `docker/prometheus/prometheus.yml` | No change required (remote-write receiver already on; span-metrics arrive via the push endpoint, not a scrape) |

### Pattern 1: OTel Collector ClickHouse exporter + connectors (config-only)
**What:** One exporter handles both traces and logs; connectors derive metrics from the trace stream and push to Prometheus.
**When to use:** This phase — it keeps the single collector as the ingestion choke point.
**Example:**
```yaml
# Source: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/clickhouseexporter [CITED]
# [ASSUMED] exact wiring — verify against the 0.103.1 README at plan time; component is present in contrib distro.
receivers:
  otlp: { protocols: { grpc: {endpoint: 0.0.0.0:4317}, http: {endpoint: 0.0.0.0:4318} } }
  filelog:
    include: [ /var/lib/docker/containers/*/*-json.log ]
    # docker json-file driver wraps each line as {"log":..,"stream":..,"time":..}
    operators:
      - type: json_parser
      - type: move
        from: attributes.log
        to: body

connectors:
  spanmetrics:
    dimensions:
      - name: service.name
      - name: span.name
      - name: operation        # preserves the Phase-3 per-op RED dimension (tempo.yaml:59)
  servicegraph:
    dimensions: [service.name]

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000?dial_timeout=10s
    database: analytics
    username: ${env:CLICKHOUSE_USER}
    password: ${env:CLICKHOUSE_PASSWORD}
    create_schema: true
    traces_table_name: otel_traces
    logs_table_name: otel_logs
    ttl: 336h                  # match Tempo's 14d for traces; logs table gets its own (see note)
  prometheusremotewrite:
    endpoint: http://prometheus:9090/prometheus/api/v1/write
    # NOTE the /prometheus route-prefix — Prometheus runs --web.route-prefix=/prometheus

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, tail_sampling, batch]
      exporters: [clickhouse, spanmetrics, servicegraph]   # + otlp/tempo DURING THE GATE only
    logs:
      receivers: [filelog]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
    metrics/spanmetrics:
      receivers: [spanmetrics, servicegraph]
      exporters: [prometheusremotewrite]
```
**Gotchas:**
- The OTel ClickHouse exporter uses a SINGLE `ttl` for all signals; logs needing 7d vs traces 14d means either accepting one TTL, two exporter instances (`clickhouse/traces`, `clickhouse/logs` with different `ttl`), or `create_schema:false` + hand DDL. **Recommend two named exporter instances** for distinct TTLs.
- `${env:VAR}` is the collector's env-var syntax (collector reads from its own container env — those CH creds must be added to the `otel-collector` service env in compose; they are NOT there today).
- The Grafana CH datasource expects the exporter's default OTel table column layout — keep `create_schema:true` (or replicate the exporter DDL exactly) so the "Use OTel" trace builder pre-fills correctly.

### Pattern 2: Grafana ClickHouse trace + log datasource (OTel mode)
**What:** Configure the existing `grafana-clickhouse-datasource` plugin with the OTel trace/log table mappings so the trace-ID search, trace view, and logs panel work without hand SQL.
**Example:**
```yaml
# Source: grafana.com/docs/plugins/grafana-clickhouse-datasource [CITED]
- name: ClickHouse-Traces
  uid: aenigma-clickhouse-traces       # NEW uid — never reuse aenigma-clickhouse/postgres/tempo (orphan-alert hazard)
  type: grafana-clickhouse-datasource
  jsonData:
    host: clickhouse
    port: 9000
    protocol: native
    defaultDatabase: analytics
    traces:
      defaultDatabase: analytics
      defaultTable: otel_traces
      otelEnabled: true
      otelVersion: latest
    logs:
      defaultDatabase: analytics
      defaultTable: otel_logs
      otelEnabled: true
  secureJsonData: { password: ${CLICKHOUSE_PASSWORD} }
```
(The existing `aenigma-clickhouse` datasource can also be extended in-place with `traces`/`logs` blocks rather than adding a new uid — that avoids a new uid entirely. Either works; extending in place is lower-churn.)

### Anti-Patterns to Avoid
- **Deleting Tempo/Loki before the new path renders.** Hard constraint (CONTEXT). Add ClickHouse export alongside, verify, THEN remove.
- **Restarting Prometheus to pick up remote-write changes.** Remote-write receiver is already enabled — no Prometheus change needed. But IF prometheus `command:` ever changes, it needs `docker compose up -d --no-deps prometheus` (recreate), NOT `make restart-prometheus` [VERIFIED: project memory + docker-compose.yml:219-229].
- **Reusing an existing datasource uid.** Reusing `PBFA97CFB590B2093`/`aenigma-postgres`/`aenigma-clickhouse` for the new trace/log source orphaned 14 alert rules historically (datasources.yml:5-9). Use a fresh uid or extend the existing CH datasource in place.
- **Leaving Grafana `depends_on: loki`.** Once Loki is removed, `docker compose up` fails on the dangling dependency. Remove it (compose:452-453) in the same change that removes Loki.
- **Dropping the `operation` span-metric dimension.** Phase 3's per-op RED metric depends on it (tempo.yaml:59). Carry it into the `spanmetrics` connector config.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Span→metric aggregation | Custom Prometheus exporter | OTel `spanmetrics` connector | Battle-tested, label-cardinality-aware, same RED semantics Tempo produced |
| Service-graph metrics | Custom edge counter | OTel `servicegraph` connector | Generates `traces_service_graph_*` identically to Tempo |
| Docker log shipping to ClickHouse | A bespoke tail-and-insert script | OTel `filelog` receiver + clickhouse exporter | Handles rotation, multiline, backpressure, batching |
| ClickHouse trace/log table DDL | Hand-rolled schema | exporter `create_schema:true` | Plugin's OTel trace builder expects the exporter's exact column layout |
| Trace-ID search UI | Custom Grafana panel | CH datasource OTel trace builder | Native trace-ID search + trace view + logs↔traces link |

**Key insight:** Every capability Tempo/Loki provided has a drop-in OTel-Collector or Grafana-plugin equivalent already shipping in the pinned images. This is a wiring exercise, not a build.

## Runtime State Inventory

> This is a retire/repoint phase touching live stateful services — inventory required.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | Tempo trace blocks in **MinIO `tempo` bucket** (S3 backend, tempo.yaml:28) + `tempo_data` volume (WAL + generator WAL). Loki chunks in `loki_data` volume (7d retention). | Both are debugging data with short TTL — safe to discard at cutover. Optionally `mc rb local/tempo` to reclaim MinIO space. Do NOT delete before the ClickHouse path is verified. |
| Live service config | Tempo `metrics_generator` is the **sole remote-writer to Prometheus** today (datasources.yml says "Tempo is the sole writer", compose:226). New traces/logs/span-metrics config lives in mounted files (in git) — not hidden in a UI/DB. | Edit mounted configs; recreate collector. No UI-only state. |
| OS-registered state | None — all services are docker-compose-managed; no Task Scheduler / systemd / pm2 entries reference Tempo/Loki. | None. |
| Secrets/env vars | OTel collector currently has **no ClickHouse creds in its env** (compose:359-371). Adding the CH exporter requires injecting `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` into the `otel-collector` service env. These already exist in `docker/.env` (used by clickhouse + grafana). | Add the two env vars to the otel-collector service (code/compose edit; no new secret). |
| Build artifacts | The orphaned MinIO `tempo` bucket + `tempo_data`/`loki_data` named volumes survive `docker compose down` (volumes persist). | `docker volume rm animeenigma_tempo_data animeenigma_loki_data` after verified cutover, if reclaiming space. Volume names: verify with `docker volume ls`. |

**Nothing found in:** OS-registered state (verified — compose-only stack).

## Common Pitfalls

### Pitfall 1: Assuming Tempo retirement breaks alerts
**What goes wrong:** Planner adds heavy compensating work fearing alert breakage.
**Why it happens:** The CONTEXT/memory landmine says "Tempo hosts span-metrics" — true, but incomplete.
**Reality (VERIFIED):** `grep` across `rules.yml`, `infra/grafana/alerts/`, and all dashboards shows `traces_spanmetrics_*` / `traces_service_graph_*` appear ONLY in `datasources.yml` (Tempo's `tracesToMetrics`/`serviceMap` convenience). No alert rule and no dashboard panel queries them. Alerts query app `/metrics` (`up`, `scheduler_job_*`, `provider_health_up`, `playability_canary_runs_total`, `parser_ad_decoy_total`, `http_request_duration_seconds`, `http_requests_total`, `proxy_active_connections`). **Retiring Tempo cannot break a firing alert.**
**How to avoid:** Still regenerate span-metrics via the OTel connector (phase goal + keeps the service-graph/RED panels usable), but treat AR-CONS-03 alert-survival as low-risk and verify it cheaply (alerts already green on app metrics).
**Warning signs:** A plan that proposes touching `rules.yml` — it shouldn't need to.

### Pitfall 2: filelog receiver double-counts or mis-parses Docker logs
**What goes wrong:** Logs land in ClickHouse mangled (whole JSON envelope as body) or duplicated.
**Why it happens:** Docker's json-file driver wraps each line; `filelog` needs a `json_parser` + `move` to extract `.log` into the body, and the include glob must match the host's actual log path/format.
**How to avoid:** Mirror Promtail's current source (`/var/lib/docker/containers`, json-file). Verify one service's logs appear once in `otel_logs` with correct body + `trace_id` attribute (logger emits `trace_id`, libs/logger/logger.go:92).
**Warning signs:** `otel_logs` body contains `{"log":...,"stream":...}` JSON instead of the message.

### Pitfall 3: TTL drift (data lives forever / disappears too soon)
**What goes wrong:** Exporter default `ttl: 0` = no expiry → ClickHouse disk grows unbounded; or a single shared TTL truncates traces at the log retention.
**Why it happens:** One `ttl` per exporter instance.
**How to avoid:** Two named exporter instances — `clickhouse/traces` (`ttl: 336h`, matches Tempo 14d) and `clickhouse/logs` (`ttl: 168h`, matches Loki 7d).
**Warning signs:** `SELECT count() FROM otel_traces` keeps climbing past 14 days.

### Pitfall 4: Prometheus remote-write path/prefix mismatch
**What goes wrong:** Span-metrics silently never arrive.
**Why it happens:** Prometheus runs with `--web.route-prefix=/prometheus`; the write endpoint is `/prometheus/api/v1/write`, not `/api/v1/write` (compose:228-229, and Tempo already uses the prefixed URL, tempo.yaml:53).
**How to avoid:** Set the collector's `prometheusremotewrite.endpoint` to `http://prometheus:9090/prometheus/api/v1/write`.
**Warning signs:** `traces_spanmetrics_calls_total` absent in Prometheus after collector recreate.

### Pitfall 5: Collector config errors are silent until recreate
**What goes wrong:** Bad YAML / unknown component crashes the collector → ALL traces stop.
**Why it happens:** Collector loads config at start; a typo in a component name fails the whole pipeline.
**How to avoid:** Validate with `otelcol-contrib validate --config=...` (or `docker run --rm -v ... otel/opentelemetry-collector-contrib:0.103.1 validate`) before recreate. Recreate via `docker compose up -d --no-deps --force-recreate otel-collector` and check logs.
**Warning signs:** otel-collector container restart-looping.

### Pitfall 6: Grafana won't start after Loki removal
**What goes wrong:** `docker compose up` errors on Grafana's `depends_on: loki`.
**Why it happens:** compose:452-453 hard-depends on Loki healthy.
**How to avoid:** Remove that `depends_on` entry in the SAME change that removes the Loki service.

## Code Examples

### Verify span-metrics survive the cutover (against the production stack)
```bash
# Source: project memory (Prometheus /prometheus route-prefix) + docker-compose.yml [VERIFIED]
# Local (server-side):
curl -s 'http://localhost:9090/prometheus/api/v1/query?query=traces_spanmetrics_calls_total' | jq '.data.result | length'
# Production admin form:
#   https://animeenigma.ru/admin/prometheus/api/v1/query?query=traces_spanmetrics_calls_total
```

### Verify traces/logs land in ClickHouse
```bash
# ClickHouse HTTP is bound 127.0.0.1:8123 (compose:326)
curl -s 'http://localhost:8123/?user=analytics&password=changeme' \
  --data 'SELECT count() FROM analytics.otel_traces'
curl -s 'http://localhost:8123/?user=analytics&password=changeme' \
  --data 'SELECT count() FROM analytics.otel_logs WHERE timestamp > now() - INTERVAL 5 MINUTE'
```

### Verify alerts still firing (AR-CONS-03)
```bash
# Unified alerting state via Grafana (admin SSO) or Prometheus rules:
curl -s 'http://localhost:9090/prometheus/api/v1/rules' | jq '.data.groups[].rules[] | {name, state}'
#   https://animeenigma.ru/admin/grafana/alerting/list  (visual)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Tempo `metrics_generator` for span-metrics | OTel Collector `spanmetrics`/`servicegraph` connectors | Connectors GA in collector ~0.85+; standard since ~2024 | Decouples RED metrics from the trace store; exactly the Phase-6 path tempo.yaml:44-45 anticipates |
| Loki for logs + Promtail | OTel `filelog` → ClickHouse | ClickHouse OTel exporter beta-stable for logs+traces | One store, SQL-queryable, joins logs↔traces↔events on `trace_id` |
| Grafana CH plugin SQL-only | CH plugin v4.x OTel trace builder + trace-ID search | plugin 4.0 (2024) | Tempo-like trace UX without SQL [CITED: clickhouse.com/blog/clickhouse-grafana-plugin-4-0] |

**Deprecated/outdated:** Tempo, Loki, Promtail — all retired this phase.

## Feature-Parity Gaps (call these out honestly to the planner/user)

| Capability Tempo/Loki had | ClickHouse replacement | Gap / Note |
|---------------------------|------------------------|------------|
| TraceQL (`{ duration > 1s }`) | CH plugin trace query builder + SQL `WHERE Duration > 1e9` | No TraceQL syntax; equivalent expressible in the builder/SQL. `backend-tracing.json`'s `traceqlSearch` query must be rewritten. **[ASSUMED]** the builder covers the "slow traces" use case — verify at plan time. |
| Tempo service-graph node graph (Grafana nodeGraph panel) | `servicegraph` connector → `traces_service_graph_*` in Prometheus; render via existing nodeGraph/Prometheus | Works, but the Tempo datasource's built-in `serviceMap` UX is gone; rebuild as a Prometheus-backed panel if the node graph is wanted. Currently NO dashboard panel uses it (only datasources.yml config) — low impact. |
| Tempo trace→logs correlation (`tracesToLogsV2`→Loki) | CH plugin logs↔traces link (both in CH, joined on `trace_id`) | Parity available natively (logs contain `trace_id`, logger.go:92). Must configure the CH datasource's trace/log linking instead of the Loki derivedField. |
| Loki LogQL + label browser | CH SQL / plugin logs builder | No LogQL; SQL filtering instead. Ad-hoc log exploration UX differs. |
| Loki long-window cheap storage | ClickHouse columnar | ClickHouse is fine/better for this volume; TTL must be set explicitly (Pitfall 3). |
| Exemplars (Tempo remote_write `send_exemplars:true`) | spanmetrics connector exemplars | **[ASSUMED]** connector emits exemplars comparably — verify if exemplar links are relied upon (no current dashboard appears to). |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The exact OTel ClickHouse exporter + filelog + connector YAML keys for 0.103.1 match the snippets shown | Pattern 1 | LOW — components confirmed present in contrib distro; key names verified against current README but 0.103.1 may differ slightly. Validate with `otelcol validate` before recreate (Pitfall 5). |
| A2 | The CH plugin's OTel trace builder fully covers the "recent slow traces >1s" panel use case | Feature-Parity Gaps | LOW — trace-ID search + trace view confirmed; duration filter is standard SQL. Verify the repointed `backend-tracing.json` renders (gated step 3). |
| A3 | spanmetrics/servicegraph connectors emit exemplars equivalently to Tempo's remote_write | Feature-Parity Gaps | LOW — no current dashboard/alert relies on exemplars. |
| A4 | Logs TTL of 7d / traces TTL of 14d is the desired retention | Pitfall 3, Stack | LOW — mirrors current Loki/Tempo retention; confirm with user if longer unified retention is wanted (this server is self-hosted, disk-bound). |
| A5 | Docker json-file log path/format (`/var/lib/docker/containers/*/*-json.log`) matches the production daemon | Pattern 1, Pitfall 2 | MEDIUM — Promtail uses docker.sock SD, not the file path; confirm the host's log driver is json-file (default) and the glob matches before relying on filelog. Fallback: keep docker.sock-based collection or a `dockerstats`/`journald` receiver. |

## Open Questions

1. **Single unified retention vs per-signal TTL?**
   - What we know: exporter `ttl` is per-instance; current retention is traces 14d / logs 7d.
   - What's unclear: whether the user wants one number now that it's one store.
   - Recommendation: two named exporter instances (336h / 168h) to preserve current behavior; revisit in a v2 if a unified policy is preferred.

2. **Keep the MinIO `tempo` bucket / `*_data` volumes, or reclaim?**
   - Recommendation: leave them through the gate; reclaim (`mc rb local/tempo`, `docker volume rm …_tempo_data …_loki_data`) only after verified cutover. Not on the critical path.

3. **Is the service-graph node graph actually used by anyone?**
   - What we know: only referenced in datasources.yml, no dashboard panel.
   - Recommendation: regenerate the metrics via the connector (cheap), but don't invest in rebuilding a Grafana nodeGraph panel unless asked.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| ClickHouse | trace/log store | ✓ | 26.3.12.3 (compose) | — |
| OTel Collector (contrib) | export pipelines | ✓ | 0.103.1 (compose) | — |
| `grafana-clickhouse-datasource` plugin | trace/log datasource | ✓ | installed at Grafana boot | — |
| Prometheus remote-write receiver | span-metrics sink | ✓ | enabled (`--web.enable-remote-write-receiver`) | — |
| Docker json-file logs at `/var/lib/docker/containers` | filelog receiver | ✓ (Promtail uses it today) | — | docker.sock SD if file glob fails (A5) |

**Missing dependencies with no fallback:** None — every component is already deployed.

## Validation Architecture

> No automated unit-test framework applies to YAML/provisioning config. Validation is live-stack verification on production (this server IS production). `workflow.nyquist_validation` not consulted in config; treating the live-verification table below as the test map.

### Phase Requirements → Verification Map
| Req ID | Behavior | Verification (live) | Gate |
|--------|----------|---------------------|------|
| AR-CONS-01 | Traces in ClickHouse; Tempo container + datasource gone; backend-tracing dashboard renders from CH | `SELECT count() FROM otel_traces` > 0; `docker compose ps` has no `tempo`; `grep -L tempo datasources.yml`; open `https://animeenigma.ru/admin/grafana/d/backend-tracing` and confirm traces render | Render BEFORE Tempo removal |
| AR-CONS-02 | Logs in ClickHouse; Loki + Promtail gone; log views render from CH | `SELECT count() FROM otel_logs WHERE timestamp>now()-INTERVAL 5 MINUTE` > 0; no `loki`/`promtail` containers; CH logs panel renders | Render BEFORE Loki removal |
| AR-CONS-03 | Prometheus + Grafana unchanged: alerts fire, metric dashboards render | `/prometheus/api/v1/rules` shows rule groups in `inactive`/`firing` (not `error`); span-metrics present via `traces_spanmetrics_calls_total`; open `Services / Overview` (animeenigma-services), `playback-health`, `product-analytics`, `activity-register-*` and confirm panels render | Final step, after cutover |

### Wave 0 Gaps
- None (no test files). The OTel config SHOULD be validated with `otelcol validate` as a pre-recreate check (Pitfall 5) — recommend the planner add this as an explicit task action.

## Security Domain

> `security_enforcement` not set false in config — included.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No new auth surface; Grafana SSO/auth-proxy unchanged |
| V4 Access Control | yes (minor) | ClickHouse ports stay bound `127.0.0.1` only (compose:325-328); collector adds CH creds via env (no new exposed port) |
| V5 Input Validation | no | No user input paths added |
| V6 Cryptography | no | Internal Docker-network plaintext (existing posture); CH creds from `.env` |
| V7 Logging | yes | Log pipeline moves stores; ensure no NEW PII is captured. `filelog` ingests container stdout same as Promtail did — same data, new store. `ip_hash` etc. already hashed in the events table; raw logs may contain IPs as they do today (no regression). |

### Known Threat Patterns for {observability config}
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Collector config typo halts all traces | Denial of Service | `otelcol validate` pre-recreate; gated rollout keeps Tempo until CH verified |
| ClickHouse becomes single SPOF for events+traces+logs | Availability | Accepted tradeoff (ROADMAP CDI note); clickhouse-backup sidecar exists (Phase 1); Prometheus alerting independent of CH |
| Exposed CH port | Information Disclosure | Ports already `127.0.0.1`-bound; do not add new published ports |
| New X-WEBAUTH bypass via new datasource | Elevation | No change to Grafana auth-proxy; datasource is provisioned read-only (`editable:false`) |

## Sources

### Primary (HIGH confidence)
- Repo files read directly: `infra/otel/collector-config.yaml`, `infra/tempo/tempo.yaml`, `docker/docker-compose.yml` (lines 215-454), `docker/grafana/provisioning/datasources/datasources.yml`, `docker/grafana/provisioning/dashboards/dashboards.yml`, `docker/prometheus/prometheus.yml`, `docker/grafana/provisioning/alerting/rules.yml`, `docker/loki/loki-config.yml`, `docker/promtail/config.yml`, `infra/grafana/dashboards/backend-tracing.json`, `services/analytics/internal/repo/clickhouse_schema.go`, `libs/logger/logger.go`, `.planning/ROADMAP.md`, `.planning/REQUIREMENTS.md`, `.planning/phases/06-.../06-CONTEXT.md`, Phase 3 plan-04 SUMMARY
- grep verification: `traces_spanmetrics_*` / `traces_service_graph_*` appear ONLY in datasources.yml; `aenigma-tempo` only in backend-tracing.json; no dashboard references `aenigma-loki`

### Secondary (MEDIUM confidence)
- OTel Collector ClickHouse exporter README — github.com/open-telemetry/opentelemetry-collector-contrib (config keys, default tables, TTL)
- Grafana ClickHouse datasource docs — grafana.com/docs/plugins/grafana-clickhouse-datasource (OTel trace builder, trace-ID search, logs↔traces)
- ClickHouse Grafana plugin 4.0 blog — clickhouse.com/blog/clickhouse-grafana-plugin-4-0

### Tertiary (LOW confidence)
- Exact 0.103.1 component key spellings (A1) — validate with `otelcol validate` before recreate

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all components present and pinned in repo; capabilities cross-checked against official docs
- Architecture / cutover sequence: HIGH — derived from the actual config files; the span-metrics landmine resolved by direct grep evidence
- Span-metrics survival (AR-CONS-03): HIGH — verified no alert/dashboard consumes Tempo-generated metrics
- Exact OTel config YAML: MEDIUM — components confirmed available; precise keys to be validated at plan/execute time
- filelog log-path match (A5): MEDIUM — confirm host log driver before relying on the file glob

**Research date:** 2026-06-08
**Valid until:** 2026-07-08 (stable infra; re-verify only if collector/grafana/clickhouse images are bumped)
```

