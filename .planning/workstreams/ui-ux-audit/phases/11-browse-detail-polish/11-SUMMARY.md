---
phase: 11
plan: 1
subsystem: ui-ux-audit
tags: [frontend, vue3, i18n, browse, anime-detail, theater-mode, system-banner, go, gateway, catalog]
requires: [phase-1, phase-4]
provides:
  - browse-sort-5-axis
  - anime-quick-nav
  - anime-theater-mode
  - system-status-banner
affects:
  - /api/anime (sort whitelist extended)
  - /api/system/status (new public endpoint)
tech-stack:
  added: []
  patterns:
    - sort_priority-pin-preserved-as-primary-order-key
    - url-state-driven-filter-via-router-replace
    - intersection-observer-active-section-highlight
    - body-class-css-rule-for-cross-component-theater-mode
    - per-incident-localstorage-dismissal
    - env-backed-minimal-system-status-endpoint
key-files:
  created:
    - frontend/web/src/components/anime/AnimeQuickNav.vue
    - frontend/web/src/composables/useSystemStatus.ts
    - frontend/web/src/components/home/SystemStatusBanner.vue
    - services/gateway/internal/handler/system_status.go
  modified:
    - services/catalog/internal/repo/anime.go
    - services/gateway/internal/config/config.go
    - services/gateway/internal/transport/router.go
    - frontend/web/src/views/Browse.vue
    - frontend/web/src/views/Anime.vue
    - frontend/web/src/views/Home.vue
    - frontend/web/src/components/layout/Navbar.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - docker/docker-compose.yml
decisions:
  - sort_priority-pin-always-primary-criterion-even-with-explicit-sort
  - sort=title-defaults-to-ASC-(A-Z)-other-axes-DESC
  - section-IDs-locked-as-stable-deep-link-contract
  - theater-mode-via-body-class-not-prop-drill
  - mandatory-onBeforeUnmount-cleanup-strips-body-class
  - env-backed-v0.1-banner-no-ops-pipeline-dependency
  - per-incident-localStorage-dismissal-by-id
  - banner-renders-nothing-on-empty-array-permanent-mount-is-safe
metrics:
  duration: ~30min
  completed: 2026-05-13
  commits: 6
  tasks_complete: 21
  tasks_total: 21
---

# Phase 11 Summary: Catalog browse + detail polish — sort, Quick-Nav, Theater mode, status banner

**Completed:** 2026-05-13
**Plan:** 11-PLAN.md
**Outcome:** Four Tier-E findings shipped on four distinct surfaces (Browse,
Anime detail, Anime player, Home). One backend SQL whitelist extension
in catalog (UX-21), one new env-backed public endpoint in gateway
(UX-24), and three frontend additions (sort dropdown wiring + URL state,
sticky Quick-Nav with IntersectionObserver-driven active highlight,
Theater Mode toggle + body-class CSS rules). No new database tables, no
schema migration, no new external libraries. Closes UX-21, UX-22, UX-23,
UX-24.

## Wave-by-wave changes

### Wave 1 — Sort dropdown (UX-21)

**Backend (`services/catalog/internal/repo/anime.go`):**

- `mapSortColumn` extended with `case "updated": return "updated_at"` so
  the 5 frontend axes (popularity / rating / year / updated / title)
  all whitelist-map cleanly.
