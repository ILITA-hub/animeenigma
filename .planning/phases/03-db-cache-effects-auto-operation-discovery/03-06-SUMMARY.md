---
phase: 03-db-cache-effects-auto-operation-discovery
plan: 06
subsystem: per-service boot wiring (DB + cache effect plane)
tags: [observability, boot-wiring, db-effects, cache-effects, p95-read-gate, D-15, D-16, D-17, AR-EFFECT-01, AR-EFFECT-02]
requires:
  - "Plan 03-01 — extended Effect contract + SetGlobalSink global sink"
  - "Plan 03-03 — RegisterEffectCallbacks + NewReadGate"
  - "Plan 03-04 — cache.CacheAggregator + RedisCache.WithAggregator"
  - "Plan 03-05 — gormtrace.ThresholdRefresher + HashReader + analytics recompute + scheduler trigger"
provides:
  - "tracing.GlobalSink() getter — gorm/cache hooks reach the process sink without threading the Producer"
  - "cache.RedisCache.HGetAll — RedisCache satisfies gormtrace.HashReader directly (no per-service adapter)"
  - "gormtrace.WireDBEffects(ctx, db, sink, reader) — one-call boot wiring (ReadGate + RegisterEffectCallbacks + ThresholdRefresher.Start) shared by all 7 GORM services"
  - "All 7 GORM services (catalog/player/themes/auth/notifications/scheduler/library) record db_write/db_read effects + run the daily-P95 refresher"
  - "catalog records cache hit/miss effects (cache aggregator wired, flush-before-drain)"
affects:
  - "Task 3 (live phase-gate verification) — PENDING, deferred to the orchestrator on main (live redeploy + ClickHouse/Prometheus/Grafana verification)"
tech-stack:
  added: []
  patterns:
    - "Shared boot helper (gormtrace.WireDBEffects) keeps the per-service edit a single uniform call — DRYs the gate/refresher boilerplate across 7 main.go files"
    - "RedisCache implements gormtrace.HashReader via a thin HGetAll delegate — no go-redis import leaks into libs/tracing, no per-service adapter type"
    - "Producer boot block (NewProducer/Start/SetGlobalSink) added to the 6 non-catalog GORM services so the db hook has a live sink (catalog already had it from Phase 2)"
    - "LIFO defer ordering: cacheAggregator.Stop() registered after effectProducer.Stop() so the aggregator flushes BEFORE the producer drains (mirror streaming HLSSessions)"
key-files:
  created:
    - libs/tracing/gormtrace/boot.go
  modified:
    - libs/tracing/client.go
    - libs/cache/cache.go
    - services/catalog/cmd/catalog-api/main.go
    - services/player/cmd/player-api/main.go
    - services/themes/cmd/themes-api/main.go
    - services/themes/internal/config/config.go
    - services/auth/cmd/auth-api/main.go
    - services/notifications/cmd/notifications-api/main.go
    - services/scheduler/cmd/scheduler-api/main.go
    - services/library/cmd/library-api/main.go
    - services/library/internal/config/config.go
decisions:
  - "Factored the per-service gate/refresher boilerplate into a shared gormtrace.WireDBEffects helper rather than copy-pasting RegisterEffectCallbacks + NewReadGate + NewThresholdRefresher.Start into 7 main.go files. Cleaner + single review surface. Consequence: the literal-grep acceptance criteria ('RegisterEffectCallbacks in main.go' / 'ThresholdRefresher in main.go') now match WireDBEffects in main.go (7×) with the underlying calls in boot.go — the must_haves truth (all 7 register callbacks + ReadGate + refresher) holds."
  - "RedisCache gained an HGetAll(ctx,key)(map[string]string,error) method so a *RedisCache IS a gormtrace.HashReader — no per-service adapter struct needed. Each GORM service passes its existing/new redisCache as the HashReader directly."
  - "The 6 non-catalog GORM services each gained the Phase-2 Producer boot block (NewProducer/Start/SetGlobalSink) because only catalog/scraper/streaming created a Producer in Phase 2 — the db hook needs a live sink. themes/library also gained a Redis cache.Config field + client; notifications constructed a client from its pre-existing cfg.Redis."
  - "library uses its rootCtx (SIGTERM-cancelled) for the refresher; the other services use a dedicated dbEffectsCtx cancelled via defer."
metrics:
  duration: ~1 work session
  completed: 2026-06-05
  tasks: 2 of 3 (Task 3 is a human-verify checkpoint — PENDING)
  files: 11
