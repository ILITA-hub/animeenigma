# Phase 1: Backend Aggregator + Static Cards - Context

**Gathered:** 2026-05-21
**Status:** Ready for planning
**Mode:** Auto-generated from approved design doc + REQUIREMENTS.md (autonomous mode)

<domain>
## Phase Boundary

`GET /api/home/spotlight` returns 4 eligible cards (`anime_of_day`,
`random_tail`, `latest_news`, `platform_stats`) in well-shaped JSON via a
new aggregator package under `services/catalog/internal/service/spotlight/`.

Each card resolver runs under an 800ms per-card context deadline with a 2s
overall request budget. Per-card Redis day-cache keys (prefix `spotlight:`)
with 24h TTL. Eligibility filter is server-side — cards with no data are
dropped from the response payload entirely. Feature flag
`SPOTLIGHT_ENABLED=true` (catalog env) gates the endpoint; 404 when false.

In scope: catalog handler, aggregator + 4 resolvers, web client for
changelog.json, gateway routing, env flag, logging.

Out of scope for this phase:
- All 5 dynamic cards (`personal_pick`, `telegram_news`, `now_watching`,
  `not_time_yet`, `continue_watching_new`) — Phase 3.
- Frontend HeroSpotlightBlock + Vue carousel — Phase 2.
- Player service fan-out + internal endpoints — Phase 3.
- `trendingRecs` removal from Home.vue — Phase 3.

</domain>

<decisions>
## Implementation Decisions

