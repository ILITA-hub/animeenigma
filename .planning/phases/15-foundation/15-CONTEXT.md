# Phase 15: Foundation - Context

**Gathered:** 2026-05-11
**Status:** Ready for planning
**Mode:** Smart discuss — infrastructure phase, no grey-area discussion needed

<domain>
## Phase Boundary

All structural seams in place so subsequent phases plug providers in without re-architecting. **No user-visible behavior change.** Adds a new `services/scraper/` Go microservice (orchestrator + interfaces + harness) plus a thin in-catalog HTTP client that calls it. Both layers respond `503 not-yet-implemented` at the end of this phase; Phase 16 swaps the 503s for real responses.

**In scope:**
- New `services/scraper/` service skeleton (`cmd/scraper-api/main.go`, `Dockerfile`, `go.mod`, `internal/{config,domain,handler,service,parser,embeds,transport,observability}/`)
- `docker/docker-compose.yml` entry for the scraper container on port 8087; `make redeploy-scraper` + `make logs-scraper` targets
- Go domain types in `services/scraper/internal/domain/`: `Provider` interface, `Stream` DTO (no `iframe_url` field at the type level), `EmbedExtractor` interface + registry, sentinel errors (`ErrNotFound`, `ErrProviderDown`, `ErrExtractFailed`), `BaseHTTPClient` type
- Orchestrator skeleton in `services/scraper/internal/service/orchestrator.go` (sequential failover, health cache, no providers registered yet)
- `MegacloudClient` Go HTTP wrapper calling `http://megacloud-extractor:3200`; registered as the first `EmbedExtractor`
- Golden-file harness with `make capture-goldens` recipe and `testdata/<provider>/` convention
- CI lint enforcing forbidden `go.mod` additions (`chromedp`, `go-rod`, `chromedp-rod`, `utls`, `tls-client`, `cloudscraper_go`, `flaresolverr`)
- Thin client at `services/catalog/internal/parser/scraper/client.go` — HTTP wrapper around the scraper service
- Catalog handler routes `/api/anime/{animeId}/scraper/{episodes,servers,stream,health}` registered, returning HTTP 503 by forwarding to scraper (which also returns 503)

**Out of scope (for this phase):**
- Any real provider implementation (AnimePahe is Phase 16; 9anime is Phase 18; AnimeKai is Phase 19)
- New EnglishPlayer.vue (Phase 16)
- Liveness probe + per-stage health gauges (Phase 17)
- Deletion of dead `aniwatch` / `consumet-api` containers or `services/catalog/internal/parser/{hianime,consumet}/` (Phase 20)

**Hard constraints carried in from v3.0-DECISIONS.md:**
- Stream DTO has NO `iframe_url` field — compile-time test asserts the absence
- `BaseHTTPClient` mandatory for all upstream calls — 10s timeout, exponential backoff via `hashicorp/go-retryablehttp`, per-host `golang.org/x/time/rate.Limiter`
- No headless browsers, no TLS spoofing, no proxy rotation — CI lint blocks deps

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (this phase is pure infrastructure)

All structural / file-layout / package-layout / lint-rule / makefile-target choices are at Claude's discretion. The decision register at `.planning/v3.0-DECISIONS.md` already locks the architectural choices (microservice + thin client, no iframe field, sequential failover, megacloud sidecar reused, etc.). The discuss phase is not the place to relitigate them.

### Locked elsewhere (do NOT re-decide here)

- **Service boundary:** new `services/scraper/` microservice + thin catalog client. See v3.0-DECISIONS §2.1.
- **Universal abstraction layer:** EmbedExtractor registry + BaseHTTPClient + Provider interface; HTML scraping is per-site. See v3.0-DECISIONS §2.2.
- **No external decryption deps:** keep megacloud-extractor sidecar; AnimeKai's in-house token gen goes inside that sidecar in Phase 19. See v3.0-DECISIONS §2.5.
- **No iframe field in Stream DTO:** type-level enforcement. See v3.0-DECISIONS §2.8.
- **Sequential failover only:** orchestrator is service-layer, providers ignorant of each other. See v3.0-DECISIONS §2.6 / §2.7.
- **Anti-bot non-goals are LOCKED:** no chromedp, no Playwright, no utls, no cloudscraper, no flaresolverr. See v3.0-DECISIONS §5 + §8.
- **TDD cadence required:** failing test → minimum impl → green; one commit pair per requirement.
- **Frontend routes:** `/api/anime/{animeId}/scraper/*` registered on catalog; no gateway change.

### Implementation guidance for the planner

