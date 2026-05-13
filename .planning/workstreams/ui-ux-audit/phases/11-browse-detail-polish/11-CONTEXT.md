# Phase 11: Catalog browse + detail polish - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, four Tier-E items batched)

<domain>
## Phase Boundary

Four small Tier-E items batched into one phase because they each touch a distinct surface and share no overlap:

- **UX-21** — Sort dropdown on `/browse` with 5 axes: popularity / rating / year / recently-updated / A-Z. URL-state-persisted via `?sort=<axis>`. Tier E #6.
- **UX-22** — Sticky Quick-Navigation anchor menu on the `/anime/:id` detail page: links to in-page anchors (poster → описание → серии → похожее → комментарии). Tier E #7.
- **UX-23** — Theater mode toggle on player views (Anime.vue when a player is active): when ON, hides the navbar + collapses the sidebar (if any) and maxes player width. Does NOT enter native fullscreen. Tier E #8.
- **UX-24** — System-status banner on Home: thin red banner at top of Home rendered only when an active incident exists. Sourced from any `AUTO-NNN` issue marked active in `maintenance-state.json` or via a new lightweight `/api/system/status` probe. Tier E #15.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**UX-21 — Sort dropdown:**
- Place above the card grid on `Browse.vue`. Use a native `<select>` styled with Tailwind to match the existing genre filter dropdown.
- Sort axes mapped to backend query param `?sort=<axis>`:
  - `popularity` (default): existing default (probably popularity-desc).
  - `rating`: order by `score DESC`.
  - `year`: order by `released_on DESC`.
  - `updated`: order by `updated_at DESC` (recently-updated catalog rows).
  - `title`: order by `name ASC` (A-Z by primary name).
- Backend: extend `GET /api/anime` (catalog service) to accept `?sort=`. Whitelist the 5 values; fall through to default on unknown input. SQL: append `ORDER BY <mapped column> <direction>`. Default ordering remains `sort_priority DESC, score DESC` per CLAUDE.md pinning convention — `sort=` overrides the SECOND criterion, never the `sort_priority` pin.
- Frontend persists choice in URL: `router.replace({ query: { ...route.query, sort: newAxis } })`. Read on mount.
- i18n keys: `browse.sortLabel`, `browse.sort.popularity`, `browse.sort.rating`, `browse.sort.year`, `browse.sort.updated`, `browse.sort.title` (6 keys × 3 locales = 18 entries).

**UX-22 — Quick-Nav anchor menu:**
- Render a sticky sidebar (or top bar on mobile) on `/anime/:id` detail page. Desktop: `sticky top-20 right-4` floating-right pill list. Mobile: collapse into a horizontally-scrolling pill row below the hero.
- Section IDs added to Anime.vue at the right scroll targets: `id="section-overview"`, `id="section-episodes"`, `id="section-similar"`, `id="section-comments"`. Anchor links: `<a href="#section-overview">{{ $t('anime.nav.overview') }}</a>` etc.
- Active section highlight: use IntersectionObserver to highlight the current section's anchor (`text-cyan-400`). Pure frontend, no backend.
- i18n keys: `anime.nav.overview`, `anime.nav.episodes`, `anime.nav.similar`, `anime.nav.comments`, `anime.nav.heading` (5 × 3 = 15 entries).

**UX-23 — Theater mode:**
- Add a `theaterMode: Ref<boolean>` state in Anime.vue. Toggle button rendered NEAR the player (top-right of player wrapper).
- When ON:
  - `<body>` gets a class `theater-mode` (or use a Pinia/composable-based provider so other components can react).
  - Navbar hidden via CSS: `body.theater-mode .navbar { display: none }`. (Navbar root element gets a class hook.)
  - Player wrapper expands to `max-w-none` and `mx-0` (full viewport width).
  - Page sidebar/content under the player is hidden via CSS `body.theater-mode .non-player-content { display: none }` (or use a v-if guard).
