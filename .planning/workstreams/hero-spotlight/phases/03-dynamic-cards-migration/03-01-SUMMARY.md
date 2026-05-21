---
phase: 03-dynamic-cards-migration
plan: 01
subsystem: api
tags: [hero-spotlight, player-service, internal-endpoint, gorm-index, chi-router, docker-network-trust, phase-3]

# Dependency graph
requires:
  - phase: 01-backend-aggregator
    provides: "Phase 1 spotlight aggregator + Card type + Resolver interface — Phase 3 plugs new resolvers into the same constructor"
  - phase: 02-frontend-hero-spotlight
    provides: "HeroSpotlightBlock.vue cardComponent lookup map — extended by Phase 3 for 5 new card components"
provides:
  - "GET /internal/users/{user_id}/list?status=watching,planned,postponed (player service, no JWT, outside /api)"
  - "domain.InternalListItem JSON contract consumed by catalog spotlight aggregator"
  - "ListService.GetUserListByStatusesWithProgress (anime_list ⋈ animes ⟕ watch_progress in one query)"
  - "GORM idx_watch_progress_updated_at index tag on WatchProgress.UpdatedAt (HSB-NF-02)"
affects:
  - "03-02 (optional-auth middleware) — needs the JWT-derived user_id flow to reach this endpoint"
  - "03-03 (5 resolvers) — not_time_yet + continue_watching_new call this endpoint via player_client.go"
  - "03-04 (gateway router test) — defense-in-depth assertion that /internal/* is NOT proxied"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Player /internal/* endpoint mounted outside /api with no AuthMiddleware (docker-network trust boundary)"
    - "Handler-package-internal interface for service substitution in tests (listInternalService)"
    - "Bare-JSON response envelope (not libs/httputil.OK) for cross-service internal contracts"
    - "Source-order structural assertion via runtime.Caller(0) + os.ReadFile when chi exposes no introspection API"

key-files:
  created:
    - "services/player/internal/handler/list_internal.go"
    - "services/player/internal/handler/list_internal_test.go"
    - "services/player/internal/service/list_internal_test.go"
    - "services/player/internal/domain/watch_index_test.go"
    - "services/player/internal/transport/router_internal_list_test.go"
    - "services/player/internal/transport/router_test_helpers_test.go"
  modified:
    - "services/player/internal/domain/watch.go"
    - "services/player/internal/service/list.go"
    - "services/player/internal/repo/list.go"
    - "services/player/internal/transport/router.go"
    - "services/player/cmd/player-api/main.go"

key-decisions:
  - "Single endpoint with multi-status CSV filter (not two endpoints) — matches HSB-BE-24/25 phrasing and the spotlight resolvers only ever fan out once per card"
  - "Status filter allow-list lives in the HANDLER, not the service — keeps the service-layer query agnostic to the spotlight resolver's roster"
  - "Bare-JSON {\"items\":[...]} response (not libs/httputil.OK envelope) — catalog player_client.go consumer parses the bare shape directly, matches Phase 1 spotlight handler divergence-3 discipline"
  - "Two handler constructors: NewInternalListHandler (concrete *service.ListService for prod) + NewInternalListHandlerFromService (interface for test fakes) — avoids dragging the heavyweight service-construction graph into the transport test binary"
  - "Source-order structural test (`TestRouter_RouteOrder_InternalBeforeAPI`) reads router.go text directly — chi exposes no introspection API so behavioral 404 tests are insufficient guards against a future refactor that nests /internal under /api"
  - "Shared metrics.Collector via sync.Once in test helpers — promauto registers histograms globally and duplicate construction panics with 'duplicate metrics collector registration attempted'"

patterns-established:
  - "Internal endpoint pattern: `if internalXHandler != nil { r.Get(\"/internal/...\", ...) }` mounted before r.Route(\"/api\"...) so nil-handler boots cleanly and gateway never proxies /internal/*"
  - "Handler-internal narrow interface (listInternalService) declared in the handler package — production service satisfies by structural typing, tests substitute fakes without import cycles"
  - "In-memory SQLite test setup with custom UDF registration (to_char shim for Postgres-only SQL) — allows production SQL text to execute unchanged in unit tests"

requirements-completed:
  - HSB-BE-26
  - HSB-NF-02

# Metrics
duration: ~25 min
completed: 2026-05-21
---

# Phase 03 Plan 01: Player Internal Endpoint + GORM Index Summary

**`GET /internal/users/{user_id}/list?status=...` exposed on the player service (no auth, outside /api) and `idx_watch_progress_updated_at` GORM tag added on `WatchProgress.UpdatedAt` — unblocks the 5 Phase 3 spotlight resolvers and the live `now_watching` SQL.**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-05-21T05:05:00Z (approx)
- **Completed:** 2026-05-21T05:30:15Z
- **Tasks:** 3
- **Files modified:** 5 (4 source + 1 main.go wiring)
- **Files created:** 6 (1 handler + 5 test files)

## Accomplishments

- **Internal endpoint live in the player binary:** `GET /internal/users/{user_id}/list?status=watching,planned,postponed` — mounted as a peer of `/health` and `/metrics`, OUTSIDE `r.Route("/api", ...)`, with NO `AuthMiddleware`. Returns `{ "items": [InternalListItem...] }` containing per-row anime metadata + `last_watched_episode` from a single joined query.
- **JOINed read in one round-trip:** `ListService.GetUserListByStatusesWithProgress` runs a raw parameterised query over `anime_list ⋈ animes ⟕ watch_progress` so the catalog resolvers do not fan out per anime. `LIMIT 200` row cap bounds the join (T-03-03 mitigation).
- **HSB-NF-02 GORM index tag:** `WatchProgress.UpdatedAt` now carries `gorm:"index:idx_watch_progress_updated_at"`. AutoMigrate creates the dedicated B-tree on next player restart so the Phase 3 `now_watching` resolver's `WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'` predicate hits an index instead of degrading to a sequential scan as `watch_progress` grows.
- **15 new unit tests** all passing under `-race`: 2 domain (reflect-based contract pins), 5 service (empty-statuses short-circuit, ORDER BY recency, LEFT JOIN missing progress → 0, user-id filter, status filter), 7 handler (happy path, no-status, unknown filtered out, unknown-only, missing user_id, service error, whitespace-trim), 5 router (mount at root, /api shadow 404, no-auth reaches handler, nil-handler 404 guard, source-order assertion).

## Endpoint Contract

```
GET http://player:8083/internal/users/{user_id}/list?status=watching,planned,postponed
   No JWT.  No Authorization header.  Docker-network only.

200 OK  Content-Type: application/json
{
  "items": [
    {
      "anime_id": "...",
      "name": "...",
      "name_ru": "...",
      "poster_url": "...",
      "episodes_aired": 14,
      "episodes_count": 28,
      "status": "watching",
      "last_watched_episode": 5,
      "updated_at": "2026-05-21T05:30:15Z"
    },
    ...
  ]
}

400 Bad Request — user_id missing
500 Internal Server Error — query failure (logged with grep-able prefix)
```

## GORM Tag Applied

```go
UpdatedAt time.Time `gorm:"index:idx_watch_progress_updated_at" json:"updated_at"`
```

## Trust Boundary

The endpoint is **NOT** protected by JWT. The trust model is:

- **Gateway does NOT proxy `/internal/*`** — the existing nginx + gateway router config has no `/internal/*` upstream rule, so external HTTP traffic cannot reach the path.
- **Defense-in-depth still owed** — Plan 03-04 will add a gateway router test that asserts `/api/internal/users/...` AND `/internal/users/...` both yield 404 from the gateway-side router. That assertion is OUT OF SCOPE for this plan.
- **Caller contract:** the catalog spotlight aggregator (Plan 03-03) calls this endpoint with a `user_id` derived from a validated JWT via Plan 03-02's optional-auth middleware. No untrusted user input crosses the gateway boundary onto this surface (T-03-02 accepted).

## Task Commits

1. **Task 1: GORM index tag + InternalListItem domain type** — `dd6d3a0` (feat)
2. **Task 2: Service method `GetUserListByStatusesWithProgress`** — `71b31c4` (feat)
3. **Task 3: Internal handler + chi route mount + router tests** — `52fe8a3` (feat)

_TDD execution: each task wrote failing tests first, then minimum-viable implementation. Build-failure RED was used (test referenced not-yet-defined symbol) instead of split test/feat commits — the work-product-per-task remained atomic._

## Files Created/Modified

- `services/player/internal/domain/watch.go` — modified: added `gorm:"index:idx_watch_progress_updated_at"` on `WatchProgress.UpdatedAt`; added `InternalListItem` struct between `BulkAnimeProgressMap` and `AnimeStatusEntry`.
- `services/player/internal/domain/watch_index_test.go` — new: reflect-based tests pinning the GORM tag presence and `InternalListItem` JSON field roster.
- `services/player/internal/repo/list.go` — modified: added `GetByUserAndStatusesWithProgress` method (parameterised raw SQL, LIMIT 200, ORDER BY al.updated_at DESC).
- `services/player/internal/service/list.go` — modified: added `GetUserListByStatusesWithProgress` (empty-statuses short-circuit, libs/errors.Wrap on query failures, nil-slice → empty-slice defense).
- `services/player/internal/service/list_internal_test.go` — new: 5 in-memory SQLite tests covering the joined query.
- `services/player/internal/handler/list_internal.go` — new: `InternalListHandler` + two constructors (`NewInternalListHandler` for prod, `NewInternalListHandlerFromService` for tests); handler-internal `listInternalService` interface; `parseStatusFilter` (allow-list `{watching, planned, postponed}` with whitespace trim + dedupe); bare-JSON `{"items": [...]}` response shape.
- `services/player/internal/handler/list_internal_test.go` — new: 7 tests against a fake `listInternalService` (happy path, no-status, unknown filtered, unknown-only, missing user_id, service error, whitespace-trim).
- `services/player/internal/transport/router.go` — modified: `NewRouter` gains `internalListHandler *handler.InternalListHandler` parameter; route registration BEFORE the `r.Route("/api"...)` block, gated by `!= nil` (mirrors catalog precedent).
- `services/player/internal/transport/router_internal_list_test.go` — new: 4 behavioral tests (mount at root, /api shadow 404, no-auth header still reaches handler, nil-handler 404 guard) + 1 source-order structural test.
- `services/player/internal/transport/router_test_helpers_test.go` — new: `zeroJWTConfig()`, `zeroMetricsCollector()` (sync.Once-guarded — promauto registers globally), `readRouterSource()` helper.
- `services/player/cmd/player-api/main.go` — modified: instantiates `internalListHandler := handler.NewInternalListHandler(listService, log)` and threads it through `transport.NewRouter(...)`.

## Decisions Made

See `key-decisions` in frontmatter. Highlights:

1. **Single multi-status endpoint over two endpoints** — the spotlight resolvers fan out at most once per card; collapsing the filter into a CSV keeps the player surface area minimal.
2. **Allow-list at the handler, not the service** — service stays agnostic; the resolver-specific status roster is a transport-layer concern.
3. **Bare-JSON response** — `{"items":[...]}` not `libs/httputil.OK`'s `{"data":...}` envelope. The catalog `player_client.go` consumer parses the bare shape. This is the same divergence-3 pattern Phase 1's spotlight handler established.
4. **Two handler constructors** — production `NewInternalListHandler` takes the concrete `*service.ListService` (keeps the prod call site terse); `NewInternalListHandlerFromService` takes the interface (lets the transport tests inject a stub without bringing the heavyweight service graph into the transport test binary).
5. **Source-order structural test** — chi has no public API to introspect mounting order, so the test reads `router.go` text directly via `runtime.Caller(0)` + `os.ReadFile` and asserts the `/internal` line precedes the `r.Route("/api"...)` line.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Promauto duplicate-registration panic in transport tests**

- **Found during:** Task 3 (router tests)
- **Issue:** `metrics.NewCollector(...)` uses `promauto.NewCounterVec`/`NewHistogramVec` which register against the global Prometheus default registerer. Constructing a fresh `*metrics.Collector` per test panics with `duplicate metrics collector registration attempted`. The plan had `<files>` listing only `router_internal_list_test.go` but did not anticipate this constraint.
- **Fix:** Introduced `router_test_helpers_test.go` with a `sync.Once`-guarded shared `*metrics.Collector` singleton via `zeroMetricsCollector(t)`. All router tests reuse the same instance.
- **Files modified:** `services/player/internal/transport/router_test_helpers_test.go` (new — additional file beyond the plan's `<files>` list).
- **Verification:** `go test ./internal/transport -count=1 -race` passes for all 5 router tests; no duplicate-registration panic.
- **Committed in:** `52fe8a3` (Task 3 commit)

**2. [Rule 2 — Missing Critical] Added 7th handler test (whitespace trim) + 5th router test (nil-handler guard)**

- **Found during:** Task 3 implementation review
- **Issue:** The plan called for 5 handler tests + 3 router tests. While implementing `parseStatusFilter` I added whitespace trimming per CSV value (callers may not canonicalise their query strings) — needed an additional test to pin that behavior. While threading `internalListHandler` through `NewRouter` I added a `!= nil` guard (mirroring the catalog precedent for optional internal handlers) — needed a test to assert the guard works.
- **Fix:** Added `TestListByStatuses_Internal_StatusValuesTrimmed` (handler) and `TestRouter_InternalListNotNil_RouteRegistered` (router) — both correctness-critical guards that would otherwise have no regression net.
- **Files modified:** `services/player/internal/handler/list_internal_test.go`, `services/player/internal/transport/router_internal_list_test.go`
- **Verification:** Both tests pass under `-race`.
- **Committed in:** `52fe8a3` (Task 3 commit)

**3. [Rule 2 — Missing Critical] Added `TestListByStatuses_Internal_UnknownStatusOnly_ReturnsEmpty` (8th handler test) and `TestRouter_RouteOrder_InternalBeforeAPI` (6th transport test)**

- **Found during:** Task 3 implementation
- **Issue:** Allow-list filtering means a caller passing only unknown statuses (`?status=foo,bar`) ends up with an empty filter and an empty result — the plan had no test pinning this corner. Separately, the plan's `<done>` block asks for a grep proving the `/internal` route line appears before `r.Route("/api"...)` — that's a structural assertion that needed an in-test guard so a future refactor cannot silently reorder the lines.
- **Fix:** Added the corner-case handler test + the source-order regex test.
- **Verification:** Both pass; source-order test catches any future move of the `/internal` route.
- **Committed in:** `52fe8a3` (Task 3 commit)

---

**Total deviations:** 3 auto-fixed (1 blocking infrastructure, 2 missing-critical correctness guards)
**Impact on plan:** No scope creep. All deviations are within-package additions that strengthen the test net without expanding the production surface. The plan's `<artifacts>` and `<key_links>` contracts are unchanged.

## Issues Encountered

- **`.gitignore` pattern `**/player-api` collision:** When staging `services/player/cmd/player-api/main.go` explicitly via `git add path`, git emits an "ignored by gitignore" warning because the project-level `.gitignore` has `**/player-api` (intended to ignore the compiled binary). However, the file was already tracked and `git status` correctly showed the modification staged. Resolution: ignored the warning — the commit succeeded with the change included. No action needed; left as a future-cleanup observation for the maintainer.

## Downstream Consumers

- **Plan 03-02 (optional-auth middleware on catalog spotlight handler):** Will derive `user_id` from the JWT and pass it to the catalog resolvers, which then call this endpoint.
- **Plan 03-03 (5 dynamic resolvers):** `not_time_yet` and `continue_watching_new` resolvers call this endpoint via the new `services/catalog/internal/service/spotlight/client/player_client.go`. They consume `InternalListItem.AnimeID`, `EpisodesAired`, `LastWatchedEpisode`, `Status` and the embedded `Anime` projection.
- **Plan 03-04 (gateway router defense-in-depth):** Will add the `/internal/*` not-proxied assertion at the gateway level. **This plan does NOT modify the gateway.**

## Verification (from plan)

| # | Check | Result |
|---|---|---|
| 1 | `go test ./internal/domain -count=1` | PASS (2 new tests + existing all pass) |
| 2 | `go test ./internal/service -count=1 -race` | PASS (5 new tests + existing all pass) |
| 3 | `go test ./internal/handler -run TestListByStatuses_Internal -count=1 -race` | PASS (7 tests) |
| 4 | `go test ./internal/transport -run "TestRouter_InternalList\|TestRouter_RouteOrder" -count=1 -race` | PASS (5 tests) |
| 5 | `go build ./services/player/...` exits 0 | PASS |
| 6 | `grep -q 'gorm:"index:idx_watch_progress_updated_at"' .../watch.go` | PASS |
| 7 | `/internal` route registered BEFORE `r.Route("/api"...)` | PASS (line 67 < line 71 in router.go) |
| 8 | `InternalListItem` JSON keys present (via reflect test, not literal grep — see note below) | PASS |

**Note on check #8:** The plan's literal grep `grep -o '"\(anime_id\|episodes_aired\|episodes_count\|last_watched_episode\)"'` returns 3 instead of 4 because the `InternalListItem` JSON tags include `,omitempty` modifiers (per the plan's `<interfaces>` block) — `last_watched_episode,omitempty` does not match the bare `"last_watched_episode"` pattern. The reflect-based `TestInternalListItem_HasExpectedJSONFields` test is the authoritative contract assertion and passes; a more accurate grep `grep -E 'json:"(anime_id|episodes_aired|episodes_count|last_watched_episode)'` finds 16 matches across all relevant structs.

## Next Phase Readiness

- **Ready for Plan 03-02 (optional-auth middleware):** the endpoint accepts a user_id verbatim — the middleware just needs to extract from JWT and pass through.
- **Ready for Plan 03-03 (resolvers):** the `InternalListItem` JSON contract is stable and pinned by tests; `player_client.go` can unmarshal directly.
- **Plan 03-04 still owes the gateway router test** — defense-in-depth assertion that `/internal/*` is not proxied. This plan deliberately did NOT modify the gateway.

## Self-Check: PASSED

Created files (verified):
- `services/player/internal/handler/list_internal.go` — FOUND
- `services/player/internal/handler/list_internal_test.go` — FOUND
- `services/player/internal/service/list_internal_test.go` — FOUND
- `services/player/internal/domain/watch_index_test.go` — FOUND
- `services/player/internal/transport/router_internal_list_test.go` — FOUND
- `services/player/internal/transport/router_test_helpers_test.go` — FOUND

Commits (verified via `git log --oneline`):
- `dd6d3a0` — FOUND (Task 1)
- `71b31c4` — FOUND (Task 2)
- `52fe8a3` — FOUND (Task 3)

---
*Phase: 03-dynamic-cards-migration*
*Plan: 01*
*Completed: 2026-05-21*
