---
phase: 03-dynamic-cards-migration
plan: 04
subsystem: hero-spotlight / catalog wiring + gateway defense
workstream: hero-spotlight
tags: [hero-spotlight, wiring, di, gateway, retrofit, latest-news, phase-3, wave-3]
requires:
  - 03-01 (player /internal/users/{id}/list)
  - 03-02 (OptionalAuthMiddleware, AdaptiveSlice, PlayerClient, jwt_context)
  - 03-03 (PersonalPick, TelegramNews, NowWatching, NotTimeYet, ContinueWatchingNew)
provides:
  - Live /api/home/spotlight endpoint with all 9 resolvers wired
  - OptionalAuthMiddleware scoped to ONLY /home/spotlight (no broadening)
  - Spotlight handler that forwards (userID *string, JWT-on-ctx) to aggregator
  - latest_news retrofitted to use AdaptiveSlice (HSB-BE-30 N=2 random rule)
  - Gateway defense-in-depth test pinning /internal/users/{id}/list NOT proxied
affects:
  - services/catalog/cmd/catalog-api/main.go (DI for 5 new resolvers + PlayerClient + rng)
  - services/catalog/internal/transport/router.go (route wrap)
  - services/catalog/internal/handler/spotlight.go (userID/JWT extraction)
  - services/catalog/internal/handler/spotlight_test.go (+3 tests)
  - services/catalog/internal/service/spotlight/cards/latest_news.go (AdaptiveSlice retrofit)
  - services/catalog/internal/service/spotlight/cards/latest_news_test.go (+3 tests, signature update)
  - services/catalog/internal/service/spotlight/aggregator_test.go (+9-card test)
  - services/gateway/internal/transport/router_internal_list_test.go (new file, defense test)
tech-stack:
  added: []
  patterns:
    - "Route-scoped middleware wrap via r.With(mw).Get(...) — same pattern as AuthMiddleware on /admin"
    - "Shared *rand.Rand passed to 5 resolvers — single source of randomness for the daily rotation"
    - "Bare 404 from gateway for unrouted /internal/* paths — chi default behavior + spy-on-backends regression test"
key-files:
  created:
    - services/gateway/internal/transport/router_internal_list_test.go
  modified:
    - services/catalog/cmd/catalog-api/main.go
    - services/catalog/internal/transport/router.go
    - services/catalog/internal/handler/spotlight.go
    - services/catalog/internal/handler/spotlight_test.go
    - services/catalog/internal/service/spotlight/cards/latest_news.go
    - services/catalog/internal/service/spotlight/cards/latest_news_test.go
    - services/catalog/internal/service/spotlight/aggregator_test.go
decisions:
  - Used cfg.JWT directly in router (no signature change) — matches the existing AuthMiddleware(cfg.JWT) pattern
  - Shared spotlightRng across all 5 random-using resolvers (single time-seeded source)
  - Handler forwards bearer token to ctx UNCONDITIONALLY — login-only resolvers gate on userID, not JWT presence
  - latest_news adaptive picking happens AFTER fetch and BEFORE cache write → already-narrowed payload is cached for the TTL window (intentional: random pick stays stable for the 24h rotation)
metrics:
  duration: <1h
  completed: 2026-05-21
---

# Phase 03 Plan 04: Wire 9 Resolvers + Gateway Defense Summary

End-to-end wiring of the hero spotlight aggregator so `GET /api/home/spotlight` returns up to 9 cards from a single catalog endpoint; gateway pinned to NOT proxy the new `/internal/users/{id}/list` player route.

## Resolver Order (Stable — Plan 07 smoke test asserts this exact sequence)

The aggregator receives resolvers in this order. The order doubles as the **default card order** on equal-priority ties at the frontend.

| # | Type                    | Constructor                                                          | Login-only? |
| - | ----------------------- | -------------------------------------------------------------------- | ----------- |
| 1 | `anime_of_day`          | `cards.NewAnimeOfDayResolver(animeRepo, redisCache, log)`            | No          |
| 2 | `random_tail`           | `cards.NewRandomTailResolver(animeRepo, redisCache, log)`            | No          |
| 3 | `latest_news`           | `cards.NewLatestNewsResolver(webClient, redisCache, rng, log)`       | No          |
| 4 | `platform_stats`        | `cards.NewPlatformStatsResolver(db.DB, redisCache, log)`             | No          |
| 5 | `personal_pick`         | `cards.NewPersonalPickResolver(catalogService, playerClient, redisCache, rng, log)` | Branches: anon → trending, login → recs |
| 6 | `telegram_news`         | `cards.NewTelegramNewsResolver(telegramClient, redisCache, rng, log)` | No          |
| 7 | `now_watching`          | `cards.NewNowWatchingResolver(NewGormNowWatchingAdapter(db.DB), redisCache, rng, log)` | No          |
| 8 | `not_time_yet`          | `cards.NewNotTimeYetResolver(playerClient, redisCache, rng, log)`    | **Yes**     |
| 9 | `continue_watching_new` | `cards.NewContinueWatchingNewResolver(playerClient, redisCache, rng, log)` | **Yes**     |

