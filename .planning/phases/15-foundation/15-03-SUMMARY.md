---
phase: 15-foundation
plan: 03
subsystem: scraper
tags: [scraper, foundation, orchestrator, embed-extractor, megacloud, http-handlers, golang, tdd]
requires:
  - 15-01 scraper container + chi+zap+metrics middleware (port 8088)
  - 15-02 domain types (Provider, EmbedExtractor, Registry, Stream w/o IframeURL, ErrNotFound/ProviderDown/ExtractFailed, BaseHTTPClient)
provides:
  - services/scraper/internal/embeds/MegacloudClient — first concrete EmbedExtractor, HTTP-wraps the Node sidecar
  - services/scraper/internal/service/Orchestrator — sequential failover with prefer-override + parser_fallback_total metric + HealthSnapshot
  - services/scraper/internal/handler/ScraperHandler — four HTTP handlers (3× 503 stubs + 1 live HealthSnapshot)
  - GET /scraper/episodes (503 stub) — replaced in Phase 16+
  - GET /scraper/servers  (503 stub) — replaced in Phase 16+
  - GET /scraper/stream   (503 stub) — replaced in Phase 16+
  - GET /scraper/health   (200 live snapshot — distinct from /health service liveness)
  - config.MegacloudExtractor{URL, Timeout} (replacing flat MegacloudExtractorURL)
  - MEGACLOUD_EXTRACTOR_TIMEOUT env var (default 15s)
affects:
  - services/scraper/cmd/scraper-api/main.go (full construction order rewrite: registry → MegacloudClient → orchestrator → handler → router)
  - services/scraper/internal/transport/router.go (signature now takes *handler.ScraperHandler; new r.Route("/scraper") block)
  - services/scraper/internal/config/config.go (nested MegacloudExtractor struct, getEnvDuration helper)
  - services/scraper/go.mod (prometheus/client_golang promoted to direct dep for testutil import)
tech-stack:
  added:
    - github.com/prometheus/client_golang/prometheus/testutil (already indirect from libs/metrics; now direct for orchestrator counter assertions)
  patterns:
    - Generic runFailover[T] loop centralizes failover logic across ListEpisodes/ListServers/GetStream (one source of truth for failure classification)
    - failoverDecision() splits errors into terminal (ctx) vs retryable (NotFound/ProviderDown/ExtractFailed) — defensive default of "unknown→retryable" for unknown errors
    - summarizeFailover() prioritizes any non-NotFound failure over an all-NotFound chain (so "everything broken" surfaces as ErrProviderDown, not silent ErrNotFound)
    - Construction order seam in main.go: registry → mc → registry.Register(mc) → orchestrator → handler → router — same order Phase 16 will use to add AnimePahe before constructing the handler
    - Shared metrics.Collector via sync.Once in router_test.go to dodge promauto's global-registry duplicate-registration panic
    - 503 stubs emit raw {error,phase} JSON (not the httputil.Response wrapper) so catalog plan 04's thin client matches on the contract verbatim
