---
phase: 15-foundation
plan: 02
subsystem: scraper
tags: [scraper, foundation, domain, golang, tdd, lint, http-client]
requires:
  - services/scraper/ scaffold (plan 15-01)
provides:
  - services/scraper/internal/domain.Provider interface — six-method contract every provider implements
  - services/scraper/internal/domain.Stream DTO with type-level no-IframeURL guard (D-DEC §2.8)
  - services/scraper/internal/domain.{Category,AnimeRef,Episode,Server,Source,Track,TimeRange,StageHealth,Health} types
  - services/scraper/internal/domain.{ErrNotFound,ErrProviderDown,ErrExtractFailed} sentinels + wrap helpers
  - services/scraper/internal/domain.EmbedExtractor interface + Registry
  - services/scraper/internal/domain.BaseHTTPClient — retryablehttp + per-host rate.Limiter + cookiejar
  - services/scraper/internal/golint forbidden-deps CI lint test
affects:
  - services/scraper/go.mod (adds retryablehttp v0.7.7, x/time v0.5.0, x/net v0.39.0, x/mod v0.20.0)
  - services/scraper/go.sum (lockfile updates)
tech-stack:
  added:
    - github.com/hashicorp/go-retryablehttp v0.7.7 (Min/Max retry wait + exponential backoff)
    - golang.org/x/time v0.5.0 (rate.Limiter — matches gateway pin)
    - golang.org/x/net v0.39.0 (publicsuffix for cookiejar etld+1 scoping)
    - golang.org/x/mod v0.20.0 (modfile parser for the forbidden-deps lint)
  patterns:
    - TDD RED→GREEN per task: separate test(...) and feat(...) commits with verifiable failing-test → passing-test transition
    - Type-level enforcement of architectural decisions via reflection-based compile-equivalent tests (no IframeURL field)
    - Compile-time interface assertion via `var _ Iface = (*fake)(nil)` to lock contract shape
    - Multi-%w wrapping (Go 1.20+) so sentinel + cause both match errors.Is
    - Functional-option pattern for BaseHTTPClient construction (With* options)
    - Lint-via-test: forbidden-deps check runs in `go test ./...`, fails CI on any forbidden dep
key-files:
  created:
    - services/scraper/internal/domain/errors.go
    - services/scraper/internal/domain/errors_test.go
    - services/scraper/internal/domain/provider.go
    - services/scraper/internal/domain/provider_test.go
    - services/scraper/internal/domain/embed.go
    - services/scraper/internal/domain/embed_test.go
    - services/scraper/internal/domain/httpclient.go
    - services/scraper/internal/domain/httpclient_test.go
    - services/scraper/internal/golint/forbidden_deps_test.go
  modified:
    - services/scraper/go.mod (4 new direct deps)
    - services/scraper/go.sum
decisions:
  - Pinned golang.org/x/time to v0.5.0 (matches services/gateway) instead of letting `go mod tidy` pull v0.15.0 → avoids workspace-wide Go 1.23 → 1.25 toolchain cascade
  - Pinned golang.org/x/mod to v0.20.0 (Go 1.18 compatible) for the same reason
  - Stream DTO field set locked in at five fields {Sources, Tracks, Intro, Outro, Headers}; TestStream_AllowedFields fails on any addition until the test list is updated explicitly (intentional change-control)
  - golint test lives as a `_test.go` file inside `package golint` — no exported runtime behavior, no doc.go needed
metrics:
  duration: ~11m
  completed: 2026-05-11T06:22:25Z
  tasks: 5
  files_created: 9
  files_modified: 2
---

# Phase 15 Plan 02: Domain Types + Provider/EmbedExtractor + BaseHTTPClient + Forbidden-Deps Lint Summary

Land the core domain contracts every scraper provider, the orchestrator, and the HTTP handler layer will implement against. Five concerns shipped: Provider interface + Stream DTO with type-level no-IframeURL guard, three sentinel errors with multi-%w wrap helpers, EmbedExtractor interface + Registry, BaseHTTPClient with retryablehttp + per-host rate.Limiter + cookiejar, and a CI test that fails the build on any forbidden anti-bot dependency. All five concerns shipped via strict TDD (RED→GREEN per task).

## Files Created

