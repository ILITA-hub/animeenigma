---
phase: 01-backend-aggregator
plan: 02
subsystem: catalog-service / spotlight aggregator
tags: [backend, go, concurrency, cache, redis, gorm, httpclient, resolver]
dependency_graph:
  requires:
    - "01-01: spotlight.Card, spotlight.Resolver interface, DateSeedUTC/DateKeyUTC helpers"
    - "libs/cache.Cache interface + ErrNotFound sentinel"
    - "libs/logger.Logger (zap.SugaredLogger)"
    - "services/catalog/internal/repo/AnimeRepository.Search"
    - "services/catalog/internal/domain.Anime + SearchFilters"
  provides:
    - "spotlight/client.WebClient — GET http://web:80/changelog.json with 500ms timeout, returns flattened []ChangelogEntry capped at 3 newest"
    - "spotlight/cards.AnimeOfDayResolver — Type() returns 'anime_of_day', cache spotlight:anime_of_day:<YYYY-MM-DD> TTL 24h"
    - "spotlight/cards.RandomTailResolver — Type() returns 'random_tail', cache spotlight:random_tail:<YYYY-MM-DD> TTL 24h"
    - "spotlight/cards.LatestNewsResolver — Type() returns 'latest_news', cache spotlight:changelog:<YYYY-MM-DD> TTL 24h"
    - "spotlight/cards.PlatformStatsResolver — Type() returns 'platform_stats', cache spotlight:stats:<YYYY-MM-DD> TTL 24h"
  affects:
    - "Plan 01-03 (aggregator concurrent fan-out): each NewXxxResolver constructor is the dependency point to wire into spotlight.NewAggregator"
    - "Plan 01-04 (handler + main.go DI): the four constructors are called from cmd/catalog-api/main.go after repo + cache + logger + web client are constructed"
tech_stack:
  added: []  # zero new dependencies
  patterns:
    - "manual cache.Get + errors.Is(cache.ErrNotFound) + cache.Set (NOT cache.GetOrSet — DELIBERATE DIVERGENCE 1)"
    - "(nil, nil) sentinel return for ineligible card — never writes the cache"
    - "handwritten struct fakes (no testify/mock); pattern follows services/catalog/internal/service/scraper_test.go"
    - "httptest.Server for web client tests; in-memory SQLite for platform_stats GORM Count"
    - "local interface decoupling at resolver boundaries (animeSearcher, changelogFetcher) so tests inject fakes without dragging the concrete repo"
key_files:
  created:
    - "services/catalog/internal/service/spotlight/client/web_client.go"
    - "services/catalog/internal/service/spotlight/client/web_client_test.go"
    - "services/catalog/internal/service/spotlight/cards/anime_of_day.go"
    - "services/catalog/internal/service/spotlight/cards/anime_of_day_test.go"
    - "services/catalog/internal/service/spotlight/cards/random_tail.go"
    - "services/catalog/internal/service/spotlight/cards/random_tail_test.go"
    - "services/catalog/internal/service/spotlight/cards/latest_news.go"
    - "services/catalog/internal/service/spotlight/cards/latest_news_test.go"
    - "services/catalog/internal/service/spotlight/cards/platform_stats.go"
    - "services/catalog/internal/service/spotlight/cards/platform_stats_test.go"
    - "services/catalog/internal/service/spotlight/cards/fakes_test.go (shared test helpers — fakeCache, fakeAnimeSearcher, testLogger, makeAnimes)"
  modified: []
decisions:
  - "Manual cache.Get + errors.Is(cache.ErrNotFound) + cache.Set across ALL 4 resolvers. NEVER cache.GetOrSet — that helper would write nil/empty zero values for the full 24h TTL, baking a 'no data' cache that masks an upstream recovery."
  - "Empty / ineligible results return (nil, nil) from Resolve, and the resolver does NOT call cache.Set. This is the explicit Pitfall-5 mitigation: an empty changelog or empty repo result must NOT pin the card dark for 24h."
  - "AnimeRepository.Search ordering gotcha (sort_priority DESC primary axis, repo/anime.go:134-147) is documented inline in random_tail.go. Page=2/PageSize=100 returns ranks 101..200 by (sort_priority DESC, score DESC) — pinned anime never reach random_tail, which is intentional per CLAUDE.md."
  - "episodes_added_7d is HARDCODED nil (skipped) for Phase 1 with rationale comment. No per-episode event log exists — only Anime.EpisodesAired snapshot."
  - "active_rooms_7d is HARDCODED nil (skipped) for Phase 1 with rationale comment. Rooms service is Redis-only (verified RESEARCH.md A7); no Postgres table for catalog's GORM connection to SELECT."
  - "platform_stats card stays eligible iff ≥1 metric is non-nil — in Phase 1 that resolves to 'iff anime_added_7d count succeeded'."
  - "Local-interface decoupling at every resolver boundary (animeSearcher in cards/anime_of_day.go, changelogFetcher in cards/latest_news.go). Tests substitute handwritten struct fakes without depending on the concrete repo or HTTP client."
  - "Test DB for platform_stats uses raw CREATE TABLE on SQLite with a 6-column minimal schema. domain.Anime's full ~30-column Postgres schema uses gen_random_uuid() defaults that SQLite cannot parse — and we only need created_at + a row count for the Count() under test."
