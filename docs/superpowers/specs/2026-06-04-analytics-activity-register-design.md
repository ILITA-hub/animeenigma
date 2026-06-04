# Activity Register (ClickHouse Unified Event Plane) — Design Spec

**Date:** 2026-06-04
**Status:** Approved (brainstorm complete) — target milestone **root v4**
**Supersedes/extends:** `project_analytics_clickhouse_deferred.md` (Postgres-first clickstream; ClickHouse was deferred — this milestone executes the deferred consolidation).

---

## Goal

A **multidimensional, pivotable register of every platform action and its effects**, unifying frontend and backend causation with egress/DB/cache aggregation, on a ClickHouse-backed wide-event store, surfaced as human-readable Grafana reports.

The mental model is a **1С регистр накопления** (accumulation register): each action is a "document" that posts one or more "movements" (effect rows) carrying **dimensions** + **resources**, pivotable along any axis. Equivalently, the modern **"wide events"** observability pattern (Honeycomb-style).

**Primary goal: awareness** — "what is the platform actually doing right now / today, by operation and by external dependency, with anomalies surfaced." Optimization insight (e.g. "a popular profile button silently runs expensive queries") is a derived perk of the same data, not a separate system.

---

## What we have today (substrate inventory)

| Capability | Exists | Where |
|---|---|---|
| Distributed tracing (OTel → Tempo), W3C propagation FE→gateway→services | ✅ | `libs/tracing/*`, `infra/otel/collector-config.yaml`, `infra/tempo/tempo.yaml` |
| Outbound transport seam (`WrapTransport`) | ✅ | `libs/tracing/client.go` |
| Inbound middleware on every service | ✅ | `libs/tracing/middleware.go` |
| Clickstream fact store + `EventStore` swap-seam interface | ✅ (Postgres) | `services/analytics/internal/domain/store.go` |
| Grafana Postgres table panels (GROUP BY any dimension) | ✅ | `infra/grafana/dashboards/product-analytics.json` |
| Coarse egress counters | ⚠️ sparse | `libs/metrics/{external,parser,bandwidth}.go` |
| FE traceparent minting | ✅ but not wired to analytics | `frontend/web/src/analytics/traceparent.ts` |

## Gaps this milestone closes

1. **No egress/effect fact is recorded.** Outbound calls aren't dimensioned rows; several clients emit nothing (Kodik extractor, scraper `BaseHTTPClient`, OpenSubtitles, idmapping AniList).
2. **Bytes aren't per-host** and are client-egress only (`proxy.go` knows the host but doesn't attribute bytes; no upstream/ingress byte count).
3. **FE→BE last mile broken:** `traceparent` minted but `trace_id` not sent to the analytics collector; spans lack business dimensions (user_id, anime_id, operation).
4. **Tempo tail-samples ~80% of normal traces** → cannot be the *counting* source. Exact counts/anomaly detection must come from the register.

---

## Architecture — Topology A: ClickHouse unified event plane

ClickHouse becomes the single columnar event store for **the activity register + traces + logs** (OTel Collector ClickHouse exporter — the SigNoz/"observability-on-ClickHouse" pattern). **Tempo + Loki are retired.** **Prometheus + Grafana stay** (metrics scrape + alerting + rendering).

```
FE (axios + PerformanceObserver) ─┐
BE goroutines / jobs ─────────────┤ baggage{origin,user_id,operation}
                                  ▼
        inbound middleware seeds baggage  ──►  spans
                                  │
        outbound WrapTransport seam ──► EFFECT RECORDER (async, batched, drop-on-full)
                                  │            │
        otel-GORM (db) / cache hooks ─────────►│
                                               ▼
                                     EventStore (ClickHouse)
                                               ▲
        OTel Collector ── traces + logs ───────┘   (retire Tempo + Loki)
                                               │
                              Grafana (ClickHouse + Prometheus datasources)
                                  ▪ wide-event pivot tables (template vars = any dimension)
                                  ▪ from → choke-point → effects report
                                  ▪ anomaly flags, awareness overview
```

**Why ClickHouse:** purpose-built for high-volume append-only event facts and OLAP pivots; columnar compression keeps per-request/per-effect fidelity cheap; `SummingMergeTree`/`AggregatingMergeTree` materialized views are *literal* pre-aggregated accumulation registers (1С analog). The existing `EventStore` interface is the swap seam — already designed for this.
**Accepted cost:** ClickHouse is a new stateful service on a single host = a SPOF and ops burden. Mitigated by sequencing: the register is proven on ClickHouse (phases 1–5) *before* traces/logs are migrated off Tempo/Loki (phase 6).

---

## Data model — wide events (1 row per effect)

One **action** (FE click / goroutine tick / job) produces **N effect rows**, all sharing `trace_id` + `operation`. Storing N rows (not 1 per action) is deliberate — it is the 1С "document → movements" shape and the only way to pivot effects independently.

**Dimensions**
- `origin` — `fe_click(route, action)` · `goroutine(name)` · `scheduled_job(name)` · `api`
- `operation` / choke-point — **auto-derived** (see below), e.g. `catalog.UpdateAnimeInfo`
- `effect_kind` — `egress` · `db_write` · `db_read` · `cache` · `fe_rum`
- `target` — egress: `provider` + `host`; db: `table` + `op`; cache: `key_class`
- correlation — `trace_id`, `session_id`
- convenience (nullable) — `user_id`, `anime_id`
- quality flags — `source` (`be` | `fe_rum`), `accuracy` (`exact` | `approx`)