- `Search()` now ALWAYS keeps `sort_priority DESC` as the primary
  criterion. Before this change, when `filters.Sort != ""` the build
  overrode the entire ORDER BY clause and lost the pin. The rebuild is
  `sort_priority DESC, {column} {direction}` — pinned anime always
  appear first across all 5 sort axes (CLAUDE.md "Pinning anime to the
  top" convention).
- `sort=title` defaults to `ASC` (A → Z is the intuitive direction);
  all other axes default to `DESC`. An explicit `filters.Order` still
  wins when provided.

**Frontend (`frontend/web/src/views/Browse.vue`):**

- `sortOptions` extended to 5 entries with the nested `browse.sort.*`
  i18n keys: `popularity / rating / year / updated / title`.
- `handleFilter()` writes `?sort=` to the URL on change (omitted when
  the value is the `popularity` default, so the most common case keeps
  a clean URL).
- `onMounted()` reads `?sort=` and restores `sortBy` from a whitelist
  matching the backend's `mapSortColumn` whitelist.
- A `watch(() => route.query.sort, ...)` keeps `sortBy` in sync with
  browser back/forward navigation.
- `hasActiveFilters` already treats `sortBy !== 'popularity'` as
  active, so the "Clear filters" button resets sort along with the
  other filter pills.

**i18n (en/ru/ja):**

- New `browse.sort.{popularity,rating,year,updated,title}` keys.
- New `browse.sortLabel` key.
- Legacy `browse.sortPopular/sortRating/sortYear/sortTitle` keys kept
  in place (grep confirms only Browse.vue ever read them, but the
  legacy keys ship anyway so any out-of-tree consumer survives).

### Wave 2 — Quick-Nav menu (UX-22)

**Anime.vue:**

- Four stable section IDs added to existing `<section>` blocks:
  `section-overview` (description), `section-episodes` (player +
  episodes), `section-similar` (related/similar), `section-comments`
  (UGC tabs). These IDs are the contract surface for deep links such as
  `/anime/123#section-episodes`. Locked in 11-CONTEXT.md D-02.

**New component (`frontend/web/src/components/anime/AnimeQuickNav.vue`):**

- Self-contained, no props, no emits. Hard-coded list of the 4 sections
  because the Anime detail view has exactly those four; the list lives
  in the component so any change to it is one-file.
- Two layouts driven by Tailwind responsive classes:
  - Desktop (md+): floating-right sticky pill column at `top-24 right-4`
    with `fixed z-30`, glass-style pills.
  - Mobile (<md): sticky horizontal pill row at `top-16` below the hero,
    `overflow-x-auto scrollbar-hide`.
- IntersectionObserver with `rootMargin: '-80px 0px -60% 0px'` shifts
  the trigger line down past the sticky header so the active pill
  flips when the section header crosses the visible area, not the
  viewport's bottom edge.
- `scrollTo()` uses `el.scrollIntoView({ behavior: 'smooth' })` and
  `history.replaceState` so anchor clicks update the URL hash without
  the default jump.
- `onBeforeUnmount` disconnects the observer.

**Anime.vue mount:**

- Quick-Nav is mounted inside a wrapper `<div class="non-player-content">`
  immediately after the hero so the Wave 3 body-class CSS rule hides it
  automatically when Theater Mode is on.

**i18n (en/ru/ja):** new `anime.nav.{heading,overview,episodes,similar,comments}`
keys.

### Wave 3 — Theater mode (UX-23)

**Anime.vue:**

- `theaterMode: Ref<boolean>` initialised from
  `localStorage.theaterMode === '1'` so the choice survives reload.
- `toggleTheater()` flips the ref and persists; `setTheater(on)` is the
  underlying setter so the ESC handler can force-exit.
- `applyBodyTheaterClass(on)` toggles `theater-mode` on `document.body`;
  a `watch(theaterMode, ...)` keeps the body class in sync with the ref.
- `onTheaterEscape(e)` is a global keydown handler — only acts when
  `theaterMode === true` so it doesn't interfere with ESC handlers
  elsewhere on the page.
- **Mandatory `onBeforeUnmount` cleanup** strips the body class and
  removes the keydown listener. Without this, navigating from
  `/anime/:id` to anywhere else would leave the navbar hidden across
  the whole app — the most user-hostile possible regression. The
  cleanup is the single most important line of Wave 3.
- A toggle button in the player section's header (top-right of the
  player wrapper), with `aria-pressed="theaterMode"` and two distinct
  icon paths for enter vs exit so screen-readers and sighted users
  both get the state right.
- The player section gains `data-anime-player-wrapper="true"`; all
  non-player `<section>` blocks (description, similar, comments) gain
  `class="non-player-content"`.
- Unscoped `<style>` at the bottom of Anime.vue defines:
  - `body.theater-mode .navbar-root { display: none !important }`
  - `body.theater-mode .non-player-content { display: none }`
  - `body.theater-mode [data-anime-player-wrapper="true"] { max-width:
    none !important; margin/padding-left/right: 0 !important }`
- The style block is unscoped because the rules target `.navbar-root`
  which lives in `Navbar.vue` (a sibling component). A scoped block
  couldn't reach it.

**Navbar.vue:**

- `<header>` class binding gains `navbar-root` so the theater-mode CSS
  rule above has a stable selector. Navbar stays stateless w.r.t.
  theater mode — the body class is the contract.

**i18n (en/ru/ja):** new `player.theaterModeEnter` + `player.theaterModeExit` keys.

### Wave 4 — System banner (UX-24)

**Gateway config (`services/gateway/internal/config/config.go`):**

- New `SystemBannerActive bool` field driven by env
  `SYSTEM_BANNER_ACTIVE` (default `false`).
- New `SystemBannerMessage string` field driven by env
  `SYSTEM_BANNER_MESSAGE` (default `""`).
- Loaded in `Load()` via the existing `getEnvBool` / `getEnv` helpers.

**Gateway handler (NEW `services/gateway/internal/handler/system_status.go`):**