metrics:
  duration_minutes: 12
  tasks_completed: 2
  files_created: 11
  files_modified: 0
  test_files: 6
  total_test_cases: 29
  commits:
    - "a6b5044 feat(01-02): web client + anime_of_day + random_tail resolvers"
    - "a155402 feat(01-02): latest_news + platform_stats resolvers"
completed: "2026-05-21"
---

# Phase 1 Plan 02: Resolvers + WebClient Summary

Four spotlight card resolvers implementing `spotlight.Resolver` interface + the
HTTP client that backs `latest_news` — all with manual-cache discipline,
eligibility-on-source filtering, and zero new dependencies.

## What Was Built

### Files Created (11)

**Production code (5 files):**
- `services/catalog/internal/service/spotlight/client/web_client.go` — `WebClient` fetches `http://web:80/changelog.json` with 500ms `http.Client.Timeout` (snug under the 800ms per-card budget HSB-BE-03). Decodes the `[{date, entries:[{type, message}]}]` shape, flattens preserving newest-first order, returns capped at 3 entries.
- `services/catalog/internal/service/spotlight/cards/anime_of_day.go` — `AnimeOfDayResolver` calls `repo.Search(Sort:"score", Order:"desc", ScoreMin:&8.0, Page:1, PageSize:200)`, picks `items[DateSeedUTC(now) % len]` for a deterministic UTC-day pick.
- `services/catalog/internal/service/spotlight/cards/random_tail.go` — `RandomTailResolver` calls `repo.Search(Sort:"score", Order:"desc", Page:2, PageSize:100)` — ranks 101..200 by (sort_priority DESC, score DESC). Same date-seed pick.
- `services/catalog/internal/service/spotlight/cards/latest_news.go` — `LatestNewsResolver` depends on the local `changelogFetcher` interface (concrete impl: `client.WebClient`). Returns `(nil, nil)` for empty entries — never caches dark.
- `services/catalog/internal/service/spotlight/cards/platform_stats.go` — `PlatformStatsResolver` computes `anime_added_7d` via GORM `db.WithContext(ctx).Model(&domain.Anime{}).Where("created_at > ?", cutoff).Count(...)`. `episodes_added_7d` + `active_rooms_7d` are HARDCODED nil with rationale comments.

**Tests (6 files, 29 cases):**

| Test file | Cases | Coverage |
|-----------|-------|----------|
| `client/web_client_test.go` | 7 | HappyPath, Caps3, Non200, ContextCanceled, BadJSON, Defaults, OverridesRespected |
| `cards/anime_of_day_test.go` | 7 | Type, CacheMiss+PicksDeterministically, CacheHit, EmptyRepo, RepoError, CacheGetError, SearchFiltersCorrect |
| `cards/random_tail_test.go` | 5 | Type, Page2PageSize100, CacheKeyPrefix, EmptyRepo, ReturnsTypedData |
| `cards/latest_news_test.go` | 5 | Type, CacheMiss+FetchesAndCaches, CacheHit, FetcherError, EmptyEntries |
| `cards/platform_stats_test.go` | 5 | Type, AllMetricsComputable, NoMetricsComputable, CacheHit, CacheKeyPrefix |
| `cards/fakes_test.go` | 0 | Shared test helpers (fakeCache, fakeAnimeSearcher, testLogger, makeAnimes) |

## Per-Resolver Specification

