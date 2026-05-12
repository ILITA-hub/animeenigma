---
phase: 17-observability
fixed_at: 2026-05-12T14:45:00Z
review_path: .planning/phases/17-observability/17-REVIEW.md
iteration: 2
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
iter1:
  fixed_at: 2026-05-12T14:30:00Z
  findings_in_scope: 14
  fixed: 14
  skipped: 0
  status: all_fixed
---

# Phase 17: Code Review Fix Report

**Fixed at:** 2026-05-12T14:30:00Z
**Source review:** .planning/phases/17-observability/17-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 14 (3 BLOCKER + 11 WARNING)
- Fixed: 14
- Skipped: 0
- INFO findings (4): out of scope per `fix_scope: critical_warning`.

All BLOCKER + WARNING findings landed as atomic commits with the project's
Co-Authored-By footer. Each commit references its REVIEW.md ID(s) in the
subject. The full `go test ./services/scraper/... ./services/gateway/...
./libs/metrics/... -count=1 -race` suite passes after every commit.

## Fixed Issues

### BLK-01: SSRF — probe segment fetcher trusts attacker-controlled URLs and follows redirects

**Files modified:** `services/scraper/internal/health/probe.go`, `services/scraper/internal/health/probe_test.go`
**Commit:** 1dbfae8
**Applied fix:** `fetchSegment` now validates scheme + host BEFORE issuing
I/O. The HTTP client's `CheckRedirect` refuses to follow 3xx Location
bounces. `isPrivateOrLoopback` rejects loopback / RFC-1918 / link-local /
docker-internal destinations (`postgres`, `auth`, `redis`, etc.). A
`WithAllowPrivateHosts` ProbeOption was added so existing httptest-driven
tests can opt out — production callers MUST NOT use it. Added
TestProbe_FetchSegmentRejectsPrivateHost and
TestProbe_HTTPClientRefusesRedirects as regressions.

### BLK-02: Gateway proxy forwards hop-by-hop headers and Authorization verbatim