- Follow the existing `services/<name>/` layout from `services/catalog/`, `services/player/`, etc. (CLAUDE.md). Per-service `cmd/<name>-api/main.go`, `internal/{config,domain,handler,service,repo,parser,transport}/`, `Dockerfile`, `go.mod`.
- Reuse `libs/{logger,metrics,errors,cache,httputil}` initialization patterns from `services/catalog/cmd/catalog-api/main.go`.
- For the scraper service's HTTP API (consumed only by catalog's thin client), use a simple internal route shape — e.g. `/scraper/episodes`, `/scraper/servers`, `/scraper/stream`, `/scraper/health`. Internal API need not match the public path.
- The `BaseHTTPClient` should be a struct with `Get(ctx, url) (*http.Response, error)` and `Do(ctx, *http.Request) (*http.Response, error)` style methods, wrapping `retryablehttp.Client` + `rate.Limiter` + `cookiejar.Jar`.
- `EmbedExtractor` interface: `Name() string`, `Matches(embedURL string) bool`, `Extract(ctx, embedURL, headers http.Header) (*Stream, error)`. Registry is a slice traversed in registration order.
- `MegacloudClient`: thin HTTP wrapper around `POST http://megacloud-extractor:3200/extract` with `{"url": embedURL}` body; parses the existing sidecar's response shape. Registered as the first `EmbedExtractor` with `Matches` returning true for `megacloud.*`, `megaup.live`, `megaup.cc`, and the variant hostnames the sidecar currently handles.
- Golden-file harness should use `sebdah/goldie/v2`. The `make capture-goldens` recipe runs `go test -tags=capture -run=Capture` which writes new fixtures to `services/scraper/testdata/<provider>/`. Default `go test` runs against the committed fixtures only.
- CI lint: a small Go test (`go.mod_test.go` or similar) that parses `services/scraper/go.mod` and fails if any forbidden module path is present. Runs as part of normal `go test ./...`.
- Catalog thin client: ~150 LOC at `services/catalog/internal/parser/scraper/client.go`. Mirrors the existing thin-client pattern for megacloud-extractor in `docker/megacloud-extractor/`. Reads `SCRAPER_API_URL` env var; defaults to `http://scraper:8087`.
- Catalog handler routes `/api/anime/{animeId}/scraper/*` resolve the UUID to MAL ID via the existing `animes` table query, then forward to the thin client. Phase 15 just returns 503 — Phase 16 plumbs real responses.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Service skeleton reference:** `services/catalog/cmd/catalog-api/main.go` shows the canonical service entrypoint pattern — config load → logger init → metrics init → DB + Redis init → service construction → router → http.Server with graceful shutdown.
- **HTTP middleware:** `libs/httputil/middleware.go` provides request logging + Prometheus metrics middleware. The scraper service uses these directly.
- **Metrics primitives:** `libs/metrics/parser.go` already defines `ParserRequestsTotal`, `ParserRequestDuration`, `ParserFallbackTotal`. Scraper reuses the same family with `{provider}` labels.
- **Cache wrapper:** `libs/cache/cache.go` (Redis + in-memory fallback). Scraper service uses it for malsync ID lookups + episode lists + stream URLs.
- **ID mapping:** `libs/idmapping/client.go` (ARM client). Catalog already initializes this; scraper service does NOT need ARM directly — it receives `mal_id` already resolved from the thin client.
- **Megacloud sidecar:** `docker/megacloud-extractor/server.js` (248 LOC). Health endpoint at `:3200/health`, extraction at `:3200/extract`. Already proven in production.
- **Errors:** `libs/errors/errors.go` for sentinel patterns (`NotFound`, `Wrap`).

### Established Patterns

- **Auto-migration on startup:** GORM `AutoMigrate()` — but scraper service has NO database access in Phase 15. UUID→MAL resolution stays in catalog.
- **Graceful shutdown:** `http.Server.Shutdown(ctx)` driven by SIGTERM trap (see `services/catalog/cmd/catalog-api/main.go`).
- **Structured logging:** `libs/logger.New(serviceName)` returns a zap sugared logger; scraper service uses `logger.New("scraper")`.
- **Prometheus exposition:** `/metrics` endpoint registered via `promhttp.Handler()`; consistent across all services.
- **Health endpoint:** `/health` returns `{"status":"ok"}`; scraper service follows this convention with extended snapshot fields for per-provider health (populated by Phase 17).

### Integration Points

- **Gateway:** no changes — `/api/anime/*` already routes to catalog:8081.
- **Catalog handler:** new routes in `services/catalog/internal/transport/router.go` registered as siblings of `/kodik/`, `/hianime/`, `/consumet/`, `/animelib/`. Existing pattern at lines 79-92.
- **Catalog service:** new method group (e.g. `GetScraperEpisodes`, `GetScraperServers`, `GetScraperStream`, `GetScraperHealth`) in `services/catalog/internal/service/catalog.go`. Each does UUID→MAL ID lookup + thin-client call.
- **docker-compose.yml:** new `scraper:` block beside `aniwatch:` and `consumet:`. Catalog environment gains `SCRAPER_API_URL=http://scraper:8087`. Megacloud-extractor block unchanged; scraper service depends_on megacloud-extractor.
- **Makefile:** new targets `redeploy-scraper`, `restart-scraper`, `logs-scraper`, mirroring existing per-service targets.

### Files the planner will need to touch

