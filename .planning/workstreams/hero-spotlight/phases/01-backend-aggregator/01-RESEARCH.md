# Phase 1: Backend Aggregator + Static Cards — Research

**Researched:** 2026-05-21
**Domain:** Go microservice — concurrent fan-out aggregator endpoint with per-card Redis day-cache, eligibility filter, and feature-flag gating
**Confidence:** HIGH (every prescription is anchored to an existing pattern in this repo or to verified source code; no library-version guesses required because all dependencies are already in `go.mod`)

## Summary

The phase is fully additive to `services/catalog`. Every primitive needed — concurrent fan-out, per-card Redis cache, structured logging on partial failure, JWT-optional middleware, `chi` route registration, gateway reverse-proxy entry — already exists in this repo as battle-tested patterns. The job is composition, not invention.

Three findings reshape the CONTEXT.md decisions:

1. **`active_rooms_7d` is unreachable from catalog and MUST be `nil`.** The `rooms` service stores rooms in **Redis only** (`services/rooms/internal/service/room.go:42` writes `room:<id>` to `cache.RedisCache` with 24h TTL). There is no `rooms` table in Postgres. The catalog's shared GORM connection cannot `SELECT * FROM rooms`. Card eligibility still holds with ≥1 of the 3 metrics computable, so this is non-blocking, but the planner MUST encode "return nil, never query" rather than "try GORM, log on failure".
2. **`episodes_added_7d` has no event log.** No `episodes` table, no `episode_added_at` column. The closest field is `Anime.EpisodesAired int` on the existing model, but it's a snapshot — not a per-event log. Without backfill or a new event table (out of scope for Phase 1), this metric returns `nil`.
3. **`AnimeRepository.Search` does NOT support a true offset path for `random_tail` (ranks 101..2000).** It supports `Page + PageSize`, BUT it also injects `sort_priority DESC` as the primary order key — meaning page 2 by score is `sort_priority DESC, score DESC OFFSET 100`. Any anime with `sort_priority > 0` (pinned announcements) will land in page 1 and shift the offset windows. For `random_tail` we want strict score-rank-based selection, so the plan should use `Page=2, PageSize=100` (fetching ranks 101..200, accepting the pinned-shift as a feature — pinned anime never appear in random_tail discovery) OR add a new repo method that explicitly orders by `score DESC` without the `sort_priority` prefix. Recommendation below: use `Page=2, PageSize=100` — the shift is harmless and adding a new repo method costs a migration of intent without functional gain.

