---
phase: 11-browse-detail-polish
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - services/catalog/internal/repo/anime.go
  - services/catalog/internal/handler/catalog.go
  - services/gateway/internal/config/config.go
  - services/gateway/internal/handler/system_status.go
  - services/gateway/internal/transport/router.go
  - services/gateway/cmd/gateway-api/main.go
  - frontend/web/src/api/client.ts
  - frontend/web/src/composables/useSystemStatus.ts
  - frontend/web/src/components/home/SystemStatusBanner.vue
  - frontend/web/src/components/anime/AnimeQuickNav.vue
  - frontend/web/src/views/Browse.vue
  - frontend/web/src/views/Anime.vue
  - frontend/web/src/views/Home.vue
  - frontend/web/src/components/layout/Navbar.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
  - docker/docker-compose.yml
autonomous: true
requirements:
  - UX-21
  - UX-22
  - UX-23
  - UX-24
must_haves:
  truths:
    - "/browse renders a sort dropdown with 5 options (popularity / rating / year / updated / title); changing the value updates ?sort= in the URL and the visible card order changes."
    - "GET /api/anime?sort=updated returns rows ordered by updated_at DESC; sort=title returns rows ordered by name ASC; sort_priority pins still appear first."
    - "/anime/:id renders a sticky Quick-Nav menu with 4 anchor links (overview / episodes / similar / comments); clicking scrolls to that section; the active section's link is highlighted as the user scrolls."
    - "On /anime/:id, clicking the Theater Mode toggle hides the global Navbar and the non-player sections; the player wrapper widens; ESC exits; the choice persists across reload via localStorage."
    - "GET /api/system/status returns { incidents: [...] } sourced from SYSTEM_BANNER_ACTIVE + SYSTEM_BANNER_MESSAGE env vars; empty array when SYSTEM_BANNER_ACTIVE != 'true'."
    - "When SYSTEM_BANNER_ACTIVE=true, Home shows a red banner at the top with the configured message and a dismiss (×) control; dismissal persists per-incident in localStorage."
    - "All new copy renders correctly in en / ru / ja (no untranslated key fallback)."
  artifacts:
    - path: "services/catalog/internal/repo/anime.go"
      provides: "mapSortColumn supports updated/popularity/rating/year/title; default ordering preserves sort_priority"
      contains: "updated_at"
    - path: "services/gateway/internal/handler/system_status.go"
      provides: "SystemStatusHandler.GetStatus + Incident DTO sourced from env"
      contains: "func (h *SystemStatusHandler) GetStatus"
    - path: "services/gateway/internal/transport/router.go"
      provides: "GET /api/system/status route (public)"
      contains: "/system/status"
    - path: "frontend/web/src/composables/useSystemStatus.ts"
      provides: "useSystemStatus composable polling /api/system/status"
      contains: "export function useSystemStatus"
    - path: "frontend/web/src/components/home/SystemStatusBanner.vue"
      provides: "Dismissible red banner with localStorage per-incident state"
      min_lines: 30
    - path: "frontend/web/src/components/anime/AnimeQuickNav.vue"
      provides: "Sticky Quick-Nav pill list with IntersectionObserver active-state highlight"
      min_lines: 50
    - path: "frontend/web/src/views/Anime.vue"
      provides: "Section IDs (section-overview/episodes/similar/comments) + Quick-Nav mount + Theater Mode toggle + ESC handler + body class application + localStorage persistence"
      contains: "theaterMode"
    - path: "frontend/web/src/views/Browse.vue"
      provides: "Sort dropdown extended to 5 axes including 'updated'; URL ?sort= persistence wired on mount + change"
      contains: "browse.sort.updated"
    - path: "frontend/web/src/components/layout/Navbar.vue"
      provides: "navbar class hook + v-show binding driven by document body theater-mode class"
      contains: "theater-mode"
  key_links:
    - from: "frontend/web/src/views/Browse.vue"
      to: "GET /api/anime?sort={axis}"
      via: "loadAnime() params.sort -> apiClient.get('/anime')"
      pattern: "sort:\\s*sortBy"
    - from: "services/catalog/internal/handler/catalog.go (parseFilters)"
      to: "services/catalog/internal/repo/anime.go (mapSortColumn)"
      via: "domain.SearchFilters.Sort"
      pattern: "mapSortColumn"
    - from: "frontend/web/src/views/Home.vue"
      to: "GET /api/system/status"
      via: "useSystemStatus composable -> SystemStatusBanner mount above search-bar block"
      pattern: "useSystemStatus"
    - from: "frontend/web/src/views/Anime.vue"
      to: "frontend/web/src/components/layout/Navbar.vue"
      via: "document.body.classList.add('theater-mode') + Navbar v-show / CSS rule"
      pattern: "theater-mode"
---

# Phase 11 Plan: Browse + detail polish

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Four Tier-E items batched on distinct surfaces (Browse, Anime detail, Anime
player, Home) with zero file-overlap risk. Backend changes are minimal: one
SQL whitelist extension in catalog (UX-21) and one new public endpoint in
gateway (UX-24). The remaining three findings are frontend-only. No new
database tables, no new external libraries.

## Tasks

### Wave 1 — Sort dropdown (UX-21)

#### W1.1 Backend: extend mapSortColumn whitelist (`services/catalog/internal/repo/anime.go`)