| File | Purpose | Lines |
|---|---|---|
| `services/scraper/internal/domain/errors.go` | Three sentinel errors (`ErrNotFound`, `ErrProviderDown`, `ErrExtractFailed`) + three wrap helpers using dual-%w so `errors.Is` matches both the sentinel and the underlying cause. Package doc explains failover semantics for orchestrator. | 51 |
| `services/scraper/internal/domain/errors_test.go` | 6 tests: sentinels non-nil, pairwise distinct, each wrapper preserves `errors.Is` against both sentinel and cause, error messages contain locked substrings for downstream log parsers. | 123 |
| `services/scraper/internal/domain/provider.go` | `Category` enum (sub/dub/raw); `AnimeRef`, `Episode`, `Server`, `Source`, `Track`, `TimeRange`, `StageHealth`, `Health` structs; `Stream` DTO (NO IframeURL field); `Provider` interface (Name/FindID/ListEpisodes/ListServers/GetStream/HealthCheck). Top-of-file comment documents the no-iframe rationale per D-DEC §2.8 and ISS-008. | 129 |
| `services/scraper/internal/domain/provider_test.go` | 5 tests: `TestStream_HasNoIframeURL` (reflection guard for D-DEC §2.8), `TestStream_AllowedFields` (locks the five-field shape), `TestCategoryConstants` (sub/dub/raw strings), `TestProviderInterface_Compiles` (`var _ Provider = (*fakeProvider)(nil)`), `TestStream_JSON_OmitsEmptyOptionals` (omitempty contract). | 139 |
| `services/scraper/internal/domain/embed.go` | `EmbedExtractor` interface (Name/Matches/Extract returning `*Stream`); `Registry` struct with `NewRegistry`/`Register`/`Find`/`Names`; `ErrNoMatchingExtractor` sentinel. `Find` iterates in registration order, first match wins. `Names` returns non-nil empty slice for empty registry. | 86 |
| `services/scraper/internal/domain/embed_test.go` | 6 tests: interface compile, register+find round-trip, registration order preserved, first-match wins on overlapping matchers, ErrNoMatchingExtractor on miss, empty-registry Names is non-nil empty slice. | 120 |
| `services/scraper/internal/domain/httpclient.go` | `BaseHTTPClient` wrapping `retryablehttp.Client` (1s→8s backoff, RetryMax=4, 10s timeout, DefaultRetryPolicy covers 5xx+429+conn errors) + per-host `map[string]*rate.Limiter` + `cookiejar.Jar` scoped to publicsuffix. Functional-option construction: `WithTimeout`, `WithRetryWaitMin/Max`, `WithMaxRetries`, `WithPerHostRPS`, `WithHeaders`. `Get(ctx,url)` and `Do(ctx,req)` methods. `Timeout()` getter for observability. Baseline UA: Chrome 131 desktop. | 178 |
| `services/scraper/internal/domain/httpclient_test.go` | 8 tests: default 10s timeout (SCRAPER-NF-01), hard timeout cuts hanging upstream, exponential 1→2→4→8 backoff sequence (SCRAPER-NF-03), per-host rate limit throttles same host, cookie jar persists across calls, baseline headers always set, per-host limiter does NOT affect other hosts, `Do` preserves caller headers. | 327 |
| `services/scraper/internal/golint/forbidden_deps_test.go` | 11 tests: 1 gate on real `services/scraper/go.mod` (passes today), 7 deliberate-red catches (chromedp, chromedp-cdproto cousin, rod, utls, tls-client, tls-client-fhttp cousin, playwright), 2 substring catches (cloudscraper, flaresolverr), 1 allowed-deps sanity. Uses `runtime.Caller(0)` to anchor the real-go.mod path; uses fixture strings for positive-catch tests. | 243 |

## Files Modified

| File | Change | Driver |
|---|---|---|
| `services/scraper/go.mod` | + `github.com/hashicorp/go-retryablehttp v0.7.7`, + `golang.org/x/time v0.5.0`, + `golang.org/x/net v0.39.0`, + `golang.org/x/mod v0.20.0`, + indirect `github.com/hashicorp/go-cleanhttp v0.5.2` | Tasks 4 + 5 |
| `services/scraper/go.sum` | Locked checksums for new deps | `go mod tidy` |

