# Phase 5: Reports & Dashboards - Research

**Researched:** 2026-06-06
**Domain:** Grafana dashboard authoring (JSON provisioning) over a ClickHouse wide-event store
**Confidence:** HIGH

## Summary

This phase is **pure Grafana dashboard authoring** — JSON files dropped under the existing
file-provisioned dashboard directories, querying the already-provisioned `aenigma-clickhouse`
datasource (ClickHouse `analytics.events` / `events_resolved`). It is **not** Vue/web frontend
work; the design-system/UI-SPEC lint machinery does not apply. No Go code is required.

**The two critical infrastructure questions both resolve GREEN — there is NO Wave-0 blocker:**
1. The `grafana-clickhouse-datasource` plugin **IS installed** (`GF_INSTALL_PLUGINS: grafana-clickhouse-datasource`, docker-compose.yml:425). [VERIFIED: codebase grep]
2. The ClickHouse datasource **IS provisioned** with `uid: aenigma-clickhouse`, `defaultDatabase: analytics`, native protocol (datasources.yml:81-99). [VERIFIED: codebase grep]

Grafana is **v10.3.3** (docker-compose.yml:374) — fully supports query template variables,
transformations, the ClickHouse `$__timeFilter`/`$__conditionalAll` macros, and unified
alerting. [VERIFIED: codebase grep]