- [ ] `mapSortColumn` currently maps `popularity|rating -> score`,
  `year|score|name|created_at|updated_at -> identity`, `title -> name`.
  Add a `case "updated": return "updated_at"` branch so the 5 frontend axes
  (popularity / rating / year / updated / title) all map cleanly. Pattern:
  ```go
  func mapSortColumn(sort string) string {
      switch sort {
      case "popularity", "rating":
          return "score"
      case "year", "score", "name", "created_at", "updated_at":
          return sort
      case "updated":
          return "updated_at"
      case "title":
          return "name"
      default:
          return "score"
      }
  }
  ```
- [ ] **Preserve `sort_priority DESC` pin** in `Search()`. Today line 105-113
  builds `orderBy` two different ways: default keeps `sort_priority DESC, score DESC`,
  but when `filters.Sort != ""` it OVERRIDES the entire clause and the pin is
  lost. Per CONTEXT.md (D-01), the pin must stay as the primary criterion.
  Replace the build with:
  ```go
  orderBy := "sort_priority DESC, score DESC"
  if filters.Sort != "" {
      column := mapSortColumn(filters.Sort)
      order := "DESC"
      if filters.Order == "asc" || filters.Sort == "title" {
          order = "ASC"
      }
      orderBy = fmt.Sprintf("sort_priority DESC, %s %s", column, order)
  }
  ```
  `sort=title` defaults to ASC (A-Z is the intuitive direction); all other
  axes default to DESC. Existing `filters.Order` still wins when set.

#### W1.2 Backend: catalog test + redeploy

- [ ] Existing tests must still pass. The catalog package does not appear to
  have a `mapSortColumn` unit test today; the change is small enough to
  verify via curl smoke after redeploy rather than a dedicated unit test.
- [ ] `cd services/catalog && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-catalog`.
- [ ] Smoke each axis returns a different first-page first-item id:
  ```bash
  for s in popularity rating year updated title; do
    echo "=== sort=$s ==="
    curl -s "http://localhost:8000/api/anime?sort=$s&page_size=3" \
      | jq -r '.data[0:3] | map({id, name, updated_at, score, year}) | .[]'
  done
  ```
  Expected: each axis surfaces a recognisably different ordering. `sort=title`
  returns rows starting with leading-alphabet names (A or Russian А). `sort=updated`
  returns the most recently `updated_at` rows.
- [ ] Verify the pin still wins: pick any anime with `sort_priority > 0`
  (CLAUDE.md "Pinning anime to the top" doc references this). If none
  currently pinned in production, skip this sub-check; otherwise confirm
  the pinned anime appears first across all 5 sort axes.

#### W1.3 Frontend: Browse.vue sort UI (`frontend/web/src/views/Browse.vue`)

- [ ] **Existing state.** `Browse.vue` already wires a `sortBy` ref +
  `sortOptions` computed + `Select` dropdown (lines 56-63, 245, 275-280)
  and `loadAnime()` already sends `params.sort = sortBy.value` (line 334).
  The dropdown today exposes 4 axes (popularity / rating / year / title);
  it is **missing `updated`** and is **not URL-persisted** (changing the
  Select does not write `?sort=` to the URL, only the genre/year/status
  filters do).
- [ ] Extend `sortOptions` to 5 entries — add `updated` between `year`
  and `title`:
  ```ts
  const sortOptions = computed(() => [
    { value: 'popularity', label: t('browse.sort.popularity') },
    { value: 'rating',     label: t('browse.sort.rating') },
    { value: 'year',       label: t('browse.sort.year') },
    { value: 'updated',    label: t('browse.sort.updated') },
    { value: 'title',      label: t('browse.sort.title') },
  ])
  ```
  This swaps the existing `browse.sortPopular`/`browse.sortRating`/
  `browse.sortYear`/`browse.sortTitle` keys for the nested `browse.sort.*`
  shape decided in CONTEXT.md (D-04). Both old + new keys ship in W1.4 so
  no other view breaks.
- [ ] URL state. Inside `handleFilter()` (line 324), include `sort` in the
  query write so the URL reflects the active sort axis. Replace the
  current body with:
  ```ts
  const handleFilter = async () => {
    currentPage.value = 1
    const query: Record<string, string | undefined> = {
      ...route.query,
      page: undefined,
      sort: sortBy.value !== 'popularity' ? sortBy.value : undefined,
    }
    router.replace({ query })
    await loadAnime()
  }
  ```
  Omitting `sort=popularity` (the default) keeps clean URLs for the most-
  common axis. Other axes appear as `?sort=rating`, `?sort=updated`, etc.
- [ ] Read `route.query.sort` on mount. Append to the existing `onMounted`
  block (after the `route.query.status` branch around line 384):
  ```ts
  if (route.query.sort && typeof route.query.sort === 'string') {
    const valid = ['popularity', 'rating', 'year', 'updated', 'title']
    if (valid.includes(route.query.sort)) {
      sortBy.value = route.query.sort
    }
  }
  ```
  Whitelist matches the backend whitelist — unknown values fall through to
  the default `popularity` already set on the ref.
- [ ] Add a route-query watcher for `sort` (mirrors the existing
  `status`/`genre` watchers around line 408-422):
  ```ts
  watch(() => route.query.sort, (newSort) => {
    const val = (newSort as string) || 'popularity'
    if (val !== sortBy.value) {
      sortBy.value = val
      handleFilter()
    }
  })
  ```
- [ ] `hasActiveFilters` already considers `sortBy.value !== 'popularity'`
  (line 252) — no change needed there.

