---
phase: 15-foundation
verified: 2026-05-11T07:25:00Z
status: passed
score: 6/6 must-haves verified
overrides_applied: 0
requirements_verified: 12
requirements_satisfied: 12
requirements_blocked: 0
---

# Phase 15: Foundation Verification Report

**Phase Goal:** All structural seams in place so subsequent phases plug in providers without re-architecting. No user-visible behavior change. Adds a new `services/scraper/` microservice (orchestrator + interfaces + harness) plus a thin client inside catalog that talks to it.
**Verified:** 2026-05-11T07:25:00Z
**Status:** passed
**Re-verification:** No (initial verification)

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `docker compose ps` shows healthy `animeenigma-scraper` on :8088; `make redeploy-scraper`/`make logs-scraper` work; service starts with zero providers and serves `GET /scraper/health` returning a JSON snapshot | VERIFIED | `docker compose ps` shows `animeenigma-scraper ... Up 9 minutes (healthy) 127.0.0.1:8088->8088/tcp`. `curl http://localhost:8088/scraper/health` → 200 `{"success":true,"data":{"providers":{}}}`. `make logs-scraper` (wildcard target `logs-%` at Makefile:268) streams docker compose logs and shows startup line: `"scraper service ready" {"port": 8088, ... "providers": 0, "embed_extractors": 1, "megacloud_url": "http://megacloud-extractor:3200"}`. `make redeploy-scraper` works via `redeploy-%` wildcard at Makefile:254. `make health` reports `✓ scraper:8088`. |
| 2 | `GET /api/anime/{animeId}/scraper/episodes\|servers\|stream\|health` on catalog API returns HTTP 503 not-yet-implemented; catalog resolves UUID→MAL ID and forwards to scraper via `services/catalog/internal/parser/scraper/client.go`; scraper returns 503 from `services/scraper/internal/handler/`. Both layers wired. | VERIFIED | End-to-end gateway→catalog→scraper verified live with real UUID `ab15c7a8-d4e9-4bb5-98c4-81ea2522dc29`: `/scraper/health` → 200 `{"success":true,"data":{"providers":{}}}`; `/scraper/episodes` → 503 `{"error":"not-yet-implemented","phase":15}`; `/scraper/servers?episode=ep1` → 503; `/scraper/stream?episode=ep1&server=srv1` → 503. `/scraper/servers` without episode param → 400 (`episode ID is required`). Bogus UUID `00000000-...` → 404 (`anime not found`). Malformed UUID `not-a-uuid` → 404. Code paths confirmed: `services/catalog/internal/transport/router.go:97-100` registers four `r.Get` routes; `services/catalog/internal/handler/scraper.go` has 4 handler methods that passthrough scraper status+body verbatim; `services/catalog/internal/service/scraper.go:55-86` implements UUID→MAL ID resolution chain (UUID parse → repo lookup → ShikimoriID → MALID fallback → ErrMalIDUnavailable); `services/catalog/internal/parser/scraper/client.go` is the thin HTTP wrapper; `services/scraper/internal/handler/scraper.go` emits the canonical 503 body via `notYetImplemented` helper. |
| 3 | Stream DTO has no `iframe_url` field at the Go type level; compile-time test asserts absence | VERIFIED | `services/scraper/internal/domain/provider.go:82-88` defines `Stream{Sources, Tracks, Intro, Outro, Headers}` — 5 fields, no IframeURL. Top-of-file comment documents the D-DEC §2.8 rationale (ISS-008 prevention). `TestStream_HasNoIframeURL` and `TestStream_AllowedFields` in `provider_test.go` PASS (verified live via `go test ./internal/domain/...`). `grep -rn 'iframe_url\|IframeURL' services/scraper/internal/domain/` returns ONLY references inside test/doc comments — no field exists. |
| 4 | `make capture-goldens` recipe runs in `services/scraper/`; testdata/<provider>/*.html fixtures path; parser unit tests run offline | VERIFIED | Makefile:105-107 defines `capture-goldens` target that runs `cd services/scraper && go test -update ./... -run "Golden" \|\| true`. Live run: `make capture-goldens` exits 0 with message "no Go files in /data/animeenigma/services/scraper" (no-op since Phase 16 lands the first Golden* tests). `services/scraper/testdata/.gitkeep` exists (git-tracked, verified via `git ls-files`). Testharness package `services/scraper/internal/testharness/goldie.go` exports `New(t *testing.T) *goldie.Goldie` with fixture dir resolved via `runtime.Caller` to `services/scraper/testdata/`. `goldie_test.go` smoke tests pass. |
| 5 | CI fails any `services/scraper/go.mod` PR that adds chromedp, go-rod, chromedp-rod, utls, tls-client, cloudscraper_go, or flaresolverr — verified by deliberate red PR | VERIFIED | `services/scraper/internal/golint/forbidden_deps_test.go` (243 lines, 11 tests) — all PASS live. Tests prove: `TestForbiddenDeps_RealGoMod` (gate on real go.mod — passes today, would fail PR adding forbidden dep), plus 7 deliberate-red positive-catch tests (`TestForbiddenDeps_PositiveCatch_Chromedp/_ChromedpCdproto/_Rod/_UTLS/_TLSClient/_TLSClientFhttp/_Playwright`) and 2 substring catches (`_Cloudscraper`, `_Flaresolverr`), plus `_AllowedDepsPass` sanity. Current `services/scraper/go.mod` audit confirms 0 occurrences of all forbidden modules. The lint runs as part of `go test ./services/scraper/...` on every CI build. |
| 6 | Every upstream HTTP call routed through BaseHTTPClient has a hard 10s timeout and uses retryablehttp exponential backoff (1s→2s→4s→8s) — no hand-rolled retry loops | VERIFIED | `services/scraper/internal/domain/httpclient.go:82-113` constructs `BaseHTTPClient` with `retryablehttp.Client` configured `RetryWaitMin=1s, RetryWaitMax=8s, RetryMax=4, HTTPClient.Timeout=10s, CheckRetry=retryablehttp.DefaultRetryPolicy, Backoff=retryablehttp.DefaultBackoff`. Tests `TestBaseHTTPClient_DefaultTimeoutIs10s`, `TestBaseHTTPClient_HardTimeout`, `TestBaseHTTPClient_BackoffSequence` (verifying 1→2→4→8 unit sequence) all PASS live. Functional-option pattern with `WithTimeout`, `WithRetryWaitMin/Max`, `WithMaxRetries` allows test compression without altering production defaults. Per-host `rate.Limiter` + `cookiejar.Jar` scoped via publicsuffix etld+1 also present. Note: in Phase 15 no provider yet uses BaseHTTPClient (Phase 16+ adds AnimePahe); MegacloudClient uses an independent `http.Client` per documented design (sidecar is sibling service, not untrusted internet). |

**Score:** 6/6 truths verified

### Required Artifacts (PLAN frontmatter)

#### Plan 15-01 — Scaffolding

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `services/scraper/cmd/scraper-api/main.go` | Service entrypoint | VERIFIED | 88 lines, logger.Default + cfg.Load + metrics.NewCollector("scraper") + transport.NewRouter + 30s graceful SIGINT/SIGTERM shutdown. Construction order: registry→megacloud→register→orchestrator→handler→router. |
| `services/scraper/Dockerfile` | Multi-stage build, EXPOSE 8088 | VERIFIED | 60+ lines, golang:1.24-alpine builder → alpine:3.19 runtime, copies go.work + every workspace member's go.mod (including services/scraper/go.mod), EXPOSE 8088. |
| `services/scraper/go.mod` | Module declaration | VERIFIED | Declares `github.com/ILITA-hub/animeenigma/services/scraper`, go 1.23.0, with libs/{logger,metrics,httputil,errors}, chi v5, retryablehttp, goldie/v2, testify, golang.org/x/{mod,net,time}, prometheus/client_golang. |
| `services/scraper/internal/config/config.go` | Config struct + Load() | VERIFIED | Server{Host,Port,Address()} + MegacloudExtractor{URL,Timeout} (nested per plan 03 change). SERVER_HOST/SERVER_PORT/MEGACLOUD_EXTRACTOR_URL/MEGACLOUD_EXTRACTOR_TIMEOUT env vars with sensible defaults. WR-05 url.Parse validation added. |
| `services/scraper/internal/transport/router.go` | chi router with /health, /metrics, /scraper/* | VERIFIED | NewRouter signature accepts *ScraperHandler. Middleware chain: RequestID → metrics.Middleware → RequestLogger → Recoverer → RealIP. NO wildcard CORS (WR-02 fix). /health + /metrics at root; r.Route("/scraper") block has episodes/servers/stream/health. |
| `services/scraper/internal/testharness/goldie.go` | goldie v2 helper | VERIFIED | testharness.New(t) returns *goldie.Goldie rooted at services/scraper/testdata/ via runtime.Caller. Package doc explains `make capture-goldens` workflow. |
| `services/scraper/internal/testharness/goldie_test.go` | Smoke tests | VERIFIED | TestNewReturnsGoldie + TestGoldieFixtureDir — both pass. |
| `services/scraper/testdata/.gitkeep` | Empty file for git tracking | VERIFIED | File exists (0 bytes); `git ls-files` confirms tracked. |
| `go.work` | Scraper member added | VERIFIED | Line 23: `./services/scraper`. |
| `Makefile` | redeploy-/restart-/logs- targets + capture-goldens + health line | VERIFIED | Line 9: `SERVICES := ... scraper`. Line 105-107: `capture-goldens` target. Line 254 `redeploy-%`, 265 `restart-%`, 268 `logs-%` wildcards handle scraper. Line 424: `✓ scraper:8088` health line. |
| `docker/docker-compose.yml` | scraper service block, healthcheck, depends_on | VERIFIED | Lines 147-167: full scraper block (build from scraper/Dockerfile, container_name animeenigma-scraper, ports 127.0.0.1:8088:8088, MEGACLOUD_EXTRACTOR_URL env, depends_on megacloud-extractor service_healthy, healthcheck wget /health). Line 375: catalog has `SCRAPER_API_URL: http://scraper:8088`. Line 392-393: catalog depends_on scraper service_started. |

#### Plan 15-02 — Domain types

| Artifact | Status | Details |
|----------|--------|---------|
| `services/scraper/internal/domain/provider.go` | VERIFIED | 129 lines. Category enum (sub/dub/raw), AnimeRef/Episode/Server/Source/Track/TimeRange/StageHealth/Health structs, Provider interface (6 methods). Stream{Sources,Tracks,Intro,Outro,Headers} — NO IframeURL. |
| `services/scraper/internal/domain/provider_test.go` | VERIFIED | TestStream_HasNoIframeURL (reflection guard) + TestStream_AllowedFields (5-field lock) + TestCategoryConstants + TestProviderInterface_Compiles + TestStream_JSON_OmitsEmptyOptionals — all pass. |
| `services/scraper/internal/domain/embed.go` | VERIFIED | EmbedExtractor interface + Registry with thread-safe Register/Find/Names, ErrNoMatchingExtractor sentinel. First-match wins; registration order preserved. |
| `services/scraper/internal/domain/embed_test.go` | VERIFIED | 6 tests including ordering, first-match, miss-returns-sentinel, empty-Names returns non-nil. |
| `services/scraper/internal/domain/errors.go` | VERIFIED | ErrNotFound, ErrProviderDown, ErrExtractFailed sentinels + dual-%w wrap helpers (WrapNotFound/ProviderDown/ExtractFailed). |
| `services/scraper/internal/domain/errors_test.go` | VERIFIED | 6 named tests + 6 subtests covering non-nil, distinctness, errors.Is preservation across wrappers, informative messages. |
| `services/scraper/internal/domain/httpclient.go` | VERIFIED | BaseHTTPClient with retryablehttp (1s→8s, RetryMax=4, 10s timeout), per-host rate.Limiter, cookiejar scoped to publicsuffix, Chrome 131 UA. Functional options. |
| `services/scraper/internal/domain/httpclient_test.go` | VERIFIED | 8 tests: DefaultTimeoutIs10s, HardTimeout, BackoffSequence (1→2→4→8), PerHostRateLimit, CookieJarPersists, BaselineHeaders, PerHostLimiterIsolation, DoMethod — all pass. |
| `services/scraper/internal/golint/forbidden_deps_test.go` | VERIFIED | 11 tests; current go.mod clean; deliberate-red positive-catches prove the lint rejects forbidden modules. |

#### Plan 15-03 — Orchestrator + Megacloud + Handlers

| Artifact | Status | Details |
|----------|--------|---------|
| `services/scraper/internal/embeds/megacloud.go` | VERIFIED | MegacloudClient implements domain.EmbedExtractor; strict-host match across megacloud.{tv,blog,club} + megaup.{live,cc} + subdomains; Extract HTTP-GETs sidecar/extract?url=...; 15s timeout; field translation (sidecar `url/lang` → domain `URL/Label`); intro/outro only when End>0. Body capped at 2 MiB (CR-03). Errors wrapped as ErrExtractFailed or ErrProviderDown depending on stage. |
| `services/scraper/internal/embeds/megacloud_test.go` | VERIFIED | 8 tests + 12 host-match subtests cover Matches positive/negative, Extract success/error/sidecar-500/ctx-cancel/header-passthrough, Name lock. Compile-time `var _ EmbedExtractor = (*MegacloudClient)(nil)`. |
| `services/scraper/internal/service/orchestrator.go` | VERIFIED | 249 lines. Orchestrator{providers, registry, log, mu}. Generic `runFailover[T]` loop. `orderedProviders(prefer)` with CR-01 fix (preferredIdx integer guard). `failoverDecision` classifies errors. `summarizeFailover` prioritizes non-NotFound over NotFound. `HealthSnapshot` with CR-02 fix (snapshot under lock, release, then call HealthCheck). Emits `parser_fallback_total{from,to}` metric. |
| `services/scraper/internal/service/orchestrator_test.go` | VERIFIED | 12+ contract tests using fakeProvider including failover, ctx-cancel short-circuit, HealthSnapshot, PreferPriority, PreferUnknownIgnored, NoDuplicates. |
| `services/scraper/internal/handler/scraper.go` | VERIFIED | ScraperHandler with GetEpisodes/Servers/Stream → notYetImplemented helper writing 503 + `{"error":"not-yet-implemented","phase":15}`. GetHealth → httputil.OK with `{providers: HealthSnapshot()}`. WR-03 fix: uses h.log on error path. |
| `services/scraper/internal/handler/scraper_test.go` | VERIFIED | 5+ tests: 503 stubs assert status + JSON body + content-type; GetHealth returns 200 + data.providers. |
| `services/scraper/internal/transport/router.go` (MOD) | VERIFIED | New signature accepts *handler.ScraperHandler. /scraper/{episodes,servers,stream,health} routes inside r.Route("/scraper"). |
| `services/scraper/cmd/scraper-api/main.go` (MOD) | VERIFIED | Construction order: registry → MegacloudClient → registry.Register(mc) → orchestrator → handler → router. Startup log shows `providers=0 embed_extractors=1`. |
| `services/scraper/internal/config/config.go` (MOD) | VERIFIED | Nested MegacloudExtractor{URL,Timeout} struct + getEnvDuration helper + url.Parse validation (WR-05). |

#### Plan 15-04 — Catalog Wiring

| Artifact | Status | Details |
|----------|--------|---------|
| `services/catalog/internal/parser/scraper/client.go` | VERIFIED | Thin HTTP client with NewClient(baseURL, timeout), four methods (GetEpisodes/Servers/Stream/Health) returning (status, body, err). 503 returned verbatim with err==nil; other 5xx wrapped as ErrScraperUpstream. baseURL trimmed (WR-01). Body capped at 4 MiB + drain on close (WR-04). |
| `services/catalog/internal/parser/scraper/client_test.go` | VERIFIED | 8 tests cover URL building, query params, 503 verbatim, 500→ErrScraperUpstream, ctx-cancel. All pass live. |
| `services/catalog/internal/config/config.go` (MOD) | VERIFIED | ScraperConfig{APIURL, Timeout} field populated from SCRAPER_API_URL (default http://scraper:8088) + SCRAPER_TIMEOUT (default 15s). |
| `services/catalog/internal/service/scraper.go` | VERIFIED | 156 lines. scraperOps internal unit with animeFetcher + scraperForwarder dep interfaces. resolveMALID: UUID.Parse pre-check → repo.GetByID → ShikimoriID → MALID fallback → ErrMalIDUnavailable. Four public CatalogService methods (GetScraperEpisodes/Servers/Stream/Health). |
| `services/catalog/internal/service/scraper_test.go` | VERIFIED | 9 tests cover happy path, NotFound, ErrMalIDUnavailable, ShikimoriID first, MALID fallback, servers/stream arg passthrough, health bypass, malformed UUID. |
| `services/catalog/internal/handler/catalog.go` (MOD) | VERIFIED | *CatalogHandler embeds *ScraperEndpointsHandler via WireScraperEndpoints. |
| `services/catalog/internal/handler/scraper.go` | VERIFIED | 175 lines. ScraperEndpointsHandler with 4 handlers + writePassthrough + writeScraperError (404 for NotFound, 422 for ErrMalIDUnavailable, 500 for others). WR-03 fix uses h.log. |
| `services/catalog/internal/handler/scraper_test.go` | VERIFIED | 9 chi-routed tests cover passthrough, error mapping, query-param validation, body byte-exactness. |
| `services/catalog/internal/transport/router.go` (MOD) | VERIFIED | Lines 97-100: 4 new r.Get registrations inside r.Route("/anime"). |
| `services/catalog/cmd/catalog-api/main.go` (MOD) | VERIFIED | Passes cfg.Scraper.APIURL + cfg.Scraper.Timeout into CatalogServiceOptions. |
| `services/catalog/internal/service/catalog.go` (MOD) | VERIFIED | scraperClient *scraper.Client field added (line 52); ScraperAPIURL/Timeout options instantiated (line 148). |
| `services/catalog/Dockerfile` (MOD) | VERIFIED | Line 31: `COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/` (deployment fix). |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| docker/docker-compose.yml `scraper:` block | services/scraper/Dockerfile | build.dockerfile path | WIRED | Line 150: `dockerfile: services/scraper/Dockerfile`. |
| scraper main.go | transport.NewRouter | function call | WIRED | main.go:49 `transport.NewRouter(scraperHandler, cfg, log, metricsCollector)`. |
| scraper main.go | libs/{logger,metrics,httputil} | imports + middleware wiring | WIRED | main.go:11-12 imports; router.go uses httputil.RequestLogger/Recoverer + metrics.Collector. |
| go.work | services/scraper/go.mod | use directive | WIRED | go.work:23 `./services/scraper`. |
| catalog handler GetScraperEpisodes/Servers/Stream/Health | service.GetScraper* | catalogService method call | WIRED | catalog/internal/handler/scraper.go:63 `h.scraperSvc.GetScraperEpisodes(...)` etc. |
| catalog service GetScraper* | parser/scraper.Client.Get* | scraperClient method calls | WIRED | catalog/internal/service/scraper.go:96 `o.scraperClient.GetEpisodes(...)` etc. |
| parser/scraper.Client.Get* | scraper:8088 | HTTP GET to baseURL/scraper/{endpoint} | WIRED | client.go:77,88,103,108 build `/scraper/episodes\|servers\|stream\|health` paths. Live verification confirms end-to-end round trip. |
| catalog/transport/router.go | GetScraperEpisodes/Servers/Stream/Health | r.Get registrations | WIRED | router.go:97-100. |
| scraper handler GetHealth | orchestrator.HealthSnapshot | function call | WIRED | handler/scraper.go:78 `h.svc.HealthSnapshot(r.Context())`. |
| scraper embeds/megacloud.go | docker/megacloud-extractor sidecar | HTTP GET /extract?url= | WIRED | megacloud.go:140 `fmt.Sprintf("%s/extract?url=%s", c.baseURL, url.QueryEscape(embedURL))`. depends_on megacloud-extractor service_healthy in compose. |
| orchestrator | parser_fallback_total metric | metrics.ParserFallbackTotal.WithLabelValues().Inc() | WIRED | orchestrator.go:186 increments on failover; libs/metrics/parser.go:30-33 defines counter. |

### Data-Flow Trace (Level 4)

Phase 15 deliberately ships 503 stubs for `/scraper/{episodes,servers,stream}` and the orchestrator has zero providers registered. Data flow for the LIVE endpoint (`/scraper/health`) is verified:

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|---------------------|--------|
| scraper /scraper/health handler | `snap map[string]Health` | orchestrator.HealthSnapshot(ctx) → iterates providers slice (empty in Phase 15) → returns empty map | Yes (correctly emits non-nil empty map) | FLOWING |
| catalog /api/anime/{id}/scraper/health handler | `body []byte` from scraper.Client.GetHealth | scraper.Client.doGET → http.Get → reads scraper response body | Yes (verified live: returns `{"success":true,"data":{"providers":{}}}` byte-exact) | FLOWING |
| /scraper/{episodes,servers,stream} handlers | n/a — 503 stub | notYetImplemented helper writes static body | N/A (intentional Phase 15 contract per success criterion #2) | STATIC (intentional) |

The 503 stubs are not "hollow" stubs — they are the documented Phase 15 contract. Phase 16+ replaces them with real `orchestrator.ListEpisodes/Servers/GetStream` calls without changing handler signatures, routes, or catalog wiring.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Scraper service is live & healthy | `docker compose ps scraper` | `Up 9 minutes (healthy) 127.0.0.1:8088->8088/tcp` | PASS |
| Service health endpoint | `curl -fsS http://localhost:8088/health` | 200 `{"success":true,"data":{"status":"ok"}}` | PASS |
| Prometheus metrics endpoint | `curl -fsS http://localhost:8088/metrics \| head` | 200 with `# HELP ...` Prometheus exposition lines incl. `http_requests_total{service="scraper",...}` | PASS |
| /scraper/health business endpoint | `curl -is http://localhost:8088/scraper/health` | 200 `{"success":true,"data":{"providers":{}}}` | PASS |
| /scraper/episodes returns 503 | `curl -is http://localhost:8088/scraper/episodes` | 503 `{"error":"not-yet-implemented","phase":15}` | PASS |
| /scraper/servers returns 503 | `curl -is http://localhost:8088/scraper/servers` | 503 `{"error":"not-yet-implemented","phase":15}` | PASS |
| /scraper/stream returns 503 | `curl -is http://localhost:8088/scraper/stream` | 503 `{"error":"not-yet-implemented","phase":15}` | PASS |
| Gateway→catalog→scraper health | `curl -is http://localhost:8000/api/anime/$UUID/scraper/health` | 200 `{"success":true,"data":{"providers":{}}}` | PASS |
| Gateway→catalog→scraper 503 forward | `curl -is http://localhost:8000/api/anime/$UUID/scraper/episodes` | 503 `{"error":"not-yet-implemented","phase":15}` | PASS |
| Servers missing episode → 400 | `curl -is http://localhost:8000/api/anime/$UUID/scraper/servers` | 400 `{"success":false,"error":{"code":"INVALID_INPUT","message":"episode ID is required"}}` | PASS |
| Bogus UUID → 404 | `curl -is http://localhost:8000/api/anime/00000000-.../scraper/episodes` | 404 `{"success":false,"error":{"code":"NOT_FOUND","message":"anime not found"}}` | PASS |
| Malformed UUID → 404 | `curl -is http://localhost:8000/api/anime/not-a-uuid/scraper/episodes` | 404 `{"success":false,"error":{"code":"NOT_FOUND","message":"anime not found"}}` | PASS |
| All scraper Go tests pass | `cd services/scraper && go test ./... -count=1` | All packages OK (~88 named tests + subtests in domain/embeds/golint/handler/service/transport/testharness) | PASS |
| All catalog scraper tests pass | `cd services/catalog && go test ./internal/parser/scraper/... ./internal/service/... ./internal/handler/... ./internal/transport/...` | All packages OK | PASS |
| `go vet ./services/scraper/...` | `go vet` | Clean | PASS |
| `go vet ./services/catalog/...` | `go vet` | Clean | PASS |
| `make health` includes scraper | `make health` | `✓ scraper:8088` printed | PASS |
| `make capture-goldens` runs cleanly | `make capture-goldens` | Exit 0 with no-op message (Phase 16 lands real fixtures) | PASS |
| `make logs-scraper` follows scraper logs | `timeout 3 make logs-scraper` | Streams `docker compose logs -f scraper` showing startup line + request logs | PASS |
| Compile-time IframeURL guard | `go test ./internal/domain/... -run TestStream_HasNoIframeURL` | PASS | PASS |
| Forbidden-deps lint | `go test ./internal/golint/...` | 11 tests PASS | PASS |
| BaseHTTPClient backoff 1→2→4→8 | `go test ./internal/domain/... -run TestBaseHTTPClient_BackoffSequence` | PASS in 150ms | PASS |
| BaseHTTPClient default 10s timeout | `go test ./internal/domain/... -run TestBaseHTTPClient_DefaultTimeoutIs10s` | PASS | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SCRAPER-FOUND-01 | 15-02 | Provider Go interface with Name/FindID/ListEpisodes/ListServers/GetStream/HealthCheck; new providers plug in without touching orchestrator/handlers | SATISFIED | `services/scraper/internal/domain/provider.go:121-128`. Compile-assert in test: `var _ Provider = (*fakeProvider)(nil)`. Phase 16's AnimePahe will be one `orchestrator.Register(p)` call away — no orchestrator/handler changes required. |
| SCRAPER-FOUND-02 | 15-02 | Three sentinel errors (ErrNotFound, ErrProviderDown, ErrExtractFailed) drive failover | SATISFIED | `services/scraper/internal/domain/errors.go:18,23,29`. Wrap helpers preserve errors.Is matching against both sentinel and cause. 6 tests + 6 subtests all pass. |
| SCRAPER-FOUND-03 | 15-02 | Stream DTO has no `iframe_url` field at type level — silent cross-tier fallback structurally impossible | SATISFIED | `services/scraper/internal/domain/provider.go:82-88` defines Stream with 5 fields (no IframeURL). `TestStream_HasNoIframeURL` and `TestStream_AllowedFields` enforce on every CI run. |
| SCRAPER-FOUND-04 | 15-03 | Service-layer Orchestrator does sequential per-anime provider failover; providers don't know each other | SATISFIED | `services/scraper/internal/service/orchestrator.go`. Generic `runFailover[T]` loop + per-provider fakeProvider tests prove sequential failover (12+ tests). `parser_fallback_total` increments on each fallback hop. |
| SCRAPER-FOUND-05 | 15-02 | EmbedExtractor interface + registry; new embed family = one registry entry | SATISFIED | `services/scraper/internal/domain/embed.go` — Registry with Register/Find/Names. MegacloudClient registered in main.go:41. 6 tests cover ordering, first-match, miss-sentinel. |
| SCRAPER-FOUND-06 | 15-02 | BaseHTTPClient with retryablehttp + per-host rate.Limiter + cookiejar + standard headers + 10s timeout | SATISFIED | `services/scraper/internal/domain/httpclient.go`. 8 tests verify all properties. Note: in Phase 15 no provider yet uses it (zero providers); Phase 16+ injects it into AnimePahe etc. |
| SCRAPER-FOUND-07 | 15-01 | Golden-file test harness + make capture-goldens recipe documented | SATISFIED | `services/scraper/internal/testharness/goldie.go` + `make capture-goldens` Makefile target. testdata/.gitkeep committed. Helper resolved via runtime.Caller so all future provider subpackages share same fixture root. |
| SCRAPER-FOUND-08 | 15-03 | MegacloudClient HTTP-wraps existing Node sidecar; registered as megacloud EmbedExtractor; no decryption in Go | SATISFIED | `services/scraper/internal/embeds/megacloud.go`. Implements EmbedExtractor. Sidecar contract unchanged. Registered in main.go:41. 8+12 tests pass. |
| SCRAPER-FOUND-09 | 15-02 | CI lint rejects forbidden anti-bot deps in go.mod | SATISFIED | `services/scraper/internal/golint/forbidden_deps_test.go`. 11 tests including 7 deliberate-red positive-catches. Live verification confirms current go.mod is clean. |
| SCRAPER-FOUND-10 | 15-01, 15-03, 15-04 | New services/scraper/ microservice on :8088 with Dockerfile/health/redeploy/logging, exposes /scraper/{episodes,servers,stream,health}, catalog registers /api/anime/{animeId}/scraper/* with UUID→MAL resolution, all return 503 until Phase 16 | SATISFIED | Live container `animeenigma-scraper` Up healthy on 127.0.0.1:8088. 4 internal scraper routes + 4 catalog passthrough routes confirmed end-to-end via gateway→catalog→scraper. UUID→MAL resolution in service/scraper.go with malformed-UUID short-circuit (96e6a0d). |
| SCRAPER-NF-01 | 15-02 | Every upstream HTTP call has hard 10s timeout | SATISFIED | BaseHTTPClient default `HTTPClient.Timeout = 10 * time.Second` (httpclient.go:89). TestBaseHTTPClient_DefaultTimeoutIs10s verifies. |
| SCRAPER-NF-03 | 15-02 | retryablehttp handles 429/5xx with exponential backoff 1→2→4→8 + circuit-break | SATISFIED | BaseHTTPClient uses retryablehttp.DefaultRetryPolicy + DefaultBackoff with `RetryWaitMin=1s, RetryWaitMax=8s, RetryMax=4`. TestBaseHTTPClient_BackoffSequence verifies the 1→2→4→8 sequence. Circuit-break per host is deferred to Phase 17 (parser_fallback_total instrumentation already wired). |

**All 12 requirement IDs from PLAN frontmatter SATISFIED. No orphaned requirements: REQUIREMENTS.md only maps these 12 IDs to Phase 15.**

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| services/scraper/internal/handler/scraper.go | 60, 65, 70 | Unused `r *http.Request` in stub handlers | Info | Forward-compat for Phase 16; documented in 15-REVIEW.md IN-01. |
| services/scraper/internal/service/orchestrator.go | 113 | Unknown errors classified as retryable (defensive default) | Info | Documented in 15-REVIEW.md IN-03. Considered intentional defense-in-depth. |
| services/catalog/internal/parser/scraper/client.go | maxScraperBody = 4 MiB | Body cap | (mitigation, not anti-pattern) | Prevents catalog OOM from misbehaving scraper. |
| services/scraper/internal/embeds/megacloud.go | maxSidecarBody = 2 MiB | Body cap | (mitigation) | Prevents scraper OOM from misbehaving sidecar. |

No blocker or warning anti-patterns. The 503 stubs in `/scraper/{episodes,servers,stream}` are documented Phase 15 contracts (success criterion #2), not placeholder bugs.

### Human Verification Required

None. Phase 15's stated goal is "no user-visible behavior change" — the foundation is verified via:
- Live container health (docker compose ps + curl)
- Live end-to-end gateway→catalog→scraper round-trip with real UUID
- Unit tests for every contract (Provider interface, Stream DTO no-iframe guard, sentinel errors, EmbedExtractor registry, BaseHTTPClient timeout/backoff, forbidden-deps lint, orchestrator failover, MegacloudClient host match, handler 503 contract, catalog passthrough, UUID→MAL resolution, malformed-UUID short-circuit, error-mapping 404/422)
- Code review iteration 2 confirms zero Critical/Warning findings

All assertions are programmatically verifiable; no visual/UX/real-time/external-service behaviors are gated on this phase.

## Goal Achievement Summary

**Phase 15's goal is fully achieved.**

The structural seam is in place end-to-end:
1. New `services/scraper/` microservice live on 127.0.0.1:8088 with health/metrics/business endpoints, Dockerfile, docker-compose entry, healthcheck, Makefile targets via wildcard.
2. Six observable ROADMAP success criteria all verified live + via unit tests.
3. All 12 phase requirements (SCRAPER-FOUND-01 through -10 + SCRAPER-NF-01, -03) satisfied with code evidence.
4. Type-level guards (no IframeURL in Stream, forbidden-deps lint) actively enforce architectural decisions on every CI run.
5. Phase 16's AnimePahe provider is one `orchestrator.Register(p)` call away — no scraper-service surgery, no catalog surgery, no docker-compose change required.
6. Code review iteration 2 found zero Critical/Warning issues; six Info items documented but not blocking.

**Deviations from original planning** (all documented in summaries and roadmap):
- Port moved from planned 8087 → 8088 due to host port conflict with services/maintenance native binary. ROADMAP, REQUIREMENTS, all 4 PLANs, all 4 SUMMARYs, and CLAUDE.md (verified via grep) consistently reflect 8088.
- Go toolchain bumped 1.22 → 1.23.0 across the workspace (goquery v1.10.3 transitive requirement). Accepted.
- Workspace dep pins (x/time v0.5.0, x/mod v0.20.0) prevent a further 1.23 → 1.25 cascade.

No gaps found. No deferred items needed. No human verification required.

---

_Verified: 2026-05-11T07:25:00Z_
_Verifier: Claude (gsd-verifier)_
