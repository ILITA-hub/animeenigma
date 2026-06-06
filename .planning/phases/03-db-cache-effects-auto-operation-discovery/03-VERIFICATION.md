---
phase: 03-db-cache-effects-auto-operation-discovery
verified: 2026-06-06T00:00:00Z
status: passed
score: 4/4 must-haves verified
overrides_applied: 0
re_verification: false
---

# Phase 3: DB/Cache Effects + Auto Operation Discovery — Verification Report

**Phase Goal:** Backend write-side and cache effects are recorded and automatically attributed to a business operation with no hand-maintained catalog, so any effect row can be pivoted by what code path caused it.
**Verified:** 2026-06-06
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | DB writes are recorded as effect rows carrying `table`, `op`, and `rows` via otel-GORM; trivial reads are NOT fact-rowed — verified by a write producing a row while a high-volume read produces none. (AR-EFFECT-01) | VERIFIED | `libs/tracing/gormtrace/gorm_effect.go` registers Create/Update/Delete callbacks that always emit `db_write` effects; Query callback only emits `db_read` when `gate.ShouldRecord()` is true. Live confirmation in 03-06-SUMMARY: `db_write` rows with real table names + non-zero `row_count`; `db_read` = 0 rows / 15 min (P95 gate suppressing fast reads). |
| 2 | Cache effects record hit/miss by key-class as effect rows — verified by exercising a cached read twice (miss then hit) and seeing both classified rows. (AR-EFFECT-02) | VERIFIED | `libs/cache/aggregator.go` + `libs/cache/keyclass.go` implement `CacheAggregator` with `Observe(ctx, keyClass, result)` and `KeyClass(raw)` classifier. `WithAggregator` wired in `libs/cache/cache.go` at all 7 Get/Set metering sites. `EffectKind:"cache"` + `TargetKind:"key_class"` confirmed in source. Live confirmation in 03-06-SUMMARY: ClickHouse `cache` rows miss→hit→success on `search` key_class + `genres` hit. |
| 3 | `operation` is auto-derived with no manual catalog, via service-layer stack-frame attribution — verified by an effect row showing a real operation like `catalog.UpdateAnimeInfo` with no code that names it explicitly. (AR-EFFECT-03) | VERIFIED | `libs/tracing/attribution.go` implements `CaptureOperationPCs` (sync PC capture, 32-frame depth) + `Operation.Resolve()` (async symbol walk to first `/internal/service/` frame, normalized to `pkg.Func`). Fallback chain: service frame → baggage op → `goroutine(origin)`. Wired in `libs/tracing/client.go` for egress; `libs/tracing/gormtrace/gorm_effect.go` uses `WithOperationPCs`; `post()` in `producer.go` calls `resolvedOperation()` async. Live confirmation in 03-06-SUMMARY: fine operations `signals.S3Trending.Precompute`, `signals.S5Attribute.persistVector` visible in ClickHouse. |
| 4 | The Tempo span-metrics generator + service graph are enabled, producing per-operation RED metrics + a service graph in Prometheus — verified by querying a per-operation request/error/duration metric and viewing the service graph in Grafana. (AR-EFFECT-04) | VERIFIED | `infra/tempo/tempo.yaml` has `metrics_generator:` block with `span-metrics` + `service-graphs` processors and `remote_write` to `http://prometheus:9090/prometheus/api/v1/write`. `docker/docker-compose.yml` has `--web.enable-remote-write-receiver`. Grafana datasource provisioning has `serviceMap` + `tracesToMetrics` both bound to Prometheus uid `PBFA97CFB590B2093`. Live confirmation in 03-06-SUMMARY: Prometheus `traces_spanmetrics_calls_total` = 15 per-op series; `traces_service_graph_request_total` = 8 edges. |