#### W1.4 i18n keys for sort (en / ru / ja)

- [ ] Add a `browse.sort` nested namespace to each locale file. vue-i18n
  allows nested under `browse` because `browse` is already an object.
  In `frontend/web/src/locales/en.json`, inside the `browse` block, add:
  ```json
  "sort": {
    "popularity": "Popularity",
    "rating": "Rating",
    "year": "Year",
    "updated": "Recently updated",
    "title": "A → Z"
  },
  "sortLabel": "Sort"
  ```
- [ ] In `frontend/web/src/locales/ru.json`, RU copy:
  ```json
  "sort": {
    "popularity": "По популярности",
    "rating": "По рейтингу",
    "year": "По году",
    "updated": "Недавно обновлённые",
    "title": "А → Я"
  },
  "sortLabel": "Сортировка"
  ```
- [ ] In `frontend/web/src/locales/ja.json`, JA copy:
  ```json
  "sort": {
    "popularity": "人気順",
    "rating": "評価順",
    "year": "年順",
    "updated": "最近更新",
    "title": "A → Z"
  },
  "sortLabel": "並び替え"
  ```
- [ ] **Leave the legacy keys (`sortPopular`, `sortRating`, `sortYear`,
  `sortTitle`) in place** to avoid breaking any consumer outside Browse.vue
  — grep before deleting:
  `cd frontend/web && grep -rn "browse.sortPopular\|browse.sortRating\|browse.sortYear\|browse.sortTitle" src/`.
  If only Browse.vue uses them, the implementer MAY remove the legacy keys
  in the same edit; otherwise leave them and document in 11-SUMMARY.md.
- [ ] Adjust trailing commas on the lines preceding each insertion so the
  JSON remains valid.

### Wave 2 — Quick-Nav menu (UX-22)

#### W2.1 Anime.vue: section IDs (`frontend/web/src/views/Anime.vue`)

- [ ] The hero/description/episodes/comments/similar sections already exist
  as `<section class="mt-8">` blocks. Tag four of them with stable IDs the
  Quick-Nav links target. Grep for each section anchor and add `id="..."`
  to the `<section>` tag (do NOT change tag type, do NOT add a wrapper):
  - `<section class="mt-8">` at line 272 (description block, immediately
    after the hero) → add `id="section-overview"`.
  - `<section class="mt-8" ref="playerSectionRef">` at line 291 (player +
    episodes) → add `id="section-episodes"`. Keep the existing `ref` —
    add the `id` alongside.
  - `<section class="mt-8">` at line 607 (UGC tabs / comments) → add
    `id="section-comments"`.
  - `<section v-if="relatedAnime.length > 0" class="mt-8">` at line 922
    (related/similar) → add `id="section-similar"`.
- [ ] Anchor names are stable (locked in CONTEXT.md D-02) — Quick-Nav and
  any deep-link `/anime/:id#section-episodes` both depend on these
  identifiers. Do not rename without updating the Quick-Nav component.

#### W2.2 New component (`frontend/web/src/components/anime/AnimeQuickNav.vue`)

