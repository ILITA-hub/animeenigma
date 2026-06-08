# Phase 1: ClickHouse Foundation + EventStore Swap — Context

**Gathered:** 2026-06-04
**Status:** Ready for planning
**Source:** Design-spec express path (`docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md`)

<domain>
## Phase Boundary

**In scope:** Stand up ClickHouse as a docker-compose service; define the wide-event schema (one row per effect); implement a ClickHouse `EventStore` behind the *existing* interface; migrate the analytics clickstream onto ClickHouse; make ingestion async + batched + drop-on-full with a dropped-event metric; document backup/restore + healthcheck + Prometheus scrape.

**Explicitly NOT in this phase** (later phases): the BE egress recorder (P2), DB/cache effects + auto operation discovery (P3), FE causation + RUM (P4), Grafana reports (P5), and the Tempo/Loki → ClickHouse consolidation (P6). This phase only delivers the *store* and proves it by carrying the existing clickstream.
</domain>

<decisions>
## Implementation Decisions (locked from spec)

### Store
- ClickHouse is the columnar event store. The existing `EventStore` interface (`services/analytics/internal/domain/store.go`: `InsertBatch` + `UpsertIdentity`) is the swap seam — implement a ClickHouse impl alongside the Postgres one; do NOT change the interface.
- Wide-event table = **one row per effect**. Dimensions: `origin`, `operation`, `effect_kind`, `target_kind`, `target`, `trace_id`, `session_id`, nullable `user_id`/`anime_id`, `source`, `accuracy`. Measures: `requests`, `bytes_in`, `bytes_out`, `duration_ms`, `rows`. (This table must also accommodate the existing clickstream event shape — reconcile the clickstream columns with the wide-event columns; clickstream pageview/click events are a subset / `effect_kind`-flavored rows or a sibling table — planner to decide, but a single unified wide-event table is preferred.)
- Engine: `MergeTree` family with an `ORDER BY` tuned for the common pivots (time + operation + target). `AggregatingMergeTree` pre-aggregation rollups are **deferred to v2** (AR-V2-01) — not this phase.

### Ingestion
- Reuse the existing analytics ingest API + batcher (`services/analytics/internal/ingest/batcher.go`, `postgres_store.go` pattern). Async + batched + drop-on-full; no synchronous write on any request hot path.
- Expose a `*_dropped_total` metric for dropped events at `/metrics`.

### Migration
- The clickstream must keep ingesting and the `product-analytics` dashboards must keep rendering after the swap. Migration strategy (dual-write vs cutover vs backfill) is **Claude's discretion** — research it; prefer the lowest-risk reversible path on a single host.

### Ops
- Single self-hosted host. Document a backup/restore procedure and dry-run a restore at least once. Healthcheck in compose. Prometheus scrape (or ClickHouse self-metrics) entry.

### Claude's Discretion
- ClickHouse version + image, exact column types/codecs, partition key + TTL, the Grafana ClickHouse datasource plugin choice, and whether the clickstream + wide-events share one table or two.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Design
- `docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md` — full milestone design; "Data model" + "Architecture" + Phase 1 of "Phase breakdown" are authoritative for this phase.

### Analytics service (the swap target)
- `services/analytics/internal/domain/store.go` — the `EventStore` interface (swap seam).
- `services/analytics/internal/repo/models.go` — current `analytics_events` schema (clickstream dims + measures + `trace_id`).
- `services/analytics/internal/repo/postgres_store.go` — current Postgres impl + batch insert pattern.
- `services/analytics/internal/ingest/batcher.go` — async batcher.
- `services/analytics/cmd/analytics-api/main.go` — bootstrap, retention purge, port 8092.

### Infra
- `docker/docker-compose.yml` — service definitions (grafana, prometheus, postgres) to mirror for ClickHouse.
- `docker/grafana/provisioning/datasources/datasources.yml` — add a ClickHouse datasource here.
- `infra/grafana/dashboards/product-analytics.json` — must still render post-swap.
- `libs/database/` — connection-management conventions.
- Memory: adding a new `libs/` module touches go.work + all service Dockerfiles (see project memory) — likely N/A here unless a shared CH client lib is created.
</canonical_refs>

<specifics>
## Specific Ideas
- The wide-event row is the 1С "register movement"; the clickstream is just one `origin`/`effect_kind` flavor of it. A single unified table is the cleanest end state but must not break the existing `analytics_events_resolved` view + identity stitching (`analytics_identities`).
- Keep the `EventStore` contract test suite backend-agnostic so it runs against BOTH Postgres and ClickHouse (AR-STORE-03).
</specifics>

<deferred>
## Deferred Ideas
- Egress recorder, DB/cache effects, FE RUM, Grafana pivot reports, Tempo/Loki consolidation — Phases 2–6.
- AggregatingMergeTree rollups (AR-V2-01); Pyroscope (AR-V2-02).
</deferred>

---

*Phase: 01-clickhouse-foundation-eventstore-swap*
*Context gathered: 2026-06-04 via design-spec express path*