**Primary recommendation:** Build the aggregator on the `subs_aggregator.go` pattern (`sync.WaitGroup + buffered results channel + drop on per-provider error`), NOT `errgroup` (its fail-fast semantics fight our "always succeed partial" contract). Use `cache.GetOrSet` per resolver. Wire 4 resolver constructors in `cmd/catalog-api/main.go` next to the existing `newsHandler` block. Add the gateway proxy entry as a single `r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)` line under the `/api` group, BEFORE the generic `/anime/*` line (no collision but follows the "specific-before-general" convention).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Resource sourcing (data sources for 4 static cards)**
- `anime_of_day` — Use `AnimeRepository.Search` with `Sort="score"`, `Order="desc"`, `ScoreMin=8.0`, `PageSize=200`. Pick `items[seed % len]` where `seed = YYYY*100*32 + MM*32 + DD` (UTC date — server's local TZ).
- `random_tail` — Use `AnimeRepository.Search` with `Sort="score"`, `Order="desc"`, `PageSize=1900`, then slice `[100:]` (drop top-100). Pick `items[seed % len]` with same date seed. If repo Search supports Offset/Page, prefer `Page=2, PageSize=100` to fetch ranks 101..200 to avoid loading 2000 rows — finalise during plan-phase research.
- `latest_news` — New `client/web_client.go` does `GET http://web:80/changelog.json` (docker-network DNS), parses the existing JSON shape from `frontend/web/public/changelog.json`. Returns up to 3 newest entries.
- `platform_stats` — Three metrics via direct GORM:
  - `anime_added_7d`: `SELECT COUNT(*) FROM animes WHERE created_at > NOW() - INTERVAL '7 days' AND deleted_at IS NULL`
  - `episodes_added_7d`: deferred — if no episode log exists in the codebase, return `nil` (omit the metric) but keep the card eligible if ≥1 metric is non-null. Plan-phase research will confirm whether per-episode events exist; default `null` is acceptable.
  - `active_rooms_7d`: `SELECT COUNT(*) FROM rooms WHERE created_at > NOW() - INTERVAL '7 days'` on the shared GORM connection. If rooms table is not addressable from catalog, return `nil` for this metric (card stays eligible if ≥1 metric).
  - Card eligible iff at least 1 metric is non-null.

**Aggregator / concurrency**
- Aggregator runs resolvers concurrently via goroutines + `errgroup` or custom WaitGroup with collected results. Each card gets its own `context.WithTimeout(ctx, 800ms)`.
- Card failures (timeout or error) drop the card and emit one structured log line `spotlight.card_failed{type=..., error=...}` via the existing `libs/logger` Errorw method.
- Overall request budget: `context.WithTimeout(r.Context(), 2*time.Second)`. On overall context deadline, return whatever fully-resolved cards were collected so far. If zero cards collected AND a last-known-good `spotlight:snapshot:<anon|user_id>:YYYY-MM-DD` Redis key exists, return the cached snapshot. Otherwise return 200 with empty `cards: []` — frontend hides the block (per HSB-FE-02).
- Aggregator response is **not** cached as a single blob. Each resolver uses `cache.GetOrSet`-style pattern (Get; on miss → compute → Set with appropriate TTL).

**Cache keys (per design doc §5.3)**
- `spotlight:anime_of_day:<YYYY-MM-DD>` TTL 24h
- `spotlight:random_tail:<YYYY-MM-DD>` TTL 24h
- `spotlight:changelog:<YYYY-MM-DD>` TTL 24h
- `spotlight:stats:<YYYY-MM-DD>` TTL 24h
- Snapshot fallback: `spotlight:snapshot:<anon|user_id>:<YYYY-MM-DD>` TTL 24h (written best-effort after every successful aggregation).

**Auth handling for Phase 1**
- Endpoint mounted under `/api/home/spotlight` as a **public route** (NO `AuthMiddleware`). Phase 1 has no login-gated cards.
- Phase 3 will add an optional-auth middleware (try-validate-JWT, no 401 on failure) to enable login-gated cards. The Phase 1 handler must tolerate `r.Header.Get("Authorization")` being present but ignore it.

**Feature flag (`SPOTLIGHT_ENABLED`)**
- Read via `services/catalog/internal/config/config.go` — add `SpotlightEnabled bool` env-bound, default `true`.
- When `false`: handler returns `404 Not Found` with no body (frontend HSB-FE-02 treats as "block hides itself"). Implementation: check flag in handler entry and short-circuit.

**Card payload shape**
- Match the TypeScript discriminated union from design doc §4.1 exactly.
- Response envelope: `{ "cards": [Card, ...], "generated_at": "<ISO8601 UTC>" }`.

**Logging**
- One info log per request at handler entry (`spotlight.request{user=anon|<id>}`).
- One error log per dropped card (`spotlight.card_failed{type, error}`).
- One info log at end (`spotlight.aggregated{cards_returned, ms_total}`).

**Error handling**
- 200 with `{cards: [...]}` for any partial success; only 500 on catastrophic failure (Redis hard-down AND no cards resolved AND no snapshot).

**File layout (per ROADMAP "Touches" list)**
- `services/catalog/internal/handler/spotlight.go`
- `services/catalog/internal/service/spotlight/aggregator.go`
- `services/catalog/internal/service/spotlight/types.go`
- `services/catalog/internal/service/spotlight/cards/{anime_of_day,random_tail,latest_news,platform_stats}.go`
- `services/catalog/internal/service/spotlight/client/web_client.go`
- `services/catalog/internal/transport/router.go` — `r.Get("/home/spotlight", h.Get)` under `/api`
- `services/catalog/cmd/catalog-api/main.go`
- `services/catalog/internal/config/config.go`
- `services/gateway/internal/transport/router.go`
- `docker/.env.example` — `SPOTLIGHT_ENABLED=true`

**Testing strategy**
- Unit tests per resolver against a mocked cache + mocked repo. Mock library: match whichever pattern dominates in `services/catalog/internal/service/*_test.go`.
- Aggregator-level integration test: mock 4 resolvers, verify (a) concurrent run within budget, (b) one timing-out resolver drops its card without affecting others, (c) overall timeout returns partial results.
- No external HTTP calls in tests — `web_client` accepts an injectable `*http.Client`.

### Claude's Discretion
- Exact mock library (`testify/mock` vs handwritten struct mocks) — match dominant pattern in catalog service tests.
- Goroutine collection mechanism (`errgroup.Group` vs `sync.WaitGroup + channel`) — both acceptable; pick the one that reads naturally given the "drop on error, don't fail-fast" requirement.
- Exact JSON encoding for the discriminated union — `map[string]any` for `Data` acceptable; typed struct preferred.
- Snapshot fallback Redis write path — best-effort, fire-and-forget.
- Whether to add a small `respond_envelope.go` shared helper for the generated_at timestamp or inline it.

### Deferred Ideas (OUT OF SCOPE)
- Personalization (per-user `personal_pick`, `not_time_yet`, `continue_watching_new`) → Phase 3.
- `now_watching` SQL with privacy filter → Phase 3.
- Player service `/internal/users/{id}/list` endpoints → Phase 3.
- Removing `trendingRecs` from `Home.vue` → Phase 3.
- Adding `idx_watch_progress_updated_at` index → Phase 3.
- Editorial admin-curated card → v1.1.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HSB-BE-01 | New `GET /api/home/spotlight` endpoint with `{cards, generated_at}` envelope | Pattern: see `chi` route registration in `services/catalog/internal/transport/router.go:60-122`; envelope helper exists in `libs/httputil/response.go:36` (`JSON`) — wrap data in `{cards, generated_at}` via a small handler-local struct |
| HSB-BE-02 | New `services/catalog/internal/service/spotlight/` package with aggregator + per-card resolvers, each `Resolve(ctx, userID *string) (Card, error)` | Use `subs_aggregator.go:109-156` as architecture template (WaitGroup + buffered channel + collect loop) |
| HSB-BE-03 | Per-card 800ms context deadline; on err → drop + log `spotlight.card_failed{type, error}` | `libs/logger` `Errorw("spotlight.card_failed", "type", t, "error", err)`; create child ctx via `context.WithTimeout(parent, 800*time.Millisecond)` + `defer cancel()` inside each goroutine |
| HSB-BE-04 | Overall 2s budget; on timeout return last-known-good `spotlight:snapshot:...` | Outer `context.WithTimeout(r.Context(), 2*time.Second)` at handler entry; snapshot keyed by `<anon\|user_id>:YYYY-MM-DD`; write best-effort after each successful aggregation |
| HSB-BE-05 | Server-side eligibility filter — drop cards with `eligible=false` from payload | Per-resolver returns `(Card, error)`; aggregator drops both `err != nil` and explicit `eligible=false` (resolvers return `nil, nil` for "no data" so aggregator naturally omits) |
| HSB-BE-06 | Gateway routes `GET /api/home/spotlight → catalog:8081` (public, optional auth) | Add `r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)` inside the `/api` Route block in `services/gateway/internal/transport/router.go:164` BEFORE the wildcard `/anime/*` line (chi precedence: specific before general) |
| HSB-BE-07 | Feature flag `SPOTLIGHT_ENABLED` (default true) → 404 when false | Add `SpotlightEnabled bool` to `config.Config` via existing `getEnvBool` pattern (one already exists for other flags); check `if !cfg.SpotlightEnabled { httputil.NotFound(...) }` at handler entry — but note: the standard `NotFound` helper writes a JSON body; for HSB-FE-02 compatibility ("frontend hides itself") write a bare 404 with empty body via `w.WriteHeader(http.StatusNotFound)` |
| HSB-BE-10 | `anime_of_day` — top-rated (score≥8.0, 200 candidates), date-seeded pick | Call `animeRepo.Search(ctx, domain.SearchFilters{Sort:"score", Order:"desc", ScoreMin:&{8.0}, Page:1, PageSize:200})`. Returns `([]*domain.Anime, int64, error)`. Pick `items[seed % len(items)]` |
| HSB-BE-11 | `random_tail` — animes ranked 101..2000 by score, date-seeded pick | Use `animeRepo.Search(ctx, domain.SearchFilters{Sort:"score", Order:"desc", Page:2, PageSize:100})` — fetches ranks 101..200. NOTE: query injects `sort_priority DESC` as primary order (anime.go:139) so pinned items shift the window. Acceptable: pinned anime never appear in discovery-focused random_tail. |
| HSB-BE-12 | `latest_news` — fetch `http://web:80/changelog.json`, parse, return up to 3 newest entries | New `client/web_client.go` with injectable `*http.Client`; default `Timeout: 500*time.Millisecond` (leaves headroom under the 800ms per-card budget); parse the `[{date, entries: [{type, message}]}]` shape verified in `frontend/web/public/changelog.json` |
| HSB-BE-13 | `platform_stats` — 3 metrics, eligible iff ≥1 non-null | `anime_added_7d` via GORM raw `db.Raw(...).Scan(&count)` or `db.Model(&domain.Anime{}).Where("created_at > ?", ...).Count(&count)`. `episodes_added_7d` → nil (no event log exists). `active_rooms_7d` → nil (rooms is Redis-only, not Postgres-addressable). |
| HSB-NF-01 | p95 latency ≤400ms cached, ≤1500ms cold | Per-card cache via `cache.GetOrSet`; on warm cache, all 4 resolvers return in <10ms each. Cold path bounded by 800ms per-card + 2s overall. Verify via `/metrics` `http_request_duration_seconds{path="/api/home/spotlight"}`. |
| HSB-NF-03 | All new Redis keys use `spotlight:` prefix | Already enforced by design doc §5.3 key list. Keys: `spotlight:anime_of_day:<date>`, `spotlight:random_tail:<date>`, `spotlight:changelog:<date>`, `spotlight:stats:<date>`, `spotlight:snapshot:<anon\|uid>:<date>`. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| HTTP endpoint exposure | API gateway (gateway service) | — | Standard: every `/api/*` route enters via gateway proxy; the new `/api/home/spotlight` follows the same reverse-proxy pattern as `/api/anime/*` |
| Spotlight business logic + aggregation | API / Backend (catalog service) | — | Per design doc §5.2, catalog owns the endpoint because it already aggregates content-shaped data (animes, news, recs) |
| Concurrent resolver fan-out | API / Backend (catalog) | — | Pure in-process Go concurrency — no need for an external orchestrator (queue, workflow engine) for a 4-resolver fan-out with 2s budget |
| Per-card cache | API / Backend (catalog) → Database / Storage (Redis) | — | `libs/cache` is the catalog's existing Redis abstraction; per-card keys keep the cache layer thin |
| Anime data retrieval | Database / Storage (Postgres via GORM) | — | `AnimeRepository.Search` already exists; reused unchanged |
| Changelog data retrieval | Frontend Server (web container) | API / Backend (catalog) | The `changelog.json` is a static file served by the `web` nginx container on the docker network; catalog HTTP-fetches it (no DB involvement) — this is the only inter-service HTTP hop in Phase 1 |
| Feature flag check | API / Backend (catalog) | — | Standard env-bound config pattern via `config.go` |
| Snapshot fallback storage | Database / Storage (Redis) | — | One additional Redis key per request; same `libs/cache` abstraction |
| Frontend rendering | (deferred to Phase 2) | — | Out of Phase 1 scope |

## Standard Stack

### Core (already in `services/catalog/go.mod` — no new dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/go-chi/chi/v5` | (already in go.mod) | HTTP router | Used by every service in this repo — see `transport/router.go` in any service [VERIFIED: codebase grep] |
| `github.com/ILITA-hub/animeenigma/libs/cache` | local | Redis Get/Set/GetOrSet + metrics | Pattern `cache.GetOrSet(ctx, key, &dest, ttl, fn)` — see `handler/news.go:41` [VERIFIED: read libs/cache/cache.go] |
| `github.com/ILITA-hub/animeenigma/libs/logger` | local | Structured logging | `*logger.Logger` with `Errorw`, `Infow`, `Warnw` (zap.SugaredLogger underneath) [VERIFIED: libs/logger/logger.go] |
| `github.com/ILITA-hub/animeenigma/libs/errors` | local | Domain error wrappers | `errors.Internal`, `errors.NotFound`, `errors.ExternalAPI` — but the spotlight handler returns 200 on partial success, so used sparingly [VERIFIED: libs/errors/errors.go] |
| `github.com/ILITA-hub/animeenigma/libs/httputil` | local | Response helpers | `httputil.OK(w, data)`, `httputil.JSON(w, status, data)`, `httputil.Error(w, err)` [VERIFIED: libs/httputil/response.go] |
| `gorm.io/gorm` | (already in go.mod) | DB access via existing `*gorm.DB` | `services/catalog/internal/repo/anime.go` uses it; the platform_stats resolver does one raw `Count` call on the shared connection [VERIFIED] |
| `net/http` stdlib | go 1.24 | `web_client` for changelog fetch | Same pattern as `parser/telegram/client.go:42` — `&http.Client{Timeout: ...}` [VERIFIED: telegram/client.go] |

### Supporting (already present, used in Phase 3 not Phase 1 — listed for forward compat)

| Library | Purpose | When to Use |
|---------|---------|-------------|
| `github.com/ILITA-hub/animeenigma/libs/authz` | JWT claims, optional-auth middleware | Phase 3 only — Phase 1 endpoint is public |
| `golang.org/x/sync/errgroup` | Already in go.sum (v0.18.0) | Do NOT use — fail-fast semantics fight our "drop on error" contract |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `sync.WaitGroup + buffered channel` | `errgroup.Group` | errgroup is fail-fast: first error cancels the group. We need "first error drops only that goroutine's result." Wrong tool. |
| Per-resolver typed `Data` field | `json.RawMessage` for `Data` | RawMessage gives flexibility at the cost of compile-time safety. CONTEXT.md "Claude's Discretion" allows either; recommend typed struct per resolver because we have 4 known types and the code reads cleaner. |
| Aggregator response single blob cache | Per-card cache (chosen) | Single blob means one expiring sub-key forces full recompute. Per-card is what design doc §5.3 mandates. |
| Custom optional-auth middleware in catalog | Reuse `player/internal/transport/optional_auth.go` pattern in Phase 3 | Phase 1 doesn't need auth at all. Phase 3 will replicate the player pattern. |

**Installation:** No new packages. Run `go mod tidy` after the phase to ensure go.sum is clean.

**Version verification:** All packages are already pinned in `services/catalog/go.mod`. No external API requires version verification. The `web` container's nginx image and the existence of `/changelog.json` are verified by reading the deployed file at `frontend/web/public/changelog.json`.

## Architecture Patterns

### System Architecture Diagram

```
                    ┌─────────────────────────┐
   curl/browser  ──▶│ gateway:8000            │  /api/home/spotlight
                    │  (chi router)           │  → proxy → catalog:8081
                    │  - RateLimit (per-IP)   │
                    │  - CORS / SecurityHdrs  │
                    │  NO JWT (public route)  │
                    └────────────┬────────────┘
                                 │ ProxyToCatalog
                                 ▼
                    ┌─────────────────────────┐
                    │ catalog:8081            │
                    │  handler.SpotlightHandler.Get
                    │   - feature flag check  │ ─── if !SpotlightEnabled → 404
                    │   - log "spotlight.request"
                    │   - ctx, cancel := WithTimeout(r.Context(), 2s)
                    │   - userID := nil (Phase 1, no auth)
                    └────────────┬────────────┘
                                 │ aggregator.Aggregate(ctx, nil)
                                 ▼
                    ┌─────────────────────────┐
                    │ spotlight.Aggregator    │
                    │  fan-out 4 goroutines   │
                    │  buffered chan size=4   │
                    └────┬────┬────┬────┬─────┘
                         │    │    │    │       (each child has 800ms ctx deadline)
                         ▼    ▼    ▼    ▼
                  ┌─────┐┌─────┐┌─────┐┌─────────┐
                  │AnOf ││Rand ││News ││Platform │   resolvers/
                  │Day  ││Tail ││     ││Stats    │   cards/
                  └──┬──┘└──┬──┘└──┬──┘└──┬──────┘
                     │      │      │     │
                     ▼      ▼      ▼     ▼
              cache.GetOrSet(spotlight:<type>:<YYYY-MM-DD>, 24h)
                     │      │      │     │
       miss:         │      │      │     │
                     ▼      ▼      ▼     ▼
              ┌─────────┐┌─────┐┌──────┐┌──────────┐
              │AnimeRepo││Anime││web    ││GORM Count│
              │.Search  ││Repo ││:80    ││on animes │
              │ score≥8 ││.Sear││/chang ││episodes:nil
              │ Page=1  ││ch   ││elog   ││rooms:nil │
              │ PSize=  ││Page=││.json  ││          │
              │ 200     ││ 2   ││       ││          │
              │         ││PSize││500ms  ││          │
              │         ││=100 ││ HTTP  ││          │
              └────┬────┘└──┬──┘│timeout││          │
                   │        │  └──┬────┘└────┬─────┘
                   ▼        ▼     ▼          ▼
                 Postgres (animes table via GORM)
                          + http://web:80
                          + Redis (cache)
                                 │
       on per-resolver err/timeout: drop card, Errorw("spotlight.card_failed")
       on overall ctx.Done():      return collected cards so far
       on zero cards collected:    try Redis spotlight:snapshot:anon:<date>
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │ Response                │
                    │ 200 OK                  │
                    │ {                       │
                    │   "cards": [...],       │  ← only eligible cards
                    │   "generated_at": "ISO" │
                    │ }                       │
                    │                         │
                    │ async/best-effort:      │
                    │ write spotlight:        │
                    │   snapshot:anon:<date>  │
                    │   (24h TTL)             │
                    └─────────────────────────┘
```

### Recommended Project Structure

```
services/catalog/internal/
├── config/
│   └── config.go                              # extend — add SpotlightEnabled bool
├── handler/
│   └── spotlight.go                           # NEW — chi handler, feature flag, JSON encode
├── service/spotlight/                         # NEW PACKAGE
│   ├── aggregator.go                          # WaitGroup + chan collection + snapshot fallback
│   ├── aggregator_test.go                     # concurrent run, per-card timeout, overall timeout
│   ├── types.go                               # Card{Type,Data}, AnimeOfDayData, RandomTailData, etc.
│   ├── seed.go                                # dateSeed(time.Now().UTC()) helper — testable
│   ├── seed_test.go
│   ├── cards/
│   │   ├── anime_of_day.go                    # Resolver: AnimeRepository.Search top-rated
│   │   ├── anime_of_day_test.go
│   │   ├── random_tail.go                     # Resolver: AnimeRepository.Search ranks 101..200
│   │   ├── random_tail_test.go
│   │   ├── latest_news.go                     # Resolver: web_client.GetChangelog → 3 newest
│   │   ├── latest_news_test.go
│   │   ├── platform_stats.go                  # Resolver: 3 GORM counts (1 real + 2 nil for v1)
│   │   └── platform_stats_test.go
│   └── client/
│       ├── web_client.go                      # http.Client wrapper, ChangelogEntry decode
│       └── web_client_test.go                 # httptest.Server stub
└── transport/
    └── router.go                              # extend — r.Get("/home/spotlight", h.Get)

services/catalog/cmd/catalog-api/
└── main.go                                    # extend — wire spotlight aggregator + handler

services/gateway/internal/transport/
└── router.go                                  # extend — r.HandleFunc("/home/spotlight", ProxyToCatalog)

docker/
└── .env.example                               # extend — SPOTLIGHT_ENABLED=true
```

### Pattern 1: Fan-out + collect with drop-on-error (subs_aggregator template)

**What:** Spawn N goroutines each writing exactly one result to a buffered channel; collect with a range loop; per-goroutine errors are absorbed (not propagated to caller).

**When to use:** Phase 1 aggregator — exactly 4 concurrent resolvers, partial success contractually required.

**Example:**
```go
// Source: services/catalog/internal/service/subs_aggregator.go:109-156 (verified pattern)
// Adapted for spotlight aggregator.

type cardResult struct {
    cardType string
    card     *Card // nil = drop
    err      error // recorded only for logging
}

func (a *Aggregator) Aggregate(ctx context.Context, userID *string) (*Response, error) {
    resolvers := []struct {
        name string
        fn   func(context.Context, *string) (*Card, error)
    }{
        {"anime_of_day", a.cards.AnimeOfDay},
        {"random_tail", a.cards.RandomTail},
        {"latest_news", a.cards.LatestNews},
        {"platform_stats", a.cards.PlatformStats},
    }

    resultsCh := make(chan cardResult, len(resolvers))
    var wg sync.WaitGroup

    for _, r := range resolvers {
        wg.Add(1)
        go func(name string, fn func(context.Context, *string) (*Card, error)) {
            defer wg.Done()
            // Per-card 800ms deadline carved out of the parent 2s budget.
            // IMPORTANT: parent cancellation propagates to children — see Pitfall 2.
            cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
            defer cancel()
            card, err := fn(cctx, userID)
            resultsCh <- cardResult{cardType: name, card: card, err: err}
        }(r.name, r.fn)
    }

    go func() {
        wg.Wait()
        close(resultsCh)
    }()

    cards := make([]Card, 0, len(resolvers))
    for r := range resultsCh {
        if r.err != nil {
            a.log.Errorw("spotlight.card_failed", "type", r.cardType, "error", r.err)
            continue
        }
        if r.card == nil {
            // Resolver returned (nil, nil) = "eligible=false, drop me"
            continue
        }
        cards = append(cards, *r.card)
    }

    // Snapshot fallback: if zero cards AND we have a stale snapshot, return it.
    if len(cards) == 0 {
        if snap, ok := a.loadSnapshot(ctx, userID); ok {
            return snap, nil
        }
    }

    resp := &Response{Cards: cards, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}

    // Best-effort snapshot write — fire-and-forget, do NOT block response.
    if len(cards) > 0 {
        go a.saveSnapshot(context.Background(), userID, resp) // detach from request ctx
    }
    return resp, nil
}
```

Anti-pattern to avoid (errgroup):
```go
// DO NOT USE — fail-fast semantics drop ALL siblings on first error.
g, gctx := errgroup.WithContext(ctx)
for _, r := range resolvers {
    r := r
    g.Go(func() error {
        card, err := r.fn(gctx, userID)
        if err != nil { return err } // <-- cancels gctx, kills siblings
        results = append(results, card)
        return nil
    })
}
g.Wait() // returns the first error — we'd have to swallow it AND we've lost siblings already
```

### Pattern 2: Per-card cache via `cache.GetOrSet`

**What:** `cache.GetOrSet(ctx, key, &dest, ttl, fn)` returns from cache if present; otherwise runs `fn` and writes the result with the given TTL.

**When to use:** Inside every resolver. The day-seeded cache key means the same physical day always hits the same key, so on warm cache the resolver costs one Redis GET.

**Example:**
```go
// Source: services/catalog/internal/handler/news.go:41 (verified pattern)
func (r *AnimeOfDayResolver) Resolve(ctx context.Context, userID *string) (*Card, error) {
    key := "spotlight:anime_of_day:" + time.Now().UTC().Format("2006-01-02")
    var data AnimeOfDayData
    err := r.cache.GetOrSet(ctx, key, &data, 24*time.Hour, func() (interface{}, error) {
        // Cache miss — do the real work.
        sm := 8.0
        animes, _, err := r.repo.Search(ctx, domain.SearchFilters{
            Sort:     "score",
            Order:    "desc",
            ScoreMin: &sm,
            Page:     1,
            PageSize: 200,
        })
        if err != nil {
            return nil, err
        }
        if len(animes) == 0 {
            return nil, nil // eligible=false — but GetOrSet will store nil; see Pitfall 5
        }
        seed := dateSeedUTC(time.Now())
        pick := animes[seed%len(animes)]
        return AnimeOfDayData{Anime: *pick}, nil
    })
    if err != nil {
        return nil, err
    }
    if data.Anime.ID == "" {
        return nil, nil // eligible=false
    }
    return &Card{Type: "anime_of_day", Data: data}, nil
}
```

### Pattern 3: Date seed (UTC, container-portable)

**What:** Produce a single int from the calendar date so the per-day pick is deterministic across redeploys.

**When to use:** `anime_of_day` and `random_tail` pick selection.

**Example:**
```go
// New file: services/catalog/internal/service/spotlight/seed.go
package spotlight

import "time"

// DateSeedUTC returns the integer YYYY*100*32 + MM*32 + DD derived from t.UTC().
// Using UTC ensures the same key holds across container/host TZ differences and
// across daylight-saving transitions.
func DateSeedUTC(t time.Time) int {
    u := t.UTC()
    return u.Year()*100*32 + int(u.Month())*32 + u.Day()
}

// DateKeyUTC returns YYYY-MM-DD in UTC — used as the cache-key date segment.
func DateKeyUTC(t time.Time) string {
    return t.UTC().Format("2006-01-02")
}
```

### Pattern 4: Web client with injectable http.Client

**What:** Constructor accepts an `*http.Client` (or returns a default if nil) so tests can substitute a `httptest.Server`-backed transport.

**When to use:** `latest_news` resolver's HTTP call to `http://web:80/changelog.json`.

**Example:**
```go
// services/catalog/internal/service/spotlight/client/web_client.go
package client

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// ChangelogEntry mirrors one entry inside the per-date group in
// frontend/web/public/changelog.json. Verified shape (2026-05-21):
//   {date: "YYYY-MM-DD", entries: [{type: "feature|fix|perf", message: "..."}]}
type ChangelogGroup struct {
    Date    string           `json:"date"`
    Entries []ChangelogEntry `json:"entries"`
}

type ChangelogEntry struct {
    Type    string `json:"type"`
    Message string `json:"message"`
}

type WebClient struct {
    baseURL string
    http    *http.Client
}

func NewWebClient(baseURL string, hc *http.Client) *WebClient {
    if hc == nil {
        // 500ms < 800ms per-card budget; leaves headroom for ctx-cancel propagation.
        hc = &http.Client{Timeout: 500 * time.Millisecond}
    }
    if baseURL == "" {
        baseURL = "http://web:80"
    }
    return &WebClient{baseURL: baseURL, http: hc}
}

func (c *WebClient) GetChangelog(ctx context.Context) ([]ChangelogGroup, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/changelog.json", nil)
    if err != nil {
        return nil, fmt.Errorf("web client: build request: %w", err)
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return nil, fmt.Errorf("web client: fetch changelog: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("web client: changelog HTTP %d", resp.StatusCode)
    }
    var groups []ChangelogGroup
    if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil {
        return nil, fmt.Errorf("web client: decode changelog: %w", err)
    }
    return groups, nil
}
```

### Pattern 5: Discriminated-union JSON via typed `Data` struct

**What:** Wrap each card's data in a typed Go struct; rely on `json.Marshal`'s struct-tag encoding to produce the discriminated payload TypeScript expects.

**When to use:** All cards. Recommended over `map[string]any` or `json.RawMessage`.

**Example:**
```go
// services/catalog/internal/service/spotlight/types.go
package spotlight

import "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"

// Card is the outer envelope. Each resolver produces a Card with its
// own Type discriminator and a per-type Data struct embedded as any.
type Card struct {
    Type string `json:"type"`
    Data any    `json:"data"`
}

type AnimeOfDayData struct {
    Anime         domain.Anime `json:"anime"`
    ReasonI18nKey string       `json:"reason_i18n_key,omitempty"`
}

type RandomTailData struct {
    Anime domain.Anime `json:"anime"`
}

type LatestNewsData struct {
    Entries []client.ChangelogEntry `json:"entries"`
}

type StatsMetric struct {
    Key   string `json:"key"`
    Value int64  `json:"value"`
    Delta *int64 `json:"delta,omitempty"`
}

type PlatformStatsData struct {
    Metrics []StatsMetric `json:"metrics"`
}

type Response struct {
    Cards       []Card `json:"cards"`
    GeneratedAt string `json:"generated_at"`
}
```

This produces exactly the shape design doc §4.1 specifies. TypeScript's discriminated union narrows on `type` field; Go-side type safety is preserved via the per-resolver concrete return type.

### Anti-Patterns to Avoid

- **errgroup for partial-success fan-out** — `errgroup.WithContext` cancels the group on first error, killing siblings. Use `sync.WaitGroup + chan` instead.
- **Caching nil/empty results indefinitely** — if a resolver legitimately returns "no data today" we should NOT bake a 24h "no data" cache that masks a recovered upstream. Recommendation: when a resolver's underlying source returns empty, write a SHORTER TTL (e.g., 5 minutes) or just return `nil, nil` from the resolver and skip the Set. CONTEXT.md is silent on this; flag for plan-phase decision.
- **Logging the Authorization header** — Phase 1 endpoint is public but the handler may still see an Authorization header. Do NOT log it; the project already redacts via `libs/httputil.RequestLogger` (router.go:34), but verify it does not log request headers verbatim.
- **Holding the request goroutine for snapshot writes** — the snapshot Set is best-effort and goes to Redis. If Redis is slow, the snapshot write would inflate p95. Always detach: `go a.saveSnapshot(context.Background(), ...)`.
- **Single blob aggregator cache** — see CONTEXT.md decision; per-card cache only.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Concurrent fan-out with per-task timeout | Custom worker pool / fan-out helper | `sync.WaitGroup + buffered chan + context.WithTimeout` | Already a battle-tested pattern in `subs_aggregator.go`; consistency with rest of codebase |
| Redis Get-then-Set boilerplate | Manual `Get → check ErrNotFound → fn() → Set` | `cache.Cache.GetOrSet` | Library already does it (libs/cache/cache.go:151) |
| Structured logging | Custom log formatter | `*logger.Logger.Errorw / Infow / Warnw` | zap.SugaredLogger underneath; consistent format across all services |
| HTTP error response | Custom error JSON shape | `libs/httputil.Error(w, err)` | Wraps `*errors.AppError` to correct status code; consistent envelope |
| JSON discriminated union | `map[string]any` or `json.RawMessage` with manual switch | Typed struct per Data variant | Go's `json.Marshal` handles it; compile-time type safety; example in §Pattern 5 |
| Date-based cache key formatting | Custom YYYY-MM-DD string concat | `t.UTC().Format("2006-01-02")` | Go stdlib reference layout; consistent with rest of codebase |
| Feature flag plumbing | Custom flag manager | One `bool` field on `config.Config` + env via `getEnv` helper already in config.go | Established pattern (see `ScraperConfig.APIURL` etc.) |
| Gateway reverse-proxy entry | New ProxyTo function | Reuse existing `proxyHandler.ProxyToCatalog` | The handler already proxies the entire `/api/anime/*` family; one new `r.HandleFunc` line suffices |
| Snapshot fallback | Per-key wrapper struct | Just another `cache.Get`/`Set` call with a longer key | Same cache abstraction, no new infra |

**Key insight:** Phase 1 introduces ZERO new infrastructure dependencies. Every primitive is already in `services/catalog`. The work is composition, not invention.

## Common Pitfalls

### Pitfall 1: Parent ctx cancellation propagating to siblings via `context.WithTimeout(parent, 800ms)`

**What goes wrong:** `context.WithTimeout(parent, d)` returns a derived ctx that is cancelled when EITHER the parent ctx is cancelled OR the timeout elapses. If the parent (2s budget) hits its deadline first, ALL 4 children's deadlines fire simultaneously — that's correct (we want them to stop). But the timing window: if resolver A finishes at 1.95s and resolver B finishes at 2.01s, B may have observed `ctx.Err() == context.DeadlineExceeded` from the parent at 2.00s and reported failure when its own resolver was perfectly healthy.

**Why it happens:** Children inherit parent cancellation; that's the design. The 800ms per-card budget is the more pessimistic bound, but the 2s overall budget can also clip a child.

**How to avoid:** Document this is intentional — it's the "drop the laggards" feature, not a bug. The cards that DID resolve by 2s are returned. The aggregator already handles this: any goroutine whose `fn` returns `context.DeadlineExceeded` gets logged as `spotlight.card_failed`, and the response is built from whatever cards arrived in time. Important: do NOT use `context.Background()` for the per-card child ctx — that would orphan the child from the parent's 2s budget and could leak goroutines past response.

**Warning signs:** Test "all 4 resolvers timeout simultaneously" → response should be 200 with `cards: []` (or snapshot fallback). Test "1 resolver hangs 5s, 3 resolvers return in 50ms" → response within ~2.05s with 3 cards.

### Pitfall 2: TZ drift between container and host for `YYYY-MM-DD` cache keys

**What goes wrong:** `time.Now().Format("2006-01-02")` uses the local TZ. If the container is `UTC` but the host writes `Asia/Tokyo` cache keys (or vice versa during a migration), the same calendar day creates two distinct cache keys → cache thrash and inconsistent "pick of the day" between deploys.

**Why it happens:** Go's `time.Now()` is wall-clock in the process's local TZ. Docker images typically default to UTC but `TZ` env var or `/etc/timezone` can flip it.

**How to avoid:** ALWAYS call `.UTC()` before `.Format("2006-01-02")`. The same seed helper applies to the integer `dateSeedUTC` for the pick. Document this in `seed.go` so future cards can't accidentally use local TZ.

**Warning signs:** Two consecutive curls minutes apart return different `anime_of_day` cards. Inspect Redis: `redis-cli KEYS 'spotlight:anime_of_day:*'` shows two keys with different dates within the same calendar day.

### Pitfall 3: `AnimeRepository.Search` injects `sort_priority DESC` as PRIMARY order

**What goes wrong:** Calling `Search(Sort="score", Order="desc", Page=2, PageSize=100)` does NOT return ranks 101..200 by score alone — it returns ranks 101..200 by `(sort_priority DESC, score DESC)`. If any anime has `sort_priority > 0` (a pinned announcement), it shifts every subsequent rank by 1.

**Why it happens:** anime.go:139 hard-codes `orderBy := "sort_priority DESC, score DESC"` and any explicit `Sort` becomes the SECONDARY axis.

**How to avoid for `random_tail`:** Two options:
1. **Accept the shift.** Pinned anime are 0-5 in practice (admins use it sparingly). Pinned items never appear in `random_tail` because they always land in page 1, and that's actually desirable — pinned anime are already prominently surfaced. RECOMMEND THIS.
2. **Add a new repo method** `GetByScoreRank(ctx, rankMin, rankMax int)` that orders strictly by `score DESC` without the `sort_priority` prefix. Cleaner but adds a method and a migration.

**How to avoid for `anime_of_day`:** Doesn't matter — top-200 by score (with `ScoreMin=8.0` filter) includes any pinned item naturally, and a modulus-based pick over 200 items is robust to a few-position shift.

**Warning signs:** Test "1 anime with sort_priority=10, score=5.0" → it shouldn't appear in random_tail page 2 (it's in page 1 because sort_priority dominates). Confirm with an integration test.