## Test Count Summary

| Package | File | Tests | Subtests |
|---|---|---|---|
| `internal/domain` | `errors_test.go` | 6 | +3 (TestSentinelsAreDistinct) + 3 (TestErrorMessagesAreInformative) |
| `internal/domain` | `provider_test.go` | 5 | — |
| `internal/domain` | `embed_test.go` | 6 | — |
| `internal/domain` | `httpclient_test.go` | 8 | — |
| `internal/golint` | `forbidden_deps_test.go` | 11 | — |
| **Total** | | **36** | + 6 subtests |

(Plan estimated 28; +8 added voluntarily: extra wrap-test coverage in errors_test.go, allowed-fields field-shape lock in provider_test.go, empty-registry Names test in embed_test.go, default-timeout-is-10s + Do-method tests in httpclient_test.go, chromedp-cdproto + bogdanfinn-fhttp cousin-prefix catches in forbidden_deps_test.go.)

## Commits

| Task | Phase | Hash | Message |
|---|---|---|---|
| 1 | RED | `08b2b60` | test(15-02): add failing tests for scraper domain sentinel errors |
| 1 | GREEN | `3371582` | feat(15-02): implement scraper domain sentinel errors |
| 2 | RED | `c990178` | test(15-02): add failing tests for Provider interface + Stream DTO |
| 2 | GREEN | `996a155` | feat(15-02): implement Provider interface + Stream DTO |
| 3 | RED | `be899e3` | test(15-02): add failing tests for EmbedExtractor interface + Registry |
| 3 | GREEN | `2d3eabd` | feat(15-02): implement EmbedExtractor interface + Registry |
| 4 | RED | `0bdf29b` | test(15-02): add failing tests for BaseHTTPClient |
| 4 | GREEN | `1270a7a` | feat(15-02): implement BaseHTTPClient (retryablehttp + rate.Limiter + cookiejar) |
| 5 | both | `9743639` | feat(15-02): add CI lint blocking forbidden go.mod dependencies |

## Verification Output

```text
$ cd services/scraper && go build ./...
(clean, no output)

$ go vet ./...
(clean, no output)

$ go test ./internal/domain/... ./internal/golint/... -count=1 -v -timeout 60s
=== RUN   TestSentinelsAreNonNil
=== RUN   TestSentinelsAreDistinct
--- PASS: TestSentinelsAreDistinct/NotFound_vs_ProviderDown (0.00s)
--- PASS: TestSentinelsAreDistinct/ProviderDown_vs_ExtractFailed (0.00s)
--- PASS: TestSentinelsAreDistinct/NotFound_vs_ExtractFailed (0.00s)
--- PASS: TestSentinelsAreDistinct (0.00s)
--- PASS: TestSentinelsAreNonNil (0.00s)
--- PASS: TestWrapNotFoundPreservesIs (0.00s)
--- PASS: TestWrapProviderDownPreservesIs (0.00s)
--- PASS: TestWrapExtractFailedPreservesIs (0.00s)
--- PASS: TestErrorMessagesAreInformative/ErrNotFound (0.00s)
--- PASS: TestErrorMessagesAreInformative/ErrExtractFailed (0.00s)
--- PASS: TestErrorMessagesAreInformative/ErrProviderDown (0.00s)
--- PASS: TestErrorMessagesAreInformative (0.00s)
--- PASS: TestStream_HasNoIframeURL (0.00s)
--- PASS: TestStream_AllowedFields (0.00s)
--- PASS: TestCategoryConstants (0.00s)
--- PASS: TestProviderInterface_Compiles (0.00s)
--- PASS: TestStream_JSON_OmitsEmptyOptionals (0.00s)
--- PASS: TestEmbedExtractorInterface_Compiles (0.00s)
--- PASS: TestRegistry_RegisterAndFind (0.00s)
--- PASS: TestRegistry_RegistrationOrderPreserved (0.00s)
--- PASS: TestRegistry_FindReturnsFirstMatch (0.00s)
--- PASS: TestRegistry_FindNoMatch (0.00s)
--- PASS: TestRegistry_EmptyRegistryNames (0.00s)
--- PASS: TestBaseHTTPClient_DefaultTimeoutIs10s (0.00s)
--- PASS: TestBaseHTTPClient_HardTimeout (0.10s)
--- PASS: TestBaseHTTPClient_BackoffSequence (0.15s)
--- PASS: TestBaseHTTPClient_PerHostRateLimit (0.50s)
--- PASS: TestBaseHTTPClient_CookieJarPersists (0.00s)
--- PASS: TestBaseHTTPClient_BaselineHeaders (0.00s)
--- PASS: TestBaseHTTPClient_PerHostLimiterIsolation (0.00s)
--- PASS: TestBaseHTTPClient_DoMethod (0.00s)
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/domain	0.507s

--- PASS: TestForbiddenDeps_RealGoMod (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_Chromedp (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_ChromedpCdproto (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_Rod (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_UTLS (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_TLSClient (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_TLSClientFhttp (0.00s)
--- PASS: TestForbiddenDeps_PositiveCatch_Playwright (0.00s)
--- PASS: TestForbiddenDeps_StringMatch_Cloudscraper (0.00s)
--- PASS: TestForbiddenDeps_StringMatch_Flaresolverr (0.00s)
--- PASS: TestForbiddenDeps_AllowedDepsPass (0.00s)
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/golint	0.004s

PASS — 36 tests pass, total wall time ~511ms.
```

