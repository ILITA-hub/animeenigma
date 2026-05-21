---
phase: 01-backend-aggregator
plan: 04
subsystem: services/catalog
tags: [hero-spotlight, http-handler, feature-flag, di-wiring, divergence-3]
dependency-graph:
  requires:
    - 01-01-PLAN.md (spotlight.Card / Response types + Resolver interface)
    - 01-02-PLAN.md (4 card resolvers under spotlight/cards)
    - 01-03-PLAN.md (Aggregator.Resolve concurrent fan-out)
  provides:
    - "GET /api/home/spotlight chi route registered on the catalog service"
    - "cfg.SpotlightEnabled feature flag (env SPOTLIGHT_ENABLED, default true)"
    - "Full DI wiring: WebClient → 4 resolvers → Aggregator → SpotlightHandler"
  affects:
    - 01-05-PLAN.md (gateway proxy — can now point at catalog:8081/api/home/spotlight)
    - 01-06-PLAN.md (smoke test — curl http://localhost:8000/api/home/spotlight)
    - Phase 2 frontend HSB-FE-02 (parses the bare {cards, generated_at} envelope)
tech-stack:
  added:
    - "encoding/json (stdlib) — direct envelope encode, DELIBERATE DIVERGENCE 3"
  patterns:
    - "Aggregator interface in handler package for test-substitutability"
    - "Bare 404 (w.WriteHeader, no body) for feature-flag-off short-circuit"
    - "getEnvBool helper mirroring getEnvInt/getEnvDuration shape"
key-files:
  created:
    - services/catalog/internal/handler/spotlight.go
    - services/catalog/internal/handler/spotlight_test.go
  modified:
    - services/catalog/internal/config/config.go
    - services/catalog/internal/transport/router.go
    - services/catalog/cmd/catalog-api/main.go
decisions:
  - "DELIBERATE DIVERGENCE 3 enforced: handler does NOT import libs/httputil. Bare json.NewEncoder + bare 404."
  - "Aggregator is taken as an interface (not concrete *spotlight.Aggregator) so handler tests can substitute a fake without instantiating the full concurrent aggregator stack."
  - "Phase 1 endpoint is public — handler reads but does not validate the Authorization header. Phase 3 will add optional-auth and pass a real userID."
metrics:
  tasks_completed: 2
  files_changed: 5
  files_created: 2
  files_modified: 3
  commits:
    - "e2ff594 feat(01-04): spotlight handler + SPOTLIGHT_ENABLED flag (Task 1)"
    - "0e2f5b3 feat(01-04): wire spotlight aggregator into catalog router + main (Task 2)"
  tests_added: 5
  tests_passing: 5
  build: "go build ./services/catalog/... — clean"
  test_suite: "go test ./services/catalog/... -count=1 -short -race — all packages green"
  duration_minutes: "~10"
  completed: 2026-05-21T02:33Z
---

# Phase 1 Plan 04: Spotlight Handler + DI Wiring Summary

Mounts the Phase 1 hero-spotlight backend on the catalog service's HTTP
surface and wires every dependency into `cmd/catalog-api/main.go`. After
this plan + Plan 05 (gateway proxy), `curl http://localhost:8000/api/home/spotlight`
returns the bare `{cards, generated_at}` envelope built by the
fan-out aggregator from Plan 03.

## What Was Built

### Task 1 — Config flag + SpotlightHandler + 5 contract tests (commit `e2ff594`)

1. **`services/catalog/internal/config/config.go`**
   - Added `SpotlightEnabled bool` field on `Config` struct (env
     `SPOTLIGHT_ENABLED`, default `true`).
   - Added `getEnvBool(key, default bool) bool` helper mirroring the
     existing `getEnvInt` / `getEnvDuration` shape. Uses
     `strconv.ParseBool`; falls back to default on missing or unparseable.
   - Wired into `Load()` after the `Library:` block.

2. **`services/catalog/internal/handler/spotlight.go`** (NEW, 92 LoC)
   - `aggregator` interface declared inside the handler package — the
     production `*spotlight.Aggregator` satisfies it implicitly via its
     `Resolve(ctx, userID) (*Response, error)` method. Tests substitute
     a handwritten fake. Pattern matches the rest of the handler package.
   - `SpotlightHandler{agg, enabled, log}` struct + `NewSpotlightHandler`
     constructor.
   - `Get(w, r)`:
     - When `enabled=false` → `w.WriteHeader(http.StatusNotFound); return`
       (bare 404, no body). HSB-BE-07 contract.
     - Always sets `Content-Type: application/json` on the 2xx/5xx paths.
     - 2s `context.WithTimeout` around `agg.Resolve(ctx, nil)`. Phase 1
       passes `userID=nil` because the endpoint is public.
     - On aggregator error → 500 with `{cards:[], generated_at:"<now>"}`
       (bare envelope shape preserved even on the catastrophic path so
       the frontend parser never blows up).
     - On success → 200 with `json.NewEncoder(w).Encode(resp)`.
     - Two info logs: `spotlight.request{user="anon"}` on entry,
       `spotlight.aggregated{cards_returned, ms_total}` on exit.
     - Never logs the `Authorization` header value (Pitfall 6).

3. **`services/catalog/internal/handler/spotlight_test.go`** (NEW, 5 tests)
   - `TestSpotlightHandler_Get_Envelope` — 200 + bare `{cards, generated_at}`
     keys; cards array length sanity check.
   - `TestSpotlightHandler_Get_FlagOff_Returns404NoBody` — 404 + empty body.
   - `TestSpotlightHandler_Get_OptionalAuth_DoesNot401` — request with
     `Authorization: Bearer fake-token` succeeds with 200, never 401.
   - `TestSpotlightHandler_Get_AggregatorError_Returns500EmptyCards` —
     500 path emits `{cards:[], generated_at:"..."}`; no `success` /
     `error` envelope keys; `cards` marshals as `[]` not `null`.
   - `TestSpotlightHandler_Get_NoEnvelopeWrapper` — regression guard:
     body string MUST NOT contain `"success":` substring anywhere.

### Task 2 — Router signature + DI wiring (commit `0e2f5b3`)

1. **`services/catalog/internal/transport/router.go`**
   - `NewRouter` signature extended with `spotlightHandler *handler.SpotlightHandler`,
     positioned after `internalEpisodesHandler` and before `cfg`.
   - `r.Get("/home/spotlight", spotlightHandler.Get)` registered INSIDE
     the `/api` Route block, directly after the existing `/anime/news`
     line. Public — no `AuthMiddleware` applied.

2. **`services/catalog/cmd/catalog-api/main.go`**
   - Three new imports: `service/spotlight`, `service/spotlight/cards`,
     `service/spotlight/client`.
   - Constructs the full stack in DI order:
     ```go
     spotlightWebClient := client.NewWebClient("", nil)
     spotlightResolvers := []spotlight.Resolver{
         cards.NewAnimeOfDayResolver(animeRepo, redisCache, log),
         cards.NewRandomTailResolver(animeRepo, redisCache, log),
         cards.NewLatestNewsResolver(spotlightWebClient, redisCache, log),
         cards.NewPlatformStatsResolver(db.DB, redisCache, log),
     }
     spotlightAggregator := spotlight.NewAggregator(redisCache, log, spotlightResolvers)
     spotlightHandler := handler.NewSpotlightHandler(spotlightAggregator, cfg.SpotlightEnabled, log)
     ```
   - `transport.NewRouter(...)` call updated to pass `spotlightHandler` in the new positional slot.
   - Note: `cards.NewPlatformStatsResolver` takes `*gorm.DB` directly,
     so we pass `db.DB` (the inner GORM handle on the wrapper).

## DELIBERATE DIVERGENCE 3 — Confirmation

The acceptance contract requires the handler to write a BARE
`{cards, generated_at}` envelope rather than the shared
`{success, data}` wrapper. Verified holds:

```
$ grep -q "libs/httputil" services/catalog/internal/handler/spotlight.go
  (no match) — handler does NOT import the shared response-helpers package.

$ grep -E "httputil\.(OK|Error|NotFound|JSON)" services/catalog/internal/handler/spotlight.go
  (no match) — no calls to the wrapping helpers.

$ grep -q "json.NewEncoder(w).Encode" services/catalog/internal/handler/spotlight.go
  (match) — direct stdlib encode present.

$ grep -q "w.WriteHeader(http.StatusNotFound)" services/catalog/internal/handler/spotlight.go
  (match) — bare 404 short-circuit present.
```

## Acceptance Criteria — Status

| Criterion | Status |
|-----------|--------|
| Config `SpotlightEnabled` field + `getEnvBool` helper, default true | ✅ |
| Handler does NOT import `libs/httputil` | ✅ |
| Handler does NOT call `httputil.OK/Error/NotFound/JSON` | ✅ |
| Handler uses `json.NewEncoder(w).Encode(resp)` direct encode | ✅ |
| Handler short-circuits to bare `w.WriteHeader(http.StatusNotFound)` when flag off | ✅ |
| Handler uses `aggregator` interface (not concrete `*spotlight.Aggregator`) | ✅ |
| 5 handler tests pass: Envelope, FlagOff, OptionalAuth, AggregatorError, NoEnvelopeWrapper | ✅ |
| `NewRouter` signature includes `spotlightHandler *handler.SpotlightHandler` | ✅ |
| Route `/home/spotlight` registered INSIDE the `/api` block | ✅ |
| main.go constructs 4 resolvers (anime_of_day, random_tail, latest_news, platform_stats) | ✅ |
| main.go's `transport.NewRouter(...)` call includes `spotlightHandler` | ✅ |
| `go build ./services/catalog/...` exits 0 | ✅ |
| `go test ./services/catalog/... -count=1 -short -race` — all green | ✅ |

## Deviations from Plan

**Two minor adjustments, no functional impact:**

1. **`db.DB` (not `db`) passed to `PlatformStatsResolver`.**
   The plan's example wiring referenced `db` as `*gorm.DB`, but in
   `main.go` `db` is the project's `*database.DB` wrapper (from
   `libs/database`) — the inner GORM handle is `db.DB`. Use of `db.DB`
   matches the existing pattern at the Anime AutoMigrate call (`db.DB.AutoMigrate(...)`)
   on the same file.

