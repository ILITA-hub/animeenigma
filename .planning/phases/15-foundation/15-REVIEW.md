---
phase: 15-foundation
reviewed: 2026-05-11T09:30:00Z
depth: standard
iteration: 2
files_reviewed: 40
files_reviewed_list:
  - services/scraper/Dockerfile
  - services/scraper/cmd/scraper-api/main.go
  - services/scraper/go.mod
  - services/scraper/internal/config/config.go
  - services/scraper/internal/domain/embed.go
  - services/scraper/internal/domain/embed_test.go
  - services/scraper/internal/domain/errors.go
  - services/scraper/internal/domain/errors_test.go
  - services/scraper/internal/domain/httpclient.go
  - services/scraper/internal/domain/httpclient_test.go
  - services/scraper/internal/domain/provider.go
  - services/scraper/internal/domain/provider_test.go
  - services/scraper/internal/embeds/megacloud.go
  - services/scraper/internal/embeds/megacloud_test.go
  - services/scraper/internal/golint/forbidden_deps_test.go
  - services/scraper/internal/handler/scraper.go
  - services/scraper/internal/handler/scraper_test.go
  - services/scraper/internal/service/orchestrator.go
  - services/scraper/internal/service/orchestrator_test.go
  - services/scraper/internal/testharness/goldie.go
  - services/scraper/internal/testharness/goldie_test.go
  - services/scraper/internal/transport/router.go
  - services/scraper/internal/transport/router_test.go
  - services/catalog/Dockerfile
  - services/catalog/cmd/catalog-api/main.go
  - services/catalog/internal/config/config.go
  - services/catalog/internal/handler/catalog.go
  - services/catalog/internal/handler/scraper.go
  - services/catalog/internal/handler/scraper_test.go
  - services/catalog/internal/parser/scraper/client.go
  - services/catalog/internal/parser/scraper/client_test.go
  - services/catalog/internal/service/catalog.go
  - services/catalog/internal/service/scraper.go
  - services/catalog/internal/service/scraper_test.go
  - services/catalog/internal/transport/router.go
  - services/catalog/internal/transport/scraper_routes_helpers_test.go
  - services/catalog/internal/transport/scraper_routes_test.go
  - Makefile
  - docker/docker-compose.yml
  - go.work
findings:
  critical: 0
  warning: 0
  info: 6
  total: 6
status: clean
---

# Phase 15: Code Review Report (Iteration 2)

**Reviewed:** 2026-05-11T09:30:00Z
**Depth:** standard
**Files Reviewed:** 40
**Status:** clean (no Critical/Warning; six Info items, five carried over from iter1 as expected)

## Summary

This is the **post-fix re-review** of Phase 15 (Foundation). All 11 Critical/Warning findings from iter1 were fixed in 11 atomic commits and the fixes have been verified end-to-end:

- **CR-01** — `orderedProviders` no longer double-counts the preferred provider. The new `preferredIdx` integer guard is correct under every traced case (`prefer=""`, `prefer="unknown"`, `prefer=A` with N providers, all providers fail). The new `TestOrchestrator_PreferPriority_NoDuplicates` test locks the contract by asserting each of A/B/C is called exactly once in a failover-exhaustion run.
- **CR-02** — `HealthSnapshot` snapshots the provider slice under the RLock, releases it, then invokes `HealthCheck`. The orchestrator can no longer be stalled by a slow provider health check.
- **CR-03** — `MegacloudClient.Extract` captures the `io.ReadAll` error and surfaces it as `ErrProviderDown`. The body is now bounded by `io.LimitReader(resp.Body, 2 << 20)` and the defer drains unread bytes before `Close()` so keep-alive remains usable.
- **WR-01** — `parser/scraper.NewClient` trims trailing slashes on `baseURL`.
- **WR-02** — Scraper router no longer registers wildcard CORS.
- **WR-03** — Both `ScraperHandler.log` (scraper service) and `ScraperEndpointsHandler.log` (catalog service) are now used on error paths.
- **WR-04** — `parser/scraper.Client.doGET` caps the body at 4 MiB and drains on defer.
- **WR-05** — `scraper/config.Load` validates `MEGACLOUD_EXTRACTOR_URL` (parse + scheme + host).
- **WR-06** — Scraper Dockerfile keeps the full set of lib `COPY`s with an inline comment explaining the `go.work` requirement.
- **WR-07** — `summarizeFailover` doc comment now states the empty-`errs` invariant explicitly.
- **WR-08** — `TestOrchestrator_FailoverFallbackTotalIncrementCount` asserts total fallback increments == `len(providers) - 1`.

Adversarial re-trace of every fix found no regressions and no new Critical/Warning issues. All scraper + catalog scraper-related tests pass with `-count=1` (cache disabled). `go vet` is clean.