**Primary recommendation:** Author 2–3 new dashboard JSON files modeled on
`infra/grafana/dashboards/product-analytics.json` (the canonical ClickHouse-bound analog),
drop them into `infra/grafana/dashboards/`, drive pivots with ClickHouse SQL query template
variables (`SELECT DISTINCT <dim>`), and add the AR-REPORT-03 anomaly rule to
`docker/grafana/provisioning/alerting/rules.yml` (ClickHouse-datasource alert query). Reload
with `make restart-grafana` (config-only — no rebuild). The live gate (inject a synthetic
spike, open Grafana, watch it flag) is **non-autonomous**, mirroring 02-04/03-06/04-04.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
*(discuss skipped — no locked decisions; all implementation at Claude's discretion)*

### Claude's Discretion
All implementation choices at Claude's discretion. Use ROADMAP success criteria + codebase conventions.

Key facts that constrain the work:
- This is **GRAFANA dashboard authoring** (JSON provisioning under `docker/grafana/provisioning/`
  / `infra/grafana/dashboards/` + datasources), **NOT Vue/web frontend**. The frontend
  design-system / UI-SPEC machinery does NOT apply.
- Data source: ClickHouse `analytics.events` (columns: `timestamp`, `trace_id`, `source`,
  `origin`, `operation`, `effect_kind`, `target`, `target_kind`, `anime_id`, `duration_ms`,
  `row_count`, and byte columns). A ClickHouse Grafana datasource must exist or be provisioned.
- Anomaly flagging: prefer **ClickHouse-query-driven baseline comparison** (count vs trailing
  avg/stddev) rendered as a panel + optional Grafana alert rule, over heavy ML. Keep it
  query-based and explainable.
- Byte aggregations **MUST filter `source='be'`** (authoritative) — never sum `fe_rum`
  approximate rows (AR-FE-03 discipline carries into the reports).
- Existing Grafana already provisioned (Prometheus + Tempo + ClickHouse datasources). Reuse
  the provisioning pattern.

### Deferred Ideas (OUT OF SCOPE)
- None — discuss skipped. (Pyroscope cost-by-function profiling AR-V2-02 is a separately-deferred
  backlog item, out of scope here.)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AR-REPORT-01 | Wide-event pivot dashboard — template vars group/filter by ANY dimension (origin, operation, provider, host, effect_kind, …) | ClickHouse SQL query template variables (`SELECT DISTINCT <dim>`) + a `$group_by`/`$filter` driving a `GROUP BY {{var}}` table panel. Macro `$__conditionalAll` handles the "All" case. (see Pattern 1) |
| AR-REPORT-02 | "from → choke-point → effects": origin → operation → per-target requests+bytes | Single table panel `GROUP BY origin, operation, target` with `count() AS requests, sum(bytes_out+bytes_in) AS bytes` filtered `source='be' AND effect_kind='egress'`; Grafana "Group to nested tables" / row-grouping transform renders the drill shape. (see Pattern 2) |
| AR-REPORT-03 | Volume anomaly flagging vs baseline (query-driven, explainable, not ML) | ClickHouse window query: recent count vs trailing baseline avg+Nσ (or ratio). Rendered as a table/state-timeline panel + a Grafana alert rule on the ClickHouse datasource mirroring `rules.yml` shape. (see Pattern 3) |
| AR-REPORT-04 | Awareness overview: current top operations + top external deps + active anomalies in one view | One dashboard combining: top-operations table (`GROUP BY operation`), top-external-deps table (`effect_kind='egress' GROUP BY target` filtered `source='be'`), and the anomaly panel from AR-REPORT-03. (see Pattern 4) |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Pivot/aggregation logic | ClickHouse (query) | — | Grafana pushes `GROUP BY`/`sum()` to CH; columnar store does the heavy lift. Do NOT aggregate in Grafana transforms what SQL can do. |
| Template-var option lists | ClickHouse (`SELECT DISTINCT`) | Grafana (var binding) | Distinct dimension values come from CH; Grafana binds them to `$var` interpolation. |
| Display drill / nesting / value mapping | Grafana (panel + transforms) | — | Row-grouping, nested tables, thresholds, color are presentation — Grafana's job. |
| Anomaly detection | ClickHouse (windowed baseline query) | Grafana (alert rule + threshold expr) | Explainable SQL baseline in CH; Grafana renders + fires the alert. NO ML tier. |
| Auth / access | Gateway (`/admin/grafana` SSO) | Grafana (auth-proxy) | Already solved — admin lands as Org Admin via `X-WEBAUTH-*`. No phase work. |

## Standard Stack

### Core
| Component | Version | Purpose | Why Standard |
|-----------|---------|---------|--------------|
| Grafana | `10.3.3` (image `grafana/grafana:10.3.3`) | Dashboard rendering, template vars, unified alerting | Already the platform's monitoring layer [VERIFIED: docker-compose.yml:374] |
| grafana-clickhouse-datasource plugin | installed via `GF_INSTALL_PLUGINS` (latest at image build) | ClickHouse query backend for panels + vars + alerts | Official Grafana Labs plugin; the ONLY way to query CH from Grafana [VERIFIED: docker-compose.yml:425, CITED: grafana.com/grafana/plugins/grafana-clickhouse-datasource] |
| ClickHouse | (Phase-1 service `clickhouse`, native port 9000) | Wide-event store (`analytics.events`, `events_resolved` view) | The Phase-1 register sink [VERIFIED: clickhouse_schema.go] |

### Supporting (provisioning files — all already wired)
| File | Purpose | When to Touch |
|------|---------|---------------|
| `docker/grafana/provisioning/datasources/datasources.yml` | `aenigma-clickhouse` datasource def | DO NOT touch — already correct |
| `docker/grafana/provisioning/dashboards/dashboards.yml` | Two file providers: `/var/lib/grafana/dashboards` (legacy) + `/var/lib/grafana/dashboards-infra` (v3.1+ source-of-truth) | DO NOT touch — new JSON just lands in a mounted dir |
| `infra/grafana/dashboards/*.json` | **Source-of-truth dashboard dir** (mounted → `/var/lib/grafana/dashboards-infra`) | **Drop new dashboard JSON HERE** |
| `docker/grafana/provisioning/alerting/rules.yml` | Provisioned unified-alerting rules | Add the AR-REPORT-03 anomaly rule here |

**Installation:** None. No packages to install. The plugin is already in the Grafana image.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| ClickHouse SQL query template vars | Grafana ad-hoc filters (`$__adHocFilters`) | Ad-hoc filters are supported (CH 22.7+) and give a free-form filter bar, but are less discoverable than explicit `$origin`/`$operation`/`$effect_kind` dropdowns. **Recommend explicit query vars** for the named pivot dimensions; ad-hoc filters are an optional add-on for power users. [CITED: grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/template-variables] |
| Table panel with row-grouping transform (AR-REPORT-02) | `nodeGraph`/Sankey panel | Sankey would literally show "from→choke-point→effects" flow but needs a community panel plugin (not installed) — adds a Wave-0 dep. **Recommend the table + nested grouping** (zero new plugins). Note as an Open Question if a visual flow is desired later. |
| Provisioned alert rule (AR-REPORT-03) | Panel-only "anomaly" table with threshold coloring | A panel alone satisfies "visibly surfaced"; an alert rule additionally notifies. **Recommend both** — panel for the dashboard, plus a rule so the spike pages. |

## Package Legitimacy Audit

> No external packages are installed in this phase. The only third-party component
> (`grafana-clickhouse-datasource`) was vetted and installed in **Phase 1** (01-03), not here.

| Package | Registry | Disposition |
|---------|----------|-------------|
| grafana-clickhouse-datasource | Grafana plugin catalog (official, Grafana Labs) | Already installed in Phase 1 — no new install this phase |

**Packages removed due to slopcheck [SLOP] verdict:** none (no installs).
**Packages flagged as suspicious [SUS]:** none.

## ClickHouse `events` Schema — Pivot Dimension Reference

Source: `services/analytics/internal/repo/clickhouse_schema.go` [VERIFIED: codebase read]

### Dimensions (good template-var candidates — LowCardinality = cheap `SELECT DISTINCT`)
| Column | Type | Notes |
|--------|------|-------|
| `origin` | `LowCardinality(String)` DEFAULT `'api'` | "from" axis. Observed values: `api` (dominant), `unknown`. [VERIFIED: grep] |
| `operation` | `LowCardinality(String)` DEFAULT `''` | The "choke-point" — auto-derived `service.Method` (e.g. `catalog.UpdateAnimeInfo`). Phase-3 attribution. |
| `effect_kind` | `LowCardinality(String)` DEFAULT `''` | **Observed values: `egress`, `db_write`, `db_read`, `cache_hit`, `cache_miss`** (`''` for clickstream rows). [VERIFIED: grep — see note below] |
| `target_kind` | `LowCardinality(String)` DEFAULT `''` | **Observed values: `host` (egress), `table` (DB), `key_class` (cache), `provider`.** [VERIFIED: grep] |
| `source` | `LowCardinality(String)` DEFAULT `'be'` | **Values: `be` (authoritative), `fe`, `fe_rum` (approximate).** Byte filter axis. [VERIFIED: collect.go:115 whitelist] |
| `accuracy` | `LowCardinality(String)` DEFAULT `'exact'` | `exact` vs `approx` (fe_rum). |
| `event_type` | `LowCardinality(String)` | Clickstream: `pageview`/`click`/`heartbeat`. |
| `device_type`, `el_tag` | `LowCardinality(String)` | Clickstream dims. |

### High-cardinality dimension (filter/group but NOT a default-open dropdown)
| Column | Type | Notes |
|--------|------|-------|
| `target` | `String` (ZSTD) | The actual host (egress), table name (DB), or key-class string. High cardinality — `SELECT DISTINCT target` should be **scoped by `effect_kind`/`target_kind`** in the var query to keep the dropdown sane, and time-bounded with `$__timeFilter`. |
| `trace_id`, `session_id`, `anime_id` | `String` / `Nullable(String)` | Correlation keys, not pivot dropdowns. |

### Measures
| Column | Type | Aggregation | Critical Note |
|--------|------|-------------|---------------|
| `requests` | `UInt32` DEFAULT 0 | `sum(requests)` or `count()` | One row per effect; `count()` = effect-row count, `sum(requests)` = logical request count (HLS aggregates many segments into one row with `requests` > 1). **For egress request volume prefer `sum(requests)`.** |
| `bytes_in` | `UInt64` | `sum(bytes_in)` | Upstream ingress bytes. **MUST filter `source='be'`** — see AR-FE-03. |
| `bytes_out` | `UInt64` | `sum(bytes_out)` | Client egress bytes. **MUST filter `source='be'`.** |
| `duration_ms` | `UInt32` | `avg`/`quantile(0.95)` | Per-effect latency. |
| `row_count` | `UInt32` | `sum(row_count)` | DB rows affected (named `row_count`, **NOT `rows`** — `rows` is reserved-ish in the CH native binder, RESEARCH note A2 from Phase 1). |
| `active_ms` | `UInt32` | `sum` | Clickstream time-on-page only. |

> **`effect_kind`/`target_kind` value-set caveat [ASSUMED → must verify against live data]:** the
> literal set above is grepped from producer code, not read from the live table. The planner
> should NOT hardcode an enum list in panel SQL — drive every dropdown from a live
> `SELECT DISTINCT <dim> FROM events WHERE $__timeFilter(timestamp)` so the report self-discovers
> whatever values actually landed. Whether `cache_miss` is emitted as a distinct literal (vs only
> `cache_hit`) was not 100% confirmed in grep; the live-discovery approach makes this moot.

## Architecture Patterns

### System Architecture Diagram

```
                  ┌─────────────────────────────────────────────┐
   admin browser  │  gateway  (JWT + AdminRoleMiddleware)        │
        │ HTTPS    │  /admin/grafana/* → strips client X-WEBAUTH-*│
        └─────────▶│  injects X-WEBAUTH-USER / X-WEBAUTH-ROLE     │
                  └───────────────┬─────────────────────────────┘
                                  │  (auth-proxy SSO, Org Admin)
                                  ▼
                  ┌─────────────────────────────────────────────┐
                  │  Grafana 10.3.3                              │
                  │  ┌─ template vars: SELECT DISTINCT <dim> ──┐ │
                  │  │  $origin $operation $effect_kind        │ │
                  │  │  $target $group_by  (drive GROUP BY)    │ │
                  │  └────────────────────────┬────────────────┘ │
                  │  panels (rawSql) ─────────┤                  │
                  │  alert rule (rawSql) ─────┤                  │
                  └───────────────────────────┼──────────────────┘
                                              │ native :9000 (aenigma-clickhouse)
                                              ▼
                  ┌─────────────────────────────────────────────┐
                  │  ClickHouse  analytics.events / events_resolved│
                  │  one row per effect: origin, operation,        │
                  │  effect_kind, target, source, requests, bytes…│
                  └─────────────────────────────────────────────┘
```

Provisioning load path (no rebuild — config reload only):
```
infra/grafana/dashboards/*.json  ──mount──▶  /var/lib/grafana/dashboards-infra
docker/grafana/provisioning/alerting/rules.yml ──mount/copy──▶ /etc/grafana/provisioning/alerting
                                   │
                          make restart-grafana  (file provider re-scans)
```

### Recommended File Layout
```
infra/grafana/dashboards/
├── activity-register-pivot.json        # AR-REPORT-01 (wide-event pivot, template vars)
├── activity-register-flow.json         # AR-REPORT-02 (origin→operation→target)
└── activity-register-overview.json     # AR-REPORT-04 (awareness) + AR-REPORT-03 anomaly panel

docker/grafana/provisioning/alerting/rules.yml   # append AR-REPORT-03 anomaly alert rule
```
> You MAY combine all into fewer files; the split above maps 1 file ≈ 1 requirement for clean
> review. Use kebab-case names, `uid` = `<name>` + discriminator (avoid colliding with the
> existing `product-analytics` uid). Validate each with `jq -e . <file>`.

### Pattern 1: ClickHouse SQL Template Variable Driving a Live Pivot (AR-REPORT-01)

**What:** A "group-by" custom var + per-dimension filter vars sourced from `SELECT DISTINCT`.
**When:** The pivot dashboard where switching a dropdown regroups the table live.

Variable definition (dashboard JSON `templating.list[]` entry — query type):
```json
{
  "name": "effect_kind",
  "type": "query",
  "datasource": { "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" },
  "query": "SELECT DISTINCT effect_kind FROM events WHERE effect_kind != '' AND $__timeFilter(timestamp) ORDER BY effect_kind",
  "includeAll": true,
  "multi": true,
  "refresh": 2
}
```
> `refresh: 2` = "On Time Range Change" so the option list respects the dashboard time range.
> The query returns **one column** → label == value. [CITED: grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/template-variables]

A `$group_by` **custom** var (not query) lets the admin pick the GROUP BY axis:
```json
{ "name": "group_by", "type": "custom",
  "query": "origin, operation, effect_kind, target_kind, target, source",
  "current": { "text": "operation", "value": "operation" } }
```

Panel `rawSql` (table panel) — note `${group_by}` is interpolated as a raw column name and
multi-value filters use `:singlequote` + `$__conditionalAll`:
```sql
-- Source: product-analytics.json pattern + CH plugin macro docs
SELECT
  ${group_by}                       AS dimension,
  sum(requests)                     AS requests,
  sumIf(bytes_out + bytes_in, source = 'be') AS bytes_be,
  round(avg(duration_ms), 1)        AS avg_ms
FROM events
WHERE $__timeFilter(timestamp)
  AND $__conditionalAll(effect_kind IN (${effect_kind:singlequote}), $effect_kind)
  AND $__conditionalAll(origin      IN (${origin:singlequote}),      $origin)
GROUP BY dimension
ORDER BY requests DESC
LIMIT 100
```
> **Gotcha:** `$__conditionalAll`'s 2nd arg MUST be a bare `$var` (no `:singlequote`); the 1st
> arg uses `:singlequote`. [CITED: grafana.com/docs/plugins/.../template-variables]
> **Gotcha:** `${group_by}` is injected as an unquoted SQL fragment — keep it a **custom var
> with a fixed column allowlist** (never a free-text/query var) to avoid SQL injection.

### Pattern 2: from → choke-point → effects (AR-REPORT-02)

**What:** One table, three grouping columns, requests+bytes measures.
```sql
SELECT
  origin,
  operation,
  target                                AS dependency,
  sum(requests)                         AS requests,
  sum(bytes_out + bytes_in)             AS bytes
FROM events
WHERE $__timeFilter(timestamp)
  AND effect_kind = 'egress'
  AND source = 'be'                      -- AR-FE-03: authoritative bytes only
GROUP BY origin, operation, target
ORDER BY bytes DESC
LIMIT 200
```
**Display:** Grafana table panel + the **"Group by" / row-grouping transformation** (or nested
tables) on `origin` then `operation` to render the drill shape. Use the "Bytes" unit (`unit:
"bytes"`) on the bytes field. [CITED: grafana.com/docs/plugins/.../query-editor]

### Pattern 3: Explainable Volume-Anomaly Baseline (AR-REPORT-03)

**What:** Per-(operation|provider) recent-window count vs trailing baseline avg+Nσ. No ML.
```sql
-- Per-operation hourly counts, last 24h; flag the latest hour vs its trailing baseline.
WITH hourly AS (
  SELECT operation,
         toStartOfHour(timestamp) AS h,
         sum(requests)            AS req
  FROM events
  WHERE timestamp >= now() - INTERVAL 24 HOUR
    AND effect_kind = 'egress' AND source = 'be'
  GROUP BY operation, h
),
stats AS (
  SELECT operation,
         avgIf(req, h <  toStartOfHour(now()))                 AS baseline_avg,
         stddevPopIf(req, h < toStartOfHour(now()))            AS baseline_sd,
         anyIf(req, h = toStartOfHour(now() - INTERVAL 1 HOUR)) AS last_req
  FROM hourly
  GROUP BY operation
)
SELECT operation, last_req, round(baseline_avg,1) AS baseline_avg,
       round(baseline_avg + 3 * baseline_sd, 1)   AS threshold,
       last_req > baseline_avg + 3 * baseline_sd  AS is_anomaly
FROM stats
WHERE baseline_avg > 0
ORDER BY (last_req - baseline_avg) / nullIf(baseline_sd,0) DESC
```
> Tunables to surface as panel options: window (24h), bucket (1h), σ multiplier (3) or a ratio
> rule (`last_req > 3 * baseline_avg`). It is **explainable** — the row shows last vs threshold.
> Render as a **table** with a threshold color override on `is_anomaly`, plus a **stat/state-timeline**
> for the awareness view.

**Provisioned alert rule** — mirror the existing `rules.yml` 3-refId shape (A=query,
B=reduce, C=threshold), but point refId A at the ClickHouse datasource:
```yaml
# append to docker/grafana/provisioning/alerting/rules.yml under the same group
- uid: activity-register-volume-anomaly
  title: Activity Register Volume Anomaly
  condition: C
  noDataState: OK
  data:
    - refId: A
      relativeTimeRange: { from: 86400, to: 0 }
      datasourceUid: aenigma-clickhouse          # NOT the Prometheus uid
      model:
        rawSql: |
          SELECT count() AS anomalies FROM ( <Pattern-3 query> ) WHERE is_anomaly
        format: table
        queryType: sql
        refId: A
    - refId: B
      datasourceUid: __expr__
      model: { type: reduce, expression: A, reducer: last, refId: B }
    - refId: C
      datasourceUid: __expr__
      model:
        type: threshold
        expression: B
        conditions: [ { evaluator: { type: gt, params: [0] } } ]
        refId: C
  for: 5m
  labels: { severity: warning }
  annotations:
    summary: "Volume anomaly: an operation/provider is far above baseline"
```
> [VERIFIED: rules.yml:9-109 is the exact 3-refId pattern to mirror]
> [CITED: grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/alerting] confirms the
> CH plugin supports alerting (instant rawSql → single numeric → threshold).

### Pattern 4: Awareness Overview (AR-REPORT-04)

Single dashboard, three regions:
1. **Top operations now/today** — `GROUP BY operation ORDER BY sum(requests) DESC LIMIT 10`.
2. **Top external dependencies** — `WHERE effect_kind='egress' AND source='be' GROUP BY target ORDER BY sum(bytes_out+bytes_in) DESC LIMIT 10`.
3. **Active anomalies** — the Pattern-3 query filtered `WHERE is_anomaly`.
Use a short default time range (`now-1h` / `now-24h`) and `refresh: 30s` for "now/today" feel.

### Anti-Patterns to Avoid
- **Summing `fe_rum` into byte totals.** Every byte aggregation MUST carry `source='be'` (or
  `sumIf(..., source='be')`). fe_rum is `accuracy=approx`, host-only, byte-poor. (AR-FE-03)
- **Hardcoding an `effect_kind`/`target` enum in SQL.** Drive dropdowns from `SELECT DISTINCT`
  so the report self-discovers live values.
- **Interpolating a free-text var as a raw column/SQL fragment.** `${group_by}` must be a
  fixed-allowlist **custom** var — never a query/text var (SQL-injection + breakage).
- **Pointing the anomaly alert at the Prometheus uid.** It queries ClickHouse — use
  `aenigma-clickhouse`. (Reusing/renaming uids orphaned 14 alert rules historically —
  datasources.yml header warning.)
- **`make redeploy-grafana`.** Not needed and slower. Dashboards + provisioning are mounts;
  `make restart-grafana` re-runs the entrypoint copy + file-provider scan.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Pivot/group-by switching | A custom panel or backend report endpoint | Grafana query template var + `GROUP BY ${var}` | Grafana does live re-query on var change for free |
| "All" filter handling | `WHERE dim IN (...)` string-built in SQL | `$__conditionalAll(dim IN (${dim:singlequote}), $dim)` | Plugin macro; correct empty/All semantics |
| Time-range filtering | `WHERE timestamp BETWEEN …` literals | `$__timeFilter(timestamp)` (or `$__timeFilter_ms`) | Plugin macro; DateTime64(3) ms precision, respects dashboard range [CITED: deepwiki CH SQL macros] |
| Anomaly detection | Python/ML job, separate service | ClickHouse windowed avg+Nσ query | Explainable, zero new infra, lives in the panel |
| Drill-down nesting | Multiple linked dashboards | One table + Grafana row-grouping/nested-table transform | Single panel, no cross-dashboard plumbing |
| Admin auth for Grafana | New login | Existing gateway `/admin/grafana` auth-proxy SSO | Already solved (X-WEBAUTH-* injection) |

**Key insight:** This phase is **declarative JSON over an already-complete data + auth + plugin
stack.** The craft is in the SQL and the pivot UX, not in building anything.

## Common Pitfalls

### Pitfall 1: fe_rum bytes contaminate authoritative totals
**What goes wrong:** Byte panels show inflated/garbage numbers.
**Why:** `fe_rum` rows are `accuracy=approx`, host-only, and byte-poor; summing them with `be`.
**How to avoid:** Every byte measure uses `source='be'` (or `sumIf(..., source='be')`).
**Warning signs:** Byte totals don't reconcile with the egress-only `source='be'` view.

### Pitfall 2: `$__conditionalAll` second-arg format
**What goes wrong:** "All" selection produces invalid SQL or never matches.
**Why:** Passing `${dim:singlequote}` as the 2nd arg instead of a bare `$dim`.
**How to avoid:** 1st arg `IN (${dim:singlequote})`, 2nd arg bare `$dim`. [CITED: CH plugin docs]

### Pitfall 3: New dashboard `uid` collision
**What goes wrong:** A new dashboard silently overwrites/clashes with `product-analytics` etc.
**Why:** Reusing an existing `uid`.
**How to avoid:** New `uid` per dashboard (`activity-register-*`). `jq` it. (README naming rule.)

### Pitfall 4: Wrong provisioning reload command
**What goes wrong:** New JSON doesn't appear, or a needless 5-min rebuild.
**Why:** `redeploy` rebuilds the image; this phase changes only mounted files.
**How to avoid:** `make restart-grafana` (config-only). The entrypoint re-copies provisioning +
the file provider re-scans `dashboards-infra`. [VERIFIED: docker-compose.yml:378-385 entrypoint]

### Pitfall 5: `row_count` vs `rows`
**What goes wrong:** Querying `rows` returns nothing / errors.
**Why:** The measure is named `row_count` (Phase-1 deliberately avoided reserved-ish `rows`).
**How to avoid:** Use `row_count`. [VERIFIED: clickhouse_schema.go:81]

## Runtime State Inventory

*(Not a rename/refactor/migration phase — greenfield dashboard authoring. Section included for
completeness; nothing carries old runtime state.)*

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — verified: phase adds query-only dashboards, writes nothing to CH/Postgres/Redis. | none |
| Live service config | Grafana dashboards/alerts are file-provisioned and live in git (`infra/grafana/`, `docker/grafana/provisioning/`). Any panel a human edits in the Grafana UI is **NOT** persisted to git (editable:true) — author in JSON, not the UI. | author in JSON files |
| OS-registered state | None. | none |
| Secrets/env vars | `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` already supplied to the grafana container; datasource already reads them. No new secrets. | none |
| Build artifacts | None — no compiled output; the plugin is already in the image. | none |

## Code Examples

### Verified ClickHouse panel target (the canonical analog to copy)
```json
// Source: infra/grafana/dashboards/product-analytics.json:50-62 [VERIFIED]
{
  "datasource": { "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" },
  "rawQuery": true,
  "format": "table",
  "queryType": "sql",
  "refId": "A",
  "rawSql": "SELECT count(DISTINCT person_id) AS visitors FROM events_resolved WHERE $__timeFilter(timestamp)"
}
```
> For time-series panels the product-analytics dashboard uses `"format": 0` (= time_series) with
> a `time` column first; for tables use `"format": "table"`.

### Top external dependencies (AR-REPORT-04 region 2)
```sql
SELECT target AS dependency,
       sum(requests)               AS requests,
       sum(bytes_out + bytes_in)   AS bytes
FROM events
WHERE $__timeFilter(timestamp)
  AND effect_kind = 'egress'
  AND source = 'be'
GROUP BY target
ORDER BY bytes DESC
LIMIT 10
```

## Validation Architecture

> `workflow.nyquist_validation` not found in config context for this phase; this section documents
> the verification approach regardless. The phase has **no automated unit-test surface** (it's
> declarative dashboard JSON) — validation is structural (jq/provision-load) + a non-autonomous
> live gate, exactly mirroring the 02-04 / 03-06 / 04-04 phase-gate plans.

### "Test Framework"
| Property | Value |
|----------|-------|
| Framework | None (no Go/TS code). Structural validation via `jq` + Grafana provisioning load. |
| Config file | n/a |
| Quick run command | `jq -e . infra/grafana/dashboards/<name>.json` (valid JSON, per README step 2) |
| Provision-load check | `make restart-grafana` then check Grafana logs for `provisioning` errors / `msg="finished to provision dashboards"` |

### Phase Requirements → Validation Map
| Req ID | Behavior | Validation Type | Command / Gate | Autonomous? |
|--------|----------|-----------------|----------------|-------------|
| AR-REPORT-01 | Template var regroups pivot live | Live (manual) | Open pivot dashboard, switch `$group_by`/`$effect_kind`, watch table regroup | ❌ non-autonomous |
| AR-REPORT-02 | origin→operation→target requests+bytes renders | Live (manual) | Open flow dashboard, read a real origin's per-target breakdown | ❌ non-autonomous |
| AR-REPORT-03 | Synthetic spike flagged | Live (manual) | Inject a synthetic volume spike (insert N `egress` rows for one operation/target into CH, or replay a burst), watch the anomaly panel flag it + alert transition | ❌ non-autonomous |
| AR-REPORT-04 | Single awareness view shows top ops + top deps + active anomalies | Live (manual) | Open overview dashboard, confirm all three regions populate | ❌ non-autonomous |
| (all) | JSON valid + provisions cleanly | Structural (autonomous) | `jq -e .` each file + restart-grafana log check | ✅ autonomous |

### Synthetic-spike injection recipe (for AR-REPORT-03 gate)
The live gate needs a controllable volume spike. Two options for the plan to choose:
- **Direct CH insert** (cleanest, isolated): `INSERT INTO analytics.events (timestamp, origin,
  operation, effect_kind, target, target_kind, source, requests) SELECT now(), 'api',
  'synthetic.SpikeOp', 'egress', 'spike.example.com', 'host', 'be', 1 FROM numbers(5000)` — then
  confirm `synthetic.SpikeOp` appears in the anomaly panel above its (zero) baseline.
- **Replay a real burst** (drive a hot endpoint repeatedly) — more end-to-end but noisier.

### Sampling Rate
- **Per dashboard file:** `jq -e .` on save.
- **Per wave:** `make restart-grafana` + provisioning-log scan (no orphaned datasource/uid).
- **Phase gate:** the four non-autonomous live checks above, in one Grafana session.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Grafana | all panels/vars/alerts | ✓ | 10.3.3 | — |
| grafana-clickhouse-datasource plugin | all CH queries | ✓ (installed via GF_INSTALL_PLUGINS) | latest @ image build | — |
| ClickHouse datasource (`aenigma-clickhouse`) | all queries | ✓ provisioned | native :9000, db `analytics` | — |
| ClickHouse `events` data (egress/DB/cache/FE) | meaningful reports | ✓ (Phases 2-4 complete) | — | — |
| `jq` | JSON validation | ✓ (standard) | — | python `json.tool` |
| File-provisioner mounts (`dashboards-infra`, alerting) | auto-load | ✓ wired | — | — |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None.

> **NO Wave-0 blocker.** The plugin-install / datasource-provision risk flagged in the brief is
> already satisfied — confirmed in docker-compose.yml:425 and datasources.yml:81. No
> `GF_INSTALL_PLUGINS` change and no grafana container recreate are required; `restart-grafana`
> (config reload) is sufficient for everything in this phase.

## Project Constraints (from CLAUDE.md)

- **Effort/impact metrics:** any plan must use UXΔ / CDI / MVQ — **no days/hours/sprints**.
  (ROADMAP already scores Phase 5: `UXΔ=+2`, `CDI=0.06*21`, `MVQ=Dragon 88%/86%`.)
- **Admin URLs:** Grafana is at `https://animeenigma.ru/admin/grafana` (prod, path-routed) /
  `127.0.0.1:3004` (local container). Prometheus is under the `/prometheus` route-prefix.
- **Reload command:** `make restart-grafana` reloads config without rebuild; `make redeploy-*`
  is for code changes (not needed here).
- **Datasource uids are load-bearing:** never reuse/rename `PBFA97CFB590B2093` (Prometheus),
  `aenigma-clickhouse`, `aenigma-tempo`, etc. — reuse orphaned 14 alert rules historically.
- **Dashboard source-of-truth:** new dashboards go in `infra/grafana/dashboards/` (kebab-case,
  unique `uid`), validated with `jq`, picked up by the file provisioner on restart.
- **After-update skill:** invoke `/animeenigma-after-update` at the end (lint/redeploy/health/
  changelog in Russian Trump-mode/commit+push). Note the changelog entry here is user-facing
  but the feature is admin-only observability.
- **Frontend DS/UI-SPEC does NOT apply** (no `.vue` touched).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `effect_kind` live values are exactly `egress`/`db_write`/`db_read`/`cache_hit`/`cache_miss`; `target_kind` are `host`/`table`/`key_class`/`provider` | Schema reference | LOW — panels drive dropdowns from `SELECT DISTINCT`, so the live value set is self-discovered regardless of this grep. |
| A2 | `cache_miss` is emitted as a distinct literal (only `cache_hit` was clearly grepped) | Schema reference | LOW — same self-discovery mitigation; only matters if a plan hardcodes a cache filter. |
| A3 | The CH plugin version in the image supports `$__conditionalAll` + `$__timeFilter_ms` (docs are "latest") | Patterns 1-3 | LOW-MED — `$__timeFilter`/`$__conditionalAll` are long-standing; `$__timeFilter_ms` is newer. If `_ms` is absent, plain `$__timeFilter` works (the table already uses it in product-analytics.json). Verify by opening the query editor once. |
| A4 | `make restart-grafana` re-runs the entrypoint copy of provisioning (so a new alert rule in rules.yml is re-read) | Validation / Pitfall 4 | LOW — entrypoint copies provisioning-tpl on every container start (docker-compose.yml:382-383); restart = container restart. If a rule edit doesn't apply, fall back to a one-off `docker compose up -d --force-recreate grafana`. |

## Open Questions (RESOLVED)

1. **Visual flow (Sankey) for AR-REPORT-02?**
   - Known: the table + row-grouping transform satisfies the requirement with zero new plugins.
   - Unclear: whether the user wants a literal flow diagram.
   - Recommendation: ship the table now; note Sankey as a future enhancement (needs a community
     panel plugin → a `GF_INSTALL_PLUGINS` addition + container recreate, i.e. a real Wave-0 cost
     if pursued). Do NOT add it unasked.

2. **Anomaly tuning defaults (σ multiplier vs ratio, window, bucket).**
   - Known: avg+3σ over a 24h/1h grid is a defensible default.
   - Unclear: the platform's real traffic shape may make 3σ too noisy or too quiet.
   - Recommendation: parameterize as dashboard custom vars (`$sigma`, `$window`) so the admin can
     tune without editing SQL; default 3σ / 24h / 1h.

3. **Per-target dropdown cardinality.**
   - Known: `target` is high-cardinality (every host/table/key-class).
   - Recommendation: scope the `$target` var query by `$effect_kind`/`$target_kind` and
     `$__timeFilter` so the dropdown stays usable.

## Sources

### Primary (HIGH confidence)
- `services/analytics/internal/repo/clickhouse_schema.go` — exact `events` columns, codecs, ORDER BY, `row_count` naming
- `docker/docker-compose.yml:373-453` — grafana service: v10.3.3, `GF_INSTALL_PLUGINS`, CH env, mounts, auth-proxy
- `docker/grafana/provisioning/datasources/datasources.yml` — `aenigma-clickhouse` datasource (uid, native, db analytics)
- `docker/grafana/provisioning/dashboards/dashboards.yml` — two file providers + mount paths
- `docker/grafana/provisioning/alerting/rules.yml` — the 3-refId provisioned-alert shape to mirror
- `infra/grafana/dashboards/product-analytics.json` — canonical CH-bound panel JSON (rawSql/format/queryType)
- `infra/grafana/dashboards/README.md` — dashboard add procedure, naming, restart-to-provision
- `docker/grafana/dashboards/playback-health.json` — query-type template variable JSON shape

### Secondary (MEDIUM confidence)
- [ClickHouse plugin for Grafana | Grafana Labs](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/)
- [ClickHouse template variables | Grafana Plugins documentation](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/template-variables/) — `:singlequote`, `$__conditionalAll`, ad-hoc filters
- [ClickHouse query editor | Grafana Plugins documentation](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/query-editor/)
- [ClickHouse alerting | Grafana Plugins documentation](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/alerting/) — CH datasource supports alert rules
- [SQL Macros | grafana/clickhouse-datasource | DeepWiki](https://deepwiki.com/grafana/clickhouse-datasource/3.4-sql-macros) — `$__timeFilter`/`$__timeFilter_ms`/`$__fromTime`/`$__toTime`

## Metadata

**Confidence breakdown:**
- Infrastructure readiness (plugin/datasource/version): HIGH — all verified in-repo
- Schema / pivot dimensions: HIGH (columns) / MEDIUM (exact live enum values — mitigated by `SELECT DISTINCT`)
- Pattern SQL + macros: HIGH (macros cited + a working CH panel exists in product-analytics.json)
- Anomaly query: MEDIUM — pattern is sound and explainable; thresholds need real-traffic tuning
- Alert-rule wiring: HIGH — mirrors the existing provisioned rules.yml shape exactly

**Research date:** 2026-06-06
**Valid until:** ~2026-07-06 (stable; the only fast-moving piece is the CH plugin doc, and the
in-repo product-analytics.json pins the working pattern regardless)