**Score:** 4/4 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `libs/tracing/attribution.go` | Operation resolver: `CaptureOperationPCs` + `Resolve()` | VERIFIED | 164 lines; contains `/internal/service/` anchor, `runtime.Callers` (5 occurrences — called once in `CaptureOperationPCs`; remaining are in walk comments/strings), `runtime.CallersFrames` in `Resolve`; never-empty fallback chain implemented |
| `libs/tracing/attribution_test.go` | Table-driven tests for resolver + fallback chain + no user_id in baggage | VERIFIED | 6308 bytes; `TestOperationResolve` present per 03-01-SUMMARY |
| `libs/tracing/effect.go` | Effect struct with `TargetKind`, `Rows`, `AnimeID`, `op Operation` carrier | VERIFIED | 63 lines; `TargetKind`, `Rows`, `AnimeID`, `op Operation` fields confirmed; `WithOperationPCs` + `resolvedOperation` helpers present |
| `libs/tracing/producer.go` | `wireProducerEffect` with `row_count` + `anime_id`; `post()` honors `e.TargetKind` | VERIFIED | `row_count` (1 match), `anime_id` (1 match), `e.TargetKind` (1 match) all confirmed |
| `libs/tracing/client.go` | Egress `RoundTrip` uses `CaptureOperationPCs` as primary; `GlobalSink()` getter | VERIFIED | `CaptureOperationPCs` (1 match), `func GlobalSink` (1 match) confirmed |
| `libs/tracing/gormtrace/readgate.go` | `ReadGate` interface + `snapshotGate` atomic snapshot + `SetSnapshot` | VERIFIED | 3296 bytes; `ShouldRecord` interface method confirmed; pure in-memory (no Redis/HTTP imports) per source assertions |
| `libs/tracing/gormtrace/gorm_effect.go` | `RegisterEffectCallbacks` with `db_write` always + `db_read` P95-gated | VERIFIED | `EffectKind:"db_write"` (3 matches), `EffectKind:"db_read"` (2 matches); no DB queries in callbacks confirmed |
| `libs/tracing/gormtrace/threshold_refresh.go` | `ThresholdRefresher` ticker: `HGetAll read_thresholds` → `gate.SetSnapshot` | VERIFIED | 5075 bytes; `SetSnapshot` called; `HashReader` interface (no go-redis import into libs/tracing) |
| `libs/tracing/gormtrace/boot.go` | `WireDBEffects` shared boot helper containing `RegisterEffectCallbacks` + `NewReadGate` + `NewThresholdRefresher` | VERIFIED | 2264 bytes; all three calls confirmed (1 each) |
| `libs/cache/keyclass.go` | `KeyClass(raw string) string` bounded ~20-class classifier | VERIFIED | 2593 bytes; fixed literal switch classifier present |
| `libs/cache/aggregator.go` | `CacheAggregator` with `EffectKind:"cache"` + `TargetKind:"key_class"` + `Stop` drain | VERIFIED | 6999 bytes; `EffectKind:"cache"` (1), `key_class` (7 matches) confirmed |
| `libs/cache/cache.go` | `WithAggregator` hook at all 7 Get/Set metering sites | VERIFIED | `WithAggregator` (3 definitions/references); 7 nil-guarded `Observe` sites confirmed per source assertions |
| `services/analytics/internal/service/read_threshold.go` | `ComputeReadThresholds` with `quantile(0.95)` + `effect_kind='db_read'` | VERIFIED | 5715 bytes; `quantile(0.95)` (1), `effect_kind.*db_read` (2 matches) confirmed |
| `services/analytics/internal/handler/read_threshold.go` | `POST /internal/read-thresholds/recompute` handler | VERIFIED | 1587 bytes; registered in `router.go` at `/internal/read-thresholds/recompute` |
| `services/scheduler/internal/jobs/read_threshold.go` | Daily cron job posting analytics recompute | VERIFIED | 2694 bytes; `read_threshold_recompute` metric label in `job.go`; registered in `JobService.Start` |
| `infra/tempo/tempo.yaml` | `metrics_generator:` block with span-metrics + service-graphs + remote_write | VERIFIED | `metrics_generator` (2 matches), `span-metrics` (1 match) confirmed |
| `docker/docker-compose.yml` | `--web.enable-remote-write-receiver` on Prometheus container | VERIFIED | 1 match confirmed |
| `docker/grafana/provisioning/datasources/datasources.yml` | `serviceMap` + `tracesToMetrics` bound to Prometheus uid | VERIFIED | 2 matches each; `PBFA97CFB590B2093` uid present |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `libs/tracing/client.go RoundTrip` | `CaptureOperationPCs` / `Operation.Resolve` | stack-frame-primary operation (D-08) | WIRED | `CaptureOperationPCs` called at record time; resolve deferred to `post()` async side |
| `libs/tracing/producer.go post()` | `e.TargetKind` / `e.Rows` / `e.AnimeID` | field mapping from Effect | WIRED | `e.TargetKind` honored; `row_count: e.Rows` mapped; `anime_id: e.AnimeID` mapped |
| `libs/tracing/gormtrace/gorm_effect.go` | `EffectSink.Record` (db_write always) | GORM After-callback for Create/Update/Delete | WIRED | Callbacks registered via `RegisterEffectCallbacks`; `db_write` emitted unconditionally |
| `libs/tracing/gormtrace/gorm_effect.go` | `ReadGate.ShouldRecord` | P95-gated `db_read` via GORM Query After-callback | WIRED | `gate.ShouldRecord(op, table, durMS)` guards `db_read` emission |
| `libs/cache/cache.go Get/Set` | `CacheAggregator.Observe` | nil-guarded `c.agg.Observe(ctx, KeyClass(key), result)` | WIRED | 7 Observe call sites confirmed in `cache.go` |
| `libs/tracing/gormtrace/boot.go WireDBEffects` | All 7 GORM services (catalog/player/themes/auth/notifications/scheduler/library) | Single boot call in each `main.go` | WIRED | `grep -c 'WireDBEffects'` returns 1 in each of the 7 service `main.go` files |
| `services/catalog/cmd/catalog-api/main.go` | `cache.CacheAggregator` + `redisCache.WithAggregator` | flush-before-drain LIFO defer | WIRED | `CacheAggregator` (1) + `WithAggregator` (1) confirmed; `Stop()` registered after `effectProducer.Stop()` |
| `services/analytics` → `POST /internal/read-thresholds/recompute` | `ReadThresholdService.Recompute` + Redis `read_thresholds` hash | router.go + service.go + repo.go | WIRED | Endpoint registered; `quantile(0.95)` ClickHouse query present; `read_thresholds` key confirmed |
| `services/scheduler/internal/jobs/read_threshold.go` | analytics `/internal/read-thresholds/recompute` | Daily cron HTTP POST | WIRED | `read_threshold_recompute` label registered in `job.go` with cron schedule; nil-job guard present |
| `libs/tracing/gormtrace/ThresholdRefresher` | `ReadGate.SetSnapshot` | ticker `HGETALL read_thresholds` → parse → `SetSnapshot` | WIRED | `SetSnapshot` called in `refreshOnce`; `HashReader` (HGetAll) interface avoids go-redis import leak |
| `libs/cache/cache.go RedisCache` | `gormtrace.HashReader` | `HGetAll(ctx, key)` delegate method | WIRED | `HGetAll` (3 matches) confirmed; `*RedisCache` satisfies `HashReader` directly |
| D-16 guard: analytics NOT wired | analytics has zero `WireDBEffects`/`RegisterEffectCallbacks` calls | No self-amplification | VERIFIED | `grep -rn 'RegisterEffectCallbacks|WireDBEffects' services/analytics/` returns no output |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `gormtrace/gorm_effect.go` | `db.Statement.RowsAffected`, `db.Statement.Table` | GORM After-callback (supplied by GORM runtime) | Yes — real GORM callback data | FLOWING |
| `libs/cache/aggregator.go` | `(keyClass, result, operation)` counters | `Observe()` called by `cache.go` Get/Set hooks with real request context | Yes — real cache hit/miss data | FLOWING |
| `analytics/service/read_threshold.go` | `quantile(0.95)(duration_ms)` | ClickHouse `events` table WHERE `effect_kind='db_read'` GROUP BY `(operation, target)` | Yes — real ClickHouse query; not a static return | FLOWING |
| `libs/tracing/attribution.go` | `operation` string | `runtime.CallersFrames` over live PC snapshot | Yes — real stack-frame symbols | FLOWING |