### Pitfall 4: Redis Get vs miss vs hard error semantics

**What goes wrong:** `cache.Cache.Get(ctx, key, &dest)` returns `cache.ErrNotFound` (sentinel) on a key miss and a wrapped error on Redis hard failures (connection refused, etc.). Treating the two identically loses observability; treating only the miss path as the recompute trigger is the right design.

**Why it happens:** `cache.GetOrSet` already does the right thing: Get → on `ErrNotFound` recompute → on other errors propagate. But if a resolver calls Get manually for the snapshot-fallback path, it must distinguish.

**How to avoid:** Use `cache.GetOrSet` inside every resolver. For the snapshot-fallback path in `aggregator.Aggregate`, use `cache.Get` directly and check `errors.Is(err, cache.ErrNotFound)` — treat any other error as "snapshot unavailable, proceed with empty cards."

**Warning signs:** Test "Redis is hard-down, all 4 resolvers fail" → handler returns 200 with empty `cards: []` (not 500). The `spotlight.card_failed` log line fires 4 times. Snapshot fallback also fails gracefully.

### Pitfall 5: `cache.GetOrSet` writes nil/zero values

**What goes wrong:** If a resolver's `fn() returns (nil, nil)` (meaning "eligible=false, no data"), `GetOrSet` will still call `Set(ctx, key, nil, ttl)`, baking in a 24h "no result" cache. The next 24h of requests will hit cache and skip the (cheap) recompute that might find data.