Five Info findings (IN-01..IN-05) carry over from iter1 as expected (iter1 deferred Info items by policy `fix_scope: critical_warning`). One additional Info finding (IN-06) was introduced by the CR-03 fix itself: the `Extract` godoc still claims "Errors are always wrapped as domain.ErrExtractFailed" but the fix added an `ErrProviderDown`-wrapped body-read path. This is documentation drift, not a code bug — orchestrator failover behavior is identical for either sentinel.

Status: **clean**. No Critical or Warning findings. Phase 15 is ready to ship.

## Info

### IN-01: `services/scraper/internal/handler/scraper.go` — unused `r *http.Request` in three stub handlers

**File:** `services/scraper/internal/handler/scraper.go:60, 65, 70`
**Issue:** Carried over from iter1. The three 503-stub handlers (`GetEpisodes`, `GetServers`, `GetStream`) accept `r *http.Request` and never read it. Phase 16+ will wire them, so this is correct as a forward-compatible signature; some linters (`unparam`) will flag it.

**Fix:** Optional — rename to `_ *http.Request` until Phase 16:
```go
func (h *ScraperHandler) GetEpisodes(w http.ResponseWriter, _ *http.Request) { h.notYetImplemented(w) }
```

---

### IN-02: `notYetImplemented` re-encodes the body on every 503

**File:** `services/scraper/internal/handler/scraper.go:45-57`
**Issue:** Carried over from iter1. Pre-encoding the canonical 503 body once and serving via `w.Write` would save one `json.NewEncoder` allocation per request. Phase 15 has zero production traffic, so the savings are theoretical.

**Fix:** Optional micro-optimization:
```go
var notYetImplementedBody = []byte(`{"error":"not-yet-implemented","phase":15}`)
func (h *ScraperHandler) notYetImplemented(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusServiceUnavailable)
    _, _ = w.Write(notYetImplementedBody)
}
```

This also has the side benefit of producing byte-exact output (no trailing newline from `json.NewEncoder.Encode`), which `TestCatalogHandler_GetScraperEpisodes_BodyExactBytes` implicitly cares about.

---

### IN-03: `failoverDecision` classifies unknown errors as retryable with `kind="unknown"`

**File:** `services/scraper/internal/service/orchestrator.go:111-114`
**Issue:** Carried over from iter1. A provider returning a non-sentinel error (e.g. bare `errors.New("oops")`) silently triggers failover with `kind=unknown`. The log line emits `kind=unknown` but there is no dedicated counter, so dashboards can't distinguish "real ProviderDown" from "provider returned an unwrapped error (bug)".

**Fix:** Optional — add a dedicated counter (`scraper_unclassified_errors_total{provider}`) or upgrade the log level to `Errorw` so misuse of the sentinel pattern is observable.

---

### IN-04: `runFailover` does not assert `len(errs) > 0` on the summarize-failover path

**File:** `services/scraper/internal/service/orchestrator.go:198`
**Issue:** Carried over from iter1. The current code is correct, but the WR-07 fix has now documented the invariant in `summarizeFailover`'s comment. Adding a defensive assertion in `runFailover` would catch a future maintainer regression at runtime.

**Fix:** Optional defensive check before `summarizeFailover`:
```go
if len(providers) > 0 && len(errs) == 0 {
    return zero, errors.New("scraper: failover invariant violated")
}
```

---

### IN-05: Test scaffolding — `freshTestRouter` shares a global `metrics.Collector` across tests

**File:** `services/scraper/internal/transport/router_test.go:23-33`
**Issue:** Carried over from iter1. The shared `sharedMC` works around `promauto.NewCounterVec`'s "duplicate metric collector registration" panic but couples test packages: any future `_test` package that registers the same metric names would still collide.

**Fix:** Out-of-scope follow-up — refactor `libs/metrics.NewCollector` to accept an `*prometheus.Registry` so tests can pass `prometheus.NewRegistry()` for full isolation.

---

### IN-06: `MegacloudClient.Extract` godoc comment is stale after the CR-03 fix

**File:** `services/scraper/internal/embeds/megacloud.go:132-138`
**Issue:** **New in iter2** — introduced by the CR-03 fix. The comment block above `Extract` still reads:

> Errors are always wrapped as domain.ErrExtractFailed so the orchestrator failover loop can match via errors.Is(err, domain.ErrExtractFailed).

This is now incorrect: the body-read failure path (line 174) wraps as `ErrProviderDown`, not `ErrExtractFailed`. The orchestrator's `failoverDecision` treats both as retryable so the live failover behavior is unchanged, but a future Phase-16 provider implementation that translates `EmbedExtractor` errors using the docstring as a contract would mis-classify body-read failures.

**Fix:** Tighten the docstring:
```go
// Errors are wrapped with a domain sentinel so the orchestrator failover
// loop can match via errors.Is:
//   - transport / body-read failures → domain.ErrProviderDown
//   - sidecar 5xx / decode / shape mismatch → domain.ErrExtractFailed
// Both sentinels are retryable from the orchestrator's POV; the distinction
// drives the `kind` label on parser_fallback_total and the upstream
// incident-dashboard signal.
```

---

_Reviewed: 2026-05-11T09:30:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Iteration: 2_