---

### Behavioral Spot-Checks

Step 7b: SKIPPED — all phase code is library/service wiring without standalone runnable entry points; live verification was performed at the orchestrator level (human-verify checkpoint in plan 06 approved 2026-06-06). See Human Verification Results below.

---

### Probe Execution

No conventional `scripts/*/tests/probe-*.sh` probes declared or found for this phase. The phase-gate checkpoint was the live production verification in 03-06 Task 3 (human-verify), not a shell probe.

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| AR-EFFECT-01 | 03-01, 03-03, 03-05, 03-06 | DB writes recorded as effect rows carrying table/op/rows; trivial reads NOT fact-rowed | SATISFIED | GORM callbacks + P95 ReadGate + ThresholdRefresher + boot wiring all present; live `db_write` rows confirmed |
| AR-EFFECT-02 | 03-04, 03-06 | Cache effects record hit/miss by key-class as effect rows | SATISFIED | `CacheAggregator` + `KeyClass` + `WithAggregator` wired; live cache rows confirmed |
| AR-EFFECT-03 | 03-01 | `operation` auto-derived via service-layer stack-frame, no manual catalog | SATISFIED | `attribution.go` `CaptureOperationPCs` + `Resolve()` wired to all effect kinds; live fine operations confirmed |
| AR-EFFECT-04 | 03-02 | Tempo span-metrics + service graph in Prometheus; per-operation RED metrics | SATISFIED | Tempo `metrics_generator` config + Prometheus remote-write-receiver + Grafana wiring present; live `traces_spanmetrics_calls_total` (15 series) + `traces_service_graph_request_total` (8 edges) confirmed |

