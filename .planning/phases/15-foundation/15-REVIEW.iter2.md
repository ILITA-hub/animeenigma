---
phase: 15-foundation
reviewed: 2026-05-11T09:00:00Z
depth: standard
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
  critical: 3
  warning: 8
  info: 5
  total: 16
status: issues_found
---

# Phase 15: Code Review Report

**Reviewed:** 2026-05-11T09:00:00Z
**Depth:** standard
**Files Reviewed:** 40
**Status:** issues_found

## Summary

Phase 15 lays solid scaffolding for the new `services/scraper/` microservice with strong type-safety guards (the no-`IframeURL`-on-Stream test is excellent), a forbidden-deps lint, an orchestrator with sequential failover, and a thin client on the catalog side. The shape is appropriate for an extensibility-first foundation.

That said, there are real defects that should be fixed before this code carries production traffic in Phase 16+:

- A **provider-duplication bug** in `orchestrator.orderedProviders` that causes the preferred provider to be invoked twice when failover happens (BLOCKER — present despite passing tests because the existing test only inspects the first call, not the iteration order).
- A **race condition** in `service.Orchestrator.HealthSnapshot` — it iterates `o.providers` under an `RLock` but the provider's `HealthCheck(ctx)` method is called inside the loop while still holding the read lock, blocking `Register` for the duration of every health check (BLOCKER — also amplifies any provider that hangs `HealthCheck`).
- A **dropped error / missing nil-guard** in `MegacloudClient.Extract` for the `io.ReadAll` result (BLOCKER — a transient body read failure becomes a misleading "decode" or "status 0" error).

Several WARNINGs cover Dockerfile / config drift, missing trailing-slash normalization, an unused logger field, and CORS-wildcard exposure. Five INFO items cover dead code, error-message inconsistency, and minor maintainability concerns.

## Critical Issues

### CR-01: `orderedProviders` returns the preferred provider TWICE on failover

**File:** `services/scraper/internal/service/orchestrator.go:65-87`
**Issue:**
When `prefer != ""` matches a registered provider, the function first appends the preferred provider, then iterates `o.providers` again and only skips re-inserting `prefer` while `len(out) == 1`. As soon as a non-prefer provider is appended (advancing `len(out)` to 2), the skip predicate evaluates to false on subsequent iterations, so the preferred provider is appended a *second* time when the loop reaches it again.

Trace with providers `[A, B]`, `prefer="B"`:
1. First loop: `out = [B]`, break.
2. Second loop iteration 1 (`p=A`): `len(out)==1 && A!=prefer` → predicate false → append. `out = [B, A]`.
3. Second loop iteration 2 (`p=B`): `len(out)==2` → predicate false → append. `out = [B, A, B]`.

Consequence: every failover invocation of `runFailover` calls the preferred provider twice. If `prefer` fails (`ErrProviderDown` etc.), the orchestrator tries the next provider, then circles back to `prefer` again — double-billing the upstream, doubling rate-limiter pressure, and emitting an extra `parser_fallback_total{from=A,to=B}` event with misleading semantics.

`TestOrchestrator_PreferPriority` passes only because it just checks `firstCalled == "B_pref"` and never asserts `len(returnedSlice)` or that each provider was called exactly once.

**Fix:**
```go
func (o *Orchestrator) orderedProviders(prefer string) []domain.Provider {
    o.mu.RLock()
    defer o.mu.RUnlock()
    if len(o.providers) == 0 {
        return nil
    }
    out := make([]domain.Provider, 0, len(o.providers))
    preferred := -1
    if prefer != "" {
        for i, p := range o.providers {
            if p.Name() == prefer {
                preferred = i
                out = append(out, p)
                break
            }
        }
    }
    for i, p := range o.providers {
        if i == preferred {
            continue
        }
        out = append(out, p)
    }
    return out
}
```

And add a test that locks in the contract:
```go
func TestOrchestrator_PreferPriority_NoDuplicates(t *testing.T) {
    // ... three providers A, B, C with prefer="B"
    // assert provider B was called EXACTLY once in a failover-exhaustion scenario
}
```

---

### CR-02: `HealthSnapshot` calls provider `HealthCheck` while holding the orchestrator read lock