key-files:
  created:
    - services/scraper/internal/embeds/megacloud.go (211 lines)
    - services/scraper/internal/embeds/megacloud_test.go (278 lines)
    - services/scraper/internal/service/orchestrator.go (197 lines)
    - services/scraper/internal/service/orchestrator_test.go (404 lines)
    - services/scraper/internal/handler/scraper.go (69 lines)
    - services/scraper/internal/handler/scraper_test.go (197 lines)
    - services/scraper/internal/transport/router_test.go (132 lines)
  modified:
    - services/scraper/internal/transport/router.go (new signature + /scraper/* routes)
    - services/scraper/internal/config/config.go (nested MegacloudExtractor + getEnvDuration)
    - services/scraper/cmd/scraper-api/main.go (orchestrator + embed registry construction)
    - services/scraper/go.mod (prometheus/client_golang promoted to direct)
decisions:
  - MegacloudClient uses an independent http.Client (NOT domain.BaseHTTPClient) because the sidecar is an in-network sibling — per-host rate limit + retry + cookiejar add nothing across docker-compose trust boundary
  - 503 stubs bypass the httputil.Response wrapper so the {error,phase} JSON shape is exactly what the plan documented (catalog plan 04 will match against this verbatim)
  - failoverDecision treats UNKNOWN errors as retryable (rather than terminal) for defense in depth — a future provider that returns a stray error should NOT halt the whole chain
  - Phase 15 HealthSnapshot has NO cache (re-queries on every call); Phase 17 owns the 60s liveness cache + skip-dead-providers behavior
  - Registry stays alphabetically empty (zero providers registered) — the structural change to wire one is the same regardless of whether providers exist; Phase 16's AnimePahe is one orchestrator.Register call away
metrics:
  duration: ~12m (code: ~7m; live deploy + smoke: ~5m)
  completed: 2026-05-11T06:35:00Z
  tasks: 3 (with strict RED→GREEN TDD per task — 6 commits)
  files_created: 7
  files_modified: 4
  tests_added: 25 (8 megacloud + 12 orchestrator + 5 handler + 3 router; many use t.Run subtests so reported count is higher when verbose)
---

# Phase 15 Plan 03: Orchestrator + MegacloudClient + HTTP Handlers Summary

Wire the inside seam of the scraper container: a sequential-failover orchestrator with parser_fallback_total emission, a MegacloudClient HTTP-wrapping the existing Node sidecar registered as the first EmbedExtractor, four HTTP handlers (three returning the canonical 503 not-yet-implemented stub, one returning a live per-provider HealthSnapshot), and updated main.go construction. After this plan: `curl http://localhost:8088/scraper/health` returns `{success:true,data:{providers:{}}}`, `/scraper/{episodes,servers,stream}` return `{error:"not-yet-implemented",phase:15}` at 503, and `make logs-scraper` shows `providers=0 embed_extractors=1` at startup.

## Files Created

| File | Purpose | Lines |
|---|---|---|
| `services/scraper/internal/embeds/megacloud.go` | `MegacloudClient` implementing `domain.EmbedExtractor`. Strict-host match across `megacloud.{tv,blog,club}` + `megaup.{live,cc}` (equality OR `.<known>` suffix). `Extract` HTTP-GETs `<sidecar>/extract?url=<urlencoded embed>`, forwards caller headers (Referer etc.), parses sidecar JSON, translates field names (`url`→Source.URL, `lang`→Track.Label), populates Intro/Outro only when End>0. All errors wrapped as `domain.ErrExtractFailed`. Default 15s timeout matches the sidecar's internal `req.setTimeout(15000)`. | 211 |
| `services/scraper/internal/embeds/megacloud_test.go` | 8 tests (with 12 host-match subtests): Matches positive (5 base hosts + 2 subdomains) / negative (kwik, animepahe, path-substring imposters, non-URL); Extract_Success with field translation; Extract_SidecarError mapping 500+JSON body to wrapped ErrExtractFailed; Extract_SendsURLParam verbatim; Extract_HonorsContextCancel (50ms ctx deadline vs hung sidecar — bails in <200ms); Extract_NoMatchingURL_StillCallsSidecar (Extract doesn't pre-filter); Extract_PassesCallerHeaders; Name lock to "megacloud". Compile-time `var _ domain.EmbedExtractor = (*MegacloudClient)(nil)`. | 278 |
| `services/scraper/internal/service/orchestrator.go` | `Orchestrator` struct + generic `runFailover[T]` loop. Pre-attempt `ctx.Err()` check → bail on cancellation. `failoverDecision` classifies error (ctx errors terminate; NotFound/ProviderDown/ExtractFailed continue; unknown defaults to retryable). On each retryable fall-through emit `parser_fallback_total{from=current_name, to=next_name_or_""}`. `summarizeFailover` returns ErrNotFound only if EVERY provider returned NotFound; otherwise returns the last non-NotFound (so ErrProviderDown wins over a chain of NotFounds). `orderedProviders(prefer)` moves the matching provider to position 0; unknown prefer is silently ignored. `HealthSnapshot` re-queries every call (no Phase 15 cache). | 197 |
| `services/scraper/internal/service/orchestrator_test.go` | 12 contract tests using a configurable `fakeProvider`: zero-providers returns ErrNotFound + empty HealthSnapshot, single-provider passthrough, ProviderDown→success failover (asserts `testutil.ToFloat64(ParserFallbackTotal{A,B}) delta == 1.0`), NotFound→success failover (counter +1), all-down → ErrProviderDown, all-NotFound → ErrNotFound, ctx-cancel short-circuits the loop AND does NOT call subsequent providers (verified via `atomic.LoadInt32(pb.listEpisodeCalls)`), HealthSnapshot re-invokes HealthCheck per call (verified via call counter), PreferPriority moves "B_pref" to front, PreferUnknown ignored, EmbedRegistry accessor returns non-nil. | 404 |
| `services/scraper/internal/handler/scraper.go` | `ScraperHandler` with `GetEpisodes` / `GetServers` / `GetStream` → `notYetImplemented` helper writing raw 503 + `{"error":"not-yet-implemented","phase":15}` with `Content-Type: application/json` (intentionally NOT the httputil.Response wrapper so the contract is exact). `GetHealth` → `httputil.OK(w, map[string]any{"providers": orchestrator.HealthSnapshot(r.Context())})`. | 69 |
| `services/scraper/internal/handler/scraper_test.go` | 5 httptest-driven tests: each 503 stub asserts status + JSON body + content-type; GetHealth returns 200 + `data.providers:{}` for zero providers; GetHealth reflects a fake provider's Health verbatim through JSON round-trip. | 197 |
| `services/scraper/internal/transport/router_test.go` | 3 wiring tests: All four `/scraper/*` routes registered (no 404, correct status per endpoint); `/health` + `/metrics` still work post-refactor; `/scraper/health` body contains `"providers"` (proves we're hitting the orchestrator path, not the service-liveness `/health`). Shared `*metrics.Collector` via `sync.Once` to avoid promauto's global-registry duplicate-registration panic. | 132 |

## Files Modified

| File | Change | Driver |
|---|---|---|
| `services/scraper/internal/transport/router.go` | `NewRouter(*handler.ScraperHandler, *config.Config, *logger.Logger, *metrics.Collector) http.Handler`; added `r.Route("/scraper")` block with episodes/servers/stream/health; kept `/health` (service liveness) + `/metrics` at root | This plan |
| `services/scraper/internal/config/config.go` | Replaced flat `MegacloudExtractorURL` with nested `MegacloudExtractor{URL, Timeout}`; added `getEnvDuration` helper; new `MEGACLOUD_EXTRACTOR_TIMEOUT` env var (default 15s) | This plan |
| `services/scraper/cmd/scraper-api/main.go` | Construction order: `domain.NewRegistry() → embeds.NewMegacloudClient(URL,Timeout) → registry.Register(mc) → service.NewOrchestrator(log, registry) → handler.NewScraperHandler(orchestrator, log) → transport.NewRouter(handler, cfg, log, mc)`. Startup log: `"scraper service ready" port=8088 address=0.0.0.0:8088 providers=0 embed_extractors=1 megacloud_url=<resolved>` | This plan |
| `services/scraper/go.mod` | `github.com/prometheus/client_golang v1.19.0` promoted from indirect to direct (testutil import in orchestrator_test.go) | `go mod tidy` after Task 2 GREEN |

## Commits

| Task | Phase | Hash | Message |
|---|---|---|---|
| 1 | RED | `41f551c` | test(15-03): add failing tests for MegacloudClient (RED) |
| 1 | GREEN | `54a5313` | feat(15-03): implement MegacloudClient HTTP sidecar wrapper (GREEN) |
| 2 | RED | `2bc7d91` | test(15-03): add failing tests for Orchestrator failover (RED) |
| 2 | GREEN | `d512bbd` | feat(15-03): implement Orchestrator with sequential failover (GREEN) |
| 3 | RED | `eece468` | test(15-03): add failing tests for ScraperHandler + router (RED) |
| 3 | GREEN | `6aca3b8` | feat(15-03): wire ScraperHandler + /scraper/* routes + orchestrator (GREEN) |

## Test Count Summary

| Package | Tests | Subtests | Notes |
|---|---|---|---|
| `internal/embeds` | 8 | +12 (host-match table) | All pass in ~60ms |
| `internal/service` | 12 | — | All pass in ~58ms (includes ctx-cancel 50ms wait) |
| `internal/handler` | 5 | — | All pass in ~10ms |
| `internal/transport` | 3 | +4 (route-table) | All pass in ~9ms |
| **Plan 15-03 total** | **28** | **+16** | |
| Plan 15-02 carry-forward | 36 | +6 | Unchanged, all still green |
| Plan 15-01 carry-forward | 2 | — | Unchanged, all still green |
| **Whole scraper module** | **66+ named** | **22+ subtests** | `go test ./...` reports 88 `=== RUN` lines |

## Verification Output

### Unit + integration tests

```text
$ cd services/scraper && go build ./... && go vet ./... && go test ./... -count=1 -timeout 60s
?   	github.com/ILITA-hub/animeenigma/services/scraper/cmd/scraper-api	[no test files]
?   	github.com/ILITA-hub/animeenigma/services/scraper/internal/config	[no test files]
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/domain	0.506s
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds	0.057s
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/golint	0.008s
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/handler	0.010s
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/service	0.058s
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/testharness	0.003s
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/transport	0.008s
```

### Live smoke (production scraper redeployed from this worktree)

```text
$ make redeploy-scraper
[...] Image docker-scraper Built; Container animeenigma-scraper Started

$ curl -is http://localhost:8088/scraper/health
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 41

{"success":true,"data":{"providers":{}}}

$ curl -is http://localhost:8088/scraper/episodes
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
Content-Length: 43

{"error":"not-yet-implemented","phase":15}

$ curl -is http://localhost:8088/scraper/servers
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
Content-Length: 43

{"error":"not-yet-implemented","phase":15}

$ curl -is http://localhost:8088/scraper/stream
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
Content-Length: 43

{"error":"not-yet-implemented","phase":15}

$ curl -is http://localhost:8088/health
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 40

{"success":true,"data":{"status":"ok"}}
```

### Startup log line

```text
animeenigma-scraper | 2026-05-11T06:34:26.139Z INFO  scraper service ready
  {"port": 8088, "address": "0.0.0.0:8088", "providers": 0, "embed_extractors": 1,
   "megacloud_url": "http://megacloud-extractor:3200"}
```

`providers=0 embed_extractors=1` confirmed — MegacloudClient registered, zero business providers, as specified.

### make health

```text
$ make health
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
```

## Deviations from Plan

### 1. [Rule 1 - Plan/code-shape mismatch] Sidecar JSON shape uses `url`/`lang`, not `file`/`label`

- **Found during:** Task 1 (drafting megacloud_test.go)
- **Issue:** The plan's `<interfaces>` block specified the sidecar returned `{sources: [{file, type, isM3U8}], tracks: [{file, label, kind, default}]}`. The actual `docker/megacloud-extractor/server.js` (read at execution time) emits `{sources: [{url, type, isM3U8}], tracks: [{url, lang, default}]}` — no `file`, no `kind`, the subtitle URL key is `url` and the label key is `lang`.
- **Fix:** Built the `sidecarResponse` Go struct to match the ACTUAL sidecar JSON (`url`/`lang`/`default`) and translated to our `domain.Stream` (`Source.URL`, `Track.File`, `Track.Label`, `Track.Kind="captions"`) inside `convertSidecarToStream`. The plan said "the Source type takes `URL string` not `File` — translate the field name", so the spirit of the translation was clear; only the source-of-truth column-name was wrong in the plan's interface doc.
- **Files modified:** `services/scraper/internal/embeds/megacloud.go` (sidecarResponse + sidecarError structs match server.js), `services/scraper/internal/embeds/megacloud_test.go` (canonical test body uses the actual sidecar shape).
- **Commit:** Rolled into Task 1 GREEN `54a5313`.
- **Rationale:** The sidecar is the source of truth; the plan's interface block was documentation drift. Adjusting Go to match the actual JSON is the correct direction — the alternative (changing the sidecar to match the plan) would have required editing JavaScript that ships in production unchanged today, violating D-DEC §2.4 ("keep the sidecar unchanged").

### 2. [Rule 3 - Blocking issue] promauto global-registry panic in transport tests

- **Found during:** Task 3 first run of `go test ./internal/transport/...`
- **Issue:** `metrics.NewCollector("scraper")` registers Prometheus metric collectors against `promauto`'s package-global registry. Calling it twice in the same test binary (once per test that calls `freshTestRouter`) panics with `duplicate metrics collector registration attempted` because the metric NAMES (`http_requests_total` etc.) are fixed across instances regardless of label values. Initial workaround attempted `metrics.NewCollector("scraper-test-" + t.Name())` — still failed because the per-name labels share names.
- **Fix:** Introduced `sync.Once`-guarded `getSharedMC()` so all three router tests share a single `*metrics.Collector` instance. Each test still gets a freshly built chi.Router (which is the unit under test); only the collector is shared.
- **Files modified:** `services/scraper/internal/transport/router_test.go` (added `sharedMC` + `sharedMCOnce` + `getSharedMC()`).
- **Commit:** Rolled into Task 3 GREEN `6aca3b8`.
- **Rationale:** The collector singleton is a property of the prometheus registry, not of our code; the test workaround mirrors what the live service does (one collector per process). Refactoring `metrics.NewCollector` to use a custom registry would be a libs/* change outside the plan's scope.

### 3. [Rule 2 - Added critical coverage] Extra test beyond plan minimum

- **Found during:** Task 1 drafting
- **Issue:** Plan listed 6 named tests for MegacloudClient. Added a 7th (`Extract_PassesCallerHeaders`) to lock the Phase-19 AnimeKai contract: the caller's Referer must reach the sidecar so AnimeKai's `Referer: animekai.to` requirement can be honored without changing MegacloudClient's signature in Phase 19. The test is two lines of setup and asserts something every future caller will rely on.
- **Fix:** Added the test alongside the other RED tests.
- **Commit:** Rolled into Task 1 RED `41f551c`.

### 4. [Rule 2 - Added critical coverage] Extra orchestrator tests for unknown-prefer + embed accessor

- **Found during:** Task 2 drafting
- **Issue:** Plan listed 10 contract tests. Added 2 more:
  - `TestOrchestrator_PreferUnknownIgnored` — explicit guard that an unknown `prefer` string doesn't break the failover loop (a future plan-04 catalog query with a stale-providers preference must not crash the scraper).
  - `TestOrchestrator_EmbedRegistryAccessor` — locks the `EmbedRegistry()` accessor that handler/main.go construction depends on.
- **Fix:** Added both alongside the other RED tests.
- **Commit:** Rolled into Task 2 RED `2bc7d91`.

## Confirmation Items

- [x] `services/scraper/internal/embeds/megacloud.go` exists; `MegacloudClient` implements `domain.EmbedExtractor` (compile-time `var _ EmbedExtractor = (*MegacloudClient)(nil)` in test file).
- [x] Strict host-match policy: `megacloud.tv|.blog|.club` + `megaup.live|.cc` + their subdomains; "megacloud" in PATH does NOT match.
- [x] Sidecar request URL: `<baseURL>/extract?url=<urlencoded embed>` — verified verbatim in `TestMegacloudClient_Extract_SendsURLParam`.
- [x] All Extract errors wrapped as `domain.ErrExtractFailed` (sidecar 500, ctx cancel, transport error, decode error).
- [x] `services/scraper/internal/service/orchestrator.go` exists; sequential failover loop, `parser_fallback_total` increment, `HealthSnapshot` re-queries per call.
- [x] Zero-provider edge case: every business method returns `ErrNotFound`, `HealthSnapshot` returns non-nil empty map — verified.
- [x] Context cancellation short-circuits the loop AND prevents subsequent provider calls — verified via call counter.
- [x] `prefer="B"` moves B to position 0; unknown prefer silently ignored.
- [x] `services/scraper/internal/handler/scraper.go` exists; 3× 503 stubs + 1× live HealthSnapshot.
- [x] 503 stub body shape: top-level `{error,phase}` (raw, NOT wrapped in httputil.Response) — matches the catalog plan 04 thin client contract.
- [x] `/scraper/health` body shape: `httputil.OK` wrapper `{success:true, data:{providers:{...}}}` — preserves the operational JSON envelope.
- [x] `services/scraper/internal/transport/router.go` registers `/scraper/{episodes,servers,stream,health}` via `r.Route("/scraper")` block; keeps `/health` + `/metrics` at root.
- [x] `services/scraper/cmd/scraper-api/main.go` construction order: registry → MegacloudClient → registry.Register(mc) → orchestrator → handler → router.
- [x] Live `curl http://localhost:8088/scraper/health` returns `200` + `{success:true,data:{providers:{}}}`.
- [x] Live `curl http://localhost:8088/scraper/{episodes,servers,stream}` returns `503` + `{"error":"not-yet-implemented","phase":15}`.
- [x] `make logs-scraper` shows `"scraper service ready"` with `providers=0 embed_extractors=1`.
- [x] `make health` reports `✓ scraper:8088` alongside all other services.
- [x] `go build ./services/scraper/...` clean, `go vet ./services/scraper/...` clean, full test suite green.

## Threat Surface Scan

The plan's `<threat_model>` (T-15-09 through T-15-12) is fully addressed:

- **T-15-09 (DoS via MegacloudClient → sidecar)** — MITIGATED. The MegacloudClient's `http.Client.Timeout` is hardcoded-defaulted to 15s (matching the sidecar's own `req.setTimeout(15000)` in server.js). Caller context cancellation propagates via `http.NewRequestWithContext`. Verified by `TestMegacloudClient_Extract_HonorsContextCancel`.
- **T-15-10 (Info disclosure via /scraper/health)** — ACCEPT, as planned. The snapshot returns provider name + per-stage Up/LastOK/LastErr; nothing secret. The endpoint is loopback-bound (`127.0.0.1:8088` per plan 15-01) — internal only.
- **T-15-11 (Tampering: Stream w/ embedded iframe in Sources.URL)** — MITIGATED structurally. Plan 15-02's `TestStream_HasNoIframeURL` reflection guard remains green. `convertSidecarToStream` populates `Source.URL` exclusively with the sidecar's `url` field, which the sidecar only emits for HLS/MP4 (the encryption-decryption path returns the same shape).
- **T-15-12 (DoS via slow provider in failover loop)** — MITIGATED. `runFailover` checks `ctx.Err()` BEFORE every provider call, so a parent-context timeout short-circuits immediately. The per-call timeout still lives in each provider's BaseHTTPClient (plan 15-02, 10s default). Phase 17 will add the 60s liveness cache to skip dead providers entirely.

No new threat surface introduced beyond what the plan documented.

## Known Stubs

Three intentional stubs from the plan body, NOT defects:

| Stub | File | Status | Resolution |
|---|---|---|---|
| `/scraper/episodes` returns 503 | `services/scraper/internal/handler/scraper.go` `GetEpisodes` | Intentional Phase 15 contract | Phase 16+ implements via `orchestrator.ListEpisodes` |
| `/scraper/servers` returns 503 | `services/scraper/internal/handler/scraper.go` `GetServers` | Intentional Phase 15 contract | Phase 16+ implements via `orchestrator.ListServers` |
| `/scraper/stream` returns 503 | `services/scraper/internal/handler/scraper.go` `GetStream` | Intentional Phase 15 contract | Phase 16+ implements via `orchestrator.GetStream` |

Plan goal explicitly: *"After this plan ships: /scraper/{episodes,servers,stream} return HTTP 503 with {error:"not-yet-implemented", phase:15}"*. The 503 is the contract, not a placeholder bug.

The `MegacloudClient` itself is "registered but unused" in Phase 15 — also intentional per the plan ("Phase 19's AnimeKai will use it"). The registration proves the wiring; Phase 16's AnimePahe provider uses Kwik (not megacloud), so the unused state persists through Phase 16-18 before AnimeKai consumes it in Phase 19.

## TDD Gate Compliance

Each of the three tasks has both RED and GREEN commits in git history per the plan's `tdd="true"` directive:

| Task | RED commit | GREEN commit |
|---|---|---|
| 1 (MegacloudClient) | `41f551c` test(15-03): add failing tests for MegacloudClient (RED) | `54a5313` feat(15-03): implement MegacloudClient HTTP sidecar wrapper (GREEN) |
| 2 (Orchestrator) | `2bc7d91` test(15-03): add failing tests for Orchestrator failover (RED) | `d512bbd` feat(15-03): implement Orchestrator with sequential failover (GREEN) |
| 3 (Handler + router) | `eece468` test(15-03): add failing tests for ScraperHandler + router (RED) | `6aca3b8` feat(15-03): wire ScraperHandler + /scraper/* routes + orchestrator (GREEN) |

Each RED commit produced an `undefined: <Symbol>` build failure when run isolated, verified manually before authoring the matching GREEN.

## Self-Check

**File existence:**

- `services/scraper/internal/embeds/megacloud.go` — FOUND
- `services/scraper/internal/embeds/megacloud_test.go` — FOUND
- `services/scraper/internal/service/orchestrator.go` — FOUND
- `services/scraper/internal/service/orchestrator_test.go` — FOUND
- `services/scraper/internal/handler/scraper.go` — FOUND
- `services/scraper/internal/handler/scraper_test.go` — FOUND
- `services/scraper/internal/transport/router_test.go` — FOUND
- `services/scraper/internal/transport/router.go` (modified) — FOUND
- `services/scraper/internal/config/config.go` (modified — nested MegacloudExtractor) — FOUND
- `services/scraper/cmd/scraper-api/main.go` (modified — orchestrator wiring) — FOUND

**Commit existence:**

- `41f551c` — FOUND in `git log`
- `54a5313` — FOUND in `git log`
- `2bc7d91` — FOUND in `git log`
- `d512bbd` — FOUND in `git log`
- `eece468` — FOUND in `git log`
- `6aca3b8` — FOUND in `git log`

**Live verification:**

- `curl http://localhost:8088/scraper/health` → 200 `{success:true,data:{providers:{}}}` — VERIFIED
- `curl http://localhost:8088/scraper/episodes` → 503 `{error:"not-yet-implemented",phase:15}` — VERIFIED
- `curl http://localhost:8088/scraper/servers` → 503 `{error:"not-yet-implemented",phase:15}` — VERIFIED
- `curl http://localhost:8088/scraper/stream` → 503 `{error:"not-yet-implemented",phase:15}` — VERIFIED
- `make logs-scraper` showed `providers=0 embed_extractors=1` startup line — VERIFIED
- `make health` shows `✓ scraper:8088` — VERIFIED

## Self-Check: PASSED