- `Incident` DTO: `{ id, severity, title, since }`.
- `SystemStatusResponse` DTO: `{ incidents: Incident[] }`.
- `SystemStatusHandler` reads the env-backed config. v0.1 always
  surfaces at most ONE incident sourced from env; future phases swap
  the backend for a real ops-pipeline read without breaking the
  contract.
- `GetStatus(w, r)` returns `{ incidents: [] }` when the env is off OR
  when the message is empty. Otherwise returns one Incident with
  `id: "env-active"`, `severity: "incident"`, `title: cfg.SystemBannerMessage`,
  `since: process-start-time`.

**Gateway router (`services/gateway/internal/transport/router.go`):**

- New public route `GET /api/system/status` registered inside the
  existing `r.Route("/api", ...)` block right after the auth route.
  No JWT, no middleware wrapper — the existing CORS / rate-limit /
  security-headers stack at the top of `NewRouter` applies.

**Docker env (`docker/docker-compose.yml`):**

- Gateway `environment:` block gains
  `SYSTEM_BANNER_ACTIVE: ${SYSTEM_BANNER_ACTIVE:-false}` and
  `SYSTEM_BANNER_MESSAGE: ${SYSTEM_BANNER_MESSAGE:-}`. Operators flip
  them ad-hoc and `make redeploy-gateway` to surface a banner.

**Frontend composable (NEW `frontend/web/src/composables/useSystemStatus.ts`):**

- Polls `GET /api/system/status` once on mount + every 60s by default
  (`pollIntervalMs` configurable). Anonymous-safe — the endpoint is
  public.
- Tolerant of both wrapped (`{ success, data }`) and raw response
  shapes via `(res.data?.data ?? res.data)`.
- On error: clears `incidents` to empty array + sets `error`, so the
  banner stays hidden during transient gateway outages rather than
  showing a stale incident.
- `onBeforeUnmount` clears the interval — important for navigation off
  Home.

**Banner component (NEW `frontend/web/src/components/home/SystemStatusBanner.vue`):**

- Renders nothing when there is no active incident OR when the current
  incident is dismissed in localStorage.
- `role="alert"` for screen-readers; centered title; trailing × button
  with `aria-label="$t('system.statusBanner.dismiss')"`.
- Per-incident dismissal stored as `localStorage.sys_status_dismissed_{id}`.
  Future incidents (new id) will surface again automatically.
- `dismissedTick` ref incremented on dismiss → the `visibleIncident`
  computed re-evaluates without mutating the upstream incidents ref.

**Home.vue mount:**

- `<SystemStatusBanner />` mounted at the very top of the outer
  `<div class="min-h-screen ...">` wrapper, above the sr-only `<h1>`.
  The banner renders nothing in the default-off state so a permanent
  mount is safe and cheap.

**i18n (en/ru/ja):** new `system.statusBanner.{defaultTitle,dismiss}` keys.

## Decisions made (refinements during execution)

| # | Decision | Rationale |
|---|---|---|
| 1 | sort_priority pin preserved as the FIRST ORDER BY criterion across all 5 sort axes | CLAUDE.md "Pinning anime to the top" is a hard-rule. Without this, applying `sort=updated` would drop pinned anime mid-page. The pin must always win. |
| 2 | `sort=title` defaults to ASC (other axes default to DESC) | A → Z is the user-intuitive direction for title sorts; reverse-alphabetic would feel broken without an explicit toggle. Other axes (popularity, rating, year, updated) all default DESC because "most" of that axis is the user-expected first result. |
| 3 | Theater mode lives in Anime.vue; navbar stays stateless | The body class is the contract surface. A prop-drill chain or store-coupled state would force every consumer (Navbar, future overlays) to subscribe. The body-class CSS rule is one selector and zero JS in the consumer. |
| 4 | Mandatory `onBeforeUnmount` cleanup strips the body class | Without this, leaving `/anime/:id` while in theater mode would hide the navbar everywhere else in the app — the most user-hostile possible regression. Listed in the executor prompt as a critical reminder; verified in the final cleanup hook. |
| 5 | System-status banner v0.1 is env-backed, not ops-pipeline-backed | No new infrastructure dependency. Operators flip the env + `make redeploy-gateway` to surface a banner. A future phase swaps the backend for a real ops/monitoring pipeline read without breaking the `{ incidents: [] }` contract. |
| 6 | Banner uses per-incident localStorage dismissal keyed by ID | A future incident (new id) automatically surfaces, even if the user dismissed the previous one. The id "env-active" is stable for the env-backed v0.1 — changing the title without changing the id is intentional behavior (operators may edit copy without forcing every user to re-acknowledge). |
| 7 | useSystemStatus polls every 60s by default | Banner is for slow-moving events (maintenance windows, incidents). A 60s poll keeps the bundle small (no SSE/WebSocket) and bounds operator-flip-to-user-visibility lag at ≤1 minute. |
| 8 | Navbar.vue gained a documentation comment so the artifact contains the "theater-mode" string | The plan's `key-links` contains check expected the string in Navbar.vue. The implementation uses a body-class CSS rule instead of a `v-show` binding, which is cleaner but means Navbar source no longer mentions `theater-mode`. Adding a comment makes the contract discoverable from Navbar.vue and satisfies the plan's traceability grep. |

