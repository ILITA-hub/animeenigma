---
phase: 17-observability
reviewed: 2026-05-12T00:00:00Z
depth: quick
iteration: 3
focus: regression_scan
files_reviewed: 7
files_reviewed_list:
  - services/scraper/internal/transport/router.go
  - services/scraper/internal/transport/router_test.go
  - services/scraper/internal/handler/scraper.go
  - services/scraper/internal/handler/scraper_test.go
  - services/scraper/internal/health/probe.go
  - services/scraper/internal/health/probe_test.go
  - services/scraper/internal/health/probe_internal_test.go
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
status: clean
---

# Phase 17: Code Review Report — Iteration 3 (Final Regression Scan)

**Reviewed:** 2026-05-12
**Depth:** quick (regression-focused)
**Iteration:** 3 of N (final)
**Status:** clean — no remaining BLOCKER or WARNING findings

## Summary

Iteration 3 is the final regression scan after iteration 2 fixed all 5 regressions
introduced by iteration 1 fixes (BLK-NEW-01 + WR-NEW-01..04, commits c9fed70,
d74586e, a071fd6).

This scan re-verified ONLY the iteration-2 surfaces and confirmed:

1. The BLK-NEW-01 fix (X-Forwarded-For / X-Real-IP / True-Client-IP bypass) holds.
2. All four WR-NEW-* fixes hold.
3. No new BLOCKER or WARNING severity issues introduced by iteration 2.
4. The iteration-2 regression tests genuinely exercise the production code path,
   not synthetic shims.

## Verification Detail

### BLK-NEW-01 — X-Forwarded-For header forging bypass

**Status:** Fixed and locked.

**Production path verified clean:**
- `services/scraper/internal/transport/router.go:79-82` — chi `middleware.RealIP`
  is NOT mounted on the router. The middleware stack is `RequestID`,
  `metricsCollector.Middleware`, `RequestLogger(log)`, `Recoverer(log)` only.
- `services/scraper/internal/transport/router.go:42-61` — `privateOnlyMiddleware`
  reads `r.RemoteAddr` directly via `net.SplitHostPort`. It NEVER consults any
  `X-Forwarded-For` / `X-Real-IP` / `True-Client-IP` headers.
- Documentation at lines 28-37 and 89-99 pins the invariant inline so a future
  maintainer who reintroduces `middleware.RealIP` is forced to read the
  warning.

**Regression test verified to exercise production path:**
- `router_test.go:166-204` (`TestRouter_AdminHealthRejectsForgedXForwardedFor`)
  uses `freshTestRouter(t)` which calls the real `NewRouter(...)` — the same
  function `cmd/scraper-api/main.go` calls in production.
- The test sets a public `RemoteAddr=8.8.8.8:54321` and forges each of:
  `X-Forwarded-For: 172.18.0.5`, `10.0.0.1`, `127.0.0.1`, `192.168.1.1, 8.8.8.8`;
  `X-Real-IP: 172.18.0.5`, `10.0.0.1`; `True-Client-IP: 172.18.0.5`. Every case
  asserts 403.
- This is NOT a synthetic test — it drives the genuine chi middleware chain. A
  future regression that re-mounts `middleware.RealIP` would cause RealIP to
  rewrite `r.RemoteAddr` from the forged header and the test would fail.

### WR-NEW-01 — Synthetic panic-recover test (now drives production path)

**Status:** Fixed and locked.

- `probe.go:97-103` — `computeInitialDelayFn` field is unexported.
- `probe.go:217-222` — `Start()` routes through the injection seam IF non-nil,
  otherwise calls real `r.computeInitialDelay()`. Production callers cannot
  reach the seam because the setter lives in `probe_internal_test.go`.
- `probe_internal_test.go:29-31` — `withComputeInitialDelayForTest` returns a
  `ProbeOption`. The `_test.go` filename suffix means this function is linked
  ONLY into the test binary; no production import path can reach it.
