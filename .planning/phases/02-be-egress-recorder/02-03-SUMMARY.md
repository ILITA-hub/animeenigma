---
phase: 02-be-egress-recorder
plan: 03
subsystem: streaming
tags: [egress, hls, aggregation, session, byte-counting, go, streaming, observability]

# Dependency graph
requires:
  - phase: 02-be-egress-recorder
    plan: 01
    provides: "libs/tracing.Effect + EffectSink + Producer (drop-on-full async sink to /internal/effects) + SetGlobalSink"
provides:
  - "libs/videoutils.newSessToken — crypto/rand per-manifest correlation id (non-authoritative)"
  - "rewriteHLSURL injects &sess=<token> on rewritten segment/child URLs (one token per manifest)"
  - "libs/videoutils.countReader — atomic bytes_in counter wrapping resp.Body (no buffering)"
  - "VideoProxy.ProxyStreamCounted / ProxyWithRefererCounted — report per-call bytes_in/bytes_out (ProxyStream/ProxyWithReferer delegate, signatures unchanged)"
  - "service.HLSSessions — in-memory per-(sess,host) tally + idle reaper + bounded map + graceful flush; emits ONE tracing.Effect per watch session"
  - "streaming main wires tracing.Producer (ANALYTICS_INTERNAL_URL) + SetGlobalSink + HLSSessions"
affects: [02-04, grafana-pivot-reports, be-egress-wiring]

# Tech tracking
tech-stack:
  added:
    - "crypto/rand + encoding/hex (stdlib — sess token)"
    - "sync/atomic (stdlib — countReader)"
  patterns:
    - "Per-manifest ?sess= correlation token minted once in rewriteM3U8URLs, threaded to every rewriteHLSURL/rewriteURIAttribute call so all segments of one watch share it"
    - "countReader wraps resp.Body BEFORE io.Copy/rateLimitedCopy to count bytes_in without io.ReadAll on the segment stream path (D-05)"
    - "Session aggregation: many ~6s segment GETs -> ONE egress row per (sess,host) on idle flush (AR-EGRESS-04); mutex guards map mutation ONLY, never held across a copy (T-02-LOCK)"
    - "Bounded map + oldest-eviction-on-overflow + idle reaper -> no OOM from distinct-token flood (T-02-DOS)"
    - "Graceful shutdown flushes all open sessions BEFORE the producer drains (defer LIFO order)"

key-files:
  created:
    - "services/streaming/internal/service/hls_sessions.go"
    - "services/streaming/internal/service/hls_sessions_test.go"
  modified:
    - "libs/videoutils/proxy.go"
    - "libs/videoutils/proxy_test.go"
    - "services/streaming/internal/handler/stream.go"
    - "services/streaming/cmd/streaming-api/main.go"

key-decisions:
  - "Token minted INSIDE rewriteM3U8URLs (not at manifest-fetch in the handler) because the handler does not see the in-manifest token. Attribution (provider/operation/user_id) is therefore captured on first segment touch via an idempotent Mint backfill rather than at manifest fetch — the only attribution signal the handler can reach for a given token."
  - "ProxyStream/ProxyWithReferer kept their original signatures (delegating to *Counted variants) so the dozens of existing callers compile unchanged; only the streaming HLSProxy handler opted into the counted variant."
  - "HLSSessions takes an injectable clock (s.now) so the reaper/idle-flush logic is unit-tested deterministically without real sleeps; flushIdle(now) is the test seam, the reaper goroutine just calls it on a 10s ticker."
  - "Streaming main constructs the tracing.Producer itself (ANALYTICS_INTERNAL_URL, default http://analytics:8092) + SetGlobalSink — the BE-egress producer wiring for the streaming service lands here because the aggregator needs a live sink; other Wave 2 services wire their own."

requirements-completed: [AR-EGRESS-04, AR-EGRESS-05]

# Metrics
duration: ~15min
completed: 2026-06-05
---

# Phase 02 Plan 03: HLS Egress Aggregation + Dual Byte Counting Summary

**HLS-proxy egress now aggregates to ONE effect row per (stream-session, host) instead of one row per ~6s segment: a per-manifest crypto/rand `?sess=` token (injected in `rewriteHLSURL`) correlates a watch's segment GETs into a bounded, idle-reaped in-memory tally that emits a single summed `tracing.Effect` on session end — and the proxy now counts both `bytes_in` (upstream `resp.Body` via a no-buffer `countReader`) and `bytes_out` (client sink).**

## Performance
- **Duration:** ~15 min
- **Tasks:** 2 (both TDD)
- **Files modified/created:** 6

## Accomplishments
- **Task 2 (proxy seams):** `newSessToken()` (crypto/rand, 16-byte hex, non-authoritative — T-02-SESS); `rewriteM3U8URLs` mints one token per manifest and threads it to every `rewriteHLSURL`/`rewriteURIAttribute` rewrite so all of a watch's segments share it; `rewriteHLSURL` appends `&sess=<token>` after the existing `exp`/`sig` provenance params (skip rule for already-proxied URLs preserved). `countReader` (atomic `bytes_in`) wraps `resp.Body` before `io.Copy`/`rateLimitedCopy` in both `ProxyStreamCounted` and `ProxyWithRefererCounted` (no `io.ReadAll` on the segment stream path — D-05). The original `ProxyStream`/`ProxyWithReferer` now delegate to the counted variants, signatures unchanged.
- **Task 1 (aggregator):** `HLSSessions` holds `map[sessKey]*sessionTally` keyed by `(sess-token, upstream-host)` under a single `sync.Mutex` held ONLY for map mutation (never across a copy — T-02-LOCK). `Observe(sess,host,in,out)` folds per-segment byte counts; `Mint(...)` captures provider/operation/user_id (idempotent backfill). A reaper goroutine scans every 10s and flushes sessions idle > `idleWindow` (45s default, tunable 30–60s) as ONE `tracing.Effect{EffectKind:"egress", Requests:segments, BytesIn/BytesOut summed, DurationMS:lastSeen-firstSeen, Host, Provider}`, then deletes them. Hard map-size cap (10k) with oldest-eviction on overflow (T-02-DOS); `Stop()` flushes ALL open sessions (D-06). Wired into `stream.go` (`HLSProxy` reads `?sess=`, Mint+Observe segment GETs) and `main.go` (producer + global sink + aggregator construction, graceful flush before producer drain).