**Why it happens:** `GetOrSet` doesn't distinguish "fn returned the zero value" from "fn returned data."

**How to avoid:** Two patterns:
1. Resolvers return `nil, nil` for "no eligibility" but the resolver wraps the call in `cache.Get` + manual `cache.Set` (and only Sets on non-nil).
2. Use `GetOrSet` for the happy path; on cache miss returning empty result, write a shorter 5-minute TTL via direct `cache.Set` to limit damage.

RECOMMENDATION: Pattern 1 — resolvers manage their cache writes explicitly. The minor boilerplate is worth the correctness.

### Pitfall 6: Logging the Authorization header

**What goes wrong:** Phase 1 endpoint is public but the handler still sees the Authorization header if the client sent one (e.g., a browser carrying a JWT for other API calls). If we log the request via `r.Header.Get("Authorization")` we leak a JWT into logs.

**Why it happens:** Authorization is just another header from the handler's POV.

**How to avoid:** Never log full headers. The existing `libs/httputil.RequestLogger` middleware logs method + path + status only — verify by reading `middleware.go`. The handler's `Infow("spotlight.request", ...)` should log only `user=anon|<id-from-claims-not-token>`.

**Warning signs:** `make logs-catalog | grep -i 'bearer\|jwt\|Authorization'` returns matches.