- [ ] Create the file. Self-contained component with no props; reads
  section IDs from a hard-coded list because the four sections are fixed
  for the Anime detail view (and locked in CONTEXT.md). Renders one
  pill list with two layouts via Tailwind responsive classes:
  - Desktop (md+): floating-right sticky list at `top-24 right-4`,
    flex-col, pointer-events-auto.
  - Mobile (<md): horizontal scrolling pill row below the hero,
    `sticky top-16`, `overflow-x-auto`, flex-row.
  Skeleton:
  ```vue
  <template>
    <!-- Desktop: floating right sticky pill column -->
    <nav
      class="hidden md:flex md:flex-col md:fixed md:top-24 md:right-4 md:z-30 gap-2"
      :aria-label="$t('anime.nav.heading')"
    >
      <a
        v-for="s in sections"
        :key="s.id"
        :href="`#${s.id}`"
        class="px-3 py-1.5 rounded-full text-xs font-medium transition-colors backdrop-blur-sm bg-white/5 border border-white/10"
        :class="active === s.id ? 'text-cyan-400 border-cyan-400/40 bg-cyan-500/10' : 'text-white/70 hover:text-white'"
        @click="scrollTo(s.id, $event)"
      >
        {{ $t(s.labelKey) }}
      </a>
    </nav>

    <!-- Mobile: sticky horizontal pill row -->
    <nav
      class="md:hidden sticky top-16 z-30 -mx-4 px-4 py-2 bg-gray-950/80 backdrop-blur-md border-b border-white/5 overflow-x-auto scrollbar-hide"
      :aria-label="$t('anime.nav.heading')"
    >
      <div class="flex gap-2 whitespace-nowrap">
        <a
          v-for="s in sections"
          :key="s.id"
          :href="`#${s.id}`"
          class="px-3 py-1.5 rounded-full text-xs font-medium flex-shrink-0"
          :class="active === s.id ? 'text-cyan-400 bg-cyan-500/10' : 'text-white/70'"
          @click="scrollTo(s.id, $event)"
        >
          {{ $t(s.labelKey) }}
        </a>
      </div>
    </nav>
  </template>

  <script setup lang="ts">
  import { ref, onMounted, onBeforeUnmount } from 'vue'

  const sections = [
    { id: 'section-overview',  labelKey: 'anime.nav.overview' },
    { id: 'section-episodes',  labelKey: 'anime.nav.episodes' },
    { id: 'section-similar',   labelKey: 'anime.nav.similar'  },
    { id: 'section-comments',  labelKey: 'anime.nav.comments' },
  ]

  const active = ref<string>('section-overview')
  let observer: IntersectionObserver | null = null

  function scrollTo(id: string, ev: Event) {
    ev.preventDefault()
    const el = document.getElementById(id)
    if (!el) return
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    // Update URL hash without jumping (history.replaceState avoids the default jump).
    history.replaceState(null, '', `#${id}`)
  }

  onMounted(() => {
    // Observe each section; pick the one closest to the top of the viewport
    // as `active`. rootMargin shifts the trigger line down past the sticky
    // header so the active pill flips when the section header crosses the
    // visible area, not when its bottom edge touches the viewport edge.
    observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible[0]) {
          active.value = visible[0].target.id
        }
      },
      { rootMargin: '-80px 0px -60% 0px', threshold: 0 },
    )
    for (const s of sections) {
      const el = document.getElementById(s.id)
      if (el) observer.observe(el)
    }
  })

  onBeforeUnmount(() => {
    observer?.disconnect()
    observer = null
  })
  </script>
  ```
- [ ] No emit, no props. The four IDs are hard-coded because the Anime view
  has exactly those four sections (locked in CONTEXT.md). If a section is
  absent from the DOM (e.g. `relatedAnime.length === 0` hides similar),
  the corresponding pill still renders but its anchor target is missing
  and IntersectionObserver simply never marks it active — acceptable
  degraded behaviour. Implementer MAY add a guard that hides pills whose
  `document.getElementById` returns null at mount; not required for v0.1.

#### W2.3 Anime.vue: mount Quick-Nav

- [ ] Import the component near the existing component imports in
  `<script setup>`:
  ```ts
  import AnimeQuickNav from '@/components/anime/AnimeQuickNav.vue'
  ```
- [ ] Mount it once near the top of the template, immediately inside the
  outermost wrapper after the hero block but before the first
  `<section class="mt-8">` that now carries `id="section-overview"`.
  The component owns its own positioning (sticky / fixed), so it does
  not need a layout slot.
- [ ] The Quick-Nav must NOT render when Theater Mode is on (Wave 3 hides
  it via the existing `non-player-content` class hook). Wrap the mount in
  the same hide-class container as the other non-player sections, or
  guard the component itself with `v-if="!theaterMode"`. Implementer's
  pick — the simpler pattern is the body-class CSS rule from Wave 3.

#### W2.4 i18n keys for Quick-Nav (en / ru / ja)

- [ ] Add a nested `anime.nav` namespace to each locale file. In
  `frontend/web/src/locales/en.json`, inside the existing `anime` object:
  ```json
  "nav": {
    "heading": "On this page",
    "overview": "Overview",
    "episodes": "Episodes",
    "similar": "Similar",
    "comments": "Comments"
  }
  ```
- [ ] RU copy (`frontend/web/src/locales/ru.json`):
  ```json
  "nav": {
    "heading": "На этой странице",
    "overview": "Описание",
    "episodes": "Серии",
    "similar": "Похожее",
    "comments": "Комментарии"
  }
  ```
- [ ] JA copy (`frontend/web/src/locales/ja.json`):
  ```json
  "nav": {
    "heading": "このページの内容",
    "overview": "概要",
    "episodes": "エピソード",
    "similar": "関連作品",
    "comments": "コメント"
  }
  ```

### Wave 3 — Theater mode (UX-23)

#### W3.1 Anime.vue: theaterMode state + toggle + ESC

- [ ] Add a `theaterMode` ref near the other component refs in
  `<script setup>`. Initialise from localStorage so the choice survives
  reload (locked in CONTEXT.md D-03):
  ```ts
  import { ref, onMounted, onBeforeUnmount, watch } from 'vue'

  const theaterMode = ref<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem('theaterMode') === '1',
  )

  function setTheater(on: boolean) {
    theaterMode.value = on
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('theaterMode', on ? '1' : '0')
    }
  }

  function toggleTheater() { setTheater(!theaterMode.value) }
  ```
- [ ] Apply / unapply the `theater-mode` class on `document.body` whenever
  the state changes. Mount + cleanup:
  ```ts
  function applyBodyClass(on: boolean) {
    if (typeof document === 'undefined') return
    document.body.classList.toggle('theater-mode', on)
  }

  // ESC handler — global; only acts when theater mode is on.
  function onEscape(e: KeyboardEvent) {
    if (e.key === 'Escape' && theaterMode.value) {
      setTheater(false)
    }
  }

  onMounted(() => {
    applyBodyClass(theaterMode.value)
    document.addEventListener('keydown', onEscape)
  })

  onBeforeUnmount(() => {
    // Always clean up the body class — leaving theater-mode set after the
    // user navigates away from Anime.vue would hide the navbar everywhere.
    applyBodyClass(false)
    document.removeEventListener('keydown', onEscape)
  })

  watch(theaterMode, (on) => applyBodyClass(on))
  ```
- [ ] Add a toggle button near the top-right of the player wrapper. The
  player wrapper is the `<section ... ref="playerSectionRef">` block.
  Render the button inside that section's header (above the player iframe
  / video element), aligned right:
  ```vue
  <button
    type="button"
    class="inline-flex items-center gap-1 px-3 py-1.5 rounded-md bg-white/5 border border-white/10 text-white/70 hover:text-white text-xs transition-colors"
    :aria-pressed="theaterMode"
    @click="toggleTheater"
  >
    <svg v-if="!theaterMode" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
        d="M4 8V4h4M16 4h4v4M20 16v4h-4M8 20H4v-4" />
    </svg>
    <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
        d="M9 9V4H4M20 4h-5v5M15 20v-5h5M4 15h5v5" />
    </svg>
    {{ theaterMode ? $t('player.theaterModeExit') : $t('player.theaterModeEnter') }}
  </button>
  ```
- [ ] Tag the non-player wrapper(s) so CSS can hide them when theater mode
  is on. The simplest cut: add `class="non-player-content"` to every
  `<section class="mt-8">` block that is NOT the player section (i.e. the
  description / similar / comments sections all carry the new class). The
  player section keeps its existing `playerSectionRef` ref but does NOT
  get `non-player-content`.
- [ ] Add scoped CSS at the bottom of `Anime.vue`. (Cannot be `scoped`
  because the rules target `body.theater-mode` from a child component —
  use an unscoped `<style>` block, or hoist to a global stylesheet.
  Implementer's pick; the unscoped block in Anime.vue is acceptable for
  v0.1 because Anime.vue is the only mount site for theater mode.)
  ```css
  /* Phase 11 / UX-23 — theater mode hides global chrome + non-player
     sections, widens the player wrapper. */
  body.theater-mode .navbar-root { display: none !important; }
  body.theater-mode .non-player-content { display: none; }
  body.theater-mode [data-anime-player-wrapper="true"] {
      max-width: none !important;
      margin-left: 0 !important;
      margin-right: 0 !important;
      padding-left: 0 !important;
      padding-right: 0 !important;
  }
  ```
- [ ] Tag the player wrapper with `data-anime-player-wrapper="true"` on the
  outer player `<section>` so the CSS rule above has a stable hook.

#### W3.2 Navbar.vue: class hook (`frontend/web/src/components/layout/Navbar.vue`)

- [ ] The Navbar root is the `<header>` element at line 2. Add a stable
  class hook so the theater-mode CSS rule (W3.1) can hide it. Append
  `navbar-root` to the existing class binding's static list. The current
  class binding is:
  ```vue
  :class="[
    'fixed top-0 left-0 right-0 z-50 transition-all duration-300',
    isVisible ? 'translate-y-0' : '-translate-y-full',
    'glass-nav'
  ]"
  ```
  Change to:
  ```vue
  :class="[
    'fixed top-0 left-0 right-0 z-50 transition-all duration-300 navbar-root',
    isVisible ? 'translate-y-0' : '-translate-y-full',
    'glass-nav'
  ]"
  ```
- [ ] No reactive binding needed in Navbar — the `body.theater-mode` class
  on `<body>` (managed by Anime.vue) drives the CSS rule from outside.
  Navbar stays stateless w.r.t. theater mode.

#### W3.3 i18n keys for theater mode (en / ru / ja)

- [ ] Add nested `player.theaterModeEnter` + `player.theaterModeExit` to
  each locale. EN:
  ```json
  "theaterModeEnter": "Theater mode",
  "theaterModeExit": "Exit theater mode"
  ```
  RU:
  ```json
  "theaterModeEnter": "Режим кинотеатра",
  "theaterModeExit": "Выйти из режима кинотеатра"
  ```
  JA:
  ```json
  "theaterModeEnter": "シアターモード",
  "theaterModeExit": "シアターモード終了"
  ```
- [ ] If the `player` namespace does not exist in some locale, create it.
  Adjust trailing commas.

### Wave 4 — System banner (UX-24)

#### W4.1 Gateway env config (`services/gateway/internal/config/config.go`)

- [ ] Add two new env-driven fields to the gateway `Config` struct:
  - `SystemBannerActive bool` (env `SYSTEM_BANNER_ACTIVE`, default false)
  - `SystemBannerMessage string` (env `SYSTEM_BANNER_MESSAGE`, default "")
- [ ] Read them in the same `Load()` / `New()` constructor that loads the
  other fields. Pattern (concrete file/function name found at exec time
  via `grep -n "func Load\|func New" services/gateway/internal/config/config.go`):
  ```go
  SystemBannerActive:  os.Getenv("SYSTEM_BANNER_ACTIVE") == "true",
  SystemBannerMessage: os.Getenv("SYSTEM_BANNER_MESSAGE"),
  ```
- [ ] Surface these via the existing `*config.Config` injection point used
  by `NewRouter` (router already receives `cfg *config.Config`).

#### W4.2 Gateway handler (`services/gateway/internal/handler/system_status.go`)

- [ ] Create a new file. Self-contained handler reading the env-backed
  config; no service-to-service calls. Pattern:
  ```go
  package handler

  import (
      "net/http"
      "time"

      "github.com/ILITA-hub/animeenigma/libs/httputil"
      "github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
  )

  // Incident is one row in the system-status response. v0.1 always
  // surfaces at most ONE incident sourced from env. Future phases will
  // back this with a real ops-pipeline read.
  type Incident struct {
      ID       string    `json:"id"`
      Severity string    `json:"severity"`
      Title    string    `json:"title"`
      Since    time.Time `json:"since"`
  }

  // SystemStatusResponse is the response shape. Empty incidents slice
  // when no banner is active.
  type SystemStatusResponse struct {
      Incidents []Incident `json:"incidents"`
  }

  type SystemStatusHandler struct {
      cfg *config.Config
      // started is the gateway process start time, used as the Incident.Since
      // floor. v0.1 has no real "since" — the env-backed incident is treated
      // as always-on since gateway boot.
      started time.Time
  }

  func NewSystemStatusHandler(cfg *config.Config) *SystemStatusHandler {
      return &SystemStatusHandler{cfg: cfg, started: time.Now().UTC()}
  }

  func (h *SystemStatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
      resp := SystemStatusResponse{Incidents: []Incident{}}
      if h.cfg.SystemBannerActive && h.cfg.SystemBannerMessage != "" {
          resp.Incidents = append(resp.Incidents, Incident{
              ID:       "env-active",
              Severity: "incident",
              Title:    h.cfg.SystemBannerMessage,
              Since:    h.started,
          })
      }
      httputil.OK(w, resp)
  }
  ```
- [ ] The endpoint is intentionally simple — no upstream call, no per-
  service health aggregation. The existing `/api/status` (line 82 of
  `router.go`) handles real health aggregation; `/api/system/status` is a
  user-facing banner channel with a stable contract.

#### W4.3 Gateway route registration (`services/gateway/internal/transport/router.go`)

- [ ] Inside the existing `r.Route("/api", ...)` block in `router.go`, add
  a new public route immediately after the auth route (line 141):
  ```go
  // System-status banner (Phase 11 / UX-24). Public, no JWT. Sourced
  // from gateway env (SYSTEM_BANNER_ACTIVE + SYSTEM_BANNER_MESSAGE).
  sysStatusHandler := handler.NewSystemStatusHandler(cfg)
  r.Get("/system/status", sysStatusHandler.GetStatus)
  ```
  No middleware wrapper — the existing CORS / rate-limit / security-headers
  stack at the top of `NewRouter` already applies.

#### W4.4 Docker env (`docker/docker-compose.yml`)

- [ ] Add the two env vars to the gateway service's `environment:` block:
  ```yaml
  - SYSTEM_BANNER_ACTIVE=${SYSTEM_BANNER_ACTIVE:-false}
  - SYSTEM_BANNER_MESSAGE=${SYSTEM_BANNER_MESSAGE:-}
  ```
  Default-off so production behaviour is unchanged unless the operator
  explicitly sets them. No new `.env` entry — operators set the vars
  ad-hoc when they need to surface a banner.

#### W4.5 Gateway redeploy + smoke

- [ ] `cd services/gateway && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-gateway`.
- [ ] Smoke (banner off, the default):
  ```bash
  curl -s http://localhost:8000/api/system/status | jq
  ```
  Expected: `{ "success": true, "data": { "incidents": [] } }`.
- [ ] Smoke (banner on) — temporarily flip the env at the docker-compose
  level and re-redeploy, or `docker compose -f docker/docker-compose.yml
  exec gateway sh -c 'env | grep BANNER'` is empty until set. Verifying
  the active branch is acceptable as a post-deploy operator check —
  skip if no incident is currently active.

#### W4.6 Frontend composable (`frontend/web/src/composables/useSystemStatus.ts`)

- [ ] Create the file. Polls once on mount + every 60s; exposes
  `incidents` ref. Anonymous-safe (no JWT required). Full contents:
  ```typescript
  import { ref, onMounted, onBeforeUnmount } from 'vue'
  import { apiClient } from '@/api/client'

  export interface Incident {
    id: string
    severity: string
    title: string
    since: string
  }

  export function useSystemStatus(pollIntervalMs = 60000) {
    const incidents = ref<Incident[]>([])
    const loading = ref(false)
    const error = ref<string | null>(null)
    let timer: ReturnType<typeof setInterval> | null = null

    async function fetchStatus() {
      loading.value = true
      error.value = null
      try {
        const res = await apiClient.get('/system/status')
        const data = (res.data?.data ?? res.data) as { incidents?: Incident[] }
        incidents.value = Array.isArray(data?.incidents) ? data.incidents : []
      } catch (e) {
        error.value = e instanceof Error ? e.message : 'failed to load system status'
        incidents.value = []
      } finally {
        loading.value = false
      }
    }

    onMounted(() => {
      fetchStatus()
      if (pollIntervalMs > 0) {
        timer = setInterval(fetchStatus, pollIntervalMs)
      }
    })

    onBeforeUnmount(() => {
      if (timer) clearInterval(timer)
      timer = null
    })

    return { incidents, loading, error, refresh: fetchStatus }
  }
  ```
- [ ] If `frontend/web/src/api/client.ts` does not currently export
  `apiClient` as a named export (only `userApi`, `animeApi`, etc.), add a
  named re-export of the underlying axios instance so the composable can
  hit arbitrary public paths:
  ```ts
  export { apiClient }
  ```
  Implementer should check the file first; the existing composables (e.g.
  `useRecs`, `useContinueWatching`) use `userApi`-style wrappers. For
  consistency, the implementer MAY add a `systemApi` wrapper instead:
  ```ts
  export const systemApi = {
    getStatus: () => apiClient.get('/system/status'),
  }
  ```
  Either pattern is acceptable; pick the one that matches the dominant
  shape in `client.ts`.

#### W4.7 Banner component (`frontend/web/src/components/home/SystemStatusBanner.vue`)

- [ ] Create the file. Hidden when incidents empty OR when the current
  incident's ID is already in localStorage as dismissed. Pattern:
  ```vue
  <template>
    <div
      v-if="visibleIncident"
      role="alert"
      class="bg-red-500/90 text-white text-sm px-4 py-2 flex items-center justify-between gap-3"
    >
      <span class="flex-1 text-center">{{ visibleIncident.title }}</span>
      <button
        type="button"
        class="text-white/80 hover:text-white p-1 rounded"
        :aria-label="$t('system.statusBanner.dismiss')"
        @click="dismiss"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  </template>

  <script setup lang="ts">
  import { computed, ref } from 'vue'
  import { useSystemStatus } from '@/composables/useSystemStatus'

  const { incidents } = useSystemStatus(60000)
  const dismissedTick = ref(0)

  function isDismissed(id: string): boolean {
    if (typeof localStorage === 'undefined') return false
    return localStorage.getItem(`sys_status_dismissed_${id}`) === '1'
  }

  // Reactive read — dismissedTick increments on dismiss so the computed
  // re-evaluates without us touching incidents.
  const visibleIncident = computed(() => {
    void dismissedTick.value
    return incidents.value.find((i) => !isDismissed(i.id)) ?? null
  })

  function dismiss() {
    const inc = visibleIncident.value
    if (!inc || typeof localStorage === 'undefined') return
    localStorage.setItem(`sys_status_dismissed_${inc.id}`, '1')
    dismissedTick.value++
  }
  </script>
  ```
- [ ] Component owns its own composable subscription — no props, no emits.

#### W4.8 Mount on Home (`frontend/web/src/views/Home.vue`)

- [ ] Import the component near the existing `import ContinueWatchingRow ...`
  line in `<script setup>`:
  ```ts
  import SystemStatusBanner from '@/components/home/SystemStatusBanner.vue'
  ```
- [ ] Mount the banner at the very top of the template, before the
  `<h1 class="sr-only">` and the search-bar block. Insert immediately
  inside the outermost wrapper `<div class="min-h-screen ...">`:
  ```vue
  <SystemStatusBanner />
  ```
  The banner renders nothing when there is no active incident (or when
  the user dismissed the current one), so a permanent mount is safe.

#### W4.9 i18n keys for the banner (en / ru / ja)

- [ ] Per CONTEXT.md the banner title typically comes from the API; we
  ship one fallback default + one dismiss label. EN:
  ```json
  "statusBanner": {
    "defaultTitle": "Service status update",
    "dismiss": "Dismiss"
  }
  ```
- [ ] RU:
  ```json
  "statusBanner": {
    "defaultTitle": "Сообщение о работе сервиса",
    "dismiss": "Закрыть"
  }
  ```
- [ ] JA:
  ```json
  "statusBanner": {
    "defaultTitle": "サービス状況のお知らせ",
    "dismiss": "閉じる"
  }
  ```
- [ ] Wrap under a top-level `system` namespace if not present
  (`"system": { "statusBanner": { ... } }`).

### Wave 5 — Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — passes.
- [ ] `cd frontend/web && bunx eslint src/composables/useSystemStatus.ts src/components/home/SystemStatusBanner.vue src/components/anime/AnimeQuickNav.vue src/views/Browse.vue src/views/Anime.vue src/views/Home.vue src/components/layout/Navbar.vue` — zero errors / zero warnings.
- [ ] JSON validity for all three locale files:
  ```bash
  cd frontend/web && bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); console.log('ok')"
  ```
- [ ] `cd services/catalog && go test ./... && go vet ./...` — clean.
- [ ] `cd services/gateway && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-catalog`, `make redeploy-gateway`, `make redeploy-web`
  all succeed; `make health` reports the three services healthy.
- [ ] **Manual smoke (UX-21).** Visit `https://animeenigma.ru/browse`:
  open the Sort dropdown; verify 5 options localised correctly. Pick
  "Recently updated"; URL becomes `…?sort=updated`; first card changes.
  Reload — sort stays at "Recently updated" (URL-state-driven). Pick
  "A → Z" — first card is now alphabetically first. Click "Clear
  filters" — sort resets to "Popularity" and `?sort=` drops from URL.