---

### Anti-Patterns Found

No TBD, FIXME, or XXX markers found in any phase-modified file.

`return nil` occurrences in `gorm_effect.go` and `read_threshold.go` are proper error-return paths and nil-guard no-ops, not stubs — confirmed by reading the surrounding context.

No placeholder text, `return {}`, or hardcoded empty data found in paths that flow to rendered output.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | — | — | — |

---

### Human Verification Results (Completed 2026-06-06)

The blocking human-verify checkpoint (03-06 Task 3) was executed live against the running production stack and approved. Recorded evidence from 03-06-SUMMARY.md:

1. **AR-EFFECT-01 (db_write):** ClickHouse `db_write` rows with fine stack-frame operations (`signals.S3Trending.Precompute`, `signals.S5Attribute.persistVector`, `signals.S1ScoreCluster.persistVector`), real tables (`rec_population_signals`, `rec_user_signals`), non-zero `row_count`.
2. **AR-EFFECT-01 (sparse ledger):** `db_read` = 0 rows / 15 min — P95 gate suppressing trivial fast reads (D-01/D-02).
3. **AR-EFFECT-02 (cache):** ClickHouse `cache` rows miss→hit→success on `search` key_class + `genres` hit (`target_kind=key_class`).
4. **AR-EFFECT-04 (Tempo/Prometheus):** `traces_spanmetrics_calls_total` = 15 per-operation series; `traces_service_graph_request_total` = 8 edges (gateway→catalog, catalog→postgresql, scheduler→postgresql, and others); 13 distinct `traces_*` metric families.

No outstanding human verification items remain.

---

### Runbook Note (Operational)

Prometheus span-metrics required container **recreate** (`docker compose up -d --no-deps prometheus`), not a plain restart — `--web.enable-remote-write-receiver` is a docker-compose command-line flag that `docker compose restart` does not re-apply. Documented in 03-06-SUMMARY.md for future operators.

---

### Gaps Summary

None. All 4 success criteria (AR-EFFECT-01 through AR-EFFECT-04) are VERIFIED at code level (artifacts exist, are substantive, are wired, and data flows through them) and confirmed by live production evidence from the approved human-verify checkpoint.

The D-16 constraint (analytics must NOT self-instrument) is verified clean by grep: zero `WireDBEffects`/`RegisterEffectCallbacks` calls in `services/analytics/`.

No deferred items affecting phase goal achievement. The only deferred item is a pre-existing `services/catalog` `GOWORK=off` build gap (missing go.sum entry for `klauspost/compress`) documented in `deferred-items.md` — not caused by this phase and not blocking phase goal.

---

_Verified: 2026-06-06_
_Verifier: Claude (gsd-verifier)_