---

# Phase 3 Plan 06: Per-Service DB + Cache Effect Boot Wiring Summary

Wired the activity-register DB-effect plane into the boot of all 7 GORM services (catalog, player, themes, auth, notifications, scheduler, library) and the cache-effect plane into catalog. Each GORM service now registers the db_write (always) + db_read (P95-gated) GORM after-callbacks and runs the daily-P95 `ReadGate` refresher off the query hot path; catalog additionally records cache hit/miss effects via the aggregator with flush-before-drain shutdown. `analytics` is left untouched (D-16). The wiring is a single uniform `gormtrace.WireDBEffects(...)` call per service, backed by a new `tracing.GlobalSink()` getter and a `RedisCache.HGetAll` HashReader delegate. `UXΔ = +1 (Better)` · `CDI = 0.07 * 13` · `MVQ = Kraken 86%/84%`.

## What Was Built

**Task 1 (`2cbae12a`) — GlobalSink getter + DB-effect callbacks + ReadGate + ThresholdRefresher across 7 GORM services:**
- `tracing.GlobalSink()` getter added to `libs/tracing/client.go` (loads the atomic globalSink pointer, returns nil when unset).
- `cache.RedisCache.HGetAll(ctx, key) (map[string]string, error)` added to `libs/cache/cache.go` — delegates to `c.client.HGetAll(...).Result()`, making `*RedisCache` satisfy `gormtrace.HashReader` directly (no per-service adapter, no go-redis leak into libs/tracing).
- `gormtrace.WireDBEffects(ctx, db, sink, reader)` (new `libs/tracing/gormtrace/boot.go`) — builds `NewReadGate(50)` (A1 cold-start), calls `RegisterEffectCallbacks`, starts `NewThresholdRefresher(reader, gate, 5m)`, returns a `stop` func. Nil-sink / nil-reader degrade to no-op so a sink-less boot never crashes. Doc comment carries the D-16 "never analytics" constraint.
- Boot wiring per service:
  - **catalog** — already had Producer + SetGlobalSink (Phase 2); added the WireDBEffects call reusing `redisCache` as HashReader.
  - **player, auth, scheduler** — reuse their existing `redisCache`; added the Producer boot block + WireDBEffects.
  - **notifications** — had `cfg.Redis` but no client; constructed `cache.New(cfg.Redis)` + Producer block + WireDBEffects.
  - **themes, library** — had NO `cfg.Redis` field; added a `Redis cache.Config` field (REDIS_HOST/PORT/PASSWORD/DB env block) to `internal/config/config.go`, constructed the client + Producer block + WireDBEffects (library uses `rootCtx`).