- [ ] **Manual smoke (UX-22).** Visit `/anime/:id` of any anime that has
  comments and similar entries. The Quick-Nav pill list is visible.
  Click each pill in turn — page smoothly scrolls to the matching
  section. Scroll manually — the active pill highlight flips as each
  section crosses the viewport's top third. Deep-link to
  `/anime/:id#section-comments` — page lands at the comments section
  and the pill highlights.
- [ ] **Manual smoke (UX-23).** Same `/anime/:id`. Click "Theater mode"
  near the player. Navbar disappears; description / similar / comments
  blocks disappear; player widens to viewport edge. Reload — theater
  mode persists. Press ESC — theater mode exits, navbar + sections
  return. Click "Exit theater mode" — same effect.
- [ ] **Manual smoke (UX-24).** With `SYSTEM_BANNER_ACTIVE=false` (default),
  Home renders without a banner. Set `SYSTEM_BANNER_ACTIVE=true` and
  `SYSTEM_BANNER_MESSAGE="Тестовое обслуживание идёт"` in
  `docker/docker-compose.yml` gateway env, `make redeploy-gateway`,
  reload Home — a thin red banner at the very top shows the message
  with a × button. Click × — banner disappears; reload — it stays
  dismissed (localStorage). In DevTools, clear
  `localStorage.sys_status_dismissed_env-active` — banner returns on
  next reload. Roll the env back when done.
