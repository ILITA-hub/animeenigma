---
phase: 15-foundation
fixed_at: 2026-05-11T09:15:00Z
review_path: .planning/phases/15-foundation/15-REVIEW.md
iteration: 1
findings_in_scope: 11
fixed: 11
skipped: 0
status: all_fixed
---

# Phase 15: Code Review Fix Report

**Fixed at:** 2026-05-11T09:15:00Z
**Source review:** .planning/phases/15-foundation/15-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 11 (3 Critical + 8 Warning; Info findings deferred per fix_scope=critical_warning)
- Fixed: 11
- Skipped: 0

All fixes verified by `go test ./services/scraper/... ./services/catalog/...`
(all packages green) and by live redeploy via `make redeploy-scraper`
and `make redeploy-catalog`. `make health` reports all eight services
healthy after the round of fixes; `/scraper/health` returns the
expected `{"providers":{}}` payload and no longer emits
`Access-Control-Allow-Origin: *`.

## Fixed Issues

### CR-01: `orderedProviders` returns the preferred provider TWICE on failover

**Files modified:** `services/scraper/internal/service/orchestrator.go`, `services/scraper/internal/service/orchestrator_test.go`
**Commit:** `7e7f13e`
**Applied fix:** Replaced the `len(out) == 1` skip predicate with an
explicit `preferredIdx int` tracking the preferred provider's index in
`o.providers`. The second loop now skips by `i == preferredIdx`, which
is unconditional and never bypassed once a non-preferred provider is
appended. Added `TestOrchestrator_PreferPriority_NoDuplicates` which
registers A,B,C with `prefer=B` and asserts each provider is called
EXACTLY once in a failover-exhaustion scenario, plus a direct check on
`len(orderedProviders) == 3` and `[0].Name() == "B_nd"`.

### CR-02: `HealthSnapshot` calls provider `HealthCheck` while holding the orchestrator read lock

**Files modified:** `services/scraper/internal/service/orchestrator.go`
**Commit:** `36ab309`
**Applied fix:** Snapshot the `o.providers` slice into a local copy
under `o.mu.RLock()`, release the lock with `o.mu.RUnlock()`, then
iterate the local copy invoking `p.HealthCheck(ctx)`. The lock-hold
time is now decoupled from provider behavior, so a future regression
where a provider's `HealthCheck` does network I/O will no longer
block concurrent `Register()` calls.

### CR-03: `MegacloudClient.Extract` silently discards `io.ReadAll` errors

**Files modified:** `services/scraper/internal/embeds/megacloud.go`
**Commit:** `ae1f861`
**Applied fix:** Three sub-changes:
1. Capture the `io.ReadAll` error and surface it as
   `domain.WrapProviderDown(readErr, "megacloud: read sidecar response body")`
   so a truncated body surfaces as a transport-level failure (the
   correct signal for upstream incident dashboards) rather than a
   misleading "decode: unexpected end of JSON input" error.
2. Wrap the body in `io.LimitReader(resp.Body, maxSidecarBody)` with
   `maxSidecarBody = 2 << 20` (2 MiB) so a misbehaving sidecar
   cannot OOM the scraper.
3. Updated the `defer` to drain unread bytes via
   `io.Copy(io.Discard, resp.Body)` before `Close()` so the
   keep-alive connection can be reused.

### WR-01: scraper `Client` does not normalize trailing slash in `baseURL`

**Files modified:** `services/catalog/internal/parser/scraper/client.go`
**Commit:** `6d4c4e6`
**Applied fix:** Added `strings.TrimRight(baseURL, "/")` in
`NewClient` so `SCRAPER_API_URL=http://scraper:8088/` (trailing slash)
no longer produces `http://scraper:8088//scraper/episodes` URLs that
proxies/IDS in the middle might reject. Mirrors the existing handling
in `embeds/megacloud.go NewMegacloudClient`.

### WR-02: Scraper service exposes wildcard CORS (`*`) in production router