2. **Doc-comment phrasing.** The handler initially carried doc comments
   that named `libs/httputil` and `httputil.OK`/`httputil.NotFound`
   explicitly to explain the divergence. The acceptance grep gates
   (`! grep -q "libs/httputil"` and
   `! grep -E "httputil\.(OK|Error|NotFound|JSON)"`) treat any string
   match as a failure regardless of context (comment vs code). Comments
   were rephrased to "the shared response-helpers package" and "the
   shared NotFound helper" so the grep gates stay strict, while the
   intent stays documented.

No bugs found, no auth gates encountered, no Rule 4 architectural
changes needed.

## Known Stubs

None. Every line in the handler is wired to real production behaviour;
the only mock paths are inside test files.

## Threat Flags

None. The new endpoint is public-read with no side effects (read-only
DB queries + cache reads + an outbound HTTP GET to `web:80` — all
already-existing surface). No new auth boundary, no new state mutation.

## Tests Added

| Test | What It Pins |
|------|--------------|
| `TestSpotlightHandler_Get_Envelope` | Bare `{cards, generated_at}` envelope shape; no `success`/`data` wrapper. |
| `TestSpotlightHandler_Get_FlagOff_Returns404NoBody` | HSB-BE-07 — bare 404 + empty body when flag off. |
| `TestSpotlightHandler_Get_OptionalAuth_DoesNot401` | Phase 1 public route tolerates `Authorization` header. |
| `TestSpotlightHandler_Get_AggregatorError_Returns500EmptyCards` | Catastrophic-error path still emits bare envelope shape with `cards:[]`. |
| `TestSpotlightHandler_Get_NoEnvelopeWrapper` | Regression guard against re-introducing the shared `OK` wrapper. |

## Score

- **UXΔ = +2 (Better)** — endpoint is now curl-able once Plan 05 ships gateway proxy.
- **CDI = 0.15 × 8** — 1 new handler + 1 handler test + 3 modified files; pure mechanical assembly atop the Plans 01-03 work.
- **MVQ = Griffin 90%/85%** — HTTP surface assembly; slop risk was using the shared envelope by reflex, defused by the strict acceptance grep.

## Self-Check: PASSED

**Created files (verified on disk):**
- `services/catalog/internal/handler/spotlight.go` — FOUND
- `services/catalog/internal/handler/spotlight_test.go` — FOUND

**Modified files (verified via `git log -p`):**
- `services/catalog/internal/config/config.go` — patched (SpotlightEnabled field + getEnvBool + Load wire)
- `services/catalog/internal/transport/router.go` — patched (NewRouter arg + /home/spotlight route)
- `services/catalog/cmd/catalog-api/main.go` — patched (imports + wiring block + NewRouter call)

**Commits in git log:**
- `e2ff594` — feat(01-04): spotlight handler + SPOTLIGHT_ENABLED flag (Task 1) — FOUND
- `0e2f5b3` — feat(01-04): wire spotlight aggregator into catalog router + main (Task 2) — FOUND