NEW files:
- `services/scraper/cmd/scraper-api/main.go`
- `services/scraper/Dockerfile`
- `services/scraper/go.mod` + `go.sum`
- `services/scraper/internal/config/config.go`
- `services/scraper/internal/domain/provider.go` (interface, Stream DTO, sentinel errors)
- `services/scraper/internal/domain/embed.go` (EmbedExtractor interface + registry)
- `services/scraper/internal/domain/httpclient.go` (BaseHTTPClient)
- `services/scraper/internal/service/orchestrator.go` (skeleton with no providers)
- `services/scraper/internal/embeds/megacloud.go` (Go HTTP wrapper around megacloud-extractor sidecar)
- `services/scraper/internal/handler/scraper.go` (HTTP handlers returning 503 + future-real responses)
- `services/scraper/internal/transport/router.go`
- `services/scraper/internal/testharness/goldie.go` (golden-file scaffold helpers)
- `services/scraper/internal/golint/forbidden_deps_test.go` (CI lint test)
- `services/scraper/testdata/.gitkeep` (provider fixtures land here in Phase 16+)
- `services/catalog/internal/parser/scraper/client.go` (thin client)
- `services/catalog/internal/parser/scraper/client_test.go` (unit tests for thin client)

MODIFIED files:
- `docker/docker-compose.yml` (add scraper service block)
- `services/catalog/cmd/catalog-api/main.go` (wire scraper client into service constructor)
- `services/catalog/internal/service/catalog.go` (new GetScraperEpisodes/Servers/Stream/Health methods)
- `services/catalog/internal/handler/catalog.go` (new handler funcs + 503 response shape)
- `services/catalog/internal/transport/router.go` (new route registrations alongside existing hianime/consumet/animelib)
- `services/catalog/internal/config/config.go` (add SCRAPER_API_URL field)
- `Makefile` (new redeploy-scraper / restart-scraper / logs-scraper targets)
- `go.work` (add `./services/scraper`)

</code_context>

<specifics>
## Specific Ideas

- **Scraper port: 8087.** Next free service port (gateway 8000, auth 8080, catalog 8081, streaming 8082, player 8083, rooms 8084, scheduler 8085, themes 8086 → scraper 8087).
- **Internal API base path:** `/scraper/*` on the scraper service. Catalog's public path `/api/anime/{id}/scraper/*` translates to internal `/scraper/{episodes,servers,stream,health}` with `mal_id` query param.
- **MegacloudClient HTTP timeout:** 15 seconds — embed decryption can take longer than the standard 10s upstream timeout because the sidecar does multiple internal hops (fetch embed page → fetch key → decrypt).
- **Forbidden go.mod deps (final list):** `github.com/chromedp/chromedp`, `github.com/chromedp/cdproto`, `github.com/go-rod/rod`, `github.com/refraction-networking/utls`, `github.com/bogdanfinn/tls-client`, `github.com/bogdanfinn/fhttp`, `github.com/playwright-community/playwright-go`. Test enumerates these as a static slice.
- **Initial test fixture:** Phase 15 ships at least one no-op fixture in `testdata/.gitkeep` so the directory exists in git. Phase 16 captures the first real golden.
- **Stream DTO compile-time check:** a test in `services/scraper/internal/domain/provider_test.go` uses `reflect.TypeOf(Stream{})` to iterate fields and assert none are named `IframeURL` or carry json tag `iframe_url`. Fails the build if anyone adds the field.

</specifics>

<canonical_refs>
## Canonical References (downstream agents MUST read)

- `/data/animeenigma/.planning/v3.0-DECISIONS.md` — Locked decisions register (signed off, authoritative)
- `/data/animeenigma/.planning/REQUIREMENTS.md` — All SCRAPER-FOUND-01..10 requirements + SCRAPER-NF-01/03
- `/data/animeenigma/.planning/ROADMAP.md` — Phase 15 goal + 6 success criteria
- `/data/animeenigma/.planning/research/SUMMARY.md` — Research synthesizer's findings (architecture, stack, provider order)
- `/data/animeenigma/.planning/research/STACK.md` — Library version pins, rejected libs
- `/data/animeenigma/.planning/research/ARCHITECTURE.md` — Provider interface + orchestrator sketch
- `/data/animeenigma/.planning/research/PITFALLS.md` — 12 pitfalls, esp. §1 (silent fallback / Kodik trap), §4 (decryption fragility), §6 (parallel fan-out ban)
- `/data/animeenigma/CLAUDE.md` — Project conventions (Go service layout, naming, deployment, lib usage)
- `/data/animeenigma/services/catalog/cmd/catalog-api/main.go` — Reference for service entrypoint pattern
- `/data/animeenigma/docker/megacloud-extractor/server.js` — Sidecar the MegacloudClient wraps
- `/root/.claude/projects/-data-animeenigma/memory/feedback_animelib_no_kodik_fallback.md` — Why DTO has no iframe field
- `/root/.claude/projects/-data-animeenigma/memory/feedback_replace_dont_preserve.md` — Why clean break, no legacy preservation

</canonical_refs>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. The 7-area scope (microservice skeleton + interfaces + harness + lint + thin client + routes + compose-entry) is fully covered by the locked requirements.

Out-of-phase items already documented in REQUIREMENTS.md "Future Requirements" / "Out of Scope" tables.

</deferred>