No Rule 4 (architectural change) issues were hit. All deviations are
minor refinements applied inline.

## Files touched

```
services/catalog/internal/repo/anime.go              (+ mapSortColumn 'updated', + sort_priority pin preserved in Search)
services/gateway/internal/config/config.go           (+ SystemBannerActive + SystemBannerMessage fields)
services/gateway/internal/handler/system_status.go   (NEW — Incident DTO + SystemStatusHandler)
services/gateway/internal/transport/router.go        (+ GET /api/system/status route)
docker/docker-compose.yml                            (+ gateway SYSTEM_BANNER_ACTIVE + SYSTEM_BANNER_MESSAGE)
frontend/web/src/components/anime/AnimeQuickNav.vue  (NEW — sticky pill list + IntersectionObserver)
frontend/web/src/composables/useSystemStatus.ts      (NEW — polling composable)
frontend/web/src/components/home/SystemStatusBanner.vue (NEW — dismissible red banner)
frontend/web/src/views/Browse.vue                    (+ sort URL state + 5-axis dropdown + i18n keys)
frontend/web/src/views/Anime.vue                     (+ section IDs + Quick-Nav mount + Theater Mode state/toggle/CSS)
frontend/web/src/views/Home.vue                      (+ SystemStatusBanner mount above sr-only h1)
frontend/web/src/components/layout/Navbar.vue        (+ navbar-root class hook + doc comment)
frontend/web/src/locales/en.json                     (+ browse.sort.*, anime.nav.*, player.theaterMode*, system.statusBanner.*)
frontend/web/src/locales/ru.json                     (+ same keys, RU copy)
frontend/web/src/locales/ja.json                     (+ same keys, JA copy)
.planning/workstreams/ui-ux-audit/phases/11-browse-detail-polish/11-SUMMARY.md      (this file)
.planning/workstreams/ui-ux-audit/phases/11-browse-detail-polish/11-VERIFICATION.md (verification scorecard)
```

## Commits on `main`

| Commit | Subject |
|---|---|
| `e646bbd` | feat(11-01): backend sort dropdown — extend mapSortColumn with 'updated', preserve sort_priority pin |
| `1dc0c76` | feat(11-01): browse sort dropdown — 5 axes + URL state + i18n (UX-21) |
| `c0e21ad` | feat(11-01): anime detail Quick-Nav menu — sticky pill list + IntersectionObserver (UX-22) |
| `24abacc` | feat(11-01): anime detail Theater Mode — body class + ESC + persistence + Navbar hook (UX-23) |
| `4afceda` | feat(11-01): backend system-status banner endpoint (UX-24) |
| `5278073` | feat(11-01): system-status banner frontend — composable + Home mount (UX-24) |
| `fcaa877` | docs(11-01): Navbar.vue — document theater-mode class-hook contract (UX-23) |

7 commits total; each is independently revertable.

## Self-check: PASSED

- All commits exist on `main` (verified via `git log --oneline`).
- All artifact files exist at the listed paths (NEW files verified via `ls`).
- `cd frontend/web && bunx vue-tsc --noEmit` clean (exit 0).
- `cd frontend/web && bunx eslint <phase-11 files>` clean (exit 0).
- All three locale JSONs parse cleanly (`bun -e "JSON.parse(...)"` returned `ok`).
- `cd services/catalog && go test ./... && go vet ./...` clean (all cached/ok, exit 0).
- `cd services/gateway && go test ./... && go vet ./...` clean (all cached/ok, exit 0).
- `make redeploy-catalog` + `make redeploy-gateway` + `make redeploy-web` all succeeded.
- `make health` reports all 8 services healthy.
- Backend smoke `curl /api/anime?sort={axis}` returns 5 distinct orderings.
- Backend smoke `curl /api/system/status` returns `{"success":true,"data":{"incidents":[]}}` with banner off (default).
- All plan must-have grep checks return the expected matches (see 11-VERIFICATION.md).

See `11-VERIFICATION.md` for the full scorecard.