**Resources (measures)** — `requests` (count), `bytes_in`, `bytes_out`, `duration_ms`, `rows` (db).

**FE RUM rows** are flagged `source=fe_rum, accuracy=approx`; bytes are typically `0` (opaque cross-origin without `Timing-Allow-Origin`) and **never summed with authoritative BE bytes**. Hard limit acknowledged: the cross-origin **Kodik iframe's internal fetches are invisible** to the parent page — we capture only the iframe load + success/failure/timing.

---

## Instrumentation — three seams, not per-button

1. **FE axios interceptor** (already mints `traceparent`): also send `trace_id` to the analytics collector and stamp each call with current route + optional semantic action. The global click auto-capture already exists — **no button code.**
2. **BE inbound middleware** (`libs/tracing/middleware.go`): seed baggage from context — `user_id` (JWT claims), route, `operation`.
3. **BE outbound transport** (`libs/tracing/client.go` `WrapTransport`): the recorder reads baggage + records the effect. **One place covers every client on the shared transport.**

**Retrofit (the real work):** migrate clients that bypass the shared transport onto it — Kodik extractor, scraper `BaseHTTPClient` (retryablehttp), OpenSubtitles, idmapping.

**HLS volume discipline:** every ~6s segment is a separate upstream GET (≈240/episode × viewers). **Aggregate to one effect row per (stream-session, host)** with summed bytes + segment count — never one row per segment.

## Auto operation discovery (no predetermined catalog)

The clean handler→service→repo layering makes the **service-layer frame the business-operation boundary**. Combine:
1. **Endpoint level (free):** `otelhttp` span name `METHOD /route` → `GROUP BY` for "top endpoints."
2. **Tempo span-metrics generator (config flag):** auto per-span-name RED metrics + service graph into Prometheus.
3. **Stack-frame attribution:** at effect-record time, walk `runtime.Callers`, pick the nearest `*/internal/service/*` frame → `operation` (e.g. `spotlight.AnimeOfDayResolver.Resolve`) automatically.
4. **Pyroscope continuous profiling (optional):** cost-by-function, zero per-function code — directly answers "which popular code path is expensive."

---

## Retention, scale, consolidation

- Register: per-effect fidelity in ClickHouse; pre-aggregated `AggregatingMergeTree` rollups for the dashboards.
- Consolidation retires **Tempo + Loki** (traces + logs into ClickHouse via OTel Collector ClickHouse exporter). **Prometheus + Grafana stay.**
- Net infra change: `{analytics-Postgres + Tempo + Loki}` → `{ClickHouse}` for the event/trace/log plane.

---

## Phase breakdown (root v4, 6 phases)

1. **ClickHouse foundation + EventStore swap** — stand up ClickHouse (compose, backup, monitoring), wide-event schema (dims + measures, 1 row/effect), ClickHouse `EventStore` behind the existing interface, migrate clickstream onto it.
2. **BE egress recorder** — async batched recorder at `WrapTransport` + baggage; retrofit non-shared clients; per-(session,host) HLS aggregation; provider+host+bytes.
3. **DB/cache effects + auto operation discovery** — otel-GORM DB-write effects, cache effects, `runtime.Caller` operation attribution, Tempo span-metrics + service graph.
4. **FE causation + RUM** — wire `trace_id` into analytics events, axios route/action tagging, `PerformanceObserver` browser→3rd-party (marked approximate).
5. **Reports & dashboards** — Grafana wide-event pivot tables (template vars = any dimension), the "from → choke-point → effects" report, anomaly flagging, awareness overview; (optional) Pyroscope.
6. **Consolidation → topology A** — OTel Collector ClickHouse exporter for traces + logs; **retire Tempo + Loki**; keep Prometheus + Grafana. Deliberately last (register proven before SPOF consolidation).

---

## Risks

- **ClickHouse SPOF / ops burden** on single host → sequence consolidation last; document backup/restore in phase 1.
- **Write-volume blow-ups** (HLS segments, per-row DB) → aggregation discipline; facts only for writes/flagged ops, spans+sampling for the long tail.
- **Cardinality** (anime_id, user_id) → ClickHouse tolerates it; keep as nullable drill-down dims, not in hot rollup keys.
- **Stack-walk overhead** → only on the async recording path, never the request hot loop.
- **Retiring Tempo/Loki** is a migration of its own → gated as the final phase, reversible until cutover.

---

## Metrics (project convention — UXΔ / CDI / MVQ; no days/hours)

- **UXΔ = +1 (Better)** — internal observability; indirect user benefit (reliability + perf insight → fewer incidents, faster pages over time).
- **CDI = 0.24 * 89** — Spread×Shift ≈ 0.6 × 0.4 (touches catalog, scraper, streaming, `libs/*`, frontend, infra; introduces ClickHouse, retires Tempo/Loki); Effort_Fib = 89 (a full 6-phase milestone).
- **MVQ = Kraken 88%/82%** — a many-tentacled system reaching into every service's egress/DB path; high match, strong slop-resistance (built on existing seams + proven patterns).