**File:** `services/scraper/internal/service/orchestrator.go:217-225`
**Issue:**
`HealthSnapshot` takes `o.mu.RLock()` and then calls `p.HealthCheck(ctx)` inside the loop while still holding it. Any provider whose `HealthCheck` does I/O or blocks (Phase 16+ providers will likely cache last upstream calls per stage and may take locks of their own) holds the orchestrator's read lock for the entire snapshot duration. This blocks any concurrent `Register` call (which takes a write lock), and `/scraper/health` is hit by the docker healthcheck (`wget -q --spider http://localhost:8088/health`? no — that's `/health` at the operational level, but the live `/scraper/health` is called by the catalog passthrough on every UI request that hits the scraper health card).

Worse, the orchestrator interface for `Provider.HealthCheck` is documented as "never returns an error — it inspects the in-memory stage cache" (provider.go:119) — i.e. callers ASSUME it's cheap. The lock-holding semantics here mean a future regression in a provider that adds a network call to `HealthCheck` will silently turn into a global service stall.

Additionally, `HealthCheck` panics inside the loop would leave `o.mu` permanently RLock'd because `defer o.mu.RUnlock()` only fires on function exit, but a panic inside the loop's provider call propagates — yes, defer would actually fire on panic, so the lock IS released. So this is not a deadlock-on-panic concern, just a hold-too-long concern.

**Fix:**
Snapshot the provider slice under the lock, release it, then call `HealthCheck`:
```go
func (o *Orchestrator) HealthSnapshot(ctx context.Context) map[string]domain.Health {
    o.mu.RLock()
    providers := make([]domain.Provider, len(o.providers))
    copy(providers, o.providers)
    o.mu.RUnlock()

    out := make(map[string]domain.Health, len(providers))
    for _, p := range providers {
        out[p.Name()] = p.HealthCheck(ctx)
    }
    return out
}
```

The same pattern should be applied to `Names()` and `EmbedRegistry` consumers in future phases. The `domain.Registry.Names()` already does the right thing (allocates the result slice while holding the lock, which is fine because `e.Name()` is required to be side-effect free).

---

### CR-03: `MegacloudClient.Extract` silently discards `io.ReadAll` errors

**File:** `services/scraper/internal/embeds/megacloud.go:155`
**Issue:**
```go
bodyBytes, _ := io.ReadAll(resp.Body)
```
If the sidecar gives us a 200 status but the connection drops mid-body (TCP RST, sidecar OOM-killed mid-response, network blip), `io.ReadAll` returns `(partialBytes, err)`. The current code silently swallows `err`, then attempts `json.Unmarshal(bodyBytes, &sr)` on a truncated body. This produces a misleading `decode sidecar response: unexpected end of JSON input` error wrapped as `ErrExtractFailed`, when the real problem was a network failure that should be observable as `ErrProviderDown` (for upstream incident dashboards).

The same pattern is used on the non-2xx path (`bodyBytes, _ := io.ReadAll(resp.Body)` then attempt JSON parse), which is slightly less critical because the error message will at least contain the HTTP status, but still: a successful read of zero bytes vs a failed read of zero bytes are indistinguishable here.

**Fix:**
```go
bodyBytes, readErr := io.ReadAll(resp.Body)
if readErr != nil {
    // Body read failure is a transport-level issue — surface as ProviderDown
    // (the sidecar IS up, but the network between us was disrupted).
    return nil, domain.WrapProviderDown(readErr, "megacloud: read sidecar response body")
}
```

Also: there is no upper bound on `io.ReadAll`. A misbehaving sidecar that streams gigabytes of garbage will OOM the scraper. Add `io.LimitReader`:
```go
const maxSidecarBody = 2 << 20 // 2 MiB; sidecar responses are <50 KiB in practice
bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSidecarBody))
```

## Warnings

### WR-01: `services/catalog/internal/parser/scraper.Client` does not normalize trailing slash in `baseURL`

**File:** `services/catalog/internal/parser/scraper/client.go:47-57, 108-112`
**Issue:**
Compare with `embeds/megacloud.go:67` which does `strings.TrimRight(baseURL, "/")`. The catalog-side client builds requests as `full := c.baseURL + path` where `path` always starts with `/`. If a deploy ever sets `SCRAPER_API_URL=http://scraper:8088/` (trailing slash — easy to land if a future operator copies the URL from a browser), every request becomes `http://scraper:8088//scraper/episodes`. chi will route that successfully because the double-slash is normalized by net/http's URL parser, but proxies/IDS in between may reject or alert.

**Fix:**
```go
func NewClient(baseURL string, timeout time.Duration) *Client {
    if timeout <= 0 {
        timeout = 15 * time.Second
    }
    return &Client{
        baseURL: strings.TrimRight(baseURL, "/"),
        httpClient: &http.Client{Timeout: timeout},
    }
}
```