- [ ] **Chrome MCP axe-core re-run** on Home, /browse, /anime/:id —
  zero new violations:
  - Red banner: contrast on white-over-red-500/90 is AA on the dark page;
    `<button>` has `aria-label`; container has `role="alert"`.
  - Quick-Nav: `<nav>` with `aria-label`, anchors link by `href="#…"`
    so keyboard / screen-reader navigation works without JS.
  - Theater toggle: `<button>` with `aria-pressed` reflecting state.

## Files touched

```
services/catalog/internal/repo/anime.go                             (+ updated case, + preserve sort_priority pin)
services/gateway/internal/config/config.go                          (+ SystemBannerActive + SystemBannerMessage fields)
services/gateway/internal/handler/system_status.go                  (NEW — Incident + SystemStatusHandler)
services/gateway/internal/transport/router.go                       (+ GET /api/system/status route)
services/gateway/cmd/gateway-api/main.go                            (touch if needed — propagate cfg to NewRouter; usually already wired)
frontend/web/src/api/client.ts                                      (+ apiClient export or systemApi wrapper)
frontend/web/src/composables/useSystemStatus.ts                     (NEW)
frontend/web/src/components/home/SystemStatusBanner.vue             (NEW)
frontend/web/src/components/anime/AnimeQuickNav.vue                 (NEW)
frontend/web/src/views/Browse.vue                                   (+ updated sort axis, + browse.sort.* keys, + ?sort= URL state)
frontend/web/src/views/Anime.vue                                    (+ section IDs, + QuickNav mount, + theaterMode state/toggle/ESC, + body class + CSS rules)
frontend/web/src/views/Home.vue                                     (+ SystemStatusBanner mount at top)
frontend/web/src/components/layout/Navbar.vue                       (+ navbar-root class hook)
frontend/web/src/locales/en.json                                    (+ browse.sort.*, browse.sortLabel, anime.nav.*, player.theaterMode*, system.statusBanner.*)
frontend/web/src/locales/ru.json                                    (+ same keys, RU copy)
frontend/web/src/locales/ja.json                                    (+ same keys, JA copy)
docker/docker-compose.yml                                           (+ gateway SYSTEM_BANNER_ACTIVE + SYSTEM_BANNER_MESSAGE env)
.planning/workstreams/ui-ux-audit/phases/11-browse-detail-polish/
  11-CONTEXT.md                                                     (already exists)
  11-PLAN.md                                                        (this file)
  11-SUMMARY.md                                                     (written at execute-phase end)
  11-VERIFICATION.md                                                (written at execute-phase end)
```