**Files modified:** `services/gateway/internal/service/proxy.go`, `services/gateway/internal/service/proxy_test.go`
**Commit:** 8f12051
**Applied fix:** New `copyForwardHeaders` filters RFC 7230 §6.1 hop-by-hop
headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`,
`Proxy-Authorization`, `Te`, `Trailer`, `Trailers`, `Transfer-Encoding`,
`Upgrade`) plus `Cookie`. Honours the `Connection: <header>, <header>`
syntax — every header named in Connection is also stripped, closing the
request-smuggling primitive. `Authorization` is intentionally NOT
stripped because JWTValidationMiddleware uses `r.Header.Set` (replace, not
append) so exactly one Authorization value reaches Forward; that value
must reach the backend for protected routes to authenticate. Tests
added: TestProxyService_Forward_StripsHopByHopHeaders,
TestProxyService_Forward_HonoursConnectionHeaderList,
TestProxyService_Forward_PreservesAuthorization.

### BLK-03: Probe panic recovery re-spawns goroutine without backoff

**Files modified:** `services/scraper/internal/health/probe.go`, `services/scraper/internal/health/probe_test.go`
**Commit:** 1dbfae8 (same commit as BLK-01)
**Applied fix:** Removed `go r.Start(ctx)` from the outer panic recover.
The recover now logs (with `debug.Stack`) and returns. The missing
heartbeat fires the dead-probe alert (RESEARCH P-07) so operators still
notice. Added TestProbe_FatalPanicDoesNotRespawn that asserts the
goroutine count is stable after a synthetic panic.

### WR-01: RegisteredProviders called twice in main.go

**Files modified:** `services/scraper/cmd/scraper-api/main.go`
**Commit:** 56b7ad3
**Applied fix:** Take ONE snapshot of the registered-providers list,
reuse for both the metric-seed loop and the probe-spawn loop. Prevents
a future bug where the seeded metric set diverges from the spawned probe
set.

### WR-02: AdminSnapshot iteration-mutate pattern

**Files modified:** `services/scraper/internal/handler/scraper.go`
**Commit:** 821b5a6
**Applied fix:** GetAdminHealth now builds a fresh `redactedStages` map
instead of mutating `ph.Stages` while iterating it. Today's in-place
write is well-defined per the Go spec but brittle; the new code is
explicit and statically-analyser-friendly.

### WR-03: IsHealthy / alert-rule divergence undocumented

**Files modified:** `services/scraper/internal/health/cache.go`, `services/scraper/internal/health/cache_test.go`
**Commit:** aefad74
**Applied fix:** Chose option 2 from REVIEW.md (lock in current behaviour
+ document the divergence). IsHealthy's docstring now explicitly states
that branch 3 (missing stream_segment) returns true even when an earlier
stage like `search` is DOWN, and that the alert rule
`provider-health-stream-segment-down` only fires on a fresh Up=false
event — the divergence is intentional per RESEARCH P-08. Added
TestCache_ShortCircuitedProbe_FailsOpen_DivergesFromAlerts as the
lock-in test.

### WR-04: IPRateLimiter eviction goroutine never stopped in tests

**Files modified:** `services/gateway/internal/transport/router.go`, `services/gateway/internal/transport/router_test.go`
**Commit:** 8bc816f
**Applied fix:** Added `RateLimitMiddlewareWithStop` and
`NewRouterWithCleanup` as siblings of the existing functions. Legacy
entry points retain their signatures for the single production
caller (`cmd/gateway-api/main.go`). The test fixture
`buildTestGatewayRouter` now uses `NewRouterWithCleanup` and registers
`rateLimiter.Stop` via `t.Cleanup`. No more per-test goroutine leak.

### WR-05: Orchestrator parallel tests share global metric label values

**Files modified:** `services/scraper/internal/service/orchestrator_test.go`
**Commit:** b436cd1
**Applied fix:** Every parallel test that reads
`metrics.ParserFallbackTotal` now embeds `t.Name()` in its fakeProvider
names (e.g. `"A_count_TestOrchestrator_FailoverFallbackTotalIncrementCount"`).
Collisions between parallel tests are now structurally impossible, not
just unlikely.

### WR-06: Probe HTTP client uses default Transport

**Files modified:** `services/scraper/internal/health/probe.go`
**Commit:** 1dbfae8 (same commit as BLK-01)
**Applied fix:** Explicit `http.Transport` with `MaxIdleConnsPerHost: 2`,
`MaxConnsPerHost: 4`, `IdleConnTimeout: 90 * time.Second`. No longer
shares `http.DefaultTransport` with unrelated callers.

### WR-07: nextSleep can return zero/negative duration on jitter bump

**Files modified:** `services/scraper/internal/health/probe.go`, `services/scraper/internal/health/probe_test.go`
**Commit:** 1dbfae8 (same commit as BLK-01)
**Applied fix:** `nextSleep` now clamps the result to
`>= probeBaseInterval/2`. A future maintainer pumping `probeJitterPct`
to ≥1.0 can no longer produce a tight-loop tick against upstream.
Added TestProbe_NextSleepClamp asserting the property across 1000
iterations.

### WR-08: io.ReadFull error swallowed in stream_segment fetch

**Files modified:** `services/scraper/internal/health/probe.go`
**Commit:** 1dbfae8 (same commit as BLK-01)
**Applied fix:** `fetchSegment` now distinguishes three cases for the
read result: `n == 0` → empty body failure; `n > 0 && err ==
io.ErrUnexpectedEOF` → success (legitimate short body, e.g. small m3u8
manifest); `n > 0 && err != nil && err != io.ErrUnexpectedEOF` →
failure (connection reset mid-stream). Previously the second and third
cases were collapsed into "success".

### WR-09: parseQuery prefer cap is post-trim only — log-injection vector

**Files modified:** `services/scraper/internal/handler/scraper.go`, `services/scraper/internal/handler/scraper_test.go`
**Commit:** 821b5a6 (same commit as WR-02 + WR-11)
**Applied fix:** Added `preferAllowed = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)`
as the defense-in-depth check after the length cap. Non-matching values
are silently coerced to empty string (matching the existing "unknown
prefer silently ignored" contract). Closes the log-injection vector where
`prefer="animepahe\n[FORGED_LOG_LINE]"` would otherwise reach a
structured-log field. The existing `TestParseQuery_PreferLengthCap` was
updated to use lowercase to survive the allow-list and pin the 64-char
truncation. Added `TestParseQuery_PreferRejectsInvalidChars` (table-driven
across newline-injection / uppercase / path-traversal / control-char /
space / dot cases) and `TestParseQuery_PreferAcceptsValid`.

### WR-10: scraper admin endpoint not enforced to docker-internal traffic

**Files modified:** `services/scraper/internal/transport/router.go`, `services/scraper/internal/transport/router_test.go`
**Commit:** e322f93
**Applied fix:** Chose option 2 from REVIEW.md (defense-in-depth header
check), implemented as a remote-address private-IP guard. New
`privateOnlyMiddleware` rejects requests whose RemoteAddr is not
loopback / RFC-1918 / link-local with a 403. Scoped to
`/scraper/health/admin` only — business routes are unaffected. Tests:
TestRouter_AdminHealthRejectsPublicRemoteAddr (public IPs fail),
TestRouter_AdminHealthAcceptsPrivateRemoteAddr (loopback, docker
bridge, RFC-1918, IPv6 loopback all pass).

### WR-11: time.Now used directly in GetAdminHealth

**Files modified:** `services/scraper/internal/handler/scraper.go`
**Commit:** 821b5a6 (same commit as WR-02 + WR-09)
**Applied fix:** `ScraperHandler` gained an injectable `now func() time.Time`
field plus a `SetNow` setter. `NewScraperHandler` defaults to `time.Now`
(unchanged production behaviour). Tests that need to lock the admin
response's `generated_at` field can now do so without patching globals.

## Skipped Issues

None. All 14 in-scope findings (3 BLOCKER + 11 WARNING) were fixed
cleanly. INFO findings IN-01..IN-04 are out of scope under the
`critical_warning` fix scope.

---

_Fixed: 2026-05-12T14:30:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_

# Phase 17: Code Review Fix Report — Iteration 2

**Fixed at:** 2026-05-12T14:45:00Z
**Source review:** .planning/phases/17-observability/17-REVIEW.md (iter 2)
**Iteration:** 2

**Summary:**
- Findings in scope: 5 (1 BLOCKER + 4 WARNING — new regressions from iter 1's fixes)
- Fixed: 5
- Skipped: 0
- INFO findings (3): out of scope per `fix_scope: critical_warning`.

All 5 in-scope iter-2 findings landed as 3 atomic commits with the
project's Co-Authored-By footer. The full
`go test ./services/scraper/... ./services/gateway/... ./libs/metrics/...
-count=1 -race` suite passes after every commit.

## Fixed Issues (Iteration 2)

### BLK-NEW-01: privateOnlyMiddleware bypassable via X-Forwarded-For (chi RealIP)

**Files modified:** `services/scraper/internal/transport/router.go`, `services/scraper/internal/transport/router_test.go`
**Commit:** c9fed70
**Applied fix:** Iter-1's WR-10 mounted `privateOnlyMiddleware` on
`/scraper/health/admin` but the same router also mounted
`middleware.RealIP`, which rewrites `r.RemoteAddr` from attacker-
controlled `X-Forwarded-For` / `X-Real-IP` / `True-Client-IP` headers.
A public-IP attacker could send `X-Forwarded-For: 10.0.0.1` and
trivially defeat the private-IP gate. Chose REVIEW.md option 1: remove
`middleware.RealIP` from the scraper router entirely. The scraper is a
backend-to-backend service with no IP-based rate limiting, no IP-based
auth, and no cookies — it has no use for the "real" client IP, so
`r.RemoteAddr` can stay as the genuine transport peer. The chi
`middleware` import is still needed (`middleware.RequestID` remains).
Added `TestRouter_AdminHealthRejectsForgedXForwardedFor` as the
regression: 7 forged header cases (XFF/XRI/True-Client-IP including a
chained XFF list) from public RemoteAddr all assert 403.

### WR-NEW-01: TestProbe_FatalPanicDoesNotRespawn did not exercise production Start() path

**Files modified:** `services/scraper/internal/health/probe.go`, `services/scraper/internal/health/probe_test.go`, `services/scraper/internal/health/probe_internal_test.go` (new)
**Commit:** d74586e (same commit as WR-NEW-02)
**Applied fix:** The previous test ran a synthetic anonymous goroutine
that panicked + recovered, then counted goroutines — only verifying a
property of the Go runtime, not the production code. A future
regression that reintroduces `go r.Start(ctx)` in `Start`'s outer
defer-recover would not have been caught. Added a
`computeInitialDelayFn` field to ProbeRunner and a
`withComputeInitialDelayForTest` test-only injection seam (in
`probe_internal_test.go`, only linked into the test binary).
Rewrote the test to (a) make `Start` invoke a panicking injected hook
from inside its real loop body, (b) assert `Start` returns within 1s,
and (c) assert the hook is called exactly once (>1 implies the outer
recover respawned Start). Test output now shows the stack trace
traces through `(*ProbeRunner).Start` outer recover, confirming the
real production code path is exercised.

### WR-NEW-02: WithAllowPrivateHosts exported public option (documented test-only)

**Files modified:** `services/scraper/internal/health/probe.go`, `services/scraper/internal/health/probe_test.go`, `services/scraper/internal/health/probe_internal_test.go` (new)
**Commit:** d74586e (same commit as WR-NEW-01)
**Applied fix:** `WithAllowPrivateHosts` was part of the package's
exported API but documented as "production callers MUST NOT use this
option" — a future caller in any package could opt out of the SSRF
guard with a one-line import. Removed the exported function from
`probe.go` and replaced with an `allowPrivateHostsForTest` helper in
the new `probe_internal_test.go` file (a `_test.go` file is only
linked into the test binary, so non-test packages cannot reach it).
Updated all 6 existing call sites in `probe_test.go` to the
construct-then-mutate pattern (`r := NewProbeRunner(...);
allowPrivateHostsForTest(r)`).

### WR-NEW-03: parseQuery byte-truncation dead code before regex allow-list

**Files modified:** `services/scraper/internal/handler/scraper.go`, `services/scraper/internal/handler/scraper_test.go`
**Commit:** a071fd6 (same commit as WR-NEW-04)
**Applied fix:** parseQuery applied byte-truncation BEFORE the regex
allow-list. The truncation could split a multi-byte UTF-8 codepoint,
and the following regex would reject the orphan continuation bytes
anyway — so the truncation step was dead code for any non-ASCII input,
and a future maintainer reordering the two checks would leave broken
UTF-8 in their logs. Removed the truncation and let the regex's
`{1,64}` quantifier be the single source of length truth. `maxPreferLength`
is retained (used by tests and by future regex pinning).

### WR-NEW-04: TestParseQuery_PreferLengthCap silently coupled to truncation impl

**Files modified:** `services/scraper/internal/handler/scraper_test.go`
**Commit:** a071fd6 (same commit as WR-NEW-03)
**Applied fix:** The existing test asserted `len(qp.prefer) == 64`
against a 1024-char input, which conflated "truncation worked" with
"regex worked" — under WR-NEW-03's regex-only enforcement the assertion
would have flipped to "prefer == ''" without anyone noticing. Loosened
the existing test to accept either contract (empty OR ≤ 64) so it
pins the upper bound regardless of implementation order. Added two
anchoring tests: `TestParseQuery_PreferRejectsOversize` (65 chars →
"", proving the regex's `{1,64}` cap is the active enforcer) and
`TestParseQuery_PreferAcceptsBoundary` (64 chars → kept unchanged,
proving the regex is `{1,64}` inclusive not `{1,63}`).

## Skipped Issues (Iteration 2)

None. All 5 in-scope iter-2 findings (1 BLOCKER + 4 WARNING) were
fixed cleanly. INFO findings IN-01..IN-03 (loose nextSleep clamp
boundary, debug.Stack inline log style, comment citation drift on
"CR-02" → "WR-01") are out of scope under the `critical_warning` fix
scope.

---

_Fixed: 2026-05-12T14:45:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 2_