---

### WR-02: Scraper service exposes wildcard CORS (`*`) in production router

**File:** `services/scraper/internal/transport/router.go:35`
**Issue:**
```go
r.Use(httputil.CORS([]string{"*"}))
```
The scraper is bound only to `127.0.0.1:8088` (docker-compose maps `127.0.0.1:8088:8088`), so the practical attack surface is limited to in-cluster callers. However, `Access-Control-Allow-Origin: *` is unnecessary because the scraper is never intended to be called directly from a browser — it's a backend-to-backend service called by catalog. Wildcard CORS makes the eventual public-internet exposure (admin Grafana panels, k8s ingress) silently more permissive than it needs to be.

Compare with `services/catalog/...` which uses the same wildcard. The reason it's tolerated on catalog is that catalog *is* called by the browser via the gateway. Scraper is not.

**Fix:**
Drop the CORS middleware entirely, or set the allowed origins to the docker-compose service name only:
```go
// CORS is unnecessary — this service is backend-to-backend only.
// r.Use(httputil.CORS(...))  // intentionally omitted
```

---

### WR-03: `ScraperHandler.log` field is unused; handler falls back to `logger.Default()` on error paths

**File:** `services/scraper/internal/handler/scraper.go:27, 32, 48`
**Issue:**
`ScraperHandler` stores `log *logger.Logger` at construction (line 27), but `notYetImplemented` (line 48) calls `logger.Default().Errorw(...)` instead of using the injected logger. This both makes the field dead and breaks the convention that injected loggers carry request-correlation context (request IDs from `middleware.RequestID`).

**Fix:**
Either remove the field, or use it:
```go
// Pass log via closure or accept as parameter:
func (h *ScraperHandler) notYetImplemented(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusServiceUnavailable)
    body := map[string]any{"error": "not-yet-implemented", "phase": 15}
    if err := json.NewEncoder(w).Encode(body); err != nil {
        h.log.Errorw("failed to encode 503 stub body", "error", err)
    }
}
```

Same issue on `services/catalog/internal/handler/scraper.go:41` (`ScraperEndpointsHandler.log`) — it's wired by `WireScraperEndpoints` but never read in the four handler methods. None of the error paths log anything, so when 500s start happening the operator has no breadcrumb.

---

### WR-04: `scraper.doGET` does not call `io.Copy(io.Discard, resp.Body)` before close on the 503 short-circuit

**File:** `services/catalog/internal/parser/scraper/client.go:124, 130-138`
**Issue:**
`doGET` reads the full body via `io.ReadAll`, then closes via `defer`. That's correct under normal flow. BUT if the body read fails (`readErr != nil`, line 127) and the deferred close runs, the http transport may still have unread bytes in the kernel buffer — preventing connection reuse via keep-alive. This is a connection-pool exhaustion risk under sustained partial-body failures.

Also note that `io.ReadAll` here has no size limit, so a misbehaving scraper can OOM the catalog service the same way CR-03 noted for the sidecar.

**Fix:**
```go
const maxScraperBody = 4 << 20 // 4 MiB
body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxScraperBody))
if readErr != nil {
    // Drain remaining bytes so the keep-alive connection can be reused.
    _, _ = io.Copy(io.Discard, resp.Body)
    return resp.StatusCode, nil, fmt.Errorf("scraper read body: %w", readErr)
}
```

---

### WR-05: `Config.Load` in scraper returns `(cfg, nil)` but the function signature implies it can fail

**File:** `services/scraper/internal/config/config.go:41-53`
**Issue:**
The function `Load() (*Config, error)` always returns `nil` for the error. The caller `main.go:26` then checks `if err != nil { log.Fatalw(...) }` — dead branch today. Worse, the `MEGACLOUD_EXTRACTOR_URL` is documented in the file header as needing to default to the docker-compose service name, but there's no validation that the user didn't accidentally provide an invalid URL (e.g. `MEGACLOUD_EXTRACTOR_URL=megacloud-extractor:3200` without scheme). The downstream `http.NewRequest` would catch it, but the failure surfaces deep in `Extract` rather than at boot.

The `cfg.MegacloudExtractor.URL == ""` warning at `main.go:30` is the right shape — it just needs to be more strict (return an error from Load when a required-for-production var is empty in production; non-prod can rely on the default).

