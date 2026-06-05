# Phase 3: DB/Cache Effects + Auto Operation Discovery - Context

**Gathered:** 2026-06-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Backend **write-side DB effects** and **cache hit/miss effects** become wide-event rows in the Phase-1 ClickHouse store (reusing the Phase-2 async `Producer`/`EffectSink` + the `Effect` struct), and **every** effect — egress, db, cache — is automatically attributed to a business `operation` by walking `runtime.Callers` to the nearest `*/internal/service/*` frame, with **no hand-maintained catalog**. Tempo's span-metrics generator + service graph are switched on (config flag, no per-span code) for per-operation RED metrics + a service graph in Prometheus.

Satisfies **AR-EFFECT-01..04** (see `.planning/REQUIREMENTS.md`).

**Explicitly NOT in this phase** (later): FE causation + RUM (Phase 4), Grafana reports / pivot dashboards / anomaly flagging (Phase 5), Tempo/Loki→ClickHouse consolidation (Phase 6). This phase wires the **BE write-side + cache half** of the register and the **auto operation discovery** the egress half (Phase 2) deferred.
</domain>

<decisions>
## Implementation Decisions

### DB read/write policy (AR-EFFECT-01)
- **D-01:** **DB writes are always fact-rowed.** INSERT/UPDATE/DELETE → one `db_write` effect row carrying `table`, `op`, and `rows` (rows-affected, from GORM `RowsAffected`).
- **D-02:** **DB reads are gated by a dynamic per-query P95 threshold.** A SELECT is fact-rowed (`db_read`) only when its execution exceeds **that specific query's own daily-recomputed P95** for its `(operation, table)` key. A **static fallback threshold** applies at cold-start (before a day-2 baseline exists). The long tail of trivial reads is **never** fact-rowed — it stays Tempo spans + sampling. This keeps AR-EFFECT-01's success criterion intact ("a high-volume read produces none").
- **D-03 (new sub-component):** The dynamic threshold requires a **daily ClickHouse → threshold-table feedback loop**: a daily job computes per-`(operation, table)` read-latency P95 from the register, writes a compact threshold table, and services load/consult it in the GORM read hook. Planner decides where the daily job runs (likely `services/scheduler`), how the threshold table is distributed to services (e.g. a small table services poll, or pushed via config/Redis), the static cold-start defaults, and the percentile (P95 default; P90 acceptable).
- **D-04 (rationale, locked):** "Most popular operation" is **NOT** answered by logging all reads — it is answered for free by **span-metrics (AR-EFFECT-04, D-13)**, which covers 100% of traffic with zero fact rows. DB fact rows are the **cost ledger** (expensive/interesting effects), deliberately sparse. Logging all reads was explicitly considered and rejected: redundant with span-metrics for popularity, adds a `runtime.Callers` stack-walk on every SELECT, and would contradict the locked AR-EFFECT-01 wording.

### Cache effects (AR-EFFECT-02)
- **D-05:** **Aggregated per key-class, not one row per op.** Keep in-process counters keyed by `(key_class, result[hit|miss], operation)`; a flush goroutine emits **summed** `cache` effect rows (`requests = N`) on a ~10s interval (planner may tune). Mirrors the Phase-2 HLS reaper aggregation pattern. Drastically lower volume than one-row-per-op while preserving full hit-rate fidelity per class.
- **D-06:** **Aggregated cache rows carry no per-`trace_id`** (they span many traces by construction) — accepted, since cache is rarely the headline effect in a causation story (egress + db_write are). They still carry `operation` (see D-09 caveat) + `key_class` + result.
- **D-07:** **key-class = prefix + sub-namespace.** Derive ~20-30 stable classes from the key structure with variable IDs stripped — e.g. `anime:list`, `anime:similar`, `anime:related`, `anime:detail`, `anime:top`, `user:profile`, `search`, `progress`, `video:manifest`, `extid`, … Distinguishes a hot list cache from a cold similar-anime cache (very different hit rates). Derive the scheme from the key builders in `libs/cache/ttl.go`.

### Operation attribution (AR-EFFECT-03)
- **D-08:** **Unified stack-frame attribution for ALL effects** (egress + db + cache). The nearest `*/internal/service/*` frame from `runtime.Callers` is the **primary** `operation` (e.g. `catalog.UpdateAnimeInfo`, `spotlight.AnimeOfDayResolver.Resolve`). This **retrofits Phase-2 egress**, which currently uses the coarse baggage label as primary — egress switches to stack-frame-primary so every effect in one trace shares the same fine `operation` (clean uniform pivots).
- **D-09:** **Fallback chain, never empty:** nearest service frame → **baggage** endpoint label (`tracing.ReadBaggage`, the coarse `service METHOD /route` from Phase 2) → **origin** name (`goroutine(name)` / `scheduled_job(name)`). Guarantees a non-empty, meaningful `operation` for cron/background/bypass effects.
- **D-10 (aggregate caveat):** Timer-flushed aggregates have **no live stack at flush time** (the HLS reaper and the cache flush goroutine). They MUST capture `operation` at **record-time** (when the segment/cache op happens), or fall back to baggage. For the **Phase-2 HLS reaper**, this is a small retrofit: capture the session's `operation` at session-start, not at flush. For cache, capturing a per-op stack-walk to key the counter reintroduces per-op walk cost — planner's discretion whether aggregated cache uses fine stack-frame op (walk per op) or the coarser baggage op (cheaper); coarser-for-cache is acceptable.
- **D-11 (perf, locked):** The stack-walk runs **only on the async recording path, never the request hot loop** (design spec risk note). Capture PCs synchronously (cheap) and resolve frame→function names on the async side where feasible.