### Resource sourcing (data sources for 4 static cards)
- `anime_of_day` — Use `AnimeRepository.Search` with `Sort="score"`,
  `Order="desc"`, `ScoreMin=8.0`, `PageSize=200`. Pick `items[seed % len]`
  where `seed = YYYY*100*32 + MM*32 + DD` (UTC date — server's local TZ).
- `random_tail` — Use `AnimeRepository.Search` with `Sort="score"`,
  `Order="desc"`, `PageSize=1900`, then slice `[100:]` (drop top-100).
  Pick `items[seed % len]` with same date seed. If repo Search supports
  Offset/Page, prefer `Page=2, PageSize=100` to fetch ranks 101..200 to
  avoid loading 2000 rows — finalise during plan-phase research.
- `latest_news` — New `client/web_client.go` does `GET http://web:80/changelog.json`
  (docker-network DNS), parses the existing JSON shape from
  `frontend/web/public/changelog.json`. Returns up to 3 newest entries.
- `platform_stats` — Three metrics via direct GORM:
  - `anime_added_7d`: `SELECT COUNT(*) FROM animes WHERE created_at > NOW() - INTERVAL '7 days' AND deleted_at IS NULL`
  - `episodes_added_7d`: deferred — if no episode log exists in the codebase,
    return `nil` (omit the metric) but keep the card eligible if ≥1 metric is
    non-null. Plan-phase research will confirm whether per-episode events
    exist; default `null` is acceptable.
  - `active_rooms_7d`: `SELECT COUNT(*) FROM rooms WHERE created_at > NOW() - INTERVAL '7 days'`
    on the shared GORM connection. If rooms table is not addressable from
    catalog, return `nil` for this metric (card stays eligible if ≥1 metric).
  - Card eligible iff at least 1 metric is non-null.

### Aggregator / concurrency
- Aggregator runs resolvers concurrently via goroutines + `errgroup` or
  custom WaitGroup with collected results. Each card gets its own
  `context.WithTimeout(ctx, 800ms)`.
- Card failures (timeout or error) drop the card and emit one structured
  log line `spotlight.card_failed{type=..., error=...}` via the existing
  `libs/logger` Errorw method.
- Overall request budget: `context.WithTimeout(r.Context(), 2*time.Second)`.
  On overall context deadline, return whatever fully-resolved cards were
  collected so far. If zero cards collected AND a last-known-good
  `spotlight:snapshot:<anon|user_id>:YYYY-MM-DD` Redis key exists, return
  the cached snapshot. Otherwise return 200 with empty `cards: []` — frontend
  hides the block (per HSB-FE-02).
- Aggregator response is **not** cached as a single blob. Each resolver
  uses `cache.GetOrSet`-style pattern (Get; on miss → compute → Set with
  appropriate TTL).

### Cache keys (per design doc §5.3)
- `spotlight:anime_of_day:<YYYY-MM-DD>` TTL 24h
- `spotlight:random_tail:<YYYY-MM-DD>` TTL 24h
- `spotlight:changelog:<YYYY-MM-DD>` TTL 24h
- `spotlight:stats:<YYYY-MM-DD>` TTL 24h
- Snapshot fallback: `spotlight:snapshot:<anon|user_id>:<YYYY-MM-DD>` TTL 24h
  (written best-effort after every successful aggregation).

### Auth handling for Phase 1
- Endpoint mounted under `/api/home/spotlight` as a **public route**
  (NO `AuthMiddleware`). Phase 1 has no login-gated cards.
- Phase 3 will add an optional-auth middleware (try-validate-JWT, no 401
  on failure) to enable login-gated cards. The Phase 1 handler must
  tolerate `r.Header.Get("Authorization")` being present but ignore it.

### Feature flag (`SPOTLIGHT_ENABLED`)
- Read via `services/catalog/internal/config/config.go` — add
  `SpotlightEnabled bool` env-bound, default `true`.
- When `false`: handler returns `404 Not Found` with no body (frontend
  HSB-FE-02 treats as "block hides itself"). Implementation: check flag
  in handler entry and short-circuit.

### Card payload shape
- Match the TypeScript discriminated union from design doc §4.1 exactly.
  Go side uses a `Card` struct with:
  ```
  type Card struct {
      Type string          `json:"type"`
      Data json.RawMessage `json:"data"` // or per-type struct via embedded interface
  }
  ```
- Each resolver returns a typed `Card` with `Type` set and `Data` containing
  the type-specific payload (struct serialized to JSON). Use `omitempty` on
  optional fields like `reason_i18n_key`.
- Response envelope: `{ "cards": [Card, ...], "generated_at": "<ISO8601 UTC>" }`.

### Logging
- Use shared `libs/logger.Logger` injected via DI (same pattern as other
  handlers/services).
- One info log per request at handler entry (`spotlight.request{user=anon|<id>}`)
  is sufficient.
- One error log per dropped card (`spotlight.card_failed{type, error}`).
- Aggregator-level timing: emit one info log at end
  (`spotlight.aggregated{cards_returned, ms_total}`) for ops visibility.

### Error handling
- Standard `libs/errors` package — `errors.Internal`, `errors.NotFound`.
- Handler returns 200 with `{cards: [...]}` for any partial success; only
  500 on catastrophic failure (Redis hard-down AND no cards resolved AND no
  snapshot). Per design doc §5.1 the 500 path is documented.

### File layout (per ROADMAP "Touches" list)
- `services/catalog/internal/handler/spotlight.go` — chi handler, JSON encode
- `services/catalog/internal/service/spotlight/aggregator.go` — concurrent
  resolver dispatch + collection + snapshot fallback
- `services/catalog/internal/service/spotlight/types.go` — `Card`, per-card
  payload structs (`AnimeOfDayData`, `RandomTailData`, etc.)
- `services/catalog/internal/service/spotlight/cards/{anime_of_day,random_tail,latest_news,platform_stats}.go`
  — one file per resolver
- `services/catalog/internal/service/spotlight/client/web_client.go` — HTTP
  client to `http://web:80/changelog.json`
- `services/catalog/internal/transport/router.go` — extend with
  `r.Get("/home/spotlight", h.Get)` under the `/api` group
- `services/catalog/cmd/catalog-api/main.go` — wire the handler
- `services/catalog/internal/config/config.go` — `SpotlightEnabled bool`
- `services/gateway/internal/transport/router.go` — proxy
  `/api/home/spotlight → catalog:8081`
- `docker/.env.example` — `SPOTLIGHT_ENABLED=true`

### Testing strategy
- Unit tests per resolver against a mocked cache + mocked repo (use
  testify/mock or simple struct-based mocks following existing project
  patterns — see `services/catalog/internal/service/catalog.go` test
  companions for the style).
- Aggregator-level integration test: mock 4 resolvers, verify (a) all
  resolvers run concurrently within budget, (b) one timing-out resolver
  drops its card without affecting others, (c) overall timeout returns
  partial results.
- No external HTTP calls in tests — `web_client` accepts an injectable
  `*http.Client` (or transport) for stub-ability.

### Claude's Discretion
- Exact mock library (`testify/mock` vs handwritten struct mocks) — match
  whichever pattern dominates in `services/catalog/internal/service/*_test.go`.
- Goroutine collection mechanism (`errgroup.Group` vs `sync.WaitGroup +
  channel`) — both acceptable; pick whichever reads more naturally given
  the "drop on error, don't fail-fast" requirement (likely raw WG +
  results channel because errgroup's fail-fast contract works against us).
- Exact JSON encoding for the discriminated union — `map[string]any` for
  Data is also acceptable; typed struct preferred for compile-time safety.
- Snapshot fallback Redis write path — best-effort, fire-and-forget after
  the response is encoded (not in the hot path).
- Whether to add a small `respond_envelope.go` shared helper for the
  generated_at timestamp or inline it.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`libs/cache`** — `Cache.Get/Set` pattern, `cache.KeyAnime(id)`-style
  key constructors (`libs/cache/ttl.go`), TTL constants. Pattern is used
  extensively in `services/catalog/internal/service/catalog.go`. New
  `spotlight:*` keys live in their own resolvers (no need to add to ttl.go).
- **`libs/logger`** — Structured `Errorw/Infow` already used by all
  catalog handlers (see `services/catalog/internal/handler/news.go`).
- **`libs/errors`** — Domain error wrappers (`errors.NotFound`,
  `errors.ExternalAPI`, `errors.Wrap`) — already used in news handler.
- **`libs/authz`** — `authz.ContextWithClaims` + `authz.IsAdmin` if we
  later add an optional-auth path (Phase 3, not Phase 1).
- **`AnimeRepository.Search`** (`services/catalog/internal/repo/anime.go:74`)
  — accepts `domain.SearchFilters{ScoreMin, Sort, Order, Page, PageSize}`.
  This is the existing path for ranked anime queries.
- **`telegram.Client`** — already wired in the news handler. Phase 1 does
  NOT use it directly (Phase 3 does); Phase 1 only needs the changelog
  web client.

### Established Patterns
- All catalog handlers are constructor-injected via
  `services/catalog/cmd/catalog-api/main.go`. Add a `spotlightHandler` to
  the constructor + pass into `transport.NewRouter`.
- Cache use: typically `if cached := cache.Get(...); ok { return cached }`
  then compute-and-Set. Wrap with TTL constant or literal `time.Hour*24`.
- Public routes mount under `/api/anime/...`, `/api/news`, etc. Add
  `/api/home/spotlight` under the same `/api` group as a sibling.
- Logger field naming: `Infow("event", "key", value, "key2", value2)`.

### Integration Points
- **catalog/cmd/catalog-api/main.go** — wire spotlight aggregator + handler
  alongside the existing `newsHandler`, `catalogHandler`, etc.
- **catalog/internal/transport/router.go** — register the route in the
  `/api` block; no auth needed (public).
- **gateway/internal/transport/router.go** — extend the reverse-proxy
  router with a `/api/home/spotlight` rule pointing to catalog:8081.
  Pattern likely already in place for `/api/anime/news` — replicate.
- **docker/.env.example** — add `SPOTLIGHT_ENABLED=true` near the other
  catalog flags.
- **web container** — already serves `changelog.json` at
  `http://web:80/changelog.json` (the LastUpdates view loads it via
  the same path). Verify in plan-phase research that this is reachable
  from inside the catalog container (it should be — both share the
  default docker compose network).

</code_context>

<specifics>
## Specific Ideas

- **Per-card deadline 800ms**, **overall budget 2s** — non-negotiable per
  HSB-BE-03 / HSB-BE-04.
- **Concurrent resolver execution** — design doc §5.5 explicitly calls
  for fan-out so total latency ≈ slowest single resolver + dispatch
  overhead, NOT sum of all.
- **Snapshot fallback is best-effort** — if Redis is hard-down the
  request still succeeds for whatever resolved within budget.
- **`active_rooms_7d`** — the rooms table is owned by a different service.
  If the shared GORM connection in catalog can `SELECT` from the `rooms`
  table directly (single Postgres database, multiple services share it)
  this is fine. If not, defer this metric to a `nil` (card still
  eligible if ≥1 of 3 metrics returns non-nil).
- **`episodes_added_7d`** — if there's no per-episode event log, this
  metric returns `nil`. Card stays eligible. **Do not synthesize**.

</specifics>

<deferred>
## Deferred Ideas

- Personalization (per-user `personal_pick`, `not_time_yet`,
  `continue_watching_new`) → Phase 3.
- `now_watching` SQL with privacy filter → Phase 3.
- Player service `/internal/users/{id}/list` endpoints → Phase 3.
- Removing `trendingRecs` from `Home.vue` → Phase 3.
- Adding `idx_watch_progress_updated_at` index → Phase 3.
- Editorial admin-curated card → v1.1.

</deferred>
</content>
</invoke>