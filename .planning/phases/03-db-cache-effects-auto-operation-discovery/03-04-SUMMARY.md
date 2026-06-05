---
phase: 03-db-cache-effects-auto-operation-discovery
plan: 04
subsystem: libs/cache (activity-register cache-effect plane)
tags: [observability, cache, hit-miss, aggregation, key-class, D-05, D-06, D-07, D-10, AR-EFFECT-02]
requires:
  - "Plan 03-01: extended Effect contract (TargetKind=key_class, Operation, Requests) + EffectSink"
  - "libs/tracing/baggage.go ReadBaggage (coarse op ctx read, D-10)"
  - "services/streaming/internal/service/hls_sessions.go (the byte-for-byte clone template)"
provides:
  - "cache.KeyClass(raw string) string — bounded ~20-class classifier over the ttl.go prefix+sub-namespace taxonomy (D-07)"
  - "cache.CacheAggregator — (key_class,result,operation) counter, ~10s flush, bounded oldest-eviction map, graceful Stop drain (D-05/D-06/D-10)"
  - "RedisCache.WithAggregator(*CacheAggregator) — optional nil-guarded hook wired at all Get/Set hit/miss/error/success metering sites"
affects:
  - "Plan 03-06 (boot wiring): each cache-using service instantiates a CacheAggregator + RedisCache.WithAggregator at main.go"
tech-stack:
  added:
    - "libs/cache now depends on libs/tracing (+ transitive libs/authz) via local replace directives"
  patterns:
    - "RESEARCH Pattern 3: in-process counter aggregator cloned from HLSSessions reaper (lock-only-on-map, oldest-eviction, injectable clock, graceful flushAll-on-Stop)"
    - "D-10/A5: coarse baggage operation read OFF-lock at Observe-time, never a runtime.Callers stack-walk on the cache hot path (D-11)"
    - "Optional nil-guarded sink field (mirrors HLSSessions sink==nil) so cache-less paths + tests need no aggregator"
key-files:
  created:
    - libs/cache/keyclass.go
    - libs/cache/keyclass_test.go
    - libs/cache/aggregator.go
    - libs/cache/aggregator_test.go
  modified:
    - libs/cache/cache.go
    - libs/cache/go.mod
    - libs/cache/go.sum
decisions:
  - "result (hit/miss/error/success) is folded into Effect.Operation as `<op> [<result>]` because the plan-01 Effect wire contract has no dedicated result field; key_class rides Target with TargetKind=\"key_class\". The aggregator counterKey keeps (keyClass,result,operation) distinct so summation is per-result."
  - "Cache aggregator flushes ALL counters every ~10s tick (no per-counter idle window like the HLS reaper) — cache outcomes re-accumulate cheaply and a fixed summed cadence is the D-05 design."
  - "go work sync propagated workspace-wide transitive version bumps (otel 1.38→1.39, golang.org/x/*); reverted all non-libs/cache go.mod/go.sum to keep the change scoped — libs/cache builds standalone with GOWORK=off."
metrics:
  duration: ~1 work session
  completed: 2026-06-05
  tasks: 2
  files: 7
---

# Phase 3 Plan 04: Cache Hit/Miss Aggregator + Key-Class Classifier Summary

Added an in-process cache hit/miss aggregator (a deliberate clone of the Phase-2 `HLSSessions` reaper) that counts `(key_class, result, operation)` and flushes summed `cache` effect rows on a ~10s interval, plus a bounded key-class classifier derived from the `libs/cache/ttl.go` taxonomy. Hooked into `RedisCache.Get`/`Set` as an OPTIONAL nil-guarded field so cache-less paths and existing tests need no aggregator. Delivers AR-EFFECT-02 (cache effects recorded as hit/miss by key-class) with full per-class hit-rate fidelity at low volume.

## What Was Built

**Task 1 (TDD) — `keyclass.go` + tests:**
- `KeyClass(raw string) string` — splits on `:`, keeps prefix + (where the builders define one) the first sub-namespace token, drops everything from the first variable ID onward. Maps the documented `ttl.go` builder shapes to stable classes: `anime:<id>`→`anime:detail`, `anime:list:*`→`anime:list`, `anime:top:*`→`anime:top`, `anime:related:*`→`anime:related`, `anime:similar:*`→`anime:similar` (related/similar deliberately KEPT distinct — D-07), `user:profile:*`→`user:profile`, `video:manifest:*`→`video:manifest`, plus prefix-level classes (`search`, `progress`, `extid`, `episode`, `session`, `genre`, `studio`, `ratelimit`, `room`, `tgauth`). Any unknown prefix → a single `"other"` bucket.
- The known-class set is a fixed literal `switch`, not dynamic key passthrough — the class set is bounded (~20 classes) because it keys the aggregator map (T-03-10 DoS guard).
- Tests: a 22-case table covering all behaviors + a `TestKeyClassBounded` asserting 1000 distinct anime ids collapse to exactly 1 class.