| Resolver | `Type()` | Cache key prefix | TTL | Source | Eligibility rule |
|----------|----------|------------------|-----|--------|------------------|
| AnimeOfDayResolver | `anime_of_day` | `spotlight:anime_of_day:` | 24h | repo.Search top-200 score≥8.0 | non-empty pool |
| RandomTailResolver | `random_tail` | `spotlight:random_tail:` | 24h | repo.Search Page=2 PageSize=100 | non-empty pool |
| LatestNewsResolver | `latest_news` | `spotlight:changelog:` | 24h | client.WebClient.GetChangelog | ≥1 entry returned |
| PlatformStatsResolver | `platform_stats` | `spotlight:stats:` | 24h | GORM Count + 2 nil metrics | ≥1 metric computed |

## DELIBERATE DIVERGENCE 1 — Verified

All four resolvers use the manual cache pattern, NOT `cache.GetOrSet`. Verification:

```bash
$ grep -l "cache.GetOrSet\|\.GetOrSet(" services/catalog/internal/service/spotlight/cards/*.go
# (empty — no usages)

$ grep -l "errors.Is.*cache.ErrNotFound" services/catalog/internal/service/spotlight/cards/anime_of_day.go services/catalog/internal/service/spotlight/cards/random_tail.go services/catalog/internal/service/spotlight/cards/latest_news.go services/catalog/internal/service/spotlight/cards/platform_stats.go | wc -l
4
```

Reason: `GetOrSet` would `Set(ctx, key, nil, 24h)` when the resolver's compute step returns `(nil, nil)` — baking a "no data" cache for the full TTL and masking upstream recoveries. Manual discipline lets us return `(nil, nil)` WITHOUT writing the cache, so the next request retries until real data appears.

## Phase 1 Nil-Metric Decisions (platform_stats)

Per RESEARCH.md A6 / A7, two of the three design-doc metrics cannot be implemented in Phase 1 and are HARDCODED nil with rationale comments in `platform_stats.go`:

1. **`episodes_added_7d`** — SKIPPED for Phase 1. No per-episode event log exists in this codebase; the closest field is `Anime.EpisodesAired int` which is a snapshot, not an event log. Re-introducing this metric requires either an `episode_added_at` column backfill or a new event table, both out of scope.

2. **`active_rooms_7d`** — SKIPPED for Phase 1. The rooms service is **Redis-only** — `services/rooms/internal/service/room.go` writes `room:<id>` to Redis with 24h TTL, no Postgres `rooms` table exists. The catalog's shared GORM connection cannot `SELECT * FROM rooms` because the table is absent.

Card stays eligible via `anime_added_7d` whenever the GORM Count succeeds.

## Test Verification

```bash
$ go test ./services/catalog/internal/service/spotlight/... -count=1 -race
ok  	github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight        1.018s
ok  	github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/cards  1.041s
ok  	github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client 1.078s

$ go vet ./services/catalog/internal/service/spotlight/...
(no output — clean)
```

All 29 test cases pass under the race detector. No new dependencies added to `services/catalog/go.mod`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking issue] SQLite cannot parse Postgres `gen_random_uuid()` defaults**

- **Found during:** Task 2 — initial `gorm.AutoMigrate(&domain.Anime{})` against in-memory SQLite failed with `near "(": syntax error` because `domain.Anime.ID` has `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"` and SQLite has no `gen_random_uuid()` function.
- **Fix:** Replaced `AutoMigrate` with a raw `CREATE TABLE animes (...)` containing only the six columns the resolver's Count query touches (id, name, score, created_at, updated_at, deleted_at). Also switched test row inserts from `db.Create(&domain.Anime{...})` to a raw `INSERT INTO animes (...) VALUES (...)` helper — `db.Create` was failing because GORM expects all ~30 Anime columns to exist.
- **Files modified:** `services/catalog/internal/service/spotlight/cards/platform_stats_test.go`
- **Commit:** Part of `a155402`

No other deviations. Plan executed exactly as written.

## Self-Check: PASSED

Verified files exist:
- `services/catalog/internal/service/spotlight/client/web_client.go` — FOUND
- `services/catalog/internal/service/spotlight/client/web_client_test.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/anime_of_day.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/anime_of_day_test.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/random_tail.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/random_tail_test.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/latest_news.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/latest_news_test.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/platform_stats.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/platform_stats_test.go` — FOUND
- `services/catalog/internal/service/spotlight/cards/fakes_test.go` — FOUND

Verified commits exist:
- `a6b5044` (Task 1) — FOUND in `git log --oneline -5`
- `a155402` (Task 2) — FOUND in `git log --oneline -5`
