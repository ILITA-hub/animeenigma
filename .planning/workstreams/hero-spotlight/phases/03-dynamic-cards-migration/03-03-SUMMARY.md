---
phase: 03-dynamic-cards-migration
plan: 03
subsystem: api
tags: [hero-spotlight, resolvers, sql, http-fanout, adaptive-slice, cache, jwt, gorm, distinct-on, telegram, recommendations]

# Dependency graph
requires:
  - phase: 01-spotlight-foundation
    provides: spotlight.Card/Resolver interfaces, manual cache discipline (Pitfall 5), fakes_test conventions, latest_news exemplar
  - phase: 03-01 (Plan 03-01)
    provides: player /internal/users/{id}/list endpoint (consumed by FetchListByStatuses), watch_progress index
  - phase: 03-02 (Plan 03-02)
    provides: PlayerClient (FetchUserRecs + FetchListByStatuses), AdaptiveSlice helper, cards.ContextWithJWT/JWTFromContext, OptionalAuth middleware
provides:
  - 5 spotlight Card Data structs (PersonalPickData, TelegramNewsData, NowWatchingData, NotTimeYetData, ContinueWatchingNewData)
  - 5 spotlight.Resolver implementations honoring (nil, nil)/(nil, err)/(*Card, nil) eligibility contract
  - DISTINCT ON SQL projection that exposes ONLY public user fields (HSB-NF-04 privacy gate)
  - Login-only contract (anon early-return) for not_time_yet + continue_watching_new (T-03-11 mitigation)
  - JWT-aware fan-out for personal_pick login path with anon-fallback when JWT missing
affects: ["03-04 (DI wiring + handler JWT attach)", "03-05 (Vue card components — JSON shape is load-bearing)", "03-07 (smoke tests — cache key greps)"]

tech-stack:
  added: []  # no new libraries; reuses GORM, math/rand, libs/cache, libs/logger
  patterns:
    - "Narrow per-resolver interface seams (trendingFetcher, playerRecsFetcher, telegramFetcher, nowWatchingDB, listByStatusesFetcher) — production types satisfy them implicitly so wiring is one line"
    - "ISO 8601 string-compare for chronological ordering — fixed-width components make lex order match time order"
    - "AdaptiveSlice applied even on cache hit (now_watching) — defensive against multi-instance cache writers that may have written >3 items"
    - "Login-only resolvers do early (nil, nil) BEFORE any data fetch — prevents accidentally returning login-curated data via cache key collision"

key-files:
  created:
    - services/catalog/internal/service/spotlight/cards/personal_pick.go
    - services/catalog/internal/service/spotlight/cards/personal_pick_test.go
    - services/catalog/internal/service/spotlight/cards/telegram_news.go
    - services/catalog/internal/service/spotlight/cards/telegram_news_test.go
    - services/catalog/internal/service/spotlight/cards/now_watching.go
    - services/catalog/internal/service/spotlight/cards/now_watching_test.go
    - services/catalog/internal/service/spotlight/cards/not_time_yet.go
    - services/catalog/internal/service/spotlight/cards/not_time_yet_test.go
    - services/catalog/internal/service/spotlight/cards/continue_watching_new.go
    - services/catalog/internal/service/spotlight/cards/continue_watching_new_test.go
  modified:
    - services/catalog/internal/service/spotlight/types.go (5 new Data structs)
    - services/catalog/internal/service/spotlight/types_test.go (round-trip + items-array tests)
    - services/catalog/internal/service/spotlight/cards/fakes_test.go (added fakeAnimeWithID helper)

key-decisions:
  - "now_watching uses a custom RawScan seam (not direct *gorm.DB) so tests stay pure — same pattern as scraper handler"
  - "personal_pick login-no-JWT path falls back to anon trending rather than returning a dark card — handler should set JWT but defensive fallback prevents user-visible regression"
  - "continue_watching_new sort uses lex compare on ISO 8601 strings instead of time.Parse — fixed-width format makes this exact and zero-allocation"
  - "AdaptiveSlice re-applied on now_watching cache hit — guards against >3 sessions persisted by another instance"
  - "telegram_news writes the raw []telegram.NewsItem to cache (not the TelegramNewsData wrapper) so the existing /api/news handler can read the same key unchanged (HSB-NF-03 exception)"

patterns-established:
  - "Narrow interface seams per resolver (each .go file declares its minimal upstream surface and the prod type satisfies implicitly) — keeps wiring tests independent of one another"
  - "Cache-hit path re-applies AdaptiveSlice — cheaper than re-warming and safe against larger-than-needed cache writes"
  - "Login-only resolvers do early-return BEFORE cache GET — anon callers cannot poke at user-scoped cache keys"

requirements-completed: [HSB-BE-20, HSB-BE-21, HSB-BE-22, HSB-BE-24, HSB-BE-25, HSB-BE-30, HSB-NF-04]

# Metrics
duration: 22min
completed: 2026-05-21
---

# Phase 03 Plan 03: Five Dynamic Spotlight Resolvers Summary