No new database tables, no new GORM auto-migrations, no new external
libraries. The only backend surface change is one extended SQL whitelist
branch (catalog) and one new public env-backed endpoint (gateway).

## Closes

| Requirement | Surface | Mechanism |
|---|---|---|
| UX-21 | /browse | Sort dropdown extended to 5 axes (popularity / rating / year / updated / title); `?sort=` URL state read on mount + written on change; backend `mapSortColumn` extended with `updated -> updated_at`; `sort_priority` pin preserved as primary order key |
| UX-22 | /anime/:id | New `AnimeQuickNav.vue` (sticky pill list, desktop floating-right column + mobile horizontal pill row); four stable section IDs (`section-overview/episodes/similar/comments`) added to existing `<section>` blocks; IntersectionObserver-driven active-state highlight |
| UX-23 | /anime/:id player view | `theaterMode` ref in Anime.vue + toggle button + global ESC handler; `body.theater-mode` class drives CSS rules that hide `.navbar-root` + `.non-player-content` and widen `[data-anime-player-wrapper="true"]`; choice persisted via `localStorage.theaterMode` |
| UX-24 | / | New gateway endpoint `GET /api/system/status` sourcing one incident from `SYSTEM_BANNER_ACTIVE` + `SYSTEM_BANNER_MESSAGE` env; `useSystemStatus` composable polls every 60s; `SystemStatusBanner.vue` renders a dismissible red banner at the top of Home; per-incident dismissal via `localStorage.sys_status_dismissed_{id}` |