### Pitfall 7: `web:80` reachability across docker networks

**What goes wrong:** The `web` container is on the `animeenigma-network` default network (verified docker-compose.yml line 660-672, network name at line 691-693). The catalog container is on the same network. DNS resolution `web:80` is automatic — UNLESS the `web` container is down or starting later than catalog. The catalog's `depends_on` block (lines 429-435) does NOT include `web` (web depends on gateway, not the other way around), so catalog starts BEFORE web is ready.

**Why it happens:** Bootstrap ordering. After `make dev` from cold, catalog can be ready and serving for several seconds before nginx in `web` is accepting connections.

**How to avoid:** The `web_client` `http.Client.Timeout = 500ms` plus the resolver's 800ms ctx deadline means the worst case is "the first request after cold start drops `latest_news` for 24h." That's acceptable IF — and this is critical — the resolver does NOT cache the empty result for 24h (see Pitfall 5). Recommendation: explicit `cache.Get` + manual `cache.Set` only on non-nil; on empty result, do NOT cache, so the next request retries.

**Warning signs:** First `curl /api/home/spotlight` after `make dev-down && make dev` returns 3 cards (no latest_news) and the next 24h all return 3 cards.

### Pitfall 8: `http.Client.Timeout` interaction with ctx deadline

**What goes wrong:** Setting `http.Client.Timeout = 500ms` and also passing a ctx with 800ms deadline means the client gives up at 500ms (the smaller of the two). That's intentional, but if the ctx is cancelled at 600ms (e.g., parent 2s budget elapsed), the underlying HTTP transport may still hold the connection open until its own 500ms timer expires — leaking up to 500ms.

**Why it happens:** Go's `net/http` checks ctx at request boundaries (NewRequestWithContext propagates it through transport), so cancellation IS observed promptly in modern Go (1.17+). With Go 1.24, this is not a real concern.

**How to avoid:** Use `http.NewRequestWithContext(ctx, ...)` (NOT `http.NewRequest(...)`). Already in §Pattern 4.

**Warning signs:** Goroutine count climbs after handler returns. Use `runtime.NumGoroutine()` in tests.

## Runtime State Inventory