**Five Phase 3 spotlight resolvers (personal_pick, telegram_news, now_watching, not_time_yet, continue_watching_new) with manual cache discipline, branched auth handling, DISTINCT ON SQL privacy projection, and AdaptiveSlice 1-2-3 rule.**

## Performance

- **Duration:** ~22 min
- **Started:** 2026-05-21T05:30:00Z (approx)
- **Completed:** 2026-05-21T05:52:38Z
- **Tasks:** 4 (Tasks 1–4 from PLAN; all green on first attempt)
- **Files modified:** 13 (10 new resolver + test files, 2 spotlight types files, 1 fakes_test helper)

## Accomplishments

- **PersonalPickResolver** (HSB-BE-20) branches on `userID`: anon path calls in-process `CatalogService.GetTrendingAnime(1, 10)` and shuffles via the injected `*rand.Rand`; login path calls `PlayerClient.FetchUserRecs(ctx, jwt)` with the JWT extracted via `cards.JWTFromContext(ctx)`. Both paths apply `AdaptiveSlice` (HSB-BE-30). Defensive fallback: login-with-missing-JWT drops to anon trending so the card never goes dark.
- **TelegramNewsResolver** (HSB-BE-21) reuses the **existing `news:telegram` Redis key** (HSB-NF-03 exception — sharing the warm cache with `handler/news.go` so neither side cold-starts the other). 30-minute TTL mirrors `handler/news.go`.
- **NowWatchingResolver** (HSB-BE-22 + HSB-NF-04) runs the verbatim `DISTINCT ON (wp.user_id)` SQL from REQUIREMENTS via a narrow `nowWatchingDB` interface (production wired via `NewGormNowWatchingAdapter`). The projection includes ONLY public fields (`username`, `public_id`, anime metadata, episode number, ISO 8601 `updated_at`). Test `TestNowWatching_NoPrivateFieldsLeaked` greps the marshaled JSON for `email`, `password`, `api_key`, and `"user_id"` — zero matches.
- **NotTimeYetResolver** (HSB-BE-24) and **ContinueWatchingNewResolver** (HSB-BE-25) are login-only: both do `(nil, nil)` early-return for anon callers BEFORE any data fetch (T-03-11 mitigation). `not_time_yet` filters to airing items (`EpisodesAired > 0`) and random-picks one. `continue_watching_new` filters with the strict `EpisodesAired > LastWatchedEpisode + 1` rule (excludes "next-after-last just aired") and picks the most-recent `UpdatedAt`.
- **5 new Card Data structs** added to `spotlight/types.go` matching the design-doc TypeScript union; round-trip + empty-array guards in `types_test.go`.
- **51 unit tests** total: 45 in `spotlight/cards` + 6 in `spotlight/types_test.go` round-trip group. All pass with `-race`.

## Cache key + TTL inventory (for Plan 07 smoke tests)

| Resolver                  | Cache key                                       | TTL  | Notes |
| ------------------------- | ----------------------------------------------- | ---- | ----- |
| personal_pick (anon)      | `spotlight:trending:<YYYY-MM-DD>`               | 24h  | UTC date roll |
| personal_pick (login)     | `spotlight:personal:<user_id>:<YYYY-MM-DD>`     | 24h  | Per-user, UTC date roll |
| telegram_news             | `news:telegram`                                 | 30m  | **Reuses existing handler/news.go key (HSB-NF-03 exception)** |
| now_watching              | `spotlight:now_watching`                        | 10s  | NOT date-keyed — live data |
| not_time_yet              | `spotlight:not_time_yet:<user_id>`              | 30s  | Per-user |
| continue_watching_new     | `spotlight:continue_new:<user_id>`              | 30s  | Per-user |

AdaptiveSlice (HSB-BE-30) is applied in 3 of the 5 new resolvers:
**personal_pick**, **telegram_news**, **now_watching**. The other two (`not_time_yet`, `continue_watching_new`) are single-item cards by design and don't need it. Plan 04 will retrofit `latest_news` to make it 4 of the 5 multi-item resolvers using the rule.

## Task Commits

1. **Task 1: 5 new Data structs in types.go + round-trip tests** — `1727119` (feat)
2. **Task 2: PersonalPickResolver (anon trending + login fan-out)** — `8714752` (feat, 10 tests)
3. **Task 3: TelegramNews + NowWatching resolvers** — `84c02fa` (feat, 14 tests)
4. **Task 4: NotTimeYet + ContinueWatchingNew (login-only)** — `50350e7` (feat, 17 tests)

_All four are squashed `feat(03-03)` commits — no test→feat→refactor split because each resolver shipped with its handwritten-fake test suite alongside._

## Files Created/Modified

