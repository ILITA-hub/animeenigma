# Phase 3: DB/Cache Effects + Auto Operation Discovery - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-05
**Phase:** 3-db-cache-effects-auto-operation-discovery
**Areas discussed:** DB read policy, Cache effect volume, Operation attribution, Instrumentation breadth

---

## DB read policy (AR-EFFECT-01)

Initial framing — how to treat SELECTs:

| Option | Description | Selected |
|--------|-------------|----------|
| Writes-only, reads never rowed | Writes rowed; all reads stay spans+sampling | |
| Writes + flagged expensive reads | Writes + reads over a duration/rows threshold | (evolved) |

User questioned the value ("is it really bad to log all reads, so we can see most popular and why?") and proposed a dynamic guard ("only log some of P90-P95 requests, recount daily"). Clarified that **popularity is served free by span-metrics (AR-EFFECT-04)**, so fact rows are the sparse cost ledger. Re-posed:

| Option | Description | Selected |
|--------|-------------|----------|
| Writes + dynamic-P95 reads | Reads rowed only above that query's own daily-recomputed P95 (per op+table), static cold-start fallback | ✓ |
| Writes + static-threshold reads | Reads rowed above a fixed env-tunable cutoff | |
| Log ALL reads as rows | Every read durable; revise AR-EFFECT-01 | |

Also resolved earlier sub-question (volume guard for common+expensive): subsumed by the dynamic-P95 approach.

**User's choice:** Writes + dynamic-P95 reads.
**Notes:** Adopts the user's adaptive-thresholding idea; adds a daily ClickHouse→threshold-table feedback loop (D-03). Keeps AR-EFFECT-01 success criterion intact. "Log all reads" explicitly rejected as redundant with span-metrics + per-SELECT stack-walk cost.

---

## Cache effect volume (AR-EFFECT-02)

**Emission strategy:**

| Option | Description | Selected |
|--------|-------------|----------|
| Aggregated per key-class | Counters by key_class×hit/miss×operation, ~10s flush, summed requests; no per-trace_id | ✓ |
| One row per cache op | Per-op rows carrying trace_id; higher volume | |
| You decide | Pick during planning | |

**Key-class granularity:**

| Option | Description | Selected |
|--------|-------------|----------|
| Prefix + sub-namespace | ~20-30 classes (anime:list, anime:similar, user:profile, …) | ✓ |
| Top-level prefix only | ~12 classes (anime:, search:, user:, …) | |
| You decide | Derive during planning | |

**User's choice:** Aggregated per key-class; prefix + sub-namespace.
**Notes:** Aggregated rows lose per-trace_id (accepted — cache is rarely the headline causation effect). Mirrors the Phase-2 HLS reaper aggregation pattern.

---

## Operation attribution (AR-EFFECT-03)

**Scope/precedence:**

| Option | Description | Selected |
|--------|-------------|----------|
| Unified — all effects | Stack-frame primary for egress+db+cache; baggage = fallback | ✓ |
| DB/cache only | Egress keeps coarse baggage label untouched | |
| You decide | Pick during planning | |

**Fallback when no service frame:**

| Option | Description | Selected |
|--------|-------------|----------|
| Baggage → origin → never empty | service-frame → baggage endpoint → origin name | ✓ |
| Single 'unknown' literal | One fallback string | |
| You decide | Design during planning | |

**User's choice:** Unified — all effects; Baggage → origin → never empty.
**Notes:** Retrofits Phase-2 egress from baggage-primary to stack-frame-primary. Timer-flushed aggregates (HLS reaper, cache flush) must capture operation at record-time (D-10).

---

## Instrumentation breadth

| Option | Description | Selected |
|--------|-------------|----------|
| All data services | DB across all 7 gormtrace services + cache-users; exclude analytics | ✓ |
| Hot-path subset for v1 | catalog + player (+ themes) only, expand later | |
| You decide | Pick service set during planning | |

**User's choice:** All data services.
**Notes:** analytics excluded as self-referential sink (baked in regardless). Redis-only edge services (gateway/rooms/watch-together) decided per-signal during planning.

---

## Claude's Discretion

- Cache flush interval (~10s default), exact key-class taxonomy.
- Static cold-start read-threshold defaults + percentile (P95 default, P90 acceptable); where the daily-P95 job runs and threshold-table distribution.
- db_read vs db_write GORM callback detection; sync-PC-capture vs async-resolve split for the stack-walk.
- Whether aggregated cache rows use fine stack-frame op (walk per op) vs coarser baggage op.
- Redis-only edge-service cache instrumentation decisions.

## Deferred Ideas

- OTel Collector spanmetrics connector to regenerate RED metrics post-Tempo-retirement → Phase 6 (cross-phase hand-off D-14).
- Pyroscope continuous profiling → v2 (AR-V2-02).
- AggregatingMergeTree/SummingMergeTree pre-agg rollups → v2 (AR-V2-01).
- Logging all DB reads → considered and rejected (D-04).
