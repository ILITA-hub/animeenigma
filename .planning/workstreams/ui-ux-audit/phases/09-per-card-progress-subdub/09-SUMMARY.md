---
phase: 9
plan: 1
subsystem: ui-ux-audit
tags: [frontend, backend, vue3, gorm, postgres, window-function, i18n, card-render, kodik]
requires: [phase-8]
provides: [bulk-anime-progress-endpoint, useAnimeProgress-composable, anime-has-dub-column, episode-aware-ongoing-link]
affects: [phase-10, phase-11]
tech-stack:
  added: []
  patterns:
    - gorm-raw-with-cte-row-number-furthest-episode
    - bulk-endpoint-ids-csv-cap-50
    - debounced-reactive-bulk-fetch-keyed-by-ids-ref
    - lazy-backfill-via-search-driven-reingest
    - episode-aware-router-link-conditional
key-files:
  created:
    - frontend/web/src/composables/useAnimeProgress.ts
  modified:
    - services/catalog/internal/domain/anime.go
    - services/catalog/internal/parser/kodik/client.go
    - services/catalog/internal/repo/anime.go
    - services/catalog/internal/service/catalog.go
    - services/player/internal/domain/watch.go
    - services/player/internal/repo/progress.go
    - services/player/internal/repo/progress_test.go
    - services/player/internal/service/progress.go
    - services/player/internal/handler/progress.go
    - services/player/internal/transport/router.go
    - frontend/web/src/api/client.ts
    - frontend/web/src/components/anime/AnimeCardNew.vue
    - frontend/web/src/views/Home.vue
    - frontend/web/src/views/Browse.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - cte-row-number-by-episode-number-desc-for-furthest-reached
  - leaf-route-under-users-no-gateway-change-mirrors-phase-8
  - dub-badge-locked-to-DUB-across-all-three-locales
  - lazy-backfill-no-one-shot-script
  - browse-wiring-included-search-deferred-to-phase-10
  - completed-badge-suppressed-when-fully-caught-up
metrics:
  duration: ~35min (Wave 1+2 from prior session, Wave 3 in current session)
  completed: 2026-05-13
---

# Phase 9 Summary: Per-card progress + Sub/Dub indicators + Episode-granular row

**Completed:** 2026-05-13
**Plan:** 09-PLAN.md
**Outcome:** Three Tier-E audit findings shipped end-to-end against the shared card-render surface. Logged-in users now see a small purple "Episode N / Y" badge on cards (Home trending row + Browse grid) for anime they have in-progress watch_progress rows, plus an amber DUB badge on cards whose anime has a dubbed Kodik translation. Home's Ongoing column links each row to the specific next-airing episode (`/anime/{id}?episode={N+1}`) when `next_episode_at` is set, hitting the Phase 8 `?episode` reader in `Anime.vue`. Closes UX-16, UX-17, UX-18.

## Changes shipped

### Wave 1 — Catalog (UX-18 / Sub-Dub indicator)

**`services/catalog/internal/domain/anime.go`** — new `HasDub bool` field on the `Anime` struct with `gorm:"default:false;index"`. GORM auto-migrate adds the `has_dub` column on next catalog start (verified post-redeploy: `\d animes` shows `has_dub | boolean | default false` + `idx_animes_has_dub btree (has_dub)`). `default:false` keeps existing rows valid without manual backfill; the index is cheap because the column is low-cardinality.

**`services/catalog/internal/repo/anime.go`** — new `AnimeRepository.SetHasDub` mirroring the existing `SetHasVideo` shape for the lazy-backfill write site.