**Task 2 (TDD) — `aggregator.go` + `cache.go` hooks + tests:**
- `CacheAggregator` clones `HLSSessions`: `sink`, `flushInterval` (default 10s), `maxEntries` (default 10000), `mu sync.Mutex`, `counters map[counterKey]*tally`, injectable `now func()`, `stop`/`doneWG`/`once`. `counterKey{keyClass, result, operation}`; `tally{requests uint32, firstSeen, lastSeen}`.
- `Observe(ctx, keyClass, result)` reads the coarse op via `tracing.ReadBaggage(ctx)` ONCE, OUTSIDE the lock (D-10/A5/T-03-12 — never a stack-walk), then takes the lock ONLY for the map increment (T-03-11/Pitfall 3). New keys trigger `evictIfFullLocked` (verbatim oldest-eviction clone). Op falls back to origin then `"unknown"` so a row never carries an empty operation.
- `recordLocked` emits `Effect{Origin:"api", Operation:"<op> [<result>]", EffectKind:"cache", Target:keyClass, TargetKind:"key_class", Requests:N}` — NO user_id, NO trace_id (D-06).
- `Start()` (ticker on flushInterval), `Stop()` (`once.Do(close)` + `doneWG.Wait()` + `flushAll`), `flushAll()`, and the injectable-clock `flush` split for deterministic tests.
- `cache.go`: added optional `agg *CacheAggregator` to `RedisCache` + a fluent `WithAggregator`; nil-guarded `c.agg.Observe(ctx, KeyClass(key), <result>)` at all 7 metering sites (Get miss/error/error/hit, Set error/error/success). `New()` unchanged — aggregator is never mandatory.
- Tests (deterministic clock + fake sink): miss-then-hit → 2 classified rows; N repeats sum to Requests=N; coarse-op never empty; bounded map never exceeds cap; Stop() flushes + is safe twice; nil-sink Observe is a no-op.

## Deviations from Plan

**[Rule 3 - Blocking] Reverted workspace-wide `go work sync` churn.**
- **Found during:** Task 2, after `go mod tidy`/`go work sync` wired the new libs/cache→libs/tracing edge.
- **Issue:** `go work sync` propagated unrelated transitive version bumps (otel 1.38→1.39, golang.org/x/{net,sys,sync,text,crypto}) into ~22 other modules' go.mod/go.sum — out of scope and a parallel-worktree merge-churn hazard.
- **Fix:** `git checkout --` reverted every non-`libs/cache` go.mod/go.sum; added explicit local `replace` directives (`../tracing`, `../authz`) to `libs/cache/go.mod` so the module builds standalone. Verified `GOWORK=off go build ./...` for libs/cache is green and the in-workspace catalog consumer build is green.
- **Files modified (kept):** `libs/cache/go.mod`, `libs/cache/go.sum` only.
- **Commit:** 14a949a7

## TDD Gate Compliance

- Task 1 RED: `8800a73a` `test(03-04): add failing tests for KeyClass classifier` (compile-fail on undefined `KeyClass` — verified failing).
- Task 1 GREEN: `b89e9b88` `feat(03-04): implement KeyClass key-class classifier`.
- Task 2 RED: `c67abd91` `test(03-04): add failing tests for CacheAggregator` (compile-fail on undefined `NewCacheAggregator` — verified failing).
- Task 2 GREEN: `14a949a7` `feat(03-04): clone HLS reaper into cache hit/miss aggregator + hook cache.go`.
- REFACTOR: none needed (code clean on first green for both tasks).

## Verification

- `cd libs/cache && go test -race -count=1 ./...` — all green (KeyClass + aggregator + existing cache_setnx suites).
- `go build ./...` + `go vet ./...` clean in-workspace AND with `GOWORK=off` (Docker-like per-module build).
- Catalog consumer (`services/catalog`) builds green in-workspace with the new cache→tracing/authz edge.
- Source assertions: `grep -c 'EffectKind: *"cache"' aggregator.go` = 1; `TargetKind` = 2 (set to `key_class`); `Observe` signature takes no user_id (D-06); 7 nil-guarded `if c.agg != nil` Observe sites in cache.go; the known-class set is a literal switch (bounded). The mutex is NOT held across `ReadBaggage` (read before `a.mu.Lock()`).

## Threat Flags

None. No new network endpoints, auth paths, or schema changes at trust boundaries. Cache rows carry only operation + key_class + result (no user_id/trace_id, D-06). The counter map is bounded with oldest-eviction (T-03-10); the lock never wraps IO (T-03-11); the op is the coarse baggage read, not a stack-walk (T-03-12). No new packages — only an in-repo module dependency edge (T-03-SC: zero installs).

## Known Stubs

None. The aggregator is ready to instantiate per cache-using service; boot wiring (`RedisCache.WithAggregator`) is the intentional plan-06 extension point, not a stub.

## Deferred Issues

Logged to `deferred-items.md`: a pre-existing `services/catalog` `GOWORK=off` build gap (missing go.sum entry for `klauspost/compress`) surfaced while validating the dependency edge. NOT caused by 03-04 (catalog go.mod reverted; the cache edge adds nothing to catalog's closure) and resolved by the Docker `go mod download` step. Out of scope.

## Self-Check: PASSED

- FOUND: libs/cache/keyclass.go
- FOUND: libs/cache/keyclass_test.go
- FOUND: libs/cache/aggregator.go
- FOUND: libs/cache/aggregator_test.go
- FOUND: libs/cache/cache.go (modified)
- FOUND commit: 8800a73a (test/RED Task 1)
- FOUND commit: b89e9b88 (feat/GREEN Task 1)
- FOUND commit: c67abd91 (test/RED Task 2)
- FOUND commit: 14a949a7 (feat/GREEN Task 2)
