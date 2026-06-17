---
phase: 08-serving-fetch-signal
plan: 03
subsystem: api
tags: [autocache, serve-signal, fire-and-forget, internal-endpoint, docker-network, raw-resolver, ae-provider, http-client]

# Dependency graph
requires:
  - phase: 08-02
    provides: "POST /internal/library/autocache/{fetch,demand} endpoints (HIT bump+serve_total{hit}; MISS enabled-gated demand+serve_total{miss}); reason forced backfill server-side"
  - phase: 06
    provides: "library.Client (GetEpisode request/drain/close idiom; cfg.APIURL base + 2s timeout); RawResolver.GetLibraryStream strict-ae path with optional library *library.Client seam"
provides:
  - "library.Client.RecordFetch(ctx, malID, episode) — best-effort POST /internal/library/autocache/fetch (HIT signal)"
  - "library.Client.RecordDemand(ctx, malID, episode, reason) — best-effort POST /internal/library/autocache/demand (MISS backfill signal)"
  - "library.Client.postInternal helper (JSON POST on cfg.APIURL base, non-2xx→wrapped error, no body parse)"
  - "GetLibraryStream fires RecordFetch on HIT + RecordDemand(backfill) on MISS as non-blocking context.WithoutCancel goroutines that never affect the resolution result or AllAnime failover"
affects:
  - "Phase 09 (Planner drains the autocache_demand rows this MISS path now writes)"
  - "Phase 11 (Grafana charts the library_autocache_serve_total{hit,miss} series these calls increment)"
tech-stack:
  added: []
  patterns:
    - "Best-effort fire-and-forget producer from a request path: nil-client-guarded go func + context.WithoutCancel + discarded error (recs recompute-hint analog)"
    - "Thin internal POST helper reusing the existing client base URL + bounded timeout — no new env var, no response-body parse (status-only)"
    - "Intentional non-instrumentation of a non-genuine MISS (empty mal_id early return) documented inline"
key-files:
  created: []
  modified:
    - services/catalog/internal/parser/library/client.go
    - services/catalog/internal/parser/library/client_test.go
    - services/catalog/internal/service/raw_resolver.go
    - services/catalog/internal/service/raw_resolver_test.go
key-decisions:
  - "Reused the existing catalog→library cfg.APIURL base + 2s httpClient.Timeout for the internal calls — NO new config field / env var (the /internal/* endpoints live on the same library service the client already targets)"
  - "postInternal does NOT parse the {ok:true} response body — only the status code matters; any non-2xx returns a wrapped error so the best-effort caller can log+drop it"
  - "The empty-ShikimoriID early NotFound is left un-instrumented (no mal_id to key a demand on); inline comment marks it intentional so it is not a 'missing' MISS signal"
  - "Only the strict-ae GetLibraryStream is instrumented (the hybrid GetStream path is untouched), keeping MISS == a genuine ae-pool miss"
patterns-established:
  - "Pattern 1: catalog serve-signal producer — request-path side effects fired as nil-guarded context.WithoutCancel goroutines with discarded errors so a library blip/client disconnect never fails a playback resolution or its failover"
  - "Pattern 2: status-only internal POST (postInternal) reusing the existing client base+timeout — best-effort, no new wiring"
requirements-completed: [SERVE-01, SERVE-02, SERVE-03]

# Metrics
duration: ~5min
completed: 2026-06-17
---

# Phase 8 Plan 03: Serving & Fetch Signal — Catalog ae-Resolution Producer Summary

**The catalog ae seam now closes the SERVE loop: `GetLibraryStream` fires a non-blocking `RecordFetch` on a pool HIT and a non-blocking `RecordDemand(backfill)` on a pool MISS to the library `/internal/library/autocache/{fetch,demand}` endpoints, drop-on-failure, with the strict-ae resolution result and the AllAnime-raw failover byte-for-byte unchanged (SERVE-03 no regression).**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-06-17T08:43Z
- **Completed:** 2026-06-17
- **Tasks:** 2 (both TDD)
- **Files modified:** 4 (0 created, 4 modified)

## Accomplishments
- `library.Client` gained `RecordFetch` / `RecordDemand` best-effort POST methods + a `postInternal` helper, reusing the existing `cfg.APIURL` base and 2s timeout (no new env var).
- `GetLibraryStream` now emits the HIT fetch-signal and the MISS backfill-demand as nil-guarded `context.WithoutCancel` goroutines with discarded errors.
- Proven by tests that the resolution `(*RawStream, error)` and the failover are unchanged on both success and internal-call failure, and that the producer is race-clean.

## Task Commits

Each task was committed atomically (TDD test+impl folded per task):

