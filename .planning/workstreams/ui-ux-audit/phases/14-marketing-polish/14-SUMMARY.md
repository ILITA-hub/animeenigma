---
phase: 14
plan: 1
subsystem: ui-ux-audit
tags: [backend, go, gorm, frontend, vue3, i18n, marketing, faq, social-proof]
requires: []
provides:
  - watchers-count-endpoint
  - watchers-count-badge
  - search-placeholder-clarity
  - about-faq-view
affects:
  - GET /api/anime/{animeId}/watchers-count (new public endpoint)
  - anime detail view (new social-proof badge near score rail)
  - all search inputs binding $t('search.placeholder')
  - new public route /about with native <details> FAQ accordion
  - Navbar mobile drawer (new About link)
tech-stack:
  added: []
  patterns:
    - public-anime-aggregate-endpoint-bypassing-auth-middleware
    - intl-numberformat-compact-notation-locale-aware
    - native-details-accordion-zero-js
    - i18n-key-fallback-during-staged-rollout
key-files:
  created:
    - frontend/web/src/views/About.vue
  modified:
    - services/player/internal/repo/list.go
    - services/player/internal/service/list.go
    - services/player/internal/handler/list.go
    - services/player/internal/transport/router.go
    - services/gateway/internal/transport/router.go
    - frontend/web/src/api/client.ts
    - frontend/web/src/views/Anime.vue
    - frontend/web/src/router/index.ts
    - frontend/web/src/components/layout/Navbar.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - services/player/Dockerfile
    - services/gateway/Dockerfile
decisions:
  - watchers-count-lives-on-player-service-because-anime_list-table-lives-there
  - public-endpoint-no-auth-because-soft-social-proof-is-marketing-not-pii
  - hide-badge-below-5-watchers-to-avoid-empty-signals-on-niche-titles
  - intl-numberformat-compact-notation-for-1.2K-rendering-locale-aware
  - native-details-accordion-for-zero-js-keyboard-accessibility-and-seo
  - drawer-only-about-link-because-no-Footer.vue-exists-and-desktop-navbar-stays-minimal
  - i18n-keys-committed-after-feature-commits-so-vue-i18n-key-fallback-prevents-crash-mid-deploy
  - dockerfile-streamprobe-copy-fix-rule-3-scoped-to-redeploys-this-phase-requires
metrics:
  duration: ~45min
  completed: 2026-05-13
  commits: 6
  tasks_complete: 17
  tasks_total: 17
---

# Phase 14 Plan 1: Marketing-surface polish Summary

Phase 14 closes three Tier-E UX items in a single batched plan: a soft-
social-proof "watchers count" badge on the anime detail view (UX-28), a
clearer search placeholder across en/ru/ja (UX-29), and a public About /
FAQ page mounted at `/about` (UX-30). 1 new backend endpoint + 1 new SPA
route + 18 i18n keys × 3 locales (54 entries), shipped as 5 atomic feature
commits + 1 fix commit for a pre-existing Dockerfile gap discovered during
verification.

## What shipped

### Wave 1 — Follower count (UX-28)

- **Backend (player service)**
  - `ListRepository.CountWatchers(ctx, animeID) (int64, error)` — single
    `SELECT COUNT(*) FROM anime_list WHERE anime_id=? AND status='watching'`,
    GORM-managed `deleted_at IS NULL` filter applied automatically.
  - `ListService.GetWatchersCount` thin wrapper.
  - `ListHandler.GetWatchersCount` — public, returns `{ count: int64 }`.
  - Route: `GET /api/anime/{animeId}/watchers-count` registered under the
    `/anime/{animeId}` public group (sibling of `/reviews`, `/rating`,
    `/comments` GET) so anonymous callers reach it without AuthMiddleware.
- **Gateway proxy**
  - `r.Get("/anime/{animeId}/watchers-count", proxyHandler.ProxyToPlayer)`
    registered BEFORE the generic `/anime/*` → catalog catch-all so chi
    routes the path to player, not catalog.
- **Frontend (Anime.vue)**
  - `animeApi.getWatchersCount(animeId)` in `client.ts`.
  - `watchersCount` ref + `formatCount(n)` helper using
    `Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 })`
    so the badge renders "1.2K" / "12K" / "1.2M" locale-aware.
  - `fetchWatchersCount()` non-blocking after `fetchReviews()`; errors are
    swallowed (missing badge is preferable to a noisy console for a non-
    critical UI signal). Reset in `loadAnimeData()` so navigating between
    anime IDs never flashes stale counts.
  - Badge near the score rail; `v-if="watchersCount >= 5"` hides it on
    niche / fresh titles. Uses existing `<Badge variant="default">` plus a
    👥 emoji. Localized via `anime.watchersCount` with `{count}` placeholder.

### Wave 2 — Search placeholder clarity (UX-29)

Updated the existing `search.placeholder` key across all three locales:

| Locale | Before | After |
|---|---|---|
| en | "Search anime..." | "Search: title or genre" |
| ru | "Поиск аниме..." | "Поиск: название или жанр" |
| ja | "アニメを検索..." | "検索: タイトルまたはジャンル" |

No code changes — `SearchAutocomplete.vue` (and Navbar's inline search
input, line 59 of Navbar.vue) already bind `$t('search.placeholder')`.
The backend search query already matches genre names via Shikimori, so
the new placeholder text correctly reflects existing behavior.

### Wave 3 — About / FAQ view (UX-30)