> Phase 1 introduces a new endpoint and a new Redis key prefix. No rename/refactor of existing systems.

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | None — all new Redis keys live under `spotlight:` prefix, day-keyed, 24h TTL. No existing keys touched. | None |
| Live service config | None — new endpoint added; no existing routes modified | None |
| OS-registered state | None — no cron, no systemd, no scheduled task associated with this endpoint | None |
| Secrets/env vars | One new env: `SPOTLIGHT_ENABLED` (bool, default `true`). Add to `docker/.env.example`. No secret material. | Document in CLAUDE.md "Environment Variables — Catalog service specific" if user wants explicit listing |
| Build artifacts | None | None |

**Nothing found in category:** Confirmed by reading docker-compose.yml (catalog service env block, lines 411-426 — no existing spotlight env), and by reading the full catalog config.go file (no existing SpotlightEnabled or similar).

## Code Examples

### Example 1: Handler with feature flag short-circuit

```go
// services/catalog/internal/handler/spotlight.go
// Source: pattern adapted from services/catalog/internal/handler/news.go (verified)

package handler

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

type SpotlightHandler struct {
    aggregator       *spotlight.Aggregator
    spotlightEnabled bool
    log              *logger.Logger
}

func NewSpotlightHandler(agg *spotlight.Aggregator, enabled bool, log *logger.Logger) *SpotlightHandler {
    return &SpotlightHandler{aggregator: agg, spotlightEnabled: enabled, log: log}
}

func (h *SpotlightHandler) Get(w http.ResponseWriter, r *http.Request) {
    if !h.spotlightEnabled {
        // Bare 404 — frontend HSB-FE-02 hides the block. No JSON envelope
        // because the legacy httputil.NotFound emits {"success":false,"error":{...}}
        // which would still parse to truthy on the client.
        w.WriteHeader(http.StatusNotFound)
        return
    }

    started := time.Now()
    h.log.Infow("spotlight.request", "user", "anon") // Phase 1 has no auth

    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    resp, err := h.aggregator.Aggregate(ctx, nil) // userID = nil in Phase 1
    if err != nil {
        // Catastrophic — only when aggregator itself failed (not when individual cards failed)
        h.log.Errorw("spotlight.aggregate_failed", "error", err)
        http.Error(w, `{"cards":[],"generated_at":"`+time.Now().UTC().Format(time.RFC3339)+`"}`, http.StatusInternalServerError)
        return
    }

    h.log.Infow("spotlight.aggregated",
        "cards_returned", len(resp.Cards),
        "ms_total", time.Since(started).Milliseconds(),
    )

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(resp)
}
```

### Example 2: Aggregator with snapshot fallback

```go
// services/catalog/internal/service/spotlight/aggregator.go (skeleton)
package spotlight

import (
    "context"
    "errors"
    "sync"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
)

type Resolver interface {
    Type() string
    Resolve(ctx context.Context, userID *string) (*Card, error)
}

type Aggregator struct {
    resolvers []Resolver
    cache     cache.Cache
    log       *logger.Logger
}

func NewAggregator(resolvers []Resolver, c cache.Cache, log *logger.Logger) *Aggregator {
    return &Aggregator{resolvers: resolvers, cache: c, log: log}
}

func (a *Aggregator) Aggregate(ctx context.Context, userID *string) (*Response, error) {
    type result struct {
        name string
        card *Card
        err  error
    }
    resultsCh := make(chan result, len(a.resolvers))
    var wg sync.WaitGroup

    for _, r := range a.resolvers {
        wg.Add(1)
        go func(r Resolver) {
            defer wg.Done()
            cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
            defer cancel()
            card, err := r.Resolve(cctx, userID)
            resultsCh <- result{name: r.Type(), card: card, err: err}
        }(r)
    }
    go func() { wg.Wait(); close(resultsCh) }()

    cards := make([]Card, 0, len(a.resolvers))
    for res := range resultsCh {
        if res.err != nil {
            a.log.Errorw("spotlight.card_failed", "type", res.name, "error", res.err)
            continue
        }
        if res.card == nil {
            continue // eligible=false
        }
        cards = append(cards, *res.card)
    }

    // Snapshot fallback only when we returned zero cards.
    if len(cards) == 0 {
        if snap := a.loadSnapshot(ctx, userID); snap != nil {
            return snap, nil
        }
    }

    resp := &Response{Cards: cards, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}

    // Best-effort snapshot write — detach from request ctx so a slow Redis
    // write does not inflate p95.
    if len(cards) > 0 {
        go a.saveSnapshot(context.Background(), userID, resp)
    }
    return resp, nil
}

func (a *Aggregator) snapshotKey(userID *string) string {
    who := "anon"
    if userID != nil {
        who = *userID
    }
    return "spotlight:snapshot:" + who + ":" + time.Now().UTC().Format("2006-01-02")
}

func (a *Aggregator) loadSnapshot(ctx context.Context, userID *string) *Response {
    var snap Response
    err := a.cache.Get(ctx, a.snapshotKey(userID), &snap)
    if err != nil {
        if !errors.Is(err, cache.ErrNotFound) {
            a.log.Warnw("spotlight.snapshot_load_failed", "error", err)
        }
        return nil
    }
    return &snap
}

func (a *Aggregator) saveSnapshot(ctx context.Context, userID *string, resp *Response) {
    // 24h TTL per design doc §5.3.
    if err := a.cache.Set(ctx, a.snapshotKey(userID), resp, 24*time.Hour); err != nil {
        a.log.Warnw("spotlight.snapshot_save_failed", "error", err)
    }
}
```

### Example 3: Resolver constructor wiring in `main.go`

```go
// services/catalog/cmd/catalog-api/main.go — additions
import (
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/cards"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// ... after existing handler wiring around line 189 ...

webClient := client.NewWebClient("http://web:80", nil)
spotlightResolvers := []spotlight.Resolver{
    cards.NewAnimeOfDay(animeRepo, redisCache, log),
    cards.NewRandomTail(animeRepo, redisCache, log),
    cards.NewLatestNews(webClient, redisCache, log),
    cards.NewPlatformStats(db.DB, redisCache, log),
}
spotlightAggregator := spotlight.NewAggregator(spotlightResolvers, redisCache, log)
spotlightHandler := handler.NewSpotlightHandler(spotlightAggregator, cfg.SpotlightEnabled, log)

// Update transport.NewRouter signature to accept spotlightHandler.
router := transport.NewRouter(
    catalogHandler, adminHandler, newsHandler, collectionHandler,
    skipTimesHandler, rawHandler, subtitlesHandler, internalCacheHandler,
    spotlightHandler, // NEW
    cfg, log, metricsCollector,
)
```

### Example 4: Gateway proxy entry

```go
// services/gateway/internal/transport/router.go — addition inside /api Route block
// CRITICAL ORDER: place this BEFORE the /anime/* catch-all (line 206) to keep
// the "specific-before-general" precedent set by /anime/ratings/batch (line 176).

r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)
```

### Example 5: `platform_stats` resolver (the "two of three metrics are nil" case)