### Span-metrics + service graph (AR-EFFECT-04)
- **D-12:** Enable Tempo's **metrics_generator** (span-metrics + service-graphs processors) with `remote_write` to Prometheus — **config only, no per-span code**. `infra/tempo/tempo.yaml` has **no** metrics_generator block today; it must be added. Produces per-operation RED metrics + a service graph queryable in Grafana.
- **D-13 (rationale):** This is the free, total-coverage "popularity / RED" layer that complements the sparse fact-row cost ledger (see D-04).
- **D-14 (cross-phase hand-off, noted):** The span-metrics generator lives in **Tempo, which Phase 6 retires**. At consolidation the per-operation RED metrics must be regenerated by the **OTel Collector's spanmetrics connector**. Enable in Tempo now per the requirement; carry this hand-off into Phase 6's plan.

### Instrumentation breadth
- **D-15:** **All data services.** Wire DB effect recording across all 7 services that already have `gormtrace.InstrumentGORM` (catalog, player, themes, auth, notifications, scheduler, library); wire cache effects across the cache-using services. Comprehensive register from day one — the marginal per-service cost is just wiring (DB span instrumentation is already in place).
- **D-16 (correctness, locked):** **EXCLUDE `services/analytics`** from effect recording. It is the EffectStore sink — recording its own ingestion DB writes as effects would be self-referential noise/amplification. (Note: analytics uses GORM but is deliberately NOT in the gormtrace-7.)
- **D-17:** Redis-only edge services (gateway / rooms / watch-together — no GORM) get cache effects only **if the signal is useful** (e.g. gateway rate-limit cache); watch-together uses Redis as a datastore, not a cache, so it may add little. Planner decides per-service.

### Claude's Discretion
- Flush interval for cache aggregation (~10s default), exact key-class taxonomy, the static cold-start read-threshold defaults + percentile (P95 default), where the daily-P95 job runs and how the threshold table is distributed, db_read vs db_write detection in the GORM callback set, sync-PC-capture vs async-resolve split for the stack-walk, whether aggregated cache uses fine vs coarse `operation` (D-10), and the per-service Redis-only decisions (D-17) — all planner/researcher, consistent with the locked decisions above.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone design + requirements
- `docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md` — the v4.0 design. Phase-3-relevant sections: **§"Data model — wide events"** (dimensions incl. `effect_kind` = `db_write`/`db_read`/`cache`, `target` = table+op / key_class, `rows` measure), **§"Auto operation discovery (no predetermined catalog)"** (the 4 combined mechanisms: otelhttp endpoint span, Tempo span-metrics generator, `runtime.Callers` stack-frame attribution, optional Pyroscope), **§"Risks"** (write-volume blow-ups → aggregation discipline; stack-walk only on async path).
- `.planning/REQUIREMENTS.md` §AR-EFFECT-01..04 — the four locked requirements this phase satisfies.
- `.planning/ROADMAP.md` → "Phase 3: DB/Cache Effects + Auto Operation Discovery" — goal, the 4 success criteria, dependency on Phase 2.

### Phase 1 carry-forward (the sink + schema this phase writes to)
- `.planning/phases/01-clickhouse-foundation-eventstore-swap/01-CONTEXT.md` — locked wide-event schema: dimensions + measures (incl. the `rows` measure used by `db_write` rows).
- `.planning/phases/01-clickhouse-foundation-eventstore-swap/01-02-SUMMARY.md` — the `EventStore` impl (`InsertBatch`, the unchanged `domain.EventStore` interface).

### Phase 2 carry-forward (the recorder seam this phase extends + retrofits)
- `.planning/phases/02-be-egress-recorder/02-CONTEXT.md` — the egress recorder, baggage model (`origin`/`operation` on W3C baggage; `user_id`/`provider` on PRIVATE ctx values), HLS per-(session,host) aggregation (the reaper retrofitted by D-10), and D-07 (Phase 2: `operation` from baggage — superseded for primary by D-08 here).
- `.planning/phases/02-be-egress-recorder/02-04-SUMMARY.md` — final wiring of middleware/producer into catalog/scraper/streaming + live ClickHouse verification (read for the established wiring pattern).

