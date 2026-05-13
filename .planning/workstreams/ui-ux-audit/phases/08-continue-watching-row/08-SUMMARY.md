---
phase: 8
plan: 1
subsystem: ui-ux-audit
tags: [frontend, backend, vue3, gorm, postgres, window-function, i18n, a11y, home, continue-watching]
requires: [phase-3]
provides: [continue-watching-row, list-continue-watching-endpoint, components-home-directory]
affects: [phase-9, phase-16]
tech-stack:
  added: []
  patterns:
    - gorm-raw-with-cte-row-number
    - vue3-self-gated-row-component
    - auth-aware-composable-mirrors-useRecs
    - flat-i18n-keys-no-nested-collision
key-files:
  created:
    - frontend/web/src/composables/useContinueWatching.ts
    - frontend/web/src/components/home/ContinueWatchingRow.vue
  modified:
    - services/player/internal/domain/watch.go
    - services/player/internal/repo/progress.go
    - services/player/internal/repo/progress_test.go
    - services/player/internal/service/progress.go
    - services/player/internal/handler/progress.go
    - services/player/internal/transport/router.go
    - frontend/web/src/api/client.ts
    - frontend/web/src/views/Home.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - cte-row-number-per-anime-completed-false
  - leaf-route-under-users-no-gateway-change
  - hide-row-on-zero-items-no-empty-cta
  - flat-i18n-keys-continueWatching-and-continueWatchingEpisode
metrics:
  duration: ~25min
  completed: 2026-05-13
---

# Phase 8 Summary: Continue-Watching home row (Phoenix new feature)