**Fix:**
Either drop the `error` return (simpler), or actually validate:
```go
func Load() (*Config, error) {
    cfg := &Config{...}
    if u := cfg.MegacloudExtractor.URL; u != "" {
        if _, err := url.Parse(u); err != nil {
            return nil, fmt.Errorf("invalid MEGACLOUD_EXTRACTOR_URL %q: %w", u, err)
        }
    }
    return cfg, nil
}
```

---

### WR-06: Scraper Dockerfile copies `libs/tracing/go.mod` but the scraper does not import `libs/tracing`

**File:** `services/scraper/Dockerfile:13`
**Issue:**
The Dockerfile copies `libs/tracing/go.mod` even though `services/scraper/go.mod` has no `require github.com/ILITA-hub/animeenigma/libs/tracing` line and no replace directive for it. `go mod download` from inside `services/scraper/` does not need tracing. The copy is dead overhead at every image build (small, but noisy and misleading — anyone reading the Dockerfile assumes scraper uses tracing).

This is a side-effect of copy-pasting `services/catalog/Dockerfile` as the template. Same applies to `libs/cache`, `libs/database`, `libs/authz`, `libs/idmapping`, `libs/pagination`, `libs/animeparser`, `libs/videoutils` — none of these are required by `services/scraper/go.mod`.

**Fix:**
Either:
(a) Strip the unused COPY lines, leaving only `libs/{errors,httputil,logger,metrics}` plus the rest of the workspace's service `go.mod`s (which are required because `go.work` is loaded);
(b) OR keep the copy as-is but add a one-line comment explaining "go.work requires every workspace member's go.mod to exist at build time, even unused ones".

Option (b) is the safer change because (a) makes the Dockerfile drift from the other services' patterns. Recommend (b).

---

### WR-07: `runFailover` does not distinguish "every provider failed" vs "context cancelled before any provider" in the empty-`errs` branch

**File:** `services/scraper/internal/service/orchestrator.go:144-185`
**Issue:**
The flow:
```go
if len(providers) == 0 {
    return zero, domain.ErrNotFound
}
errs := make([]error, 0, len(providers))
for i, p := range providers {
    if err := ctx.Err(); err != nil {
        return zero, err  // ← terminal, errs may be empty
    }
    ...
}
return zero, summarizeFailover(errs)
```
This is correct today, but the comment on `summarizeFailover` (line 116) says "Empty errs (zero providers) → ErrNotFound" — which is a slightly different meaning than what the code does. Specifically: if `len(providers) > 0` but all providers' calls returned terminal errors (impossible today because only ctx errors are terminal, and ctx errors return early), the comment would still describe correct behavior. But future maintainers who add a new terminal-error category to `failoverDecision` could land a regression where `summarizeFailover` is called with empty `errs` despite non-empty providers.

**Fix:**
Tighten the comment or add an explicit guard:
```go
// summarizeFailover collapses N per-provider errors into a single error.
// PRECONDITION: errs may be empty ONLY when len(providers)==0 — the loop
// is guaranteed to either append to errs or return early on terminal err.
```

Plus an INFO finding (IN-04 below) about adding a debug assertion in `runFailover` for the empty-errs-on-nonempty-providers case.

---

### WR-08: Cross-test global `metrics.ParserFallbackTotal` counter pollution

**File:** `services/scraper/internal/service/orchestrator_test.go:146, 154, 179, 187`
**Issue:**
Tests use `testutil.ToFloat64(metrics.ParserFallbackTotal.WithLabelValues("A_down", "B_ok"))` with `before/after` diff. This works for a single test run because each test uses unique label values (`A_down`, `A_nf`, etc.). But because `metrics.ParserFallbackTotal` is a `promauto.NewCounterVec` registered against the *global* default registry, those counters persist for the lifetime of the process. If `go test -count=N` is ever run (or if a future maintainer adds a test that reuses `A_down`/`B_ok` labels), the `before` value will be non-zero and the assertion `delta == 1.0` will still hold — BUT if the same label is incremented twice in a single test (a real bug, see CR-01), the test silently passes because `delta` is still `1.0` per assertion (the test only checks one increment).

Specifically: with CR-01's bug, `prefer="B"` causes B to be tried twice. If B fails on first attempt and succeeds on second, you would see `parser_fallback_total{from=B, to=A}` AND `parser_fallback_total{from=A, to=B}` both increment — and no current test catches this because no test combines `prefer=X` with a failover scenario that would force the double-call.

**Fix:**
Add a dedicated test that asserts the *total* count of `ParserFallbackTotal` increments equals `len(providers) - 1` (the maximum possible). Using `testutil.CollectAndCount(metrics.ParserFallbackTotal)` and asserting the family size.