### Code seams (integration points)
- `libs/tracing/effect.go` — the `Effect` struct + `EffectSink` interface. Extend with the `rows` measure + `db_write`/`db_read`/`cache` effect kinds + db `target` (table+op) / cache `target` (key_class).
- `libs/tracing/producer.go` — async batched drop-on-full `Producer`; **`wireProducerEffect` has NO `rows` field** — must be added (and threaded to analytics `/internal/effects` + the ClickHouse `events` columns).
- `libs/tracing/baggage.go` — `ReadBaggage` (coarse operation fallback), `WithUserID`/`UserIDFromContext`, `WithProvider`/`ProviderFromContext`. The stack-frame resolver (D-08) is new code that sits in front of these.
- `libs/tracing/gormtrace/gorm.go` — `InstrumentGORM` (otel-GORM spans, `WithoutMetrics()`). Already called in 7 services. The DB effect hook (GORM callback registering on Create/Update/Delete + Query) lives here or alongside.
- `libs/cache/cache.go` — `RedisCache` Get/Set/etc.; already meters hit/miss via `metrics.CacheOperationsTotal`. The cache effect aggregator (D-05) hooks here.
- `libs/cache/ttl.go` — key prefixes + key builders; the source-of-truth for the D-07 key-class taxonomy.
- `infra/tempo/tempo.yaml` — add the `metrics_generator` block (D-12); `docker/grafana/provisioning/datasources/datasources.yml` + `infra/otel/collector-config.yaml` for the wiring.
- `services/analytics/internal/repo/clickhouse_store.go` + the `/internal/effects` handler — the `rows` measure plumbing endpoint (D-16 exclusion applies to analytics' OWN db writes).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Phase-2 recorder stack** (`Effect` + `EffectSink` + `Producer`, async/batched/drop-on-full) — the sink for db/cache effects; reuse, don't rebuild. Just extend the `Effect` shape + wire contract for `rows` + new effect kinds.
- **`gormtrace` + otel-GORM** — already a dependency (`gorm.io/plugin/opentelemetry/tracing`) and already registered in 7 services for spans. The DB effect hook extends this seam (GORM callbacks expose `Statement.Table`, the op, and `RowsAffected`).
- **`libs/cache` hit/miss metering** — `metrics.CacheOperationsTotal{op,result}` already classifies hit/miss/error; the aggregator counts the same events into per-key-class effect rows.
- **Phase-2 HLS reaper** — the existing per-(session,host) aggregation + flush pattern is the template for the cache flush aggregator (D-05) AND the thing retrofitted for record-time operation capture (D-10).

### Established Patterns
- **Async + batched + drop-on-full** ingestion (Phase 1/2) — the db/cache recorder MUST follow it; never block/fail a request hot path.
- **Baggage vs private-ctx split** — `origin`/`operation` on W3C baggage; `user_id`/`provider` on private non-propagated ctx (security: never leak `user_id` to 3rd parties). New stack-frame `operation` resolution must respect this and remain the primary, with baggage as fallback (D-08/D-09).
- **Stack-walk discipline** — `runtime.Callers` only on the async recording path (design risk note); capture PCs sync, resolve async where feasible (D-11).

### Integration Points
- GORM callback (Create/Update/Delete/Query) → build db effect → `EffectSink.Record` → Phase-2 `Producer` → analytics `/internal/effects` → Phase-1 ClickHouse `EventStore`.
- Cache Get/Set → increment per-(key_class, result, operation) counter → ~10s flush → summed `cache` effect rows → same Producer → sink.
- `runtime.Callers` at record-time → nearest `*/internal/service/*` frame → `operation` for the effect being built (egress recorder also switches to this primary path — D-08).
- Tempo `metrics_generator` (span-metrics + service-graphs) → `remote_write` → Prometheus → Grafana RED metrics + service graph (D-12).

</code_context>

<specifics>
## Specific Ideas

- User's reframing of the read-policy: *"Is it really bad to log all reads, so we can see what are the most popular and why?"* → resolved by clarifying that **popularity comes free from span-metrics (AR-EFFECT-04)**; fact rows are the cost ledger (D-04).
- User's compromise on the read threshold: *"Is it possible to make a dynamic guard... only log some of P90-P95 requests and recount that guard daily?"* → adopted as the **dynamic per-query daily-recomputed P95** policy (D-02/D-03), chosen over a static threshold and over logging all reads.

</specifics>

<deferred>
## Deferred Ideas

- **OTel Collector spanmetrics connector** to regenerate per-operation RED metrics after Tempo is retired → **Phase 6** (D-14 cross-phase hand-off).
- **Pyroscope continuous profiling** (cost-by-function, design §"Auto operation discovery" point 4) → already deferred to v2 (AR-V2-02); not in Phase 3.
- **AggregatingMergeTree / SummingMergeTree pre-aggregated rollups** for the db/cache effect dashboards → v2 (AR-V2-01); Phase 5 builds dashboards on per-effect fidelity first.
- **Logging all DB reads** for ad-hoc per-query drill-down → explicitly considered and rejected (D-04); revisit only if a future report needs per-read durability that span-metrics + sampled traces can't provide.

None of the above expand Phase 3 scope — discussion stayed within the DB/cache-effects + operation-discovery boundary.

</deferred>

---

*Phase: 03-db-cache-effects-auto-operation-discovery*
*Context gathered: 2026-06-05*