## Wave outline

| Wave | Tasks | Rationale |
|---|---|---|
| 1 (UX-21) | W1.1 repo SQL → W1.2 catalog test + redeploy + curl smoke → W1.3 Browse.vue dropdown + URL state → W1.4 i18n keys | Smallest backend change; the frontend already had a partial dropdown so this wave mostly finishes wiring it. |
| 2 (UX-22) | W2.1 section IDs → W2.2 AnimeQuickNav component → W2.3 mount → W2.4 i18n | Pure frontend, single component + Anime.vue edits. No backend touch. |
| 3 (UX-23) | W3.1 Anime.vue theater state + toggle + ESC + body class + CSS → W3.2 Navbar class hook → W3.3 i18n | Pure frontend. Depends on Wave 2 only by file proximity (both edit Anime.vue) — sequential to avoid merge churn. |
| 4 (UX-24) | W4.1 gateway config → W4.2 handler → W4.3 route → W4.4 docker env → W4.5 gateway redeploy + smoke → W4.6 composable → W4.7 banner component → W4.8 Home mount → W4.9 i18n | Backend + frontend, but small surface; ships as one slice because the banner has no consumer until the endpoint exists. |
| 5 (Verification) | vue-tsc + eslint + go test + go vet + redeploys + make health + manual smoke per UX-21/22/23/24 + axe re-run | Single gate covering all four findings before commit. |