- Persist via `localStorage.theaterMode` so refresh keeps state.
- Exit via the toggle button OR ESC key (mirrors the drawer ESC pattern from Phase 6).
- Distinct from native fullscreen — theater stays in the browser chrome but maximizes the player visual real-estate. WCAG/UX research convention.
- Icon: simple "expand" SVG (`M3 4h7v2H5v5H3V4z M14 4h7v7h-2V6h-5V4z` etc.) — use heroicons-style arrows-out / arrows-in.
- i18n keys: `player.theaterModeEnter`, `player.theaterModeExit` (2 × 3 = 6 entries).

**UX-24 — System-status banner:**
- New endpoint: `GET /api/system/status` (catalog service or gateway-direct). Returns `{ "incidents": [{ id, severity, title, since }] }`. Empty array when no active incidents.
- For v0.1: source data from `.claude/maintenance-state.json` — but that's local to claude-code, not production. Alternative for v0.1: hardcode an `IsActive bool` toggle in env via `SYSTEM_BANNER_MESSAGE` + `SYSTEM_BANNER_ACTIVE` env vars. Reads from gateway service config and exposes via `/api/system/status`. Simpler MVP. When `SYSTEM_BANNER_ACTIVE=false`, endpoint returns empty array.
- Frontend: `useSystemStatus()` composable polls every 60s (or load-once). When `incidents.length > 0`, render banner at top of Home: `<div class="bg-red-500/90 text-white text-sm px-4 py-2 text-center">{{ incident.title }}</div>`.
- Banner is dismissible via local-storage flag `sys_status_dismissed_{incident.id}` — same incident won't re-show after dismissal.
- i18n keys: `system.statusBanner.defaultTitle` (3 × 1 = 3 entries; usually the title comes from the API).

### Locked from ROADMAP

- Phase 11 depends on Phase 1 + Phase 4. Both complete. No blockers.
- Phase 15 (Dragon — multi-axis sidebar) builds on the URL-state pattern established here. Sort dropdown is a building block.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/catalog/internal/handler/anime.go` — existing `/api/anime` handler. Add `sort` query param parsing.
- `services/catalog/internal/repo/anime.go` — likely has the search query builder. Add sort branch.
- `frontend/web/src/views/Browse.vue` — sort UI mounts here, above card grid.
- `frontend/web/src/views/Anime.vue` — Quick-Nav menu mounts here; theater mode lives here too.
- `frontend/web/src/views/Home.vue` — system-status banner renders at top, above existing content.

### Established Patterns

- URL state via `router.replace({ query: ... })` is the convention.
- i18n via vue-i18n; locale files in `frontend/web/src/locales/`.
- Composables for cross-cutting state: `useContinueWatching`, `useAnimeProgress`, etc.

### Integration Points

- Existing genre filter pattern in Browse.vue is the visual reference for the sort dropdown.
- No new tables. No DB migration.
- Gateway routing: `/api/anime` already proxies to catalog; `/api/system/status` needs a new route (probably under `/api/system/*` in gateway).

</code_context>

<specifics>
## Specific Ideas

- The 4 items batch cleanly because each touches a distinct view (Browse, Anime, Anime player, Home). Zero overlap risk.
- For UX-24, hardcoding via env var sidesteps the "where do incidents come from" problem for v0.1. A future phase introduces an admin tool to set incidents dynamically.
- Theater mode using a body class is the dirtiest-but-cleanest pattern for hiding nav across components. Avoid a deep prop-drill chain.

</specifics>

<deferred>
## Deferred Ideas

- Theater mode keyboard shortcut (T key) — defer to Phase 20 polish.
- Quick-Nav table-of-contents on other views (Profile, Themes, etc.) — pattern lives in Anime.vue for v0.1.
- Status banner severity colors (red for incident, amber for maintenance, blue for info) — v0.1 always uses red. Severity logic deferred.
- Status banner sourced from real incidents pipeline — defer to a future ops/monitoring phase.

</deferred>