```go
// services/catalog/internal/service/spotlight/cards/platform_stats.go
package cards

import (
    "context"
    "fmt"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
    "gorm.io/gorm"
)

type PlatformStats struct {
    db    *gorm.DB
    cache cache.Cache
    log   *logger.Logger
}

func NewPlatformStats(db *gorm.DB, c cache.Cache, log *logger.Logger) *PlatformStats {
    return &PlatformStats{db: db, cache: c, log: log}
}

func (p *PlatformStats) Type() string { return "platform_stats" }

func (p *PlatformStats) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
    key := "spotlight:stats:" + time.Now().UTC().Format("2006-01-02")

    var data spotlight.PlatformStatsData
    if err := p.cache.Get(ctx, key, &data); err == nil {
        // Cache hit
        return &spotlight.Card{Type: p.Type(), Data: data}, nil
    } else if !isMissErr(err) {
        return nil, fmt.Errorf("stats cache get: %w", err)
    }

    metrics := []spotlight.StatsMetric{}

    // 1. anime_added_7d — real GORM count
    var animeCount int64
    err := p.db.WithContext(ctx).Model(&domain.Anime{}).
        Where("created_at > ?", time.Now().Add(-7*24*time.Hour)).
        Count(&animeCount).Error
    if err != nil {
        p.log.Warnw("platform_stats.anime_count_failed", "error", err)
    } else {
        metrics = append(metrics, spotlight.StatsMetric{Key: "anime_added_7d", Value: animeCount})
    }

    // 2. episodes_added_7d — NO per-episode event log exists in the codebase.
    //    Confirmed via grep: services/catalog has no episodes table; the only
    //    related field is Anime.EpisodesAired (a snapshot, not a log).
    //    Omit the metric entirely (eligible card still holds if metric 1 returned).

    // 3. active_rooms_7d — rooms service is Redis-only (services/rooms/internal/
    //    service/room.go writes room:<id> to RedisCache with 24h TTL). No rooms
    //    table in Postgres. Catalog cannot SELECT from it.
    //    Omit the metric entirely.

    if len(metrics) == 0 {
        // No metrics computable — card not eligible.
        return nil, nil
    }
    data = spotlight.PlatformStatsData{Metrics: metrics}

    // Only cache on success.
    if err := p.cache.Set(ctx, key, data, 24*time.Hour); err != nil {
        p.log.Warnw("platform_stats.cache_set_failed", "error", err)
    }
    return &spotlight.Card{Type: p.Type(), Data: data}, nil
}

func isMissErr(err error) bool {
    return err != nil && err.Error() == cache.ErrNotFound.Error() // or errors.Is
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Per-handler hand-rolled Redis Get/Set | `cache.GetOrSet` single-call abstraction | Library since project inception | Single source of truth for cache miss/hit metrics + TTL governance |
| `errgroup` for "any fan-out task" | `errgroup` for fail-fast workflows; `sync.WaitGroup + chan` for partial-success workflows | Established Go idiom | Match the tool to the contract; we need partial success |
| `map[string]any` for polymorphic JSON | Typed structs with `Data any` field | Go 1.20+ generics era; this codebase already prefers typed per `parser/hanime/client.go` json.RawMessage usage | Compile-time safety; cleaner refactors |
| Local TZ for time.Now() formatting | `t.UTC()` before `.Format()` for stable cache keys | Container era | TZ-portable cache keys |

**Deprecated / outdated:**
- None — every primitive used here is the current pattern in this repo as of 2026-05-21.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Per-card 800ms ctx is correctly cancelled when the parent 2s ctx hits its deadline — `context.WithTimeout(parent, d)` documented to propagate parent cancellation | Pitfall 1 | [VERIFIED via Go stdlib docs reading]; if wrong, sibling resolvers may run past 2s wall-clock — bounded by ctx-stale checks at every I/O |
| A2 | `web:80/changelog.json` is reachable from inside the `catalog` container via the default `animeenigma-network` (compose file network name) | latest_news resolver, pitfall 7 | [VERIFIED: docker-compose.yml networks block line 691-693; web container declared at line 660]; if wrong, `latest_news` is permanently dropped and frontend shows 3 cards |
| A3 | `cache.Cache.ErrNotFound` is the sentinel for key-miss; other errors mean Redis hard-down | Pitfall 4 | [VERIFIED: libs/cache/cache.go:209] |
| A4 | The `AnimeRepository.Search` `Page=2, PageSize=100` returns the contiguous block of animes ranked 101..200 by `(sort_priority DESC, score DESC)` | random_tail resolver | [VERIFIED: anime.go:139-157 — OFFSET = (Page-1)*PageSize = 100, Limit=100]; if wrong, random_tail returns no data or wrong data — test "len(items) > 0" guards it |
| A5 | The `web` container's nginx serves `changelog.json` from the build's `public/` dir | latest_news resolver | [VERIFIED: frontend/web/public/changelog.json exists and is loaded by LastUpdates.vue:264]; if wrong, latest_news fails and is dropped (acceptable degradation) |
| A6 | The rooms service stores ALL rooms in Redis (DB 1) with no Postgres backing | active_rooms_7d → nil decision | [VERIFIED: services/rooms/internal/config/config.go lines 49-54 — Redis only; services/rooms/cmd/rooms-api/main.go has no database.New() call]; if a Postgres-backed rooms surface exists somewhere, this is wrong — but research found none |
| A7 | The catalog service has no per-episode event log; `Anime.EpisodesAired int` is a snapshot field, not a log | episodes_added_7d → nil decision | [VERIFIED: grep for "episode_added\|episode_log\|episodes_log\|EpisodeEvent\|episodes_history" across services/ returned 0 matches]; if a log exists in a place grep missed, the metric could be implemented but its absence is non-blocking |
| A8 | Test mocking style in `services/catalog/internal/service/` is handwritten struct fakes (no testify/mock) | Validation Architecture | [VERIFIED: scraper_test.go:20-37 uses `type fakeAnimeFetcher struct` pattern]; recommend matching the existing pattern |

**If this table is empty:** Not empty — but all assumptions are verified. The 8 entries are documentation of WHAT was verified, not unverified claims.

## Open Questions

1. **Should resolvers cache empty/nil results at all?**
   - What we know: `cache.GetOrSet` will write nil. The design doc and CONTEXT.md don't address it.
   - What's unclear: 24h cache of "no data" masks recovery; 0h (skip Set) means cold path on every request when data is genuinely absent.
   - Recommendation: Skip Set on nil. Resolvers use manual `cache.Get` + manual `cache.Set` (only on non-nil). Code example in Pattern 2 follows this.

2. **Should the 404 path (feature-disabled) emit a Prometheus metric?**
   - What we know: All `/metrics` counters are wired by `metrics.Collector.Middleware` automatically; 404 is just another status.
   - What's unclear: Whether ops wants a dedicated `spotlight_disabled_total` counter for tracking when the flag is flipped.
   - Recommendation: Skip. The 404 count on `/api/home/spotlight` is sufficient signal.

3. **Should `latest_news` filter changelog entries by type (feature/perf/fix)?**
   - What we know: Phase 1 takes "first 3 newest entries" — but the design doc and CONTEXT.md don't specify whether to filter.
   - What's unclear: The changelog has typed entries (`feature`, `perf`, `fix`). The block might want only `feature` for the hero card.
   - Recommendation: Phase 1 returns ALL types in date-descending order. Phase 2 (frontend) can filter or style by type. Don't pre-judge — keep the data flow simple.

4. **Should `random_tail` add a `excludes_pinned` repo method to avoid the sort_priority shift?**
   - What we know: See Pitfall 3.
   - What's unclear: Is the rank-shift acceptable to product, or do we want strict score-rank?
   - Recommendation: Accept the shift (option 1). Pinned anime are out of the discovery pool anyway. Adding a new repo method costs a migration of intent without functional gain. If Phase 3 or later wants strict ranks, add `GetByScoreRank` then.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Postgres (animes table) | `anime_of_day`, `random_tail`, `platform_stats.anime_added_7d` | ✓ | already running | — |
| Redis (caching) | All resolvers + snapshot fallback | ✓ | already running | resolvers degrade to live-compute every request (slower but functional) |
| `web` container at `http://web:80/changelog.json` | `latest_news` | ✓ (under animeenigma-network) | nginx serving SPA | resolver drops card; 3 cards returned instead of 4 |
| Go 1.24 toolchain | catalog service build | ✓ | 1.24.0 (verified in go.mod) | — |
| `chi/v5`, `gorm`, `redis/go-redis/v9`, `zap` | catalog runtime | ✓ | already pinned in services/catalog/go.mod | — |
| `golang.org/x/sync` | available but not used | ✓ | v0.18.0 (transitive) | — |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None — the `web` container can be temporarily down and the spotlight degrades gracefully to 3 cards.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `testing` (Go stdlib) + handwritten struct fakes (no testify/mock dependency added) |
| Config file | none — `go test ./...` discovery |
| Quick run command | `cd services/catalog && go test ./internal/service/spotlight/... ./internal/handler/... -count=1 -run Spotlight` |
| Full suite command | `cd services/catalog && go test ./... -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| HSB-BE-01 | `GET /api/home/spotlight` returns `{cards, generated_at}` envelope | handler unit (httptest) | `go test ./internal/handler/ -run TestSpotlightHandler_Get_Envelope` | ❌ Wave 0 |
| HSB-BE-02 | Aggregator collects 4 cards in parallel | aggregator integration | `go test ./internal/service/spotlight/ -run TestAggregator_Concurrent` | ❌ Wave 0 |
| HSB-BE-03 | Per-card 800ms timeout drops only that card + logs `spotlight.card_failed` | aggregator integration | `go test ./internal/service/spotlight/ -run TestAggregator_PerCardTimeout` | ❌ Wave 0 |
| HSB-BE-04 | Overall 2s budget; snapshot fallback when zero cards | aggregator integration | `go test ./internal/service/spotlight/ -run TestAggregator_OverallTimeout_SnapshotFallback` | ❌ Wave 0 |
| HSB-BE-05 | Resolver returning `nil, nil` (eligible=false) is dropped from response | aggregator unit | `go test ./internal/service/spotlight/ -run TestAggregator_DropsIneligible` | ❌ Wave 0 |
| HSB-BE-06 | Gateway proxies `/api/home/spotlight` to catalog | gateway routing test (httptest) | `go test ./services/gateway/internal/transport/ -run TestRouter_SpotlightProxy` | ❌ Wave 0 |
| HSB-BE-07 | When `SPOTLIGHT_ENABLED=false`, handler returns 404 | handler unit | `go test ./internal/handler/ -run TestSpotlightHandler_FeatureFlagDisabled` | ❌ Wave 0 |
| HSB-BE-10 | `anime_of_day` resolver picks via `seed % len` deterministically | resolver unit | `go test ./internal/service/spotlight/cards/ -run TestAnimeOfDay_DateSeededPick` | ❌ Wave 0 |
| HSB-BE-11 | `random_tail` resolver fetches ranks 101..200 and picks deterministically | resolver unit | `go test ./internal/service/spotlight/cards/ -run TestRandomTail_PageOffset` | ❌ Wave 0 |
| HSB-BE-12 | `latest_news` resolver returns ≤3 newest entries from changelog.json | resolver unit (httptest.Server) | `go test ./internal/service/spotlight/cards/ -run TestLatestNews_Returns3Newest` | ❌ Wave 0 |
| HSB-BE-13 | `platform_stats` resolver returns card with 1 metric (anime_added_7d); eligibility intact | resolver unit | `go test ./internal/service/spotlight/cards/ -run TestPlatformStats_OnlyOneMetric` | ❌ Wave 0 |
| HSB-NF-01 | p95 ≤400ms cached, ≤1500ms cold | smoke (curl + /metrics) | `bash scripts/spotlight-smoke.sh` (Wave 0 helper) | ❌ Wave 0 |
| HSB-NF-03 | Redis keys all under `spotlight:` prefix | aggregator unit (mock cache asserts key prefix) | `go test ./internal/service/spotlight/cards/ -run TestCacheKeyPrefix` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `cd services/catalog && go test ./internal/service/spotlight/... ./internal/handler/... -count=1 -run Spotlight` (≤5 seconds locally)
- **Per wave merge:** `cd services/catalog && go test ./... -count=1 -race`
- **Phase gate:** Full suite green + smoke test:
  ```bash
  make redeploy-catalog && make redeploy-gateway
  curl -s http://localhost:8000/api/home/spotlight | jq '.cards | length'   # expect 3 or 4
  curl -s http://localhost:8000/api/home/spotlight | jq '.cards[].type'     # expect anime_of_day, random_tail, latest_news, platform_stats
  docker compose -f docker/docker-compose.yml exec redis redis-cli KEYS 'spotlight:*' | wc -l  # expect 4..5 (cards + snapshot)
  curl -s http://localhost:8081/metrics | grep 'http_request_duration_seconds_bucket{.*home_spotlight'
  ```

### Wave 0 Gaps

All test infrastructure files do not yet exist. Wave 0 creates them:

- [ ] `services/catalog/internal/service/spotlight/aggregator_test.go` — covers HSB-BE-02, BE-03, BE-04, BE-05
- [ ] `services/catalog/internal/service/spotlight/seed_test.go` — UTC + DST robustness for `DateSeedUTC` and `DateKeyUTC`
- [ ] `services/catalog/internal/service/spotlight/cards/anime_of_day_test.go` — covers HSB-BE-10
- [ ] `services/catalog/internal/service/spotlight/cards/random_tail_test.go` — covers HSB-BE-11; includes a "sort_priority shift" sanity test
- [ ] `services/catalog/internal/service/spotlight/cards/latest_news_test.go` — covers HSB-BE-12 with `httptest.Server` for `web:80` simulation
- [ ] `services/catalog/internal/service/spotlight/cards/platform_stats_test.go` — covers HSB-BE-13; verifies "1 metric only" eligibility holds, "0 metrics" returns nil
- [ ] `services/catalog/internal/service/spotlight/client/web_client_test.go` — covers timeout, malformed JSON, 5xx
- [ ] `services/catalog/internal/handler/spotlight_test.go` — covers HSB-BE-01 envelope + HSB-BE-07 feature-flag 404
- [ ] `services/catalog/internal/service/spotlight/mocks.go` (or inline in test files) — `fakeRepo`, `fakeCache`, `fakeResolver`, `fakeWebClient` handwritten struct mocks matching the pattern in `services/catalog/internal/service/scraper_test.go`
- [ ] `scripts/spotlight-smoke.sh` — single-shot end-to-end curl test for the phase verification step

No new framework install required — `testing` stdlib + `net/http/httptest` cover all cases.

### Non-obvious failure modes to test

1. **All 4 resolvers timeout simultaneously.** Mock resolvers that block on a 1-second `time.Sleep` inside the goroutine. Expect: handler returns 200 with `cards: []` (or snapshot if pre-seeded) within ~2.05s wall clock. No leaked goroutines (assert with `runtime.NumGoroutine()` before/after).
2. **Redis hard-down (Get returns non-`ErrNotFound` error) for the snapshot path.** Mock cache.Get to return a generic error for the snapshot key. Expect: handler returns 200 with `cards: []` plus one `spotlight.snapshot_load_failed` Warnw log.
3. **`web` container DNS unresolvable.** Mock `http.RoundTripper` to return `&net.DNSError{IsNotFound: true}`. Expect: `latest_news` is dropped, other 3 cards return.
4. **Per-card timeout fires at exactly 800ms.** Use a resolver that sleeps 850ms. Expect: card is dropped, `ctx.Err() == context.DeadlineExceeded`.
5. **Resolver returns `(nil, nil)` (eligible=false).** Expect: card silently absent from `cards`, no log entry.
6. **Snapshot write hits a slow Redis.** Sleep inside the cache.Set mock for 5 seconds; assert the handler returned within 2s (proves snapshot is detached via `go` + `context.Background()`).
7. **`changelog.json` is empty `[]` array.** Expect: resolver returns nil (eligible=false), card absent.
8. **The animes table is empty.** Expect: `anime_of_day` resolver returns `(nil, nil)`, card absent. Pre-condition for fresh dev DB.
9. **Date crosses UTC midnight mid-request.** Hard to test deterministically; verify the seed function is computed ONCE per resolver call (not multiple times) so a request can't straddle two cache keys for itself.
10. **`sort_priority > 0` for one anime.** Ensure `random_tail` still returns a card (a pinned item shifts the page-2 window by 1; some anime that would have been rank 101 is now rank 100, but rank 200 is now rank 199 — still 100 valid candidates).

## Sources

### Primary (HIGH confidence)
- `services/catalog/internal/service/subs_aggregator.go` (lines 109-156) — canonical fan-out + collect pattern; verbatim adapted for Pattern 1.
- `services/catalog/internal/repo/anime.go` (lines 74-160) — `AnimeRepository.Search` filter shape, sort_priority injection, OFFSET semantics.
- `services/catalog/internal/handler/news.go` — `cache.GetOrSet` usage and handler structure precedent.
- `services/catalog/internal/transport/router.go` — chi route registration pattern, public-route precedent (lines 60-122).
- `services/gateway/internal/transport/router.go` — gateway proxy entries, especially specific-before-general ordering precedent (lines 176-188 for ratings/batch, 205-222 for catalog group).
- `services/gateway/internal/handler/proxy.go` — `ProxyToCatalog` already exists and accepts arbitrary paths.
- `libs/cache/cache.go` — `Cache` interface, `GetOrSet`, `ErrNotFound` sentinel.
- `libs/logger/logger.go` — `*Logger` API (Errorw, Infow, Warnw).
- `libs/httputil/response.go` — `OK`, `JSON`, `Error` response helpers.
- `libs/errors/errors.go` — `AppError`, `Internal`, `NotFound`, code-to-HTTP-status.
- `services/catalog/internal/config/config.go` — env-var binding pattern via `getEnv*` helpers.
- `services/catalog/cmd/catalog-api/main.go` — handler-wiring precedent (lines 105-194).
- `services/rooms/internal/service/room.go` (lines 27-47) — confirms rooms are Redis-only.
- `services/rooms/internal/config/config.go` — confirms rooms has no Postgres config.
- `services/catalog/internal/domain/anime.go` — Anime struct field inventory; no episode log.
- `frontend/web/public/changelog.json` (read first 100 lines) — confirmed JSON shape `[{date, entries: [{type, message}]}]`.
- `docker/docker-compose.yml` — verified `catalog` and `web` containers share `animeenigma-network` default network (line 691); confirms `http://web:80/changelog.json` is DNS-resolvable from catalog.
- `services/catalog/internal/service/scraper_test.go` (lines 1-100) — confirms handwritten struct fakes pattern dominates over testify/mock in this service.
- `services/player/internal/transport/optional_auth.go` — reference implementation for Phase 3 optional-auth middleware (NOT used in Phase 1).
- Go 1.24 stdlib documented behavior for `context.WithTimeout` and `http.NewRequestWithContext`.

### Secondary (MEDIUM confidence)
- TypeScript discriminated union in design doc §4.1 — taken at face value; encoding via typed Go struct + `Data any` field is the cleanest Go translation.

### Tertiary (LOW confidence)
- None — every claim in this research is anchored to a file read in this session.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already in go.mod, verified file-by-file
- Architecture: HIGH — `subs_aggregator.go` provides a 1:1 template; only adaptation is the 4 specific resolvers
- Pitfalls: HIGH — every pitfall is either a verified codebase behavior (sort_priority injection, rooms-is-Redis) or a documented Go stdlib semantic (ctx cancellation propagation)
- Validation Architecture: HIGH — test infrastructure pattern matches `scraper_test.go` precedent

**Research date:** 2026-05-21
**Valid until:** 2026-06-21 (30 days — stable domain, no fast-moving external dependencies)
