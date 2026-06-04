# Requirements: v4.0 Activity Register (ClickHouse unified event plane)

**Defined:** 2026-06-04
**Milestone:** v4.0
**Core Value:** Platform-wide awareness — see every action and its effects, pivotable by any dimension (a 1С-style accumulation register / wide-event store).
**Design spec:** `docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md`

## v1 Requirements

Each maps to a roadmap phase (1–6).

### Store — ClickHouse foundation (Phase 1)

- [ ] **AR-STORE-01**: ClickHouse runs as a docker-compose service with a documented backup/restore procedure, a healthcheck, and a Prometheus scrape (or self-metrics) entry.
- [ ] **AR-STORE-02**: Wide-event schema defined — **one row per effect** — dimensions (`origin`, `operation`, `effect_kind`, `target_kind`, `target`, `trace_id`, `session_id`, nullable `user_id`/`anime_id`, `source`, `accuracy`) + measures (`requests`, `bytes_in`, `bytes_out`, `duration_ms`, `rows`).
- [ ] **AR-STORE-03**: A ClickHouse `EventStore` implementation (`InsertBatch` + `UpsertIdentity`) passes the same contract tests as the Postgres impl.
- [ ] **AR-STORE-04**: The existing analytics clickstream is migrated onto ClickHouse — clickstream events still ingest and the `product-analytics` dashboards still render.
- [ ] **AR-STORE-05**: Ingestion is async + batched + drop-on-full (no added latency on hot paths); a metric exposes dropped-event count.

### Egress recorder — BE (Phase 2)

- [ ] **AR-EGRESS-01**: An effect recorder at the `libs/tracing` `WrapTransport` outbound seam emits one egress effect row per third-party request (provider, host, status, bytes, duration).
- [ ] **AR-EGRESS-02**: `origin` + `operation` + `user_id` ride OTel **baggage** from the inbound middleware to the recorder.
- [ ] **AR-EGRESS-03**: All currently-uninstrumented outbound clients are migrated onto the wrapped transport: Kodik extractor, scraper `BaseHTTPClient`, OpenSubtitles, idmapping (ARM/AniList).
- [ ] **AR-EGRESS-04**: HLS proxy egress is aggregated to **one effect row per (stream-session, host)** with summed bytes + segment count — never one row per segment.
- [ ] **AR-EGRESS-05**: Both `bytes_out` (client egress) and `bytes_in` (upstream ingress) are captured where the proxy reads upstream.

### Effects + auto operation discovery — BE (Phase 3)

- [ ] **AR-EFFECT-01**: DB writes are recorded as effect rows (`table`, `op`, `rows`) via otel-GORM; trivial reads are NOT fact-rowed (spans + sampling only).
- [ ] **AR-EFFECT-02**: Cache effects (hit/miss by key-class) are recorded.
- [ ] **AR-EFFECT-03**: `operation` is auto-derived with **no manual catalog**, via service-layer stack-frame attribution (nearest `*/internal/service/*` frame).
- [ ] **AR-EFFECT-04**: Tempo span-metrics generator + service graph enabled → per-operation RED metrics in Prometheus, no code.

### FE causation + RUM (Phase 4)

- [ ] **AR-FE-01**: The axios interceptor sends the active `trace_id` to the analytics collector and stamps each call with route + optional semantic action.
- [ ] **AR-FE-02**: Click auto-capture events carry `trace_id` so clicks join to backend traces/effects.
- [ ] **AR-FE-03**: A `PerformanceObserver` beacons browser→3rd-party resource timings (host, count, timing) flagged `source=fe_rum, accuracy=approx`; never summed with authoritative BE bytes.

### Reports — Grafana (Phase 5)

- [ ] **AR-REPORT-01**: A wide-event pivot dashboard with template variables lets an admin group/filter by ANY dimension (origin, operation, provider, host, effect_kind, …).
- [ ] **AR-REPORT-02**: The "from → choke-point → effects" report renders the target shape (origin → operation → per-target requests + bytes).
- [ ] **AR-REPORT-03**: Volume anomalies are flagged (a provider/operation request count far above its baseline).
- [ ] **AR-REPORT-04**: An "awareness overview" answers "what is the platform doing now/today" at a glance.

### Consolidation — topology A (Phase 6)

- [ ] **AR-CONS-01**: OTel Collector exports traces to ClickHouse; **Tempo is retired** (datasource + container removed; backend-tracing dashboard repointed).
- [ ] **AR-CONS-02**: Logs are shipped to ClickHouse; **Loki is retired** (datasource + container removed; log views repointed).
- [ ] **AR-CONS-03**: Prometheus + Grafana remain the metrics + alerting + rendering layer (unchanged).

## v2 / Deferred

- **AR-V2-01**: AggregatingMergeTree pre-aggregated rollups (1С-style accumulation registers) beyond dashboard needs.
- **AR-V2-02**: Pyroscope continuous profiling (cost-by-function) integration.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Per-segment HLS fact rows | Aggregated per stream-session by design (volume). |
| Replacing Prometheus / Grafana alerting | They stay; only Tempo + Loki consolidate into ClickHouse. |
| Browser-internal iframe egress (Kodik iframe internals) | Cross-origin iframe — invisible to our JS; hard browser limit. |