**Completed:** 2026-05-13
**Plan:** 08-PLAN.md
**Outcome:** New backend endpoint + new frontend row shipped end-to-end. Closes audit finding UA-061 (Tier-E #1 — the single largest UX delta in the 2026-05-12 audit). Logged-in users now see a Continue-Watching row at the top of Home, above the trending row, populated from `watch_progress`. Anonymous users and logged-in users with zero in-progress rows see no degraded affordance — the row is fully hidden in those cases. Phase 9 (per-card progress) is unblocked.

## Changes shipped

### Wave 1 — Backend (`services/player/`)

**`services/player/internal/domain/watch.go`** — new `ContinueWatchingItem` DTO appended after `WatchlistStats`. Contains `Anime AnimeInfo` (poster/title projection inlined), `EpisodeNumber`, `Progress`, `Duration`, `LastWatchedAt`, optional `DroppedOffAt`. Not a GORM model — populated via raw-SQL scan in the repo.

**`services/player/internal/repo/progress.go`** — new `ListContinueWatching(ctx, userID, limit)` method using a window-function CTE:

```sql
WITH ranked AS (
    SELECT wp.*, ROW_NUMBER() OVER (
        PARTITION BY wp.anime_id ORDER BY wp.last_watched_at DESC
    ) AS rn
    FROM watch_progress wp
    WHERE wp.user_id = ? AND wp.completed = false
)
SELECT r.*, a.name, a.name_ru, a.name_jp, a.poster_url, a.episodes_count
FROM ranked r JOIN animes a ON a.id = r.anime_id
WHERE r.rn = 1 AND a.deleted_at IS NULL
ORDER BY r.last_watched_at DESC LIMIT ?
```

Limit clamped to `[1, 20]` (default 10 when caller passes `<=0`). Single statement — no per-anime fan-out. `a.deleted_at IS NULL` honors soft-deletes.

**`services/player/internal/service/progress.go`** — `ProgressService.ListContinueWatching` as a thin delegate (no prefs-engine interaction; Continue-Watching is purely driven by `watch_progress`).

**`services/player/internal/handler/progress.go`** — `ProgressHandler.ListContinueWatching` HTTP handler. JWT-gated via `authz.ClaimsFromContext`. Optional `?limit=N` query param parsed; handler defaults to 10, repo clamps to `[1, 20]`. Returns `httputil.OK(items)` wrapping a non-nil slice (so the frontend can safely call `.length` on the unwrapped envelope). Added `"strconv"` import.

**`services/player/internal/transport/router.go`** — `r.Get("/continue-watching", progressHandler.ListContinueWatching)` mounted inside the existing JWT-protected `/users` group, immediately after the `/progress` route block. **No gateway change** — the existing `r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)` at `services/gateway/internal/transport/router.go:229` catches the path automatically.

**`services/player/internal/repo/progress_test.go`** — new `TestProgressRepository_ListContinueWatching`:

- **Happy path:** two anime, Anime A E1 completed (must be excluded), Anime A E2 in-progress (must surface), Anime B E5 in-progress (older `last_watched_at`). Different-user row seeded to confirm isolation. Asserts ordering (A then B), correct episode (E2 not E1), correct poster/name/episodes_count projection.
- **Empty path:** user with zero rows returns empty slice without error.
- **Limit clamp smoke:** `limit=0` (default 10) and `limit=999` (clamp 20) both return the same fixture data.

Builds on the existing `setupProgressTestDB` + `seedProgressRow` helpers from earlier phases; creates a minimal `animes` table inline because the JOIN needs it.

### Wave 2 — Frontend (`frontend/web/`)

**`frontend/web/src/api/client.ts`** — `userApi.getContinueWatching(limit?)` axios call. Conditionally includes `limit` in `params` so omitting it omits the query param (rather than sending `?limit=undefined`).

**`frontend/web/src/composables/useContinueWatching.ts`** (NEW) — mirrors the `useRecs` shape:

- Exposes `{ items, isLoading, error, refresh }`.
- Skips the fetch entirely when `!auth.token` (anonymous) — no 401 noise in network logs.
- `onMounted(fetchItems)` fires the initial load.
- `watch(() => auth.token, ...)` re-fetches on real auth transitions (ignores the mount-time `undefined → token` fire to avoid double-trigger).
- Unwraps `res.data.data ?? res.data` to tolerate either envelope shape.

**`frontend/web/src/components/home/ContinueWatchingRow.vue`** (NEW) — self-contained horizontal-scroll row:

- `v-if="items.length > 0"` so anonymous + zero-progress users see nothing (no empty-state CTA in v0.1).
- Card layout matches the existing trending-row pattern (`w-32 md:w-40 lg:w-48 aspect-[2/3]`).
- Each card: lazy-loaded poster, top-right episode badge (`Episode N` / `Серия N` / `第N話`), 2px cyan progress bar at the bottom of the poster, localized title under the poster.
- `progressPct(item)` clamps to `[0, 100]` to handle clock-skew between heartbeats (`progress > duration`).
- Click navigates to `/anime/{id}?episode={N}` so the player resumes on the right episode.
- `v-else-if="isLoading"` shimmer skeleton matches the trending-row loading skeleton at Home.vue lines 89–97.
- Component owns its own data via the composable — no props, no emits.

Created the `frontend/web/src/components/home/` directory (it did not exist before). Phase 16 (schedule row) will mount its component in the same directory.

**`frontend/web/src/views/Home.vue`** — two-line edit:

- `import ContinueWatchingRow from '@/components/home/ContinueWatchingRow.vue'` added after the existing `LastUpdates` import.
- `<ContinueWatchingRow />` mounted immediately after the search-bar `</div>` and before the existing `<!-- Trending Now Row -->` block. The component's internal `v-if` handles the visibility gate.

**`frontend/web/src/locales/{en,ru,ja}.json`** — two new flat keys inside the existing `home` object:

| Key | en | ru | ja |
|---|---|---|---|
| `home.continueWatching` | Continue Watching | Продолжить просмотр | 続きから見る |
| `home.continueWatchingEpisode` | Episode {n} | Серия {n} | 第{n}話 |

Flat (not nested under `continueWatching`) to match the existing flat shape of the `home` block and to side-step vue-i18n's rule against having both a string value and a nested object at the same key path (a problem the original CONTEXT spec at `home.continueWatching.episode` would have hit).

## Verification

Full success-criteria scorecard in `08-VERIFICATION.md`. Wave-by-wave summary:

**Wave 1 — Backend:**
- `cd services/player && go build ./...` clean.
- `cd services/player && go test ./...` clean — new `TestProgressRepository_ListContinueWatching` passes (happy + empty + limit clamp), all existing tests still pass.
- `cd services/player && go vet ./...` clean.
- `make redeploy-player` succeeded, container healthy.
- Authenticated smoke: `curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/users/continue-watching` returns 2 items for `ui_audit_bot`. Items have full `anime` projection (id, name, name_ru, name_jp, poster_url, episodes_count) plus episode_number, progress, duration, last_watched_at.
- Unauthenticated smoke: `curl http://localhost:8000/api/users/continue-watching` returns HTTP 401.
- Limit smoke: `?limit=1` returns 1 item.

**Wave 2 — Frontend:**
- `bunx vue-tsc --noEmit` clean.
- `bunx eslint` on the 4 touched files clean.
- JSON validity check on en/ru/ja locales passes.
- `make redeploy-web` succeeded, container healthy.
- Bundle smoke: `curl http://localhost:3003/assets/index-*.js` contains all three of `Continue Watching`, `continueWatching`, `continue-watching` (i18n value, key, endpoint path).

## Files touched

```
services/player/internal/domain/watch.go                          (+ ContinueWatchingItem DTO)
services/player/internal/repo/progress.go                         (+ ListContinueWatching repo method)
services/player/internal/repo/progress_test.go                    (+ TestProgressRepository_ListContinueWatching)
services/player/internal/service/progress.go                      (+ ListContinueWatching delegate)
services/player/internal/handler/progress.go                      (+ ListContinueWatching handler + strconv import)
services/player/internal/transport/router.go                      (+ GET /users/continue-watching route)
frontend/web/src/api/client.ts                                    (+ userApi.getContinueWatching)
frontend/web/src/composables/useContinueWatching.ts               (NEW)
frontend/web/src/components/home/ContinueWatchingRow.vue          (NEW; new directory components/home/)
frontend/web/src/views/Home.vue                                   (+ import + <ContinueWatchingRow /> mount)
frontend/web/src/locales/en.json                                  (+ home.continueWatching, home.continueWatchingEpisode)
frontend/web/src/locales/ru.json                                  (+ same keys, RU copy)
frontend/web/src/locales/ja.json                                  (+ same keys, JA copy)
.planning/workstreams/ui-ux-audit/phases/08-continue-watching-row/
  08-CONTEXT.md                                                   (already existed)
  08-PLAN.md                                                      (already existed)
  08-SUMMARY.md                                                   (this file — NEW)
  08-VERIFICATION.md                                              (NEW)
```

No new database tables. No new gateway routes. No new external libraries. No new GORM auto-migration step.

## Commits

1. `feat(08): ContinueWatchingItem DTO`
2. `feat(08): ProgressRepository.ListContinueWatching with CTE query`
3. `feat(08): ProgressService + Handler for continue-watching`
4. `feat(08): GET /users/continue-watching route`
5. `test(08): ListContinueWatching repo test (happy + empty)`
6. `feat(08): frontend useContinueWatching composable + API client`
7. `feat(08): ContinueWatchingRow component + Home mount`
8. `feat(08): i18n keys for continue-watching (en/ru/ja)`

Plus the metadata commit (this SUMMARY + VERIFICATION).

## Deviations from plan

None — plan executed exactly as written.

**One sanity-check observation (not a deviation):** the plan flagged that the `seed-ui-audit-user.sh` script seeds `watch_progress` rows with `completed=TRUE`, which would normally make the smoke curl return zero items. Production reality differed — `ui_audit_bot` has **two** in-progress (`completed=false`) rows from real audit usage post-seed (Maou no Musume wa Yasashisugiru!! E10 and Saenai Heroine no Sodatekata ♭ E1). The curl correctly returned both, ordered by `last_watched_at DESC`. The phase 9 work-item to surface "completed-but-next-episode-available" is unaffected.

## Closes

| Requirement | Finding | Surface | Mechanism |
|---|---|---|---|
| UX-15 | UA-061 (Tier-E #1) | New Continue-Watching row above trending on logged-in Home | `GET /api/users/continue-watching` window-function query on `watch_progress` JOIN `animes`; new `ContinueWatchingRow.vue` mounted in `Home.vue` self-gates on `items.length > 0` so anonymous and zero-progress users see no degraded affordance |

## Notes for downstream phases

- **Phase 9 (per-card progress + Sub/Dub + Episode-granular row)** — the 2px cyan progress bar pattern lives in `ContinueWatchingRow.vue` and is the reference implementation. Phase 9 will generalize it to `RecItem`, Browse, and Search cards. The composable's `progressPct(item)` clamp logic (`Math.max(0, Math.min(100, pct))`) handles clock-skew safely; reuse it.
- **Phase 9 also revisits** the "completed E1 but E2 exists and hasn't been started" case currently deferred. The DTO's `EpisodesCount` is already populated and can be combined with `MAX(episode_number) WHERE completed=true` from `watch_progress` to derive `next_episode_number` server-side without a second round-trip.
- **Phase 16 (broadcast schedule row)** — the `frontend/web/src/components/home/` directory now exists. Mount the new schedule row component there with the same self-gating idiom (`v-if="items.length > 0"`).
- **Audit framework** — `ui_audit_bot` is now an excellent test vector for the Continue-Watching row because it has real (non-seeded) in-progress rows. The Chrome MCP audit framework can probe Home as `ui_audit_bot` and reliably see the new row.

## Threat Flags

None — no new attack surface. The new endpoint reads existing `watch_progress` and `animes` tables under the user's own JWT claim; no cross-user data path, no file access, no write endpoints. The window-function SQL is parameterized via GORM's `.Raw(?, ?)` placeholders so it inherits the same SQL-injection posture as the rest of the repo.

## Self-Check: PASSED

All claimed files exist and all 8 task commits are present in `git log`. Verified:

- `services/player/internal/domain/watch.go` — FOUND (DTO `ContinueWatchingItem` greps cleanly at the end of the file)
- `services/player/internal/repo/progress.go` — FOUND (`ListContinueWatching` method present)
- `services/player/internal/repo/progress_test.go` — FOUND (`TestProgressRepository_ListContinueWatching` present)
- `services/player/internal/service/progress.go` — FOUND (`ListContinueWatching` delegate present)
- `services/player/internal/handler/progress.go` — FOUND (`ListContinueWatching` handler + `"strconv"` import)
- `services/player/internal/transport/router.go` — FOUND (`/continue-watching` route in `/users` group)
- `frontend/web/src/api/client.ts` — FOUND (`getContinueWatching` in `userApi`)
- `frontend/web/src/composables/useContinueWatching.ts` — FOUND (NEW)
- `frontend/web/src/components/home/ContinueWatchingRow.vue` — FOUND (NEW, in NEW directory)
- `frontend/web/src/views/Home.vue` — FOUND (import + mount)
- `frontend/web/src/locales/{en,ru,ja}.json` — FOUND (new keys in each)
- 8 commits FOUND in `git log` (4f04e86, e8ac9d3, d192fd6, 5a840a7, 13a6bae, e6932b3, 8c1ae75, fd687bc)
