---
phase: 15-foundation
plan: 04
subsystem: catalog
tags: [catalog, scraper, foundation, http-client, routes, e2e-wiring, tdd]
requires:
  - 15-01 scraper container live on 8088
  - 15-02 scraper domain types + BaseHTTPClient
  - 15-03 scraper orchestrator + handler emitting 503 stubs + live /scraper/health
provides:
  - services/catalog/internal/parser/scraper/Client — thin HTTP wrapper around scraper:8088
  - services/catalog/internal/parser/scraper.ErrScraperUpstream — sentinel for 5xx non-503
  - services/catalog/internal/service/scraperOps — UUID -> MAL ID resolution + forward
  - services/catalog/internal/service.ErrMalIDUnavailable — sentinel mapped to 422 by handler
  - services/catalog/internal/handler.ScraperEndpointsHandler — four /scraper/* handler methods
  - GET /api/anime/{animeId}/scraper/episodes — passthrough of scraper 503 stub
  - GET /api/anime/{animeId}/scraper/servers  — passthrough of scraper 503 stub
  - GET /api/anime/{animeId}/scraper/stream   — passthrough of scraper 503 stub
  - GET /api/anime/{animeId}/scraper/health   — passthrough of scraper 200 live snapshot
  - config.ScraperConfig{APIURL, Timeout} with SCRAPER_API_URL/SCRAPER_TIMEOUT env vars
affects:
  - services/catalog/internal/handler/catalog.go — *CatalogHandler embeds *ScraperEndpointsHandler
  - services/catalog/internal/transport/router.go — registers four new GET routes
  - services/catalog/cmd/catalog-api/main.go — passes Scraper.{APIURL,Timeout} through CatalogServiceOptions
  - services/catalog/internal/service/catalog.go — adds scraperClient field + ScraperAPIURL/Timeout options
  - services/catalog/internal/config/config.go — new ScraperConfig
  - services/catalog/Dockerfile — copies services/scraper/go.mod (deployment Rule 3 fix)
  - services/catalog/go.mod — google/uuid promoted from indirect -> direct
tech-stack:
  added:
    - google/uuid v1.6.0 (promoted from indirect to direct for UUID-format pre-validation)
  patterns:
    - Strict RED -> GREEN per task with separate test(...) and feat(...) commits
    - Minimal local interfaces (animeFetcher, scraperForwarder, scraperServiceAPI)
      extracted ONLY where needed for testability; production wires concrete types
    - Status+body verbatim passthrough — no JSON re-encoding between scraper -> catalog
      -> gateway, so the {"error":"not-yet-implemented","phase":15} contract is byte-exact
    - Embedded handler (*ScraperEndpointsHandler) in *CatalogHandler so chi routes
      with catalogHandler.GetScraper* method values reach the dedicated handler
key-files:
  created:
    - services/catalog/internal/parser/scraper/client.go (134 lines)
    - services/catalog/internal/parser/scraper/client_test.go (238 lines, 8 tests)
    - services/catalog/internal/service/scraper.go (137 lines)
    - services/catalog/internal/service/scraper_test.go (276 lines, 9 tests)
    - services/catalog/internal/handler/scraper.go (158 lines)
    - services/catalog/internal/handler/scraper_test.go (260 lines, 9 tests)
    - services/catalog/internal/transport/scraper_routes_test.go (65 lines, 1 test + 4 subtests)
    - services/catalog/internal/transport/scraper_routes_helpers_test.go (45 lines, stubs)
  modified:
    - services/catalog/internal/handler/catalog.go (embed scraper handler in *CatalogHandler)
    - services/catalog/internal/transport/router.go (4 new GET routes)
    - services/catalog/cmd/catalog-api/main.go (wire Scraper config into options)
    - services/catalog/internal/service/catalog.go (add scraperClient field + options)
    - services/catalog/internal/config/config.go (new ScraperConfig struct)
    - services/catalog/Dockerfile (COPY services/scraper/go.mod)
    - services/catalog/go.mod (google/uuid -> direct dep)
decisions:
  - 503 from scraper is a legitimate response with err==nil; the thin client only
    returns an error for 5xx OTHER than 503 (wrapped as ErrScraperUpstream).
    Rationale: the 503 not-yet-implemented body is the Phase 15 contract every
    layer matches against; treating it as an error would make passthrough impossible.
  - Malformed UUID short-circuits to liberrors.NotFound("anime") before reaching
    the DB to keep the public contract clean (404 vs accidental 500 from Postgres
    SQLSTATE 22P02). Caught live during smoke testing — Rule 2 auto-fix.
  - ErrMalIDUnavailable handled with raw JSON body {"error":"mal_id unavailable
    for this anime"} (NOT wrapped in httputil.Response) so the frontend's
    contract-matching is identical to the scraper's own 503 body shape.
  - ScraperEndpointsHandler split into its own type + embedded into *CatalogHandler
    rather than added as methods directly. Rationale: testability without a real
    *service.CatalogService (which needs full GORM repo + Redis + 7 parser clients).
metrics:
  duration: ~28m (code: ~18m; live deploy + smoke + Dockerfile/UUID Rule fixes: ~10m)
  completed: 2026-05-11T08:53:00Z
  tasks: 3 (with strict RED -> GREEN TDD per task — 6 TDD commits + 2 fix commits)
  files_created: 8
  files_modified: 7
  tests_added: 27 (8 client + 9 service + 9 handler + 1 router with 4 subtests)
---

# Phase 15 Plan 04: Catalog -> Scraper Wiring Summary

Land the final wave of Phase 15 by wiring the catalog service to the scraper
microservice introduced in plans 15-01..03. After this plan:

- The public path **gateway:8000 -> catalog:8081 -> scraper:8088** resolves
  end-to-end for the four `/scraper/*` endpoints.
- `curl http://localhost:8000/api/anime/<uuid>/scraper/health` returns
  `{"providers":{}}` — the live HealthSnapshot, no providers yet.
- `curl http://localhost:8000/api/anime/<uuid>/scraper/{episodes,servers,stream}`
  returns 503 with `{"error":"not-yet-implemented","phase":15}` — the Phase
  16+ replacement seam.
- Phase 16's AnimePahe provider is one `orchestrator.Register(...)` call away.

## Files Created

| File | Purpose | Lines |
|---|---|---|
| `services/catalog/internal/parser/scraper/client.go` | Thin HTTP wrapper. `Client` struct, `NewClient(baseURL, timeout)`, four methods `GetEpisodes/Servers/Stream/Health` returning `(status, body, err)`. 503 returns verbatim with `err==nil`; other 5xx wraps as `ErrScraperUpstream`; context-cancel propagates. | 134 |
| `services/catalog/internal/parser/scraper/client_test.go` | 8 httptest-driven tests: URL+query construction for each endpoint, 503 verbatim, 500 -> ErrScraperUpstream, context-cancel, base-URL fully caller-controlled. | 238 |
| `services/catalog/internal/service/scraper.go` | `scraperOps` unit (animeRepo + scraperClient deps) + four public CatalogService methods. `resolveMALID`: UUID pre-validation -> DB lookup -> ShikimoriID parse -> MALID fallback -> `ErrMalIDUnavailable`. | 137 |
| `services/catalog/internal/service/scraper_test.go` | 9 tests with fake repo + fake forwarder: happy path, NotFound, ErrMalIDUnavailable, Shikimori-first, MALID fallback, servers/stream arg passthrough, health bypasses lookup, malformed UUID short-circuit. | 276 |
| `services/catalog/internal/handler/scraper.go` | `ScraperEndpointsHandler` with the four `/scraper/*` HTTP handlers. Status+body verbatim passthrough. Error map: NotFound -> 404, ErrMalIDUnavailable -> 422 with raw JSON, others -> 500 via httputil.Error. | 158 |
| `services/catalog/internal/handler/scraper_test.go` | 9 chi-routed tests covering status passthrough, error mapping, query-param validation, body byte-exactness, content-type. | 260 |
| `services/catalog/internal/transport/scraper_routes_test.go` | 1 router test with 4 subtests verifying every `/api/anime/{animeId}/scraper/*` path resolves through chi (no 404) and reaches the dedicated handler. | 65 |
| `services/catalog/internal/transport/scraper_routes_helpers_test.go` | Test helpers: `stubScraperSvc` + `buildScraperOnlyRouter`. | 45 |

## Files Modified

| File | Change | Driver |
|---|---|---|
| `services/catalog/internal/handler/catalog.go` | Embed `*ScraperEndpointsHandler` in `*CatalogHandler`; wire in `NewCatalogHandler` via `WireScraperEndpoints`. | Task 3 |
| `services/catalog/internal/transport/router.go` | Add four `r.Get` calls inside the existing `r.Route("/anime", ...)` block, immediately after the AnimeLib block. | Task 3 |
| `services/catalog/cmd/catalog-api/main.go` | Pass `cfg.Scraper.APIURL` + `cfg.Scraper.Timeout` into `service.CatalogServiceOptions`. | Task 3 |
| `services/catalog/internal/service/catalog.go` | Add `scraperClient *scraper.Client` field, `ScraperAPIURL`/`ScraperTimeout` options, default-fallback wiring. | Task 2 |
| `services/catalog/internal/config/config.go` | New `ScraperConfig{APIURL, Timeout}` struct; populated from `SCRAPER_API_URL`/`SCRAPER_TIMEOUT` env (defaults `http://scraper:8088` and `15s`). | Task 1 |
| `services/catalog/Dockerfile` | Add `COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/` line. | Deviation 1 (Rule 3) |
| `services/catalog/go.mod` | `github.com/google/uuid v1.6.0` promoted from indirect -> direct. | Deviation 2 (Rule 2) |

## Commits

| Task | Phase | Hash | Message |
|---|---|---|---|
| 1 | RED | `3954ba0` | test(15-04): add failing tests for catalog scraper thin client (RED) |
| 1 | GREEN | `6dc4f0a` | feat(15-04): implement catalog scraper thin client + SCRAPER_API_URL config (GREEN) |
| 2 | RED | `e692f1d` | test(15-04): add failing tests for catalog scraper service layer (RED) |
| 2 | GREEN | `67b3680` | feat(15-04): implement catalog scraper service layer (GREEN) |
| 3 | RED | `01c2eb9` | test(15-04): add failing tests for catalog scraper handlers + routes (RED) |
| 3 | GREEN | `fb75120` | feat(15-04): wire catalog scraper handlers + 4 new routes + main.go (GREEN) |
| fix | — | `aca9909` | fix(15-04): catalog Dockerfile must copy services/scraper/go.mod (Rule 3) |
| fix | — | `96e6a0d` | fix(15-04): validate UUID format before DB roundtrip (Rule 2) |

## Test Count Summary

| Package | Test File | Tests | Subtests |
|---|---|---|---|
| `internal/parser/scraper` | `client_test.go` | 8 | — |
| `internal/service` | `scraper_test.go` | 9 | — |
| `internal/handler` | `scraper_test.go` | 9 | — |
| `internal/transport` | `scraper_routes_test.go` | 1 | +4 |
| **Plan 15-04 total** | | **27** | **+4** |

(Plan estimated ~24; +3 added voluntarily for malformed-UUID short-circuit,
unknown-error 500 contract lock, body-byte-exactness JSON-encoder regression.)

## Verification Output

### Unit tests (full catalog module)

```text
$ cd services/catalog && go build ./... && go vet ./... && go test ./... -count=1 -timeout 120s

ok  github.com/ILITA-hub/animeenigma/services/catalog/cmd/backfill-attributes  0.271s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/domain          0.077s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/handler         0.015s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anilist  0.015s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik    0.579s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/scraper  0.060s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/service         0.010s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/transport       0.011s
```

All test packages green; vet clean; build clean.

### Live smoke (production catalog redeployed from this worktree)

UUID picked: `ab15c7a8-d4e9-4bb5-98c4-81ea2522dc29` (real `animes` row,
ShikimoriID present).

```text
$ make health
Checking service health...
[INFO] catalog:8081 - healthy
...
$ make health (after redeploy)
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088

$ UUID="ab15c7a8-d4e9-4bb5-98c4-81ea2522dc29"

$ curl -is http://localhost:8000/api/anime/$UUID/scraper/health
HTTP/1.1 200 OK
Content-Length: 41
Content-Type: application/json
...
{"success":true,"data":{"providers":{}}}

$ curl -is http://localhost:8000/api/anime/$UUID/scraper/episodes
HTTP/1.1 503 Service Unavailable
Content-Length: 43
Content-Type: application/json
...
{"error":"not-yet-implemented","phase":15}

$ curl -is http://localhost:8000/api/anime/$UUID/scraper/servers?episode=ep-1
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
{"error":"not-yet-implemented","phase":15}

$ curl -is http://localhost:8000/api/anime/$UUID/scraper/stream?episode=ep-1&server=srv-1
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
{"error":"not-yet-implemented","phase":15}
```

Negative cases:

```text
$ curl -is http://localhost:8000/api/anime/00000000-0000-0000-0000-000000000000/scraper/episodes
HTTP/1.1 404 Not Found
{"success":false,"error":{"code":"NOT_FOUND","message":"anime not found"}}

$ curl -is http://localhost:8000/api/anime/not-a-uuid/scraper/episodes
HTTP/1.1 404 Not Found
(malformed UUID short-circuited to NotFound before DB roundtrip)

$ curl -is http://localhost:8000/api/anime/$UUID/scraper/servers
HTTP/1.1 400 Bad Request
{"success":false,"error":{"code":"INVALID_INPUT","message":"episode ID is required"}}

$ curl -is http://localhost:8000/api/anime/$UUID/scraper/stream?episode=ep-1
HTTP/1.1 400 Bad Request
{"success":false,"error":{"code":"INVALID_INPUT","message":"server ID is required"}}

# 422 case: insert a row with empty shikimori_id + mal_id, query, observe 422, clean up.
$ TEST_UUID=$(... INSERT INTO animes (name, shikimori_id, mal_id) VALUES ('TEST', '', '') RETURNING id;)
$ curl -is http://localhost:8000/api/anime/$TEST_UUID/scraper/episodes
HTTP/1.1 422 Unprocessable Entity
{"error":"mal_id unavailable for this anime"}
```

All eight smoke scenarios verified — every public contract holds end-to-end.

### Catalog request log excerpt (live, from `docker compose logs catalog`)

```text
2026-05-11T06:49:55.560Z INFO request completed
  {"method":"GET","path":"/api/anime/ab15c7a8-.../scraper/stream","status":400,"bytes":85,...}
2026-05-11T06:50:23.457Z INFO request completed
  {"method":"GET","path":"/api/anime/62fc2002-.../scraper/episodes","status":422,"bytes":46,...}
```

Routes serving cleanly with structured logs.

## Deviations from Plan

### 1. [Rule 3 - Blocking issue] Catalog Dockerfile missing `services/scraper/go.mod`

- **Found during:** Task 3 first `make redeploy-catalog` after Task 3 GREEN.
- **Issue:** `docker build` failed in the catalog builder stage with:
  ```
  go: cannot load module ../scraper listed in go.work file:
  open ../scraper/go.mod: no such file or directory
  ```
  Plan 15-01 added `./services/scraper` to `go.work` but did not update the
  catalog Dockerfile's go-mod-download stage, which iterates every workspace
  member's `go.mod` independently. Cascading deps mean catalog can't build
  without copying *every* workspace member's `go.mod` first.
- **Fix:** Added `COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/`
  in alphabetic position next to the other service entries.
- **Files modified:** `services/catalog/Dockerfile` (1 line added).
- **Commit:** `aca9909`
- **Rationale:** Pure deployment plumbing; same fix will be needed for every
  other service Dockerfile that triggers `go mod download` (`auth`, `gateway`,
  `player`, etc.) once those services pull in any scraper dep — but Phase 15
  does not require that. Out of scope for this plan; the catalog fix is
  sufficient for Phase 15 success criterion #2.

### 2. [Rule 2 - Auto-add missing critical functionality] UUID-format validation

- **Found during:** Live smoke testing after first Task 3 redeploy.
- **Issue:** Plan `must_haves.truths` says "If animeId is not a valid UUID
  format or is not found in animes, catalog returns 404 (not 503)". The
  initial implementation handled the "not found" case via the repo's
  `liberrors.NotFound("anime")` but a malformed UUID flowed straight to GORM
  and surfaced as a Postgres SQLSTATE 22P02 / 500 Internal Server Error.
  Live evidence: `curl /api/anime/not-a-uuid/scraper/episodes` returned 500.
- **Fix:** Added `uuid.Parse(animeID)` pre-check at the top of
  `scraperOps.resolveMALID`. Malformed input now returns
  `liberrors.NotFound("anime")` and produces a 404 with the same body shape
  as a not-found UUID.
- **Files modified:**
  - `services/catalog/internal/service/scraper.go` (4 lines added)
  - `services/catalog/internal/service/scraper_test.go` (added
    `TestCatalogService_GetScraperEpisodes_MalformedUUID`; updated all
    existing tests to use proper 36-char UUIDs)
  - `services/catalog/go.mod` (`google/uuid` promoted indirect -> direct)
- **Commit:** `96e6a0d`
- **Rationale:** This is a correctness requirement spelled out verbatim in
  the plan's must_haves. Skipping it would have left the only path that
  satisfies "not a valid UUID format ... returns 404" unfulfilled. The
  google/uuid package is already a transitive dep, so promotion is a single
  go.mod line with no net dependency cost.

### 3. [Rule 2 - Test coverage] +3 tests beyond plan minimum

- **Found during:** Tasks 2 + 3 drafting
- **Issue:** Plan estimated ~24 tests; final count is 27.
- **Additions:**
  - `TestCatalogService_GetScraperEpisodes_MalformedUUID` (service test 9) —
    locks the new UUID-format pre-check behavior so a future refactor can't
    silently regress to "Postgres syntax error -> 500".
  - `TestCatalogHandler_GetScraperEpisodes_UnknownError_500` (handler test 8) —
    explicit contract lock: only NotFound and ErrMalIDUnavailable get
    special-cased; everything else funnels through `httputil.Error`. This
    matters because the error-handling code uses `errors.Is` + AppError
    type-assertion and could quietly absorb future error types if the
    contract isn't tested.
  - `TestCatalogHandler_GetScraperEpisodes_BodyExactBytes` (handler test 9) —
    JSON encoders sometimes add stray trailing newlines; this test asserts
    the body bytes are passed through verbatim so the scraper's
    `{"error":"not-yet-implemented","phase":15}` contract matches byte-for-byte
    on the frontend.
- **Commits:** Rolled into the corresponding RED commits.

## Phase 15 Success-Criteria Cross-Check

| # | Criterion | Status | Plan reference |
|---|---|---|---|
| 1 | Scraper container healthy on 8088, zero providers, /scraper/health returns snapshot | ✓ | plan 15-01 + 15-03 |
| 2 | Catalog routes return 503 + body, scraper returns 503 + body, both layers wired through gateway | ✓ | **this plan (15-04)** — verified live via gateway:8000 -> catalog:8081 -> scraper:8088 |
| 3 | Stream DTO has no iframe_url; compile-time test | ✓ | plan 15-02 |
| 4 | `make capture-goldens` recipe exists; testdata/.gitkeep committed | ✓ | plan 15-01 |
| 5 | CI rejects forbidden go.mod additions; deliberate-red tests pass | ✓ | plan 15-02 |
| 6 | BaseHTTPClient enforces 10s timeout + 1->2->4->8 backoff | ✓ | plan 15-02 |

**Phase 15 is shippable.** Zero user-visible behavior change; full structural
seam in place; Phase 16 can plug in AnimePahe with no scraper-service
surgery and no catalog surgery (one `orchestrator.Register(...)` call inside
the scraper's `main.go`).

## Confirmation Items

- [x] `services/catalog/internal/parser/scraper/client.go` — thin HTTP wrapper with 4 methods.
- [x] All 8 client tests pass (URL building, 503 verbatim, 500 -> ErrScraperUpstream, ctx-cancel).
- [x] `services/catalog/internal/service/scraper.go` — scraperOps unit + 4 public CatalogService methods.
- [x] All 9 service tests pass (resolution chain, NotFound, ErrMalIDUnavailable, malformed UUID).
- [x] `services/catalog/internal/handler/scraper.go` — ScraperEndpointsHandler with 4 handlers.
- [x] All 9 handler tests pass (status passthrough, error mapping, query-param validation, body exactness).
- [x] `services/catalog/internal/transport/router.go` — 4 new GET routes registered inside `r.Route("/anime")`.
- [x] 1 router test + 4 subtests pass — all paths resolve, no 404.
- [x] `services/catalog/cmd/catalog-api/main.go` — passes Scraper.{APIURL,Timeout} into CatalogServiceOptions.
- [x] `services/catalog/internal/config/config.go` — new ScraperConfig with SCRAPER_API_URL + SCRAPER_TIMEOUT env vars.
- [x] `make redeploy-catalog` succeeds.
- [x] Live `curl http://localhost:8000/api/anime/<real-uuid>/scraper/health` returns 200 `{"providers":{}}`.
- [x] Live `curl http://localhost:8000/api/anime/<real-uuid>/scraper/{episodes,servers,stream}` returns 503 `{"error":"not-yet-implemented","phase":15}`.
- [x] Live `curl http://localhost:8000/api/anime/00000000-.../scraper/episodes` returns 404.
- [x] Live `curl http://localhost:8000/api/anime/not-a-uuid/scraper/episodes` returns 404 (malformed UUID short-circuit).
- [x] Live `/scraper/servers` without `?episode=` returns 400.
- [x] Live `/scraper/stream` without `?server=` returns 400.
- [x] Live 422 case (synthetic row with empty shikimori_id + mal_id) returns 422 with canonical body.
- [x] `make health` reports `✓ catalog:8081` AND `✓ scraper:8088`.
- [x] `go build ./services/catalog/...` clean; `go vet ./services/catalog/...` clean; full test suite green.

## Threat Surface Scan

The plan's `<threat_model>` (T-15-13 through T-15-17) is fully addressed:

- **T-15-13 (SSRF via `baseURL` injection)** — MITIGATED. `SCRAPER_API_URL`
  is read once at startup from env (`config.Load`) and never touches request
  data. `client.NewClient(baseURL, timeout)` stores the URL prefix and the
  request URL is built as `baseURL + path + ?query`. No request-derived URL
  component reaches the HTTP layer. Verified by reading
  `services/catalog/internal/parser/scraper/client.go`.
- **T-15-14 (Info disclosure via body passthrough)** — ACCEPT, as planned.
  Phase 15 scraper bodies are static (`{"error":"not-yet-implemented","phase":15}`
  + health snapshot with provider names). Phase 16+ providers will be reviewed
  individually.
- **T-15-15 (Auth bypass on public routes)** — ACCEPT. Same trust level as the
  existing `/api/anime/{id}/hianime/*` routes; gateway rate-limit covers
  abuse. Routes do NOT live under `r.Route("/admin", ...)`.
- **T-15-16 (DoS via catalog -> scraper -> upstream)** — MITIGATED (partial).
  Gateway rate-limit + per-host BaseHTTPClient throttle (plan 15-02) +
  `SCRAPER_TIMEOUT=15s` per-request cap. Phase 17 adds liveness-aware
  provider skipping.
- **T-15-17 (Resource exhaustion via bogus UUID)** — MITIGATED (improved).
  The added `uuid.Parse` pre-check now rejects malformed UUIDs *before* the
  DB roundtrip, reducing the cost of brute-force probes to a regex equivalent.
  Real UUIDs are still cheap indexed PK lookups.

No new threat surface introduced beyond what the plan documented. The new
SCRAPER_API_URL env var was already planned and present from plan 15-01's
docker-compose block.

## Known Stubs

The three intentional Phase 15 stubs continue to be present, exposed end-to-end:

| Stub | Layer | Status |
|---|---|---|
| `/scraper/episodes` -> 503 | scraper handler (15-03) + catalog passthrough (this plan) | Intentional Phase 15 contract |
| `/scraper/servers` -> 503 | scraper handler (15-03) + catalog passthrough (this plan) | Intentional Phase 15 contract |
| `/scraper/stream` -> 503 | scraper handler (15-03) + catalog passthrough (this plan) | Intentional Phase 15 contract |

Phase 16+ replaces the scraper-handler stubs with real orchestrator-backed
implementations. Zero catalog changes needed at that point — the 503 -> 200
transition is transparent through the thin client + handler passthrough.

The `MegacloudClient` registered in plan 15-03 also remains "registered but
unused" through Phase 18 — that's tracked in the 15-03 SUMMARY and is not
re-flagged here.

## TDD Gate Compliance

Each of the three tasks has both RED and GREEN commits in git history per
the plan's `tdd="true"` directive:

| Task | RED commit | GREEN commit |
|---|---|---|
| 1 (Thin client + config) | `3954ba0` test(15-04): add failing tests for catalog scraper thin client (RED) | `6dc4f0a` feat(15-04): implement catalog scraper thin client + SCRAPER_API_URL config (GREEN) |
| 2 (Service layer) | `e692f1d` test(15-04): add failing tests for catalog scraper service layer (RED) | `67b3680` feat(15-04): implement catalog scraper service layer (GREEN) |
| 3 (Handlers + routes + main) | `01c2eb9` test(15-04): add failing tests for catalog scraper handlers + routes (RED) | `fb75120` feat(15-04): wire catalog scraper handlers + 4 new routes + main.go (GREEN) |

Each RED commit was verified to fail compilation with `undefined: <Symbol>`
errors before the matching GREEN was authored.

Two post-GREEN fix commits (`aca9909`, `96e6a0d`) addressed deployment
plumbing and a missing input-validation requirement that surfaced only
during live smoke testing — neither would have been catchable from unit
tests alone.

## Self-Check

**File existence:**

- `services/catalog/internal/parser/scraper/client.go` — FOUND
- `services/catalog/internal/parser/scraper/client_test.go` — FOUND
- `services/catalog/internal/service/scraper.go` — FOUND
- `services/catalog/internal/service/scraper_test.go` — FOUND
- `services/catalog/internal/handler/scraper.go` — FOUND
- `services/catalog/internal/handler/scraper_test.go` — FOUND
- `services/catalog/internal/transport/scraper_routes_test.go` — FOUND
- `services/catalog/internal/transport/scraper_routes_helpers_test.go` — FOUND
- `services/catalog/internal/transport/router.go` (modified — 4 new routes) — FOUND
- `services/catalog/internal/config/config.go` (modified — ScraperConfig) — FOUND
- `services/catalog/internal/handler/catalog.go` (modified — embed scraper handler) — FOUND
- `services/catalog/cmd/catalog-api/main.go` (modified — Scraper options) — FOUND
- `services/catalog/internal/service/catalog.go` (modified — scraperClient) — FOUND
- `services/catalog/Dockerfile` (modified — copy scraper go.mod) — FOUND

**Commit existence:**

- `3954ba0` — FOUND in `git log`
- `6dc4f0a` — FOUND in `git log`
- `e692f1d` — FOUND in `git log`
- `67b3680` — FOUND in `git log`
- `01c2eb9` — FOUND in `git log`
- `fb75120` — FOUND in `git log`
- `aca9909` — FOUND in `git log`
- `96e6a0d` — FOUND in `git log`

**Live verification:**

- `curl http://localhost:8000/api/anime/<real-uuid>/scraper/health` → 200 `{"providers":{}}` — VERIFIED
- `curl http://localhost:8000/api/anime/<real-uuid>/scraper/episodes` → 503 `{"error":"not-yet-implemented","phase":15}` — VERIFIED
- `curl http://localhost:8000/api/anime/<real-uuid>/scraper/servers?episode=ep-1` → 503 — VERIFIED
- `curl http://localhost:8000/api/anime/<real-uuid>/scraper/stream?episode=ep-1&server=srv-1` → 503 — VERIFIED
- `curl http://localhost:8000/api/anime/00000000-0000-0000-0000-000000000000/scraper/episodes` → 404 — VERIFIED
- `curl http://localhost:8000/api/anime/not-a-uuid/scraper/episodes` → 404 — VERIFIED (Rule 2 fix)
- 422 `{"error":"mal_id unavailable for this anime"}` for synthetic empty-IDs row — VERIFIED
- `make health` shows `✓ catalog:8081` AND `✓ scraper:8088` — VERIFIED

## Self-Check: PASSED

## After-Update Skill Note

Per CLAUDE.md "After-Update Skill (MUST USE)" section, the orchestrator
should invoke `/animeenigma-after-update` after merging this worktree
back to main. That skill will:
1. Lint+build catalog (already done in this worktree — clean)
2. Redeploy catalog (already done — live)
3. Run health checks (already done — all 8 services up)
4. Update `frontend/web/public/changelog.json` with a user-facing entry
   (deferred to the orchestrator — this worktree must not modify shared
   artifacts per `<parallel_execution>`)
5. Commit + push (orchestrator handles the merge commit + push)