## Task Commits
1. **Task 2: ?sess= injection + dual byte counters** — `6efcec12` (test, RED verified), `973700e7` (feat, GREEN)
2. **Task 1: HLS session aggregator** — `7d21c7a3` (test, RED verified), `01008ff7` (feat, GREEN)

_Plan metadata commit (this SUMMARY) follows separately._

## Files Created/Modified
- `services/streaming/internal/service/hls_sessions.go` — `HLSSessions` aggregator (tally map + reaper + bounded eviction + graceful flush)
- `services/streaming/internal/service/hls_sessions_test.go` — 4 tests: aggregation, eviction, bounded map, graceful flush
- `libs/videoutils/proxy.go` — `newSessToken`, `countReader`, `&sess=` injection, `*Counted` proxy variants
- `libs/videoutils/proxy_test.go` — `TestSessTokenInjection`, `TestDualByteCount`
- `services/streaming/internal/handler/stream.go` — `NewStreamHandlerWithSessions`, `observeEgress`, counted-proxy call
- `services/streaming/cmd/streaming-api/main.go` — producer + `SetGlobalSink` + aggregator construction + graceful shutdown order

## Deviations from Plan
The plan's `<interfaces>` block typed the `Effect` measures as `uint64`/`uint32` and described `Mint` as called "at the manifest-fetch path". Two adjustments, neither a functional deviation:
1. **[Rule 3 - alignment] Effect measure types.** The real `libs/tracing.Effect` (from 02-01) uses `int` for `BytesIn/BytesOut/DurationMS/Requests`, not the `uint64`/`uint32` the plan's interface sketch showed. The aggregator carries `uint64`/`uint32` internally (correct for accumulation) and converts to `int` at the `Record` boundary. No behavior change.
2. **[design clarification] Mint at segment first-touch, not manifest fetch.** The `?sess=` token is minted INSIDE `rewriteM3U8URLs` (per the plan's own seam choice), so the handler never sees the token at manifest-fetch time — only on the subsequent segment GETs that carry `?sess=`. Attribution is therefore captured via an idempotent `Mint` backfill on the first segment touch (provider derived from upstream host / ctx, operation+user_id from request ctx when present). This is the only attribution signal reachable for a given token and matches the plan's "one row per (session,host)" intent. Documented as a Known Limitation below.

## Known Limitations
- **Attribution on browser segment GETs is usually sparse.** Segment GETs are fresh browser requests without W3C baggage or private ctx values, so `operation`/`user_id` are typically empty on the aggregated row (provider is still derived from host). The plan anticipated capturing these at manifest fetch, but the in-manifest token is not visible to the handler. The aggregated row's load-bearing fields (host, summed bytes, request count, duration) are always populated; richer attribution would require threading the token back out of `rewriteM3U8URLs` to the manifest-fetch handler — deferred as it is not required by AR-EGRESS-04/05.

## Known Stubs
None. The aggregator is fully wired: the producer ships to analytics `/internal/effects`, the proxy counts real upstream/client bytes, and segment GETs feed the tally. A nil aggregator (test/degraded) cleanly no-ops.

## Threat Flags
None. No new network endpoints, auth paths, or trust boundaries beyond the plan's threat register (`?sess=` is crypto/rand non-authoritative; the map is bounded+reaped; the mutex is never held across a copy).

## TDD Gate Compliance
- Task 2: `test(02-03)` (`6efcec12`, RED — build failure on undefined `newSessToken`/`countReader` + wrong `rewriteHLSURL` arity) -> `feat(02-03)` (`973700e7`, GREEN). ✓
- Task 1: `test(02-03)` (`7d21c7a3`, RED — undefined `HLSSessions`/`NewHLSSessions`) -> `feat(02-03)` (`01008ff7`, GREEN). ✓

## Verification
- `cd services/streaming && go test ./internal/service/ -count=1 && go build ./...` — PASS (also `-race` clean)
- `cd libs/videoutils && go test ./ -count=1 && go build ./` — PASS
- `grep -n 'sess=' libs/videoutils/proxy.go` — present in `rewriteHLSURL`
- `grep -c 'crypto/rand' libs/videoutils/proxy.go` — 3 (≥1)
- `sync.Mutex` in hls_sessions.go = 1; no real `io.Copy` in the locked region (the lone match is a comment); `idleWindow`/cap present
- No `go.work`/cross-workspace `go.mod` churn — change set scoped to the 6 plan files

---
*Phase: 02-be-egress-recorder*
*Completed: 2026-06-05*

## Self-Check: PASSED
- Created files present: hls_sessions.go, hls_sessions_test.go; modified: proxy.go, proxy_test.go, stream.go, main.go.
- All task commits present: 6efcec12, 973700e7, 7d21c7a3, 01008ff7.
- libs/videoutils + services/streaming build and pass tests (race-clean).
- Working tree scoped to plan files; no shared-artifact (STATE/ROADMAP) writes.