**Task 2 (`93ac3279`) — cache aggregator into catalog with flush-before-drain:**
- In `services/catalog/cmd/catalog-api/main.go`: `cache.NewCacheAggregator(tracing.GlobalSink(), 0, 0)` (default 10s flush / 10k cap), attached via `redisCache.WithAggregator(...)`, `Start()` + `defer Stop()`. The aggregator `Stop()` defer is registered AFTER `effectProducer.Stop()`, so LIFO ordering flushes the aggregator BEFORE the producer drains (T-03-19 mitigation, mirrors streaming's HLSSessions).
- gateway is N/A (D-17): its only Redis use is the per-user rate limiter on a raw `redis.Client` + redis_rate/v10 — no `libs/cache.RedisCache` seam to hook. rooms + watch-together skipped (Redis-as-datastore). All confirmed by grep returning nothing.

## Deviations from Plan

**[Refactor — DRY] Factored the per-service gate/refresher boilerplate into `gormtrace.WireDBEffects`.**
- **Found during:** Task 1, while wiring the 2nd of 7 identical blocks.
- **Issue:** Copy-pasting `NewReadGate` + `RegisterEffectCallbacks` + `NewThresholdRefresher(...).Start()` + adapter into 7 main.go files is 7× duplication and a drift hazard.
- **Fix:** Added a single `WireDBEffects(ctx, db, sink, reader) (stop, err)` helper in a new `boot.go`; each service calls it once. The underlying `RegisterEffectCallbacks` / `NewThresholdRefresher` calls live in `boot.go`.
- **Impact on acceptance criteria:** The plan's literal-grep criteria expected `RegisterEffectCallbacks` and `ThresholdRefresher` tokens *in each main.go*. They now appear as `WireDBEffects` in main.go (7×) with the actual calls in `boot.go`. The `must_haves` truth — "All 7 GORM services register the DB-effect callbacks + a ReadGate + a ThresholdRefresher after InstrumentGORM" — is fully satisfied (every service wires all three via the one call). No behavior change.
- **Files:** `libs/tracing/gormtrace/boot.go` (new), all 7 service main.go.
- **Commit:** `2cbae12a`

**[Rule 2 — completeness] Added `RedisCache.HGetAll` instead of 7 per-service adapter structs.**
- Plan-05's `HashReader` needs `HGetAll(ctx,key)(map[string]string,error)`. Rather than write a redis-client adapter in each of 7 main.go files, added one `HGetAll` method to `libs/cache.RedisCache` so the existing `*RedisCache` IS a `HashReader`. Scoped, reused everywhere. Commit `2cbae12a`.

## Task 3 — PENDING (human-verify checkpoint)

**Task 3 (`checkpoint:human-verify`, gate="blocking") is NOT executed by this worktree executor.** It requires redeploying LIVE PRODUCTION services (`make redeploy-catalog player themes auth notifications scheduler library`, `make restart-tempo prometheus grafana`, `make health`) and verifying the four AR-EFFECT criteria end-to-end against the running ClickHouse / Prometheus / Grafana stack. That is the correct division of labor: this executor produces only the code-writing commits in an isolated worktree; the orchestrator merges this worktree to main, redeploys, and runs the live verification with the user.

**Status:** PENDING — human-verify checkpoint, deferred to the orchestrator on main tree (live redeploy + verification of: db_write rows with non-zero row_count, miss-then-hit cache rows, a fine `catalog.*` operation, per-op `traces_spanmetrics_*` in Prometheus, and the Grafana service graph; plus `tracing_effects_dropped_total` not climbing pathologically).

## Verification

- `libs/tracing` + `libs/cache` build + vet clean; `libs/tracing/gormtrace`, `libs/cache`, `libs/tracing` tests green (`go test -count=1`).
- All 7 GORM services build (`go build ./...` each prints OK) → `ALL_GORM_SERVICES_BUILD`.
- Source assertions: `WireDBEffects` in 7 main.go (= all GORM services register callbacks + ReadGate + refresher); `func GlobalSink` present (1); themes+library now have a `Redis cache.Config` field (2); notifications/themes/library each construct `cache.New(cfg.Redis)` (3); `SetGlobalSink` present in all 7 GORM service mains (7).
- D-16: `grep -rn "RegisterEffectCallbacks\|WireDBEffects" services/analytics/` returns NOTHING (analytics untouched).
- Task 2: catalog builds (`CACHE_WIRED_BUILDS`); `CacheAggregator` referenced in catalog main.go (1); `cacheAggregator.Stop()` deferred AFTER `effectProducer.Stop()` (flush-before-drain); D-17 N/A — `grep -rn "CacheAggregator" services/gateway services/rooms services/watch-together` returns NOTHING.

## Threat Flags

None new. T-03-18 (analytics self-amplification) mitigated — D-16 verified by grep, analytics has zero callback/WireDBEffects wiring. T-03-19 (aggregator not flushed before drain) mitigated — LIFO defer ordering flushes the aggregator before the producer drains. T-03-20 (user_id leak via new Producer) mitigated — the Phase-2 stripWireBaggagePII guard + no-user_id-in-baggage path apply to every Producer-bearing service; no new baggage member seeded. T-03-SC (package installs) — ZERO new packages; all wiring uses existing in-repo modules.

## Known Stubs

None. The `read_thresholds` hash each refresher reads is empty until the first daily analytics recompute (plan-05) populates it — the gate serves the static 50ms cold-start default meanwhile, which is correct cold-start behavior, not a stub.

## Self-Check: PASSED

- FOUND: libs/tracing/client.go
- FOUND: libs/cache/cache.go
- FOUND: libs/tracing/gormtrace/boot.go
- FOUND: services/catalog/cmd/catalog-api/main.go
- FOUND: services/player/cmd/player-api/main.go
- FOUND: services/themes/cmd/themes-api/main.go
- FOUND: services/themes/internal/config/config.go
- FOUND: services/auth/cmd/auth-api/main.go
- FOUND: services/notifications/cmd/notifications-api/main.go
- FOUND: services/scheduler/cmd/scheduler-api/main.go
- FOUND: services/library/cmd/library-api/main.go
- FOUND: services/library/internal/config/config.go
- FOUND commit: 2cbae12a (Task 1)
- FOUND commit: 93ac3279 (Task 2)
