# Roadmap: AnimeEnigma

## Milestones

- ✅ **v1.0 Smart Watch Picker Overhaul** — Phases 1-8 (shipped 2026-05-03) — see `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v2.0 Recommendations Engine** — Phases 9-14 (shipped 2026-05-07) — see `.planning/milestones/v2.0-ROADMAP.md`
- ✅ **v3.0 Universal Anime Scraper** — Phases 15-20 (shipped 2026-05-18; Phase 20 cutover landed but over-rotated — regression repaired in v3.1 Phase 24) — see `.planning/milestones/v3.0-ROADMAP.md`
- ✅ **v3.1 Scraper Self-Healing** — Phases 21-28 shipped + closed 2026-06-04 (orig 21-23 @2026-05-13 tagged `v3.1`; reopened 24-28 + `18anime` group shipped) — see `.planning/MILESTONES.md` + `.planning/milestones/v3.1-ROADMAP.md`
- 🟡 **v4.0 Activity Register (ClickHouse unified event plane)** — root milestone started 2026-06-04 — design: `docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md`

## Phases

**Phase Numbering:**
- v4.0 resets phase numbering to **1–6** (prior milestones used continuous numbering 1–28; those phase dirs were cleared and archived into the per-milestone `*-ROADMAP.md` files linked above).
- Decimal phases (e.g., 2.1) are reserved for urgent insertions that must execute between integer phases.

**Milestone goal:** A multidimensional, pivotable register of every platform action and its effects (egress / DB / cache), unifying frontend + backend causation on a ClickHouse-backed wide-event store, surfaced as human-readable Grafana reports. **Awareness first**; optimization insight is a derived perk.

- [x] **Phase 1: ClickHouse Foundation + EventStore Swap** — Stand up ClickHouse, define the wide-event schema (1 row per effect), implement the ClickHouse `EventStore` behind the existing interface, migrate the clickstream onto it. (completed 2026-06-05)
- [x] **Phase 2: BE Egress Recorder** — Async batched effect recorder at the `WrapTransport` outbound seam + OTel baggage; retrofit non-shared HTTP clients; per-(stream-session, host) HLS aggregation. (completed 2026-06-05)
- [ ] **Phase 3: DB/Cache Effects + Auto Operation Discovery** — otel-GORM DB-write effects, cache hit/miss effects, stack-frame operation attribution, Tempo span-metrics + service graph.
- [ ] **Phase 4: FE Causation + RUM** — Wire `trace_id` into analytics events, axios route/action tagging, `PerformanceObserver` browser→3rd-party RUM (flagged approximate).
- [ ] **Phase 5: Reports & Dashboards** — Grafana wide-event pivot tables (template vars = any dimension), the "from → choke-point → effects" report, anomaly flagging, awareness overview.
- [ ] **Phase 6: Consolidation → Topology A** — OTel Collector ClickHouse exporter for traces + logs; retire Tempo + Loki; keep Prometheus + Grafana. Deliberately last (register proven before SPOF consolidation).

## Phase Details

### Phase 1: ClickHouse Foundation + EventStore Swap
**Goal**: A ClickHouse-backed wide-event store exists and serves the existing analytics clickstream behind the unchanged `EventStore` interface, with ingestion that never adds latency to a hot path.
**Depends on**: Nothing (first v4.0 phase)
**Requirements**: AR-STORE-01, AR-STORE-02, AR-STORE-03, AR-STORE-04, AR-STORE-05
**Success Criteria** (what must be TRUE):
  1. `docker compose ps` shows a healthy ClickHouse container with a passing healthcheck and a Prometheus scrape (or self-metrics) entry; a documented backup/restore procedure exists and a restore has been dry-run at least once. (AR-STORE-01)
  2. The wide-event table holds **one row per effect** with the agreed dimensions (`origin`, `operation`, `effect_kind`, `target_kind`, `target`, `trace_id`, `session_id`, nullable `user_id`/`anime_id`, `source`, `accuracy`) and measures (`requests`, `bytes_in`, `bytes_out`, `duration_ms`, `rows`); a schema test asserts the column set. (AR-STORE-02)
  3. The ClickHouse `EventStore` implementation (`InsertBatch` + `UpsertIdentity`) passes the **same contract test suite** the Postgres impl passes — verified by running the shared suite against both backends. (AR-STORE-03)
  4. The existing analytics clickstream ingests into ClickHouse and the `product-analytics` Grafana dashboards still render from the new datasource — confirmed by a live event flowing end-to-end and a dashboard panel showing it. (AR-STORE-04)
  5. Ingestion is async + batched + drop-on-full (no synchronous write on any request hot path); a `*_dropped_total`-style metric exposes the dropped-event count and is observable in `/metrics`. (AR-STORE-05)
**Plans**: 3 plans (2 waves)
- [x] 01-01-PLAN.md — ClickHouse + clickhouse-backup compose services, native Prometheus self-metrics, backup/restore runbook with dry-run (Wave 1, AR-STORE-01)
- [x] 01-02-PLAN.md — ClickHouse `EventStore` impl: wide-event MergeTree schema, native batch insert, append-only identity + argMax view, GDPR erase, backend-agnostic contract suite (Wave 1, AR-STORE-02/03/05)
- [x] 01-03-PLAN.md — Dual-write wiring + backend selector, clickstream migration, `aenigma-clickhouse` datasource + 6 rewritten panels, live end-to-end smoke (Wave 2, AR-STORE-04)
**Metrics**: `UXΔ = +1 (Better)` (internal observability foundation; user benefit is indirect — reliability/perf insight) · `CDI = 0.12 * 34` (introduces a new stateful service + new schema, but the `EventStore` swap seam already exists; significant phase-of-work effort) · `MVQ = Phoenix 80%/88%` (transformative foundation — the event plane is reborn on a purpose-built columnar store; strong slop-resistance built on the existing swap interface)

### Phase 2: BE Egress Recorder
**Goal**: Every third-party request made by the backend posts one dimensioned egress effect row, with HLS aggregated per stream-session so the register never drowns in per-segment noise.
**Depends on**: Phase 1 (the ClickHouse `EventStore` is the sink for the recorded effects)
**Requirements**: AR-EGRESS-01, AR-EGRESS-02, AR-EGRESS-03, AR-EGRESS-04, AR-EGRESS-05
**Success Criteria** (what must be TRUE):
  1. A request through the `libs/tracing` `WrapTransport` outbound seam produces exactly one egress effect row carrying provider, host, status, bytes, and duration — verified by an integration test that issues an outbound call and reads back the row. (AR-EGRESS-01)
  2. `origin`, `operation`, and `user_id` set by the inbound middleware ride OTel **baggage** all the way to the recorder and appear on the emitted row — verified end-to-end (inbound request → outbound effect carries the same baggage values). (AR-EGRESS-02)
  3. The previously-uninstrumented clients now route through the wrapped transport and emit effect rows: Kodik extractor, scraper `BaseHTTPClient`, OpenSubtitles, and idmapping (ARM/AniList) — each verified by triggering it and observing its egress row. (AR-EGRESS-03)
  4. An HLS stream session produces **one effect row per (stream-session, host)** with summed `bytes` and a segment count — never one row per ~6s segment; verified by playing/replaying a session and asserting a single aggregated row per host. (AR-EGRESS-04)
  5. Where the proxy reads upstream, both `bytes_out` (client egress) and `bytes_in` (upstream ingress) are populated on the row — verified by a proxied stream showing non-zero values in both measures. (AR-EGRESS-05)
**Plans**: 4 plans (3 waves)
- [x] 02-01-PLAN.md — Effect fields on domain.Event + /internal/effects ingestion + async drop-on-full producer + baggage seed/read helpers + recording RoundTripper core (Wave 1, AR-EGRESS-01/02)
- [x] 02-02-PLAN.md — Retrofit the 4 uninstrumented clients (Kodik/OpenSubtitles/idmapping via catalog injection; scraper BaseHTTPClient + stream-provider tag) (Wave 2, AR-EGRESS-03)
- [x] 02-03-PLAN.md — HLS per-(session,host) aggregation via ?sess= token + idle reaper + dual byte counters in the proxy (Wave 2, AR-EGRESS-04/05)
- [x] 02-04-PLAN.md — Baggage-PII strip + e2e baggage proof + wire middleware/producer into catalog/scraper/streaming + redeploy + live ClickHouse verification (Wave 3, AR-EGRESS-01..05, non-autonomous)
**Metrics**: `UXΔ = +1 (Better)` (observability of external dependencies; indirect user benefit via faster incident triage) · `CDI = 0.18 * 21` (touches `libs/tracing`, the HLS proxy, and four retrofit clients across catalog/scraper/streaming; mostly extends existing seams, the retrofit is the real spread) · `MVQ = Kraken 86%/82%` (many tentacles reaching into every service's egress path; high match, strong slop-resistance — built on the single shared transport seam)

### Phase 3: DB/Cache Effects + Auto Operation Discovery
**Goal**: Backend write-side and cache effects are recorded and automatically attributed to a business operation with no hand-maintained catalog, so any effect row can be pivoted by what code path caused it.
**Depends on**: Phase 2 (the recorder + baggage + `operation` attribution path established at the egress seam is reused for DB/cache effects)
**Requirements**: AR-EFFECT-01, AR-EFFECT-02, AR-EFFECT-03, AR-EFFECT-04
**Success Criteria** (what must be TRUE):
  1. DB writes are recorded as effect rows carrying `table`, `op`, and `rows` via otel-GORM; trivial reads are NOT fact-rowed (they remain spans + sampling) — verified by a write producing a row while a high-volume read produces none. (AR-EFFECT-01)
  2. Cache effects record hit/miss by key-class as effect rows — verified by exercising a cached read twice (miss then hit) and seeing both classified rows. (AR-EFFECT-02)
  3. `operation` is auto-derived with **no manual catalog**, via service-layer stack-frame attribution (nearest `*/internal/service/*` frame) — verified by an effect row showing a real operation like `catalog.UpdateAnimeInfo` with no code that names it explicitly. (AR-EFFECT-03)
  4. The Tempo span-metrics generator + service graph are enabled (config flag, no per-span code), producing per-operation RED metrics + a service graph in Prometheus — verified by querying a per-operation request/error/duration metric and viewing the service graph in Grafana. (AR-EFFECT-04)
**Plans**: 6 plans (4 waves)
- [x] 03-01-PLAN.md — Effect wire-contract extension (TargetKind/Rows/anime_id) + runtime.Callers operation resolver + egress stack-frame retrofit (Wave 1, AR-EFFECT-03)
- [x] 03-02-PLAN.md — Tempo metrics_generator (span-metrics + service-graphs) + Prometheus remote-write + Grafana Tempo→Prometheus wiring (Wave 1, AR-EFFECT-04)
- [x] 03-03-PLAN.md — GORM DB-effect callbacks (db_write always, db_read P95-gated) + in-memory ReadGate snapshot (Wave 2, AR-EFFECT-01)
- [x] 03-04-PLAN.md — Cache aggregator (HLSSessions clone) + key-class classifier + cache.go hit/miss hooks (Wave 2, AR-EFFECT-02)
- [x] 03-05-PLAN.md — Daily ClickHouse P95 → read_thresholds Redis hash + scheduler cron + ThresholdRefresher ticker (Wave 3, AR-EFFECT-01)
- [ ] 03-06-PLAN.md — Per-service boot wiring (7 GORM + catalog cache; gateway N/A — rate-limit cache bypasses libs/cache) + live ClickHouse/Prometheus/Grafana phase-gate verification (Wave 4, AR-EFFECT-01..04, non-autonomous)
**Metrics**: `UXΔ = +1 (Better)` (operation-level cost attribution; the "expensive popular button" insight derives from here) · `CDI = 0.14 * 21` (GORM hook + cache hook + a `runtime.Callers` attribution helper + a Tempo config flag; new-but-compatible patterns across the DB/cache libs, attribution is the novel piece) · `MVQ = Griffin 85%/84%` (elegantly fuses GORM hooks, cache hooks, stack-walking, and Tempo span-metrics into one attribution story; strong slop-resistance)

### Phase 4: FE Causation + RUM
**Goal**: A frontend action and its resulting backend effects share one `trace_id`, and browser→third-party resource timings are beaconed as clearly-flagged approximate rows that never contaminate authoritative backend bytes.
**Depends on**: Phase 1 (the ClickHouse-backed analytics collector is the sink); integrates with the Phase 2/3 dimensions so FE rows join to BE effects on `trace_id`/`operation`
**Requirements**: AR-FE-01, AR-FE-02, AR-FE-03
**Success Criteria** (what must be TRUE):
  1. The axios interceptor sends the active `trace_id` to the analytics collector and stamps each call with the current route + optional semantic action — verified by an FE call appearing in the register joined (same `trace_id`) to its backend effects. (AR-FE-01)
  2. Click auto-capture events carry `trace_id` so a click joins to the backend traces/effects it triggers — verified by a captured click event and its downstream BE effect sharing the same `trace_id`. (AR-FE-02)
  3. A `PerformanceObserver` beacons browser→3rd-party resource timings (host, count, timing) flagged `source=fe_rum, accuracy=approx`; a dashboard/query proves these rows are **never summed** with authoritative BE bytes (e.g. byte aggregations filter `source=be`). (AR-FE-03)
**Plans**: TBD
**Metrics**: `UXΔ = +1 (Better)` (closes the FE→BE causation last mile; RUM gives real client-side perf signal) · `CDI = 0.08 * 13` (frontend axios interceptor + click-capture + a `PerformanceObserver`; extends the existing `traceparent` minting, contained to the FE analytics layer) · `MVQ = Griffin 80%/85%` (joins frontend causation to the backend register into one trace-linked form; the `accuracy=approx` discipline is the crafted, slop-resistant detail)
**UI hint**: yes

### Phase 5: Reports & Dashboards
**Goal**: An admin can answer "what is the platform doing now/today, by operation and by external dependency, with anomalies surfaced" through pivotable Grafana reports built on the populated register.
**Depends on**: Phases 2, 3, and 4 (needs egress, DB/cache, and FE effect data populated before the reports have anything meaningful to render)
**Requirements**: AR-REPORT-01, AR-REPORT-02, AR-REPORT-03, AR-REPORT-04
**Success Criteria** (what must be TRUE):
  1. A wide-event pivot dashboard with template variables lets an admin group/filter by **any** dimension (origin, operation, provider, host, effect_kind, …) — verified by switching a template var and watching the pivot regroup live. (AR-REPORT-01)
  2. The "from → choke-point → effects" report renders the target shape: origin → operation → per-target requests + bytes — verified by opening it and reading a real origin's effect breakdown. (AR-REPORT-02)
  3. Volume anomalies are flagged: a provider/operation whose request count is far above its baseline is visibly surfaced (panel/alert) — verified by injecting a synthetic volume spike and seeing it flagged. (AR-REPORT-03)
  4. An "awareness overview" answers "what is the platform doing now/today" at a glance — verified by a single dashboard view showing current top operations + top external dependencies + active anomalies. (AR-REPORT-04)
**Plans**: TBD
**Metrics**: `UXΔ = +2 (Better)` (this is where awareness becomes usable — the admin-facing payoff of the whole register) · `CDI = 0.06 * 21` (Grafana dashboards + template vars + anomaly rules; new dashboards on an existing Grafana, no code-structure disruption, but a large authoring effort to get the pivots right) · `MVQ = Dragon 88%/86%` (the showy high-impact centerpiece — the register made human-readable; high match, strong slop-resistance if the pivots are genuinely insightful rather than panel-spam)
**UI hint**: yes

### Phase 6: Consolidation → Topology A
**Goal**: ClickHouse becomes the single event/trace/log plane — Tempo and Loki are retired and their views repointed — while Prometheus + Grafana stay as the metrics + alerting + rendering layer.
**Depends on**: Phase 1 (the ClickHouse plane) and Phase 5 (the register proven end-to-end before retiring the existing trace/log SPOFs). Deliberately the **last** phase — register must be proven before the consolidation.
**Requirements**: AR-CONS-01, AR-CONS-02, AR-CONS-03
**Success Criteria** (what must be TRUE):
  1. The OTel Collector exports traces to ClickHouse and **Tempo is retired** — its datasource and container are removed and the backend-tracing dashboard is repointed to ClickHouse and still renders. (AR-CONS-01)
  2. Logs are shipped to ClickHouse and **Loki is retired** — its datasource and container are removed and the log views are repointed to ClickHouse and still render. (AR-CONS-02)
  3. Prometheus + Grafana remain unchanged as the metrics + alerting + rendering layer — verified by existing alerts still firing and existing metric dashboards still rendering after the consolidation. (AR-CONS-03)
**Plans**: TBD
**Metrics**: `UXΔ = +1 (Better)` (lower ops surface + unified querying; user benefit indirect via reliability) · `CDI = 0.10 * 21` (retires two stateful services and repoints their datasources/dashboards; new-but-compatible topology, gated/reversible until cutover — the risk is concentration onto the ClickHouse SPOF) · `MVQ = Phoenix 82%/85%` (rises-from-ashes consolidation — three stores collapse into one; the deliberate last-phase sequencing is the slop-resistant craft)

## Backlog / Reserved Future

Deferred out of v4.0 (tracked in `.planning/REQUIREMENTS.md` § v2 / Deferred):
- **AR-V2-01**: `AggregatingMergeTree` pre-aggregated rollups (1С-style accumulation registers) beyond what the dashboards need.
- **AR-V2-02**: Pyroscope continuous profiling (cost-by-function) integration — touched as an optional spike in Phase 5's design notes; promoted to its own deferred item.

After v4.0 ships, run `/gsd-new-milestone` to start the next cycle. Prior-milestone reserved ideas still on the shelf (unnumbered until committed):
- VibePlayer Recovery via WARP egress (revive VibePlayer by routing scraper egress through Cloudflare WARP; separate spec when there is appetite).
- MinIO Hot Archival (rip popular HLS streams to MinIO; serve from there to decouple from upstream availability; separate spec).

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-8 | v1.0 | 18/18 | ✅ Complete | 2026-04-27 → 2026-05-03 |
| 9-14 | v2.0 | 8/8 | ✅ Complete | 2026-05-06 → 2026-05-07 |
| 15-20 | v3.0 | — | ✅ Complete | 2026-05-11 → 2026-05-18 |
| 21-28 | v3.1 | — | ✅ Complete | 2026-05-13 → 2026-06-04 |
| 1 | v4.0 | 3/3 | Complete   | 2026-06-05 |
| 2 | v4.0 | 4/4 | Complete   | 2026-06-05 |
| 3 | v4.0 | 5/6 | In Progress|  |
| 4 | v4.0 | 0/? | Not started | — |
| 5 | v4.0 | 0/? | Not started | — |
| 6 | v4.0 | 0/? | Not started | — |