**`services/catalog/internal/parser/kodik/client.go`** — new `ResultsHaveDub(results []SearchResult) bool` + `TranslationsHaveDub(translations []Translation) bool` helpers. True when at least one entry has `Translation.Type == "voice"` (Kodik's dub indicator).

**`services/catalog/internal/service/catalog.go`** — `GetKodikTranslations` now diffs `anime.HasDub` against the computed value and writes via `SetHasDub` when it changes. Best-effort, decorative — never blocks playback. **Lazy backfill:** every cache-miss Kodik search re-touches `has_dub` for the anime; existing rows backfill naturally over time without a one-shot script (per CONTEXT decision).

### Wave 2 — Player (UX-16 / Per-card progress endpoint)

**`services/player/internal/domain/watch.go`** — new `BulkAnimeProgressEntry` DTO + `BulkAnimeProgressMap` alias. Not GORM models — populated via raw-SQL scan in the repo. Shape:

```go
type BulkAnimeProgressEntry struct {
    LatestEpisode int  `json:"latest_episode"`
    EpisodesCount int  `json:"episodes_count"`
    EpisodesAired int  `json:"episodes_aired"`
    Completed     bool `json:"completed"`
    Dropped       bool `json:"dropped"`
}
type BulkAnimeProgressMap map[string]BulkAnimeProgressEntry
```

**`services/player/internal/repo/progress.go`** — new `ProgressRepository.GetBulkProgress(ctx, userID, animeIDs)` using a window-function CTE:

```sql
WITH ranked AS (
    SELECT wp.anime_id, wp.episode_number, wp.completed, wp.dropped_off_at,
           ROW_NUMBER() OVER (
               PARTITION BY wp.anime_id
               ORDER BY wp.episode_number DESC, wp.last_watched_at DESC
           ) AS rn
    FROM watch_progress wp
    WHERE wp.user_id = ? AND wp.anime_id IN (?)
)
SELECT r.anime_id, r.episode_number, r.completed, r.dropped_off_at,
       a.episodes_count, COALESCE(a.episodes_aired, 0) AS episodes_aired
FROM ranked r JOIN animes a ON a.id = r.anime_id
WHERE r.rn = 1 AND a.deleted_at IS NULL
```

Ordering by `episode_number DESC` (not `last_watched_at DESC` like Continue-Watching) — per-card progress cares about *furthest* position, not recency. Empty-IDs fast path returns an empty map without hitting the DB. `Completed` semantics for the badge are stricter than `watch_progress.completed=true`: the latest row must be `completed=true` AND `LatestEpisode >= EpisodesCount` (or `>= EpisodesAired` when `EpisodesCount = 0`). A user who completed only E1 of a 12-ep show is still "in progress" for badge purposes.

**`services/player/internal/repo/progress_test.go`** — new `TestProgressRepository_GetBulkProgress` covering:

- **Happy path:** Anime A (E1 in progress + E3 completed → latest=E3, Completed=false because 3<12), Anime B (E5 in progress only → latest=E5), Anime C (every episode E1..E12 completed → latest=E12, Completed=true). Missing anime ID asks for an entry that's not in the map.
- **Empty IDs:** returns empty map without error.
- **Cross-user isolation:** seeds a different user's row for the same anime; the foreign user's `GetBulkProgress` returns empty.

**`services/player/internal/service/progress.go`** — `ProgressService.GetBulkProgress` thin delegate. No prefs-engine interaction — pure read-through.

**`services/player/internal/handler/progress.go`** — `ProgressHandler.GetBulkProgress` HTTP handler. JWT-required via `authz.ClaimsFromContext`. Parses comma-separated `?ids=`, trims whitespace, rejects empty/whitespace-only entries silently, returns empty map on missing `?ids=`, returns HTTP 400 with `"ids must contain at most 50 entries"` when over the cap. Added `"strings"` import.

**`services/player/internal/transport/router.go`** — `r.Get("/anime-progress", progressHandler.GetBulkProgress)` mounted inside the existing JWT-protected `/users` group, immediately after `/continue-watching`. **No gateway change** — the existing `/users/*` wildcard proxy catches it (same pattern as Phase 8 `/continue-watching`).

### Wave 3 — Frontend (UX-16 + UX-17 + UX-18)

**`frontend/web/src/api/client.ts`** — `userApi.getAnimeProgress(ids: string[])` axios call. Sends `?ids=a,b,c` via `params.ids = ids.join(',')`.

**`frontend/web/src/composables/useAnimeProgress.ts`** (NEW) — debounced bulk fetch keyed by a reactive `ids: Ref<string[]>`:

- Exposes `{ progressMap: Ref<Map<string, ProgressEntry>>, loading, error, refresh }`.
- Skips the fetch entirely when `!auth.token` (anonymous) — no 401 noise.
- Caller-side `.slice(0, 50)` mirrors the backend cap; trailing cards render without a badge until the user paginates.
- `watch([ids, () => auth.token], ..., { immediate: true })` debounces by 200ms; rapidly-changing grids (paginated Browse) don't fire per-card requests.
- Unwraps `res.data?.data ?? res.data` to tolerate either envelope shape.

**`frontend/web/src/components/anime/AnimeCardNew.vue`** — three additions:

1. `Anime` interface gains optional `hasDub?: boolean` (UX-18).
2. New optional `progress?: ProgressEntry | null` prop (UX-16). Parents wire it via `useAnimeProgress`.
3. Top-left badge cluster restructured to a flex-col stack: existing quality badge on top, new amber `DUB` badge (`bg-amber-500/90 text-white`) beneath when `anime.hasDub === true`.
4. Bottom-left badge cluster restructured to a flex-col stack: existing watchlist-status badge on top, new purple progress badge (`bg-purple-500/80 text-white`) beneath when `progressBadgeText` is non-empty.
5. `progressBadgeText` computed picks the right i18n key (`card.episodeProgress`); hidden when fully complete (the watchlist completed badge already signals that).

**`frontend/web/src/views/Home.vue`** — three changes:

1. (UX-17) Ongoing column `router-link :to` now navigates to `/anime/{id}?episode={(episodes_aired||0)+1}` when both `next_episode_at` and `episodes_aired` are set; falls back to bare `/anime/{id}` otherwise. Phase 8's `?episode` reader in `Anime.vue` picks this up and auto-loads the next-aired episode in the player.
2. (UX-16) Trending row wires `useAnimeProgress(trendingIds)` keyed off the 20 visible recs. A small purple progress badge renders over each poster bottom-left when the user has in-progress rows on that anime.
3. `HomeAnime` local interface extended with `episodes_aired?: number` + `next_episode_at?: string` to satisfy vue-tsc on the new conditional `:to` expression (the fields already lived on the backing store).

**`frontend/web/src/views/Browse.vue`** — single-paragraph wiring: `useAnimeProgress(browseIds)` keyed off `animeList`, passes per-card entries to `AnimeCardNew` via the new `:progress=` prop. Anonymous users see no badges (composable auto-skips).

**`frontend/web/src/locales/{en,ru,ja}.json`** — new top-level `card` namespace with two keys:

- `card.dubBadge`: locked to `"DUB"` across all three locales (CONTEXT decision — universal recognition; localizing creates three identifiers for the same concept).
- `card.episodeProgress`: `"Episode {n} / {total}"` (en) / `"Серия {n} / {total}"` (ru) / `"第{n}話 / {total}"` (ja).

## Decisions made (deviations from / refinements to plan)

| # | Decision | Rationale |
|---|---|---|
| 1 | Browse.vue wiring included; Search.vue deferred | Browse was a single-paragraph edit (composable + `:progress=` on the card); Search would have cascaded into multi-file plumbing per F5's caveat. Phase 10 picks it up if needed. |
| 2 | Plan B2's helper file was `services/catalog/internal/service/catalog.go` (not the `parser/kodik/client.go` file the plan suggested as a placeholder) | The actual write site for Kodik translations lives in the service layer where `GetKodikTranslations` runs the cache-miss path. The helper itself stays in the parser package; the service consumes it. |
| 3 | Repo additionally exposes `AnimeRepository.SetHasDub` (not in the plan's artifacts list) | Plan B2 called for a `.Model(...).Where(...).Update(...)` inline in the service; the existing `SetHasVideo` repo helper provides a cleaner pattern, so the implementer mirrored that shape. Adds 8 LOC, removes a literal `Update("has_dub", hasDub)` from the service. |
| 4 | The `Completed` flag on the BulkAnimeProgressEntry uses a stricter definition than `watch_progress.completed=true` | Per the plan's "reachedAll" logic in B5. A user who finished E1 of a 12-ep series should still see the progress badge ("Episode 1 / 12") — the watchlist "completed" status semantically means "finished the whole anime", not "finished one episode". |

No issues required Rule 4 (architectural change). All deviations were Rule 2 / Rule 3 surface cleanups.

## Self-check: PASSED

- All commits exist (8 atomic commits on `main`).
- All artifact files exist at the listed paths.
- `cd services/catalog && go vet ./...` clean.
- `cd services/player && go test ./...` clean (all suites pass, no cached failures).
- `cd frontend/web && bunx vue-tsc --noEmit` clean.
- `cd frontend/web && bunx eslint src/...` clean for all five touched files.
- All three locale JSONs parse cleanly.
- Catalog + player + web redeployed; `has_dub` column verified live; bulk endpoint smoke-passed against `ui_audit_bot`'s real anime IDs.

See `09-VERIFICATION.md` for the full scorecard.