1. **Task 1: RecordFetch / RecordDemand best-effort POST methods on the library client** — `7fc130bf` (feat)
2. **Task 2: Fire HIT/MISS calls from GetLibraryStream (non-blocking, failover unchanged)** — `61403eca` (feat)

**Plan metadata:** committed separately with this SUMMARY.

## Files Created/Modified
- `services/catalog/internal/parser/library/client.go` — added `postInternal` helper + `RecordFetch` (HIT) + `RecordDemand` (MISS) best-effort POST methods; new `bytes` import.
- `services/catalog/internal/parser/library/client_test.go` — httptest units: both methods hit the right path with the right JSON body + return nil on 200; a 5xx yields a wrapped error (no panic).
- `services/catalog/internal/service/raw_resolver.go` — `GetLibraryStream` fires the two best-effort goroutines (HIT before the stream return, MISS before the NotFound return); empty-mal_id early return left intentionally un-instrumented (inline comment).
- `services/catalog/internal/service/raw_resolver_test.go` — resolver tests: HIT fires fetch, MISS fires demand (via channel/poll for the goroutine), and a signal 500 leaves the result byte-for-byte unchanged.

## Decisions Made
- Reused `cfg.APIURL` + the 2s `httpClient.Timeout` for the internal calls — no new config field or env var (Claude's discretion per CONTEXT, which preferred reusing the existing catalog→library base).
- `postInternal` is status-only (no `{ok:true}` body parse); non-2xx → wrapped error for the caller to log+drop.
- Left the empty-ShikimoriID early NotFound un-instrumented because there is no mal_id to key a demand on — it is not a genuine ae-pool MISS. Marked intentional with an inline code comment.
- Only the strict-ae `GetLibraryStream` is instrumented; the hybrid `GetStream` path is deliberately untouched so MISS stays a genuine ae miss.

## Deviations from Plan

None - plan executed exactly as written.

No bugs, missing critical functionality, blocking issues, or architectural changes were encountered. No package installs (threat T-08-SC N/A — stdlib `net/http` + `bytes`/`encoding/json` + existing catalog deps; slopcheck not applicable).

All threat-register dispositions held by design:
- **T-08-08** (tampering on `reason`): catalog always sends `"backfill"`, and the library side (Plan 02) also forces backfill server-side — defense in depth.
- **T-08-07 / T-08-09** (DoS / repudiation): one bounded 2s-timeout goroutine per ae resolution, `context.WithoutCancel` + drop-on-failure — a slow/down library cannot block or fail the resolver; failures are logged-and-dropped by design.

## Issues Encountered
None. RED was confirmed for both tasks (Task 1: undefined methods; Task 2: fire-tests timed out waiting for the internal call), then GREEN after wiring. The shared-server goroutine test was verified race-clean with `go test -race`.

## User Setup Required
None - no external service configuration required (reuses the existing catalog→library base URL; the `/internal/*` endpoints are Docker-network-only by construction).

## Next Phase Readiness
- SERVE-01/02/03 are now functionally complete across both services: the library records HITs/MISSes (08-01/08-02) and catalog now produces those signals (08-03).
- Phase 9's Planner has a populated `autocache_demand` table to drain; Phase 11's Grafana has the `library_autocache_serve_total{hit,miss}` series to chart.
- No blockers. NOTE: no debounce on the fetch bump in P8 (at-least-once is acceptable per CONTEXT — `fetch_count` is a popularity counter, not money); flagged as a P11 tuning lever.

## Verification
- `cd services/catalog && go build ./... && go vet ./...` — clean (FINAL-BUILD-VET-OK).
- `go test ./internal/parser/library/... ./internal/service/ -count=1` — both packages pass.
- `go test ./internal/service/ -race -run GetLibraryStream` — race-clean.
- grep confirms `RecordFetch` / `RecordDemand` / `context.WithoutCancel` wired in `raw_resolver.go`, and the `/internal/library/autocache/{fetch,demand}` paths + method signatures in `client.go`.
- `GetLibraryStream`'s returned `(*RawStream, error)` and the AllAnime-raw failover control flow are unchanged — only ADD goroutine side effects; the empty-mal_id branch fires no demand.

## Self-Check: PASSED

- `services/catalog/internal/parser/library/client.go` (modified) — FOUND
- `services/catalog/internal/parser/library/client_test.go` (modified) — FOUND
- `services/catalog/internal/service/raw_resolver.go` (modified) — FOUND
- `services/catalog/internal/service/raw_resolver_test.go` (modified) — FOUND
- `.planning/phases/08-serving-fetch-signal/08-03-SUMMARY.md` — FOUND
- Commit `7fc130bf` (Task 1) — present in branch history
- Commit `61403eca` (Task 2) — present in branch history

---
*Phase: 08-serving-fetch-signal*
*Completed: 2026-06-17*