- **New view** `frontend/web/src/views/About.vue`
  - Wrapper: `max-w-3xl mx-auto px-4 py-12`.
  - Header: `t('about.title')` + `t('about.subtitle')`.
  - 8 FAQ items rendered via `v-for` over a static `faqs[]` array of
    `{ qKey, aKey }` pairs. Each renders as:
    ```html
    <details class="border-b border-white/10 py-3 group">
      <summary class="cursor-pointer text-lg font-medium text-white flex items-center justify-between list-none">
        <span>{{ t(item.qKey) }}</span>
        <svg class="w-5 h-5 transition-transform group-open:rotate-180 ..." />
      </summary>
      <p class="mt-3 text-white/70 leading-relaxed">{{ t(item.aKey) }}</p>
    </details>
    ```
  - Native `<details>`: zero-JS accordion, keyboard-accessible by default,
    SEO-friendly (content is always in the DOM). `summary::-webkit-details-marker
    { display: none }` hides the default disclosure triangle so the rotating
    chevron is the only visual indicator.
- **Router** — new `/about` route, lazy-loaded, `titleKey: 'about.title'`.
- **Navbar drawer** — added an About link between Profile/Login and the
  language toggle, only in the mobile drawer (no Footer.vue exists in this
  project; the desktop navbar stays intentionally minimal — desktop users
  reach `/about` via direct URL or a future Footer pattern).

### Wave 4 — i18n (18 keys × 3 locales = 54 entries)

- `nav.about` — drawer link label
- `anime.watchersCount` — UX-28 badge ("{count} watching")
- `about.title`, `about.subtitle` — About.vue header
- `about.faqs.q1..q8.{q,a}` — 8 FAQ entries covering platform overview,
  monetization, sources, recommendations, MAL import, mobile, bug reports,
  and ownership

All 54 entries pass `bash scripts/i18n-lint.sh` (0 missing keys, 0 syntax
errors). The 16 "unused keys" and 20 "hardcoded text" warnings reported by
the linter are all pre-existing and unrelated to Phase 14.

## Deviations from Plan

### Auto-fixed issues

**1. [Rule 3 — Blocking] Add `libs/streamprobe` COPY to player + gateway Dockerfiles**

- **Found during:** Wave 5 (`make redeploy-player`)
- **Issue:** `go.work` declares `./libs/streamprobe` under the `use (...)`
  block (added by an earlier scraper workstream commit), but the player,
  gateway, and 6 other service Dockerfiles never copy that module's
  `go.mod` alongside the other libs. `RUN cd services/player && go mod
  download` therefore fails with:
  ```
  go: cannot load module /app/libs/streamprobe listed in go.work file:
  open /app/libs/streamprobe/go.mod: no such file or directory
  ```
  Pre-existing; blocks Phase 14's mandatory `make redeploy-player &&
  make redeploy-gateway` verification step.
- **Fix:** Added `COPY libs/streamprobe/go.mod libs/streamprobe/go.sum*
  ./libs/streamprobe/` to the two services Phase 14 needs to redeploy.
- **Files modified:** `services/player/Dockerfile`,
  `services/gateway/Dockerfile`
- **Commit:** 5dd6f72

**Scope discipline:** 6 other service Dockerfiles (auth, catalog, rooms,
scheduler, streaming, themes) carry the same pre-existing bug. Phase 14
did not fix them — they're out of scope and tracked separately. Phase 14
only touched the two Dockerfiles whose redeploys it required.

### Auth gates

None.

### Architectural changes

None.

## Risk profile

Low. One new public endpoint (`{count: int64}` JSON, no auth, single
`COUNT(*)` query against an existing indexed column), one new SPA route
(read-only static FAQ), and a placeholder text refresh. No schema changes,
no new tables, no new external dependencies, no auth changes. The Anime.vue
patch only adds a non-blocking fetch + a conditionally-rendered badge; if
either the endpoint or the i18n key is missing, the badge degrades to
hidden (count<5) or renders the key string (vue-i18n fallback).

## Closes

| Req | Surface | Mechanism |
|---|---|---|
| UX-28 | Anime detail | Backend `CountWatchers` + frontend badge (hidden < 5) |
| UX-29 | All search inputs | Updated `search.placeholder` i18n (en/ru/ja) |
| UX-30 | `/about` route | New `About.vue` + native `<details>` FAQ + drawer link |

## Self-Check: PASSED

- `services/player/internal/repo/list.go` — FOUND (`CountWatchers` at line 144)
- `services/player/internal/service/list.go` — FOUND (`GetWatchersCount` at line 417)
- `services/player/internal/handler/list.go` — FOUND (`GetWatchersCount` handler)
- `services/player/internal/transport/router.go` — FOUND (`r.Get("/watchers-count", ...)` route registered)
- `services/gateway/internal/transport/router.go` — FOUND (`/anime/{animeId}/watchers-count` proxy at line 162)
- `frontend/web/src/api/client.ts` — FOUND (`getWatchersCount` at line 265)
- `frontend/web/src/views/Anime.vue` — FOUND (badge at line 104, fetch at line 1523, reset at line 1988, call at line 2085)
- `frontend/web/src/views/About.vue` — FOUND (new file, 73 lines, 8 FAQ items)
- `frontend/web/src/router/index.ts` — FOUND (`/about` route at line 87)
- `frontend/web/src/components/layout/Navbar.vue` — FOUND (`nav.about` link in drawer)
- `frontend/web/src/locales/{en,ru,ja}.json` — FOUND (18 new keys × 3 locales; JSON parses clean)
- `services/player/Dockerfile` + `services/gateway/Dockerfile` — FOUND (streamprobe COPY added)

Commits verified in `git log`:
- deb2d0e (Wave 1 backend)
- efb9d12 (Wave 1 frontend)
- a73e8de (Wave 2)
- 76cfad3 (Wave 3)
- d691581 (Wave 4)
- 5dd6f72 (Dockerfile fix)