## Grep Gates (per <verification>)

| Check                                                          | Expected | Actual |
| -------------------------------------------------------------- | -------- | ------ |
| `grep -c "OptionalAuthMiddleware(" router.go`                  | `1`      | `1`    |
| `grep -c "NewP*Resolver/NewT*Resolver/NewN*Resolver/NewC*Resolver"` in `main.go` (5 dynamic) | `5`      | `5`    |
| `grep -c "NewPlayerClient" main.go`                            | `1`      | `1`    |
| `grep -c "cards.New" main.go` (all 9 resolvers)                | `≥9`     | `9`    |
| `grep -c "/internal/users" services/gateway/.../router.go`     | `0`      | `0`    |
| `grep -c "spotlight.AdaptiveSlice" latest_news.go`             | `≥1`     | `3`    |
| `grep -c "cards.ContextWithJWT" handler/spotlight.go`          | `≥1`     | `2`    |
| `grep -c "authz.ClaimsFromContext" handler/spotlight.go`       | `≥1`     | `1`    |
| `grep -c "json.NewEncoder" handler/spotlight.go` (DIV 3)       | `≥1`     | `3`    |
| `grep -v '^//' handler/spotlight.go \| grep -c httputil.OK\|JSON` (DIV 3) | `0`      | `0`    |

## Tasks Executed

### Task 1 — latest_news AdaptiveSlice retrofit + handler ctx wiring
- `latest_news.go`: added `*rand.Rand` field; constructor gains `rng` arg (defaults to time-seeded source when nil). `entries[:3]` → `spotlight.AdaptiveSlice(entries, r.rng)`. AdaptiveSlice runs AFTER fetch / BEFORE cache write so the cache stores the already-narrowed payload.
- `latest_news_test.go`: 6 existing tests updated to pass deterministic `rand.New(rand.NewSource(42))`; 3 new tests cover N=1 (passthrough), N=2 (random-pick of 1), N=5 (top-3 in input order).
- `handler/spotlight.go`: reads `authz.ClaimsFromContext` to build `*string` userID; forwards `httputil.BearerToken(r)` via `cards.ContextWithJWT` regardless of claims (login-only resolvers gate on userID, not JWT presence — invalid JWT is harmless on ctx).
- `handler/spotlight_test.go`: 3 new tests cover (no-claims → userID=nil, JWT=""), (with-claims → userID=u1, JWT=abc forwarded), (invalid-JWT-stripped → userID=nil but JWT=invalid still on ctx). Existing 5 tests unchanged.
- Commit: `72bd335`

### Task 2 — Router wrap + Aggregator 9-card test
- `router.go`: `r.Get("/home/spotlight", ...)` → `r.With(OptionalAuthMiddleware(cfg.JWT)).Get("/home/spotlight", ...)`. Used `cfg.JWT` directly (same pattern as `AuthMiddleware(cfg.JWT)` already used elsewhere in the file) — no signature change required.
- `aggregator_test.go`: added `capturingResolver` fake that records (userID, JWT-on-ctx) per invocation, and `TestAggregator_NineCards_PassesUserIDAndJWT` that wires 9 fakes and asserts all 9 received the same userID + JWT through the concurrent fan-out. Defines a local `jwtKeyForTest` to read the same ctx-value the cards package writes (avoids the cards → spotlight → cards package cycle).
- Commit: `92a3141`