## Stream Field Enumeration (Reflection)

Confirmed live via a temporary `runtime` test (since `internal/` packages can't be reached from outside the module):

```text
Stream type has 5 fields:
  - Sources ([]domain.Source, json="sources")
  - Tracks  ([]domain.Track, json="tracks,omitempty")
  - Intro   (*domain.TimeRange, json="intro,omitempty")
  - Outro   (*domain.TimeRange, json="outro,omitempty")
  - Headers (map[string]string, json="headers,omitempty")
```

No `IframeURL` field. No `iframe_url` json tag. `TestStream_HasNoIframeURL` enforces this on every CI run.

## go.mod Forbidden-Dep Audit (services/scraper)

```text
chromedp:                          0 occurrences
go-rod/rod:                        0 occurrences
refraction-networking/utls:        0 occurrences
bogdanfinn:                        0 occurrences
playwright-community/playwright-go: 0 occurrences
cloudscraper:                      0 occurrences
flaresolverr:                      0 occurrences
```

All current direct deps:
- `github.com/ILITA-hub/animeenigma/libs/{httputil,logger,metrics,errors}` (in-repo)
- `github.com/go-chi/chi/v5 v5.0.12`
- `github.com/hashicorp/go-retryablehttp v0.7.7`
- `github.com/sebdah/goldie/v2 v2.5.5`
- `github.com/stretchr/testify v1.9.0`
- `golang.org/x/mod v0.20.0`
- `golang.org/x/net v0.39.0`
- `golang.org/x/time v0.5.0`

All allowed per D-STACK §5.

## Deviations from Plan

### 1. [Rule 3 - Blocking issue] Pinned x/time + x/mod to older versions to avoid workspace cascade

- **Found during:** Task 4 (`go mod tidy` after adding retryablehttp + x/time), then again at Task 5 (`go mod tidy` after adding x/mod).
- **Issue:** `go mod tidy` greedily resolves to the newest version, which for `golang.org/x/time` is v0.15.0 (requires Go ≥1.25) and `golang.org/x/mod` is v0.36.0 (requires Go ≥1.25). Both triggered an automatic toolchain bump to go1.25.10 and bumped the `go` directive in `go.work` from 1.23.0 → 1.25.0 plus a similar bump in `services/gateway/go.mod` (gateway has x/time as a direct dep). Plan 15-01 already accepted one toolchain bump (1.22 → 1.23.0) for goquery v1.10.3; cascading again is unnecessary.
- **Fix:** Pinned `golang.org/x/time v0.5.0` (matches the version `services/gateway` already uses) and `golang.org/x/mod v0.20.0` (Go 1.18 compatible). Both pins are compatible with all current usage (`rate.Limiter` API has been stable since v0.1.0; `modfile` parser API stable). Re-ran `go mod tidy` and `go work sync` and confirmed: `go.work` stays at 1.23.0, `services/gateway/go.mod` is unchanged, no other module shifts.
- **Files modified:** `services/scraper/go.mod` (4 direct deps pinned), `services/scraper/go.sum`. NO changes to `go.work`, `go.work.sum`, or any other module's `go.mod`.
- **Commits:** `1270a7a` (Task 4 GREEN), `9743639` (Task 5).
- **Rationale:** Workspace-wide Go toolchain bumps should be deliberate, plan-driven choices — not incidental side-effects of a single service adding a dep. The plan's allowed-versions list for `x/time` is `v0.5.0` (the gateway-compatible pin), not `latest`.

### 2. [Rule 2 - Test coverage] Added 8 tests beyond plan minimum

- **Found during:** All tasks
- **Issue:** Plan estimated 28 tests across the five test files; final count is 36.
- **Addition rationale:**
  - `errors_test.go`: Added `TestWrapProviderDownPreservesIs` and `TestWrapExtractFailedPreservesIs` (plan only specified the NotFound variant — mirror coverage for the other two sentinels is essential since orchestrator code switches on all three).
  - `provider_test.go`: Added `TestStream_AllowedFields` (explicit field-shape lock — complements `TestStream_HasNoIframeURL` by also catching renames, not just additions).
  - `embed_test.go`: Added `TestRegistry_EmptyRegistryNames` (non-nil empty slice contract for JSON marshaling).
  - `httpclient_test.go`: Added `TestBaseHTTPClient_DefaultTimeoutIs10s` (separate from the timeout-cuts-off test so the production default is locked in independently from the test-side override) and `TestBaseHTTPClient_DoMethod` (lower-level Do path was untested in the plan).
  - `forbidden_deps_test.go`: Added `TestForbiddenDeps_PositiveCatch_ChromedpCdproto` and `TestForbiddenDeps_PositiveCatch_TLSClientFhttp` — explicit cousin-prefix tests to prove the prefix-match logic catches sibling modules under the same forbidden org.
- **Files modified:** Five test files (additive only).
- **Commits:** Rolled into each task's existing RED commit.

### 3. [Rule 3 - Test bug] Cleanup ordering fix in TestBaseHTTPClient_HardTimeout

- **Found during:** Task 4 first test run
- **Issue:** RED commit `0bdf29b` registered `t.Cleanup(func() { close(hang) })` BEFORE `t.Cleanup(srv.Close)`. Cleanups run LIFO, so `srv.Close` ran first and blocked indefinitely waiting for the still-blocked handler goroutine. Test timed out at 30s.
- **Fix:** Reversed cleanup registration order so `close(hang)` runs first (released the handler), then `srv.Close` runs (cleanly tears down). Also removed unused `uB` variable in `TestBaseHTTPClient_PerHostLimiterIsolation` (compiler error blocked the run).
- **Files modified:** `services/scraper/internal/domain/httpclient_test.go` (two small diffs).
- **Commit:** Rolled into Task 4 GREEN commit `1270a7a` with explicit note in the commit body.
- **Rationale:** Both fixes were test-side bugs introduced in my RED commit. Per the project's "Replace, don't preserve dead identity" memory, the right thing is to land the corrected test alongside the impl rather than amend the RED commit (the user instruction explicitly says NEVER amend).

## Confirmation Items

- [x] `services/scraper/internal/domain/errors.go` exists; three sentinels + three wrap helpers.
- [x] `services/scraper/internal/domain/provider.go` exists; Stream has 5 fields, none named IframeURL or tagged iframe_url.
- [x] `services/scraper/internal/domain/embed.go` exists; EmbedExtractor interface + Registry with ordered Find.
- [x] `services/scraper/internal/domain/httpclient.go` exists; BaseHTTPClient wraps retryablehttp + per-host rate.Limiter + cookiejar; Timeout()==10s by default.
- [x] `services/scraper/internal/golint/forbidden_deps_test.go` exists; CI-gates real go.mod; positive-catches every forbidden module.
- [x] `go build ./services/scraper/...` clean.
- [x] `go vet ./services/scraper/...` clean.
- [x] `go test ./services/scraper/internal/domain/... ./services/scraper/internal/golint/... -count=1` — 36 tests pass in ~511ms.
- [x] `go.work` and `services/gateway/go.mod` unchanged — no workspace cascade.
- [x] No forbidden modules in `services/scraper/go.mod` (chromedp/rod/utls/bogdanfinn/playwright/cloudscraper/flaresolverr all 0 occurrences).

## Threat Surface Scan

This plan ships:

- **Sentinel errors** — pure values, no I/O, no new attack surface.
- **Provider interface** — Go-internal contract, no JSON/HTTP exposure yet (handlers land in plan 15-03).
- **EmbedExtractor + Registry** — Go-internal, no I/O.
- **BaseHTTPClient** — adds an outbound HTTP capability to the scraper service, but the trust boundary (`scraper → upstream provider site`) was already documented by plan 15-01's threat model and is mitigated here by:
  - 10s per-attempt timeout (SCRAPER-NF-01) — caps DoS-from-upstream impact at ~40s per request (4 retries × max 8s wait + 10s).
  - Per-host `rate.Limiter` — caps outbound request rate per upstream.
  - Cookie jar scoped via publicsuffix to etld+1 — no cross-site cookie leakage.
- **Forbidden-deps lint** — defensive only; no runtime behavior.

No new threat flags. The threat register in plan 15-02 (T-15-04 through T-15-08) is fully addressed by the artifacts shipped here:
- T-15-04 (runaway retries from upstream) — mitigated by retryablehttp config + per-host limiter.
- T-15-05 (forbidden dep in future PR) — mitigated by `TestForbiddenDeps_RealGoMod` failing CI on regression.
- T-15-06 (silent "no episodes" masking scrape failure) — mitigated by sentinel errors; future provider implementations are contract-bound to distinguish.
- T-15-07 (Stream DTO leaking iframe URL) — mitigated by reflection-based no-IframeURL guard at the type level.
- T-15-08 (cookie jar persisting cross-anime credentials) — explicitly `accept`'d in the plan; no change here.

## Known Stubs

None. All five concerns ship complete implementations with passing tests — no placeholder values, no TODO/FIXME, no empty data sources.

## TDD Gate Compliance

Each task (1-4) has both RED and GREEN commits in git history per the plan's `tdd="true"` directive:

| Task | RED commit | GREEN commit |
|---|---|---|
| 1 | `08b2b60` test(15-02): add failing tests for scraper domain sentinel errors | `3371582` feat(15-02): implement scraper domain sentinel errors |
| 2 | `c990178` test(15-02): add failing tests for Provider interface + Stream DTO | `996a155` feat(15-02): implement Provider interface + Stream DTO |
| 3 | `be899e3` test(15-02): add failing tests for EmbedExtractor interface + Registry | `2d3eabd` feat(15-02): implement EmbedExtractor interface + Registry |
| 4 | `0bdf29b` test(15-02): add failing tests for BaseHTTPClient | `1270a7a` feat(15-02): implement BaseHTTPClient (retryablehttp + rate.Limiter + cookiejar) |
| 5 | n/a (test-only task) | `9743639` feat(15-02): add CI lint blocking forbidden go.mod dependencies |

Each RED commit was verified to produce a failing build (undefined symbol errors) BEFORE the corresponding GREEN was authored. Task 5 has no separate RED because the lint logic lives inside the test file itself per the plan instructions — there is no production code in `internal/golint/`.

## Self-Check

**File existence:**

- `services/scraper/internal/domain/errors.go` — FOUND
- `services/scraper/internal/domain/errors_test.go` — FOUND
- `services/scraper/internal/domain/provider.go` — FOUND
- `services/scraper/internal/domain/provider_test.go` — FOUND
- `services/scraper/internal/domain/embed.go` — FOUND
- `services/scraper/internal/domain/embed_test.go` — FOUND
- `services/scraper/internal/domain/httpclient.go` — FOUND
- `services/scraper/internal/domain/httpclient_test.go` — FOUND
- `services/scraper/internal/golint/forbidden_deps_test.go` — FOUND

**Commit existence:**

- `08b2b60` — FOUND in `git log`
- `3371582` — FOUND in `git log`
- `c990178` — FOUND in `git log`
- `996a155` — FOUND in `git log`
- `be899e3` — FOUND in `git log`
- `2d3eabd` — FOUND in `git log`
- `0bdf29b` — FOUND in `git log`
- `1270a7a` — FOUND in `git log`
- `9743639` — FOUND in `git log`

## Self-Check: PASSED