- `services/catalog/internal/service/spotlight/types.go` — Added 5 Card Data structs (PersonalPickItem/Data, TelegramPost/Data, NowWatchingSession/Data, NotTimeYetData, ContinueWatchingNewData) extending the discriminated union
- `services/catalog/internal/service/spotlight/types_test.go` — Added `TestNewCardDataShapes_RoundTrip` (5 sub-tests) + `TestPersonalPickData_ItemsMarshalAsArray`
- `services/catalog/internal/service/spotlight/cards/personal_pick.go` — Anon trending + login fan-out, JWT-aware, AdaptiveSlice
- `services/catalog/internal/service/spotlight/cards/telegram_news.go` — Reuses existing `news:telegram` cache key, AdaptiveSlice
- `services/catalog/internal/service/spotlight/cards/now_watching.go` — DISTINCT ON SQL via `nowWatchingDB` seam + `gormNowWatchingAdapter`, AdaptiveSlice, 10s TTL
- `services/catalog/internal/service/spotlight/cards/not_time_yet.go` — Login-only, random pick from airing items in planned/postponed
- `services/catalog/internal/service/spotlight/cards/continue_watching_new.go` — Login-only, strict `>` filter on episode count, picks most-recent UpdatedAt
- 5 corresponding `_test.go` files with handwritten fakes (no testify/mock)
- `services/catalog/internal/service/spotlight/cards/fakes_test.go` — Added `fakeAnimeWithID(id)` helper for resolver tests that need a populated `domain.Anime` value in cache fixtures

## Decisions Made

- **JWT-missing defensive fallback in personal_pick login path** — rather than emit a dark card if the handler fails to attach the JWT, the resolver re-keys to the anon cache slot and serves trending. Edge case but cheap to handle.
- **Cache-hit AdaptiveSlice in now_watching only** — the other multi-item resolvers cache the already-sliced payload, so re-slicing on hit is a no-op. now_watching caches the post-slice data too, but the cache-hit code path still calls AdaptiveSlice as belt-and-suspenders against multi-instance race writes that exceed 3 sessions.
- **telegram_news writes raw `[]telegram.NewsItem` to cache (not the wrapper struct)** — keeps the existing `/api/news` handler reading the same key without a shape mismatch. Mapping to `TelegramPost` happens on read in both code paths.
- **continue_watching_new sort uses string comparison on ISO 8601 strings** — equivalent to `time.Parse + Sort` for fixed-width timestamps, zero allocations, fewer error paths.
- **NowWatchingResolver ignores userID** — every viewer sees the same global snapshot; the data is public by design (HSB-NF-04). Caching is keyed globally (`spotlight:now_watching`) rather than per-user so 1000 anon viewers share one cache slot.

## Deviations from Plan

None - plan executed exactly as written. All 4 tasks GREEN on first test run after RED; no auto-fixes required.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Self-Check Verifications

- **Files created (10 new):** all 10 resolver + test files exist on disk
- **Types extended (5 structs):** `grep -c "^type \(PersonalPickData\|TelegramNewsData\|NowWatchingData\|NotTimeYetData\|ContinueWatchingNewData\) struct" services/catalog/internal/service/spotlight/types.go` → 5
- **DIVERGENCE 1 (no `cache.GetOrSet` in any new resolver):** `grep -l "cache.GetOrSet" services/catalog/internal/service/spotlight/cards/{personal_pick,telegram_news,now_watching,not_time_yet,continue_watching_new}.go` → empty (PASS)
- **`errors.Is(err, cache.ErrNotFound)` discipline:** all 5 files reference it (verified via `grep -l "cache.ErrNotFound" ... | wc -l` → 5)
- **AdaptiveSlice in 3 of 5 multi-item resolvers:** personal_pick (2 calls — anon + login), telegram_news (1), now_watching (2 — miss + hit)
- **Privacy gate (now_watching, HSB-NF-04):** `grep -v '^//\|^\s*//' now_watching.go | grep -E "(email|password|api_key)"` → 0 matches
- **Cache keys conform to spec:** trending (anon, date), personal (login, user+date), now_watching (no date), not_time_yet+continue_new (user). News reuses `news:telegram` (existing handler key, HSB-NF-03 exception).
- **Build:** `cd services/catalog && go build ./...` → clean
- **All 45 cards + 6 types tests pass:** `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race` → ok across `spotlight`, `spotlight/cards`, `spotlight/client`

## Next Phase Readiness

Plan 03-03 is now blocking on Plan 03-04 (DI wiring + handler JWT attach) and Plan 03-05 (frontend Vue cards). Specifically Plan 04 will:
- Wire all 5 new resolvers into `catalog/cmd/catalog-api/main.go` (aggregator constructor)
- Wrap `/home/spotlight` with the OptionalAuth middleware from Plan 02
- Attach the JWT to ctx via `cards.ContextWithJWT(ctx, jwt)` BEFORE the handler dispatches to the aggregator
- Retrofit `latest_news` (Phase 1) to apply AdaptiveSlice — making 4 of 5 multi-item resolvers use the 1-2-3 rule

Plan 05 will build the matching Vue cards consuming the JSON shapes pinned by `TestNewCardDataShapes_RoundTrip`.

No blockers carried forward.

## Self-Check: PASSED

All files exist on disk, all 51 tests pass with `-race`, `go build ./...` clean, no DIVERGENCE 1 violations, privacy gate verified.

---
*Phase: 03-dynamic-cards-migration*
*Workstream: hero-spotlight v1.0*
*Completed: 2026-05-21*