- `probe_test.go:457-513` (`TestProbe_FatalPanicDoesNotRespawn`) — drives the
  real `r.Start(ctx)`, makes `computeInitialDelay` panic INSIDE the outer
  defer-recover, asserts that `Start` returns AND that the panic hook was
  called exactly once. A respawn regression would produce >1 calls and a
  timeout on `done`.

This is a genuine production-path test, not the iteration-1 synthetic
anonymous goroutine.

### WR-NEW-02 — Public SSRF bypass option removed

**Status:** Fixed and locked.

- The previously-exported `WithAllowPrivateHosts` functional option has been
  deleted from `probe.go`. The remaining `allowPrivateHosts` struct field at
  `probe.go:92-95` is unexported.
- The only setter is `allowPrivateHostsForTest` in
  `probe_internal_test.go:19-21`, which lives in a `_test.go` file and is
  therefore NOT linked into the production binary. No sibling package or
  integration test in `services/scraper/cmd/...` can reach it.
- Production callers cannot opt out of the SSRF guard at all.

### WR-NEW-03 — Byte-truncation removed; regex enforces length

**Status:** Fixed and locked.

- `scraper.go:114` — `preferAllowed = regexp.MustCompile(\`^[a-z0-9_-]{1,64}$\`)`
  — the `{1,64}` quantifier structurally enforces the cap.
- `scraper.go:116-141` — `parseQuery` applies the regex and coerces non-matches
  to empty. The previous byte-truncation step (which could split a UTF-8
  codepoint) is gone.
- `maxPreferLength = 64` is kept as a named constant for documentation /
  readability but is no longer the active enforcer.

### WR-NEW-04 — Boundary regression tests

**Status:** Fixed and locked.

- `scraper_test.go:506-514` (`TestParseQuery_PreferLengthCap`) — locks the
  upper-bound invariant regardless of implementation order: parsed value must
  be empty OR ≤ `maxPreferLength`.
- `scraper_test.go:522-533` (`TestParseQuery_PreferRejectsOversize`) — 65-char
  all-`a` input asserts empty result. This proves the regex (not silent
  truncation) is the active enforcer.
- `scraper_test.go:539-547` (`TestParseQuery_PreferAcceptsBoundary`) — 64-char
  input passes through unchanged. Proves the regex is `{1,64}` inclusive, not
  off-by-one to `{1,63}`.

## Additional Spot-Checks

- `GetAdminHealth` at `scraper.go:274-312` — the iteration-1 fix that builds a
  fresh `redactedStages` map (avoiding iterate-while-mutate) is retained
  correctly.
- `h.now` nil-guard at `scraper.go:303-306` is defensive — `NewScraperHandler`
  always initializes `h.now = time.Now`, but the nil-guard handles a
  hypothetical caller that sets it via `SetNow(nil)` (which is itself
  re-mapped to `time.Now`). No bug.
- `fetchSegment` SSRF allowlist at `probe.go:489-511` correctly checks
  loopback, RFC-1918, link-local-unicast, link-local-multicast, unspecified,
  and multicast IPs. The internal-docker-service-name allowlist is unchanged
  from iteration 2.
- IPv6 RemoteAddr handling: chi router yields `[::1]:54321` style; the admin
  middleware's `net.SplitHostPort` correctly extracts `::1`, and
  `net.ParseIP("::1").IsLoopback()` returns true. Covered by
  `TestRouter_AdminHealthAcceptsPrivateRemoteAddr`.

## Conclusion

Iteration 2 introduced no new BLOCKER or WARNING regressions. All previously
identified findings (BLK-01..03, WR-01..11, BLK-NEW-01, WR-NEW-01..04) are
fixed and locked by tests that exercise the genuine production code path.

The scraper observability surface is ready to ship for phase 17.

---

_Reviewed: 2026-05-12_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: quick (regression scan)_
_Iteration: 3 (final)_