### Task 3 — main.go DI + gateway defense-in-depth
- `main.go`: added `math/rand` import; replaced the Phase-1 4-resolver block with the Phase-3 9-resolver block. `spotlightPlayerClient` and `spotlightRng` constructed once and reused by all consuming resolvers. NowWatching wraps `db.DB` via `cards.NewGormNowWatchingAdapter` (adapter exists in `cards/now_watching.go` to satisfy the resolver's narrow `nowWatchingDB` interface).
- `router_internal_list_test.go` (new): 3 tests pin `/internal/users/{id}/list` is NOT routed by the gateway:
  - `TestRouter_InternalListNotProxied` — GET returns 404/405; neither catalog nor scraper backend receives the request.
  - `TestRouter_InternalListNotProxied_PostMethod` — POST returns 404/405 too.
  - `TestRouter_InternalListNotProxied_AnyUserID` — parametric sweep across UUIDs, escaped IDs, sub-paths, query strings — all 404/405.
- Commit: `2e7a1e3`

## Deviations from Plan

**None — plan executed exactly as written.**

One simplification (NOT a deviation): the plan suggested extending `transport.NewRouter`'s signature with an `authz.JWTConfig` parameter. I observed `cfg.JWT` is already an `authz.JWTConfig` and the same router already uses `AuthMiddleware(cfg.JWT)` for `/admin/*`. Using `cfg.JWT` directly keeps the signature stable and matches local convention. The plan's must_haves require only that the middleware be wired onto `/home/spotlight`, which is satisfied.

## Behavior

After this plan the endpoint is **live with all 9 cards** when data permits. Specifically:

- Anonymous caller (`Authorization` absent): 7 cards eligible — `anime_of_day`, `random_tail`, `latest_news`, `platform_stats`, `personal_pick` (anon trending branch), `telegram_news`, `now_watching`. The 2 login-only resolvers (`not_time_yet`, `continue_watching_new`) return `(nil, nil)` BEFORE any data fetch (T-03-11 mitigation).
- Authenticated caller (valid JWT, OptionalAuth attaches claims): all 9 cards eligible. `personal_pick` flips to its login branch and fans out to `player /api/users/recs` with the user's JWT forwarded; the 2 login-only resolvers query `player /internal/users/{id}/list` via the Docker-network-only route.
- Invalid JWT: OptionalAuth strips claims → handler treats as anon (userID=nil) → 7 cards. The bearer token is still on ctx but is harmless because login-only resolvers gate on userID.

## Test Run

```
cd services/catalog && go test ./... -count=1 -short -race
  → all packages OK (incl. spotlight 3.8s, transport 1.0s, handler 1.0s)
cd services/gateway && go test ./... -count=1 -short -race
  → all packages OK (incl. transport 1.16s — the 3 new tests + 19 existing all pass)
go build ./services/catalog/... ./services/gateway/...
  → exits 0
```

## Downstream Consumers

- **Plan 03-05 (frontend):** builds the matching `useSpotlight` composable + 5 new card components for the 5 dynamic types. Renders the response of this endpoint. Should treat each card as optional (anon path returns 7, login path returns 9).
- **Plan 03-06 (cleanup):** removes the legacy `trendingRecs` row from `Home.vue` since `personal_pick` now subsumes that surface.
- **Plan 03-07 (e2e + smoke):** asserts `len(cards) == 9` for a logged-in caller and `len(cards) >= 7` for an anon caller; verifies card-type set matches the resolver order table above; verifies gateway does NOT route `/internal/users/.../list`.

## Threat Mitigations Applied

| Threat ID | Mitigation                                                                                       |
| --------- | ------------------------------------------------------------------------------------------------ |
| T-03-15   | New gateway test `TestRouter_InternalListNotProxied*` pins absence of any `/internal/*` route.   |
| T-03-16   | `grep -c "OptionalAuthMiddleware(" router.go == 1` enforced via the verification step; the middleware is scoped to `/home/spotlight` only. |
| T-03-17   | latest_news caches the already-adaptive-sliced payload (random pick stays stable for 24h TTL — the intentional contract). |

## Metrics (per CONVENTIONS.md)

- **UXΔ** = +2 (Better — endpoint now live with all 9 cards once data permits; login users get personalized + list-derived cards)
- **CDI** = 0.06 × 13 (DI assembly + middleware wrap + retrofit + new tests — medium reach, high coupling, gated by greps)
- **MVQ** = Griffin 89%/85% (wiring + retrofit, slop risk is forgetting the latest_news N=2 retrofit or broadening middleware too far — both gated by the grep-count assertions in <verification>)

## Self-Check: PASSED

- All 3 task commits exist:
  - `72bd335` Task 1 — latest_news + handler
  - `92a3141` Task 2 — router wrap + 9-card test
  - `2e7a1e3` Task 3 — main.go DI + gateway defense
- All target files modified/created (verified via `git diff --name-only` on the 3 commits).
- Catalog + gateway tests green; `go build` exits 0; all 10 grep gates from `<verification>` pass.