Also: consider unregistering the metric in TestMain or using a private registry per test (Prometheus `NewRegistry` + manual collector registration) for stricter isolation. This is a refactor larger than this review can demand, but worth filing as a follow-up.

## Info

### IN-01: `services/scraper/internal/handler/scraper.go` — `r` parameter is unused in three handlers

**File:** `services/scraper/internal/handler/scraper.go:53, 58, 63`
**Issue:**
The three 503-stub handlers (`GetEpisodes`, `GetServers`, `GetStream`) accept `r *http.Request` and never read from it. Go's linter will flag these in some configurations (`unparam`). They will be wired up in Phase 16+, so this is INFO not WARNING.

**Fix:** Either leave as-is and silence linter, or use `_ *http.Request` until Phase 16:
```go
func (h *ScraperHandler) GetEpisodes(w http.ResponseWriter, _ *http.Request) {
    notYetImplemented(w)
}
```

---

### IN-02: `notYetImplemented` is a free function but reads no state — could be a `var`

**File:** `services/scraper/internal/handler/scraper.go:42-50`
**Issue:**
Pre-encoding the body once and serving it via `bytes.NewReader` would be marginally faster (saves one `json.NewEncoder` allocation per 503). Phase 15 traffic is zero, so this is purely cosmetic.

**Fix:**
```go
var notYetImplementedBody = []byte(`{"error":"not-yet-implemented","phase":15}`)

func notYetImplemented(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusServiceUnavailable)
    _, _ = w.Write(notYetImplementedBody)
}
```

This also has the side benefit that the body is byte-exact (no trailing newline from `json.NewEncoder.Encode`), which `TestCatalogHandler_GetScraperEpisodes_BodyExactBytes` (scraper_test.go:255) implicitly cares about.

---

### IN-03: `failoverDecision` returns "unknown" kind for unrecognized errors but treats them as retryable

**File:** `services/scraper/internal/service/orchestrator.go:104-107`
**Issue:**
```go
default:
    // Defensive: unknown error → treat as provider_down for failover.
    return true, "unknown"
```
This is a defensible choice (don't propagate a misclassified error to the user), but it means any provider that returns a non-sentinel error (e.g. a bare `errors.New("oops")`) silently triggers failover with `kind=unknown`. The fallback-metric label `kind` is not part of `parser_fallback_total{from, to}`, but the log line emits `kind=unknown` which dashboards may key on.

If a future provider implementation accidentally returns a non-wrapped error, the failure mode is invisible at the metric level (looks like ProviderDown failover). Add a separate counter `scraper_unclassified_errors_total{provider}` so misuse of the sentinel pattern is observable.

**Fix:**
Optional — add a new metric. At minimum, the log line at `orchestrator.go:174` should include a hint:
```go
if kind == "unknown" {
    log.Warnw("scraper: provider returned non-sentinel error — likely a bug",
        "provider", p.Name(), "error", err.Error())
}
```

---

### IN-04: `runFailover` does not assert `len(errs) > 0` on the summarize-failover path

**File:** `services/scraper/internal/service/orchestrator.go:184`
**Issue:**
As noted in WR-07, if a future maintainer adds a new terminal-error category that *doesn't* return early, `summarizeFailover` could be called with empty `errs` despite non-empty providers — silently returning `ErrNotFound`. Add a debug-build assertion.

**Fix:**
Not blocking. If this becomes a recurring footgun, consider:
```go
if len(providers) > 0 && len(errs) == 0 {
    // Should never happen given current failoverDecision semantics.
    log.Errorw("scraper: failover loop exited without errors — invariant violated")
    return zero, errors.New("scraper: failover invariant violated")
}
return zero, summarizeFailover(errs)
```

---

### IN-05: Test scaffolding — `freshTestRouter` reuses a shared `metrics.Collector` due to promauto global registration

**File:** `services/scraper/internal/transport/router_test.go:18-33`
**Issue:**
The comment correctly explains why the singleton exists (`promauto.NewCounterVec` panics on duplicate registration). However, this means any test that imports both `transport_test` AND another `_test` package using the same metric names would still collide. This is a hidden coupling between test packages.

The right long-term fix is for `libs/metrics.NewCollector` to accept an `*prometheus.Registry` parameter so tests can pass `prometheus.NewRegistry()` and get full isolation. That's a `libs/metrics` change outside Phase 15's scope — note it as future work.

**Fix:** None for this phase. File a follow-up to make `libs/metrics.NewCollector` accept a registry.

---

_Reviewed: 2026-05-11T09:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