**Files modified:** `services/scraper/internal/transport/router.go`
**Commit:** `ecf9685`
**Applied fix:** Removed the `r.Use(httputil.CORS([]string{"*"}))`
middleware. Added an inline comment explaining the rationale: the
scraper is backend-to-backend only (bound to `127.0.0.1:8088`,
called only by catalog), so the CORS header would be unnecessary and
silently permissive if the bind address changes in the future. Live
verification: `curl -D -` against `localhost:8088/scraper/health`
confirms no `Access-Control-Allow-Origin` header is emitted.

### WR-03: `ScraperHandler.log` field is unused; handler falls back to `logger.Default()` on error paths

**Files modified:** `services/scraper/internal/handler/scraper.go`, `services/catalog/internal/handler/scraper.go`
**Commit:** `11d4704`
**Applied fix:** Two sub-changes:
1. **scraper service:** Promoted `notYetImplemented` from a free
   function to a `*ScraperHandler` method so it can use the injected
   `h.log` (with fallback to `logger.Default()` if `h.log` is nil),
   preserving request-correlation context from
   `middleware.RequestID` in error logs.
2. **catalog service:** Promoted `writeScraperError` from a free
   function to a `*ScraperEndpointsHandler` method so the wired
   `h.log` is used on the unexpected-error path (the default fallthrough
   to `httputil.Error`). Operators now have a breadcrumb log line
   when catalog↔scraper 500s start happening.

### WR-04: `scraper.doGET` does not cap body size or drain on close

**Files modified:** `services/catalog/internal/parser/scraper/client.go`
**Commit:** `b27b5ea`
**Applied fix:** Wrapped the body read in `io.LimitReader` with
`maxScraperBody = 4 << 20` (4 MiB) so a misbehaving scraper can't OOM
the catalog. Updated the `defer` to call `io.Copy(io.Discard, resp.Body)`
before `Close()` so the keep-alive connection can be reused even on
partial-body failures.

### WR-05: `Config.Load` returns nil error unconditionally; no validation of `MEGACLOUD_EXTRACTOR_URL`

**Files modified:** `services/scraper/internal/config/config.go`
**Commit:** `e8c708d`
**Applied fix:** Added a `url.Parse` validation step that rejects an
invalid `MEGACLOUD_EXTRACTOR_URL` (parse error OR missing
scheme/host, e.g. `megacloud-extractor:3200` without `http://`) at
boot. Empty URL is still allowed — `main.go` already warns on that.

### WR-06: Scraper Dockerfile copies unused `libs/*` go.mods

**Files modified:** `services/scraper/Dockerfile`
**Commit:** `7ed0bee`
**Applied fix:** Applied the reviewer's recommended option (b): kept
the COPYs and added an inline comment explaining that `go.work`
requires every workspace member's go.mod to exist at build time, so
trimming the COPYs would diverge from the other services' Dockerfile
patterns and silently break the next time scraper picks up another
lib.

### WR-07: `summarizeFailover` comment about empty-`errs` semantics is loose

**Files modified:** `services/scraper/internal/service/orchestrator.go`
**Commit:** `531f92e`
**Applied fix:** Tightened the doc comment to declare an explicit
PRECONDITION: `errs` may be empty ONLY when there are zero providers.
The `runFailover` loop is the only caller and is guaranteed to either
append to `errs` or return early on a terminal error. Future
maintainers adding a new terminal-error category to `failoverDecision`
are now reminded of the invariant inline.

### WR-08: Cross-test global `metrics.ParserFallbackTotal` counter pollution

**Files modified:** `services/scraper/internal/service/orchestrator_test.go`
**Commit:** `2aacf14`
**Applied fix:** Added `TestOrchestrator_FailoverFallbackTotalIncrementCount`
which asserts the SUM of per-hop `ParserFallbackTotal` increments
across an A→B→C failover chain equals exactly
`len(providers)-1 = 2`. The existing per-label fallback tests only
checked single-label deltas and would silently pass if the same
provider were called twice in different `from`/`to` orderings — this
test catches duplicate hops directly.

The deeper recommendation (refactor `libs/metrics.NewCollector` to
accept an `*prometheus.Registry` so tests can have full isolation)
is filed as a follow-up; it is outside Phase 15's scope.

## Skipped Issues

None — all 11 in-scope findings were fixed cleanly and committed.

---

_Fixed: 2026-05-11T09:15:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
