---
phase: 15-multi-axis-filter-sidebar
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - services/catalog/internal/domain/anime.go
  - services/catalog/internal/repo/anime.go
  - services/catalog/internal/handler/catalog.go
  - services/catalog/internal/service/catalog.go
  - services/catalog/internal/parser/kodik/client.go
  - services/catalog/internal/parser/animelib/client.go
  - services/catalog/internal/parser/hianime/client.go
  - services/catalog/internal/parser/consumet/client.go
  - frontend/web/src/composables/useBrowseFilters.ts
  - frontend/web/src/components/browse/FilterSection.vue
  - frontend/web/src/components/browse/BrowseSidebar.vue
  - frontend/web/src/views/Browse.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
autonomous: true
requirements:
  - UX-31
must_haves:
  truths:
    - "GET /api/anime?kind=tv&providers=kodik,hianime returns rows where kind='tv' AND (has_kodik=true OR has_hianime=true); empty providers / empty kind = no filter applied."
    - "/browse renders a left sidebar (desktop) / top-toggle drawer (mobile) with 7 collapsible sections: search, genres, format, status, year, provider, sort, plus a reset button at bottom."
    - "Changing any filter writes the corresponding key to ?route.query (genre / kind / status / year_from / year_to / providers / sort) and reloads the grid; reading ?route.query on mount restores the filter state."
    - "When any filter is active, the mobile toggle button shows a count badge with the number of non-empty filter axes."
    - "Each of the 4 video parsers (kodik / animelib / hianime / consumet) persists `has_{provider}=true` on the matching Anime row when the catalog touches that provider for the anime (lazy backfill; mirrors Phase 9 HasDub pattern)."
    - "Mobile drawer opens on toggle click, traps focus via useFocusTrap, closes on ESC + on outside click + on reset, and restores focus to the toggle button on close."
    - "All new copy renders correctly in en / ru / ja (no untranslated key fallback)."
    - "axe-core re-run on /browse desktop + mobile reports zero new violations."
  artifacts:
    - path: "services/catalog/internal/domain/anime.go"
      provides: "HasKodik / HasAnimeLib / HasHiAnime / HasConsumet boolean columns + SearchFilters.Kind + SearchFilters.Providers"
      contains: "HasKodik"
    - path: "services/catalog/internal/repo/anime.go"
      provides: "Search() WHERE clauses for Kind + Providers (OR-set across has_kodik/has_animelib/has_hianime/has_consumet); SetHasKodik/SetHasAnimeLib/SetHasHiAnime/SetHasConsumet helpers"
      contains: "filters.Providers"
    - path: "services/catalog/internal/handler/catalog.go"
      provides: "parseFilters reads `kind` and `providers` query params (providers = comma-separated)"
      contains: "filters.Providers"
    - path: "services/catalog/internal/parser/kodik/client.go"
      provides: "ResultsHaveKodik helper (always true when any result returned) used by service to set has_kodik"
      contains: "ResultsHaveKodik"
    - path: "frontend/web/src/composables/useBrowseFilters.ts"
      provides: "Reactive filter state + URL ↔ state sync + computed API params + activeCount + reset()"
      contains: "export function useBrowseFilters"
      min_lines: 80
    - path: "frontend/web/src/components/browse/FilterSection.vue"
      provides: "Collapsible <details>-based wrapper with a labeled <summary> and slot content"
      min_lines: 25
    - path: "frontend/web/src/components/browse/BrowseSidebar.vue"
      provides: "Full sidebar with 7 sections + reset button; emits no events (uses useBrowseFilters directly)"
      min_lines: 150
    - path: "frontend/web/src/views/Browse.vue"
      provides: "Desktop grid `grid-cols-[280px_1fr]` + mobile drawer with role=dialog/aria-modal/ESC/focus-trap + filter count badge"
      contains: "BrowseSidebar"
  key_links:
    - from: "frontend/web/src/views/Browse.vue"
      to: "GET /api/anime?kind=…&providers=…"
      via: "useBrowseFilters → loadAnime() params → animeApi.getAnimeList"
      pattern: "providers:"
    - from: "services/catalog/internal/handler/catalog.go (parseFilters)"
      to: "services/catalog/internal/repo/anime.go (Search)"
      via: "domain.SearchFilters.Kind + domain.SearchFilters.Providers"
      pattern: "filters.Providers"
    - from: "services/catalog/internal/service/catalog.go (GetKodikTranslations / GetAnimeLibTranslations / GetConsumetEpisodes / GetHiAnimeEpisodes)"
      to: "services/catalog/internal/repo/anime.go (SetHasKodik / SetHasAnimeLib / SetHasHiAnime / SetHasConsumet)"
      via: "lazy backfill on first successful provider call per anime"
      pattern: "SetHas(Kodik|AnimeLib|HiAnime|Consumet)"
---

# Phase 15 Plan: Multi-axis catalog filter sidebar (Dragon)

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Rebuild `/browse` as a 6-axis filter sidebar (genre / format / status / year /
provider / sort) — the provider axis is the headline feature and AnimeEnigma's
competitive moat (no competitor exposes "filter to anime with a Kodik track"
or "anime with a HiAnime dub"). Backend adds four `has_{provider}` boolean
columns mirroring Phase 9's `has_dub`, plus a `kind` filter and a
multi-value `providers` filter. Frontend extracts the filter state into a
composable, ships a desktop sidebar + mobile drawer, and reuses the Phase 6
`useFocusTrap` composable for the drawer a11y. No new external libraries; no
new database tables.

## Tasks

### Wave 1 — Backend: provider booleans + kind/providers filter

#### W1.1 Domain + repo: 4 new columns + SearchFilters extension (`services/catalog/internal/domain/anime.go`, `services/catalog/internal/repo/anime.go`)

- [ ] In `domain/anime.go`, add 4 boolean fields to `Anime` immediately after
  `HasDub` (line 40). Mirror the exact GORM tags Phase 9 used for `has_dub`
  (default false, indexed):
  ```go
  // Phase 15 (UX-31) — per-provider availability booleans. Each parser
  // (kodik / animelib / hianime / consumet) lazily sets its corresponding
  // flag the first time the catalog touches that provider for the anime.
  // Mirrors HasDub. Default false; existing rows backfill over time.
  HasKodik    bool `gorm:"default:false;index" json:"has_kodik"`
  HasAnimeLib bool `gorm:"default:false;index" json:"has_animelib"`
  HasHiAnime  bool `gorm:"default:false;index" json:"has_hianime"`
  HasConsumet bool `gorm:"default:false;index" json:"has_consumet"`
  ```
  GORM AutoMigrate adds the columns on service startup (per CLAUDE.md
  "Database migrations" section); no manual SQL needed.
- [ ] In `domain/anime.go`, extend `SearchFilters` (line 275) with two new
  fields:
  ```go
  Kind      string   // "tv" | "movie" | "ova" | "ona" | "special" | "" (no filter)
  Providers []string // any subset of {"kodik","animelib","hianime","consumet"}; "" = no filter
  ```
- [ ] In `repo/anime.go` `Search()` (after the existing `filters.Status`
  branch around line 95), add a Kind branch and a Providers OR-set branch.
  Kind is a simple equality; Providers builds an OR group so a row passes
  when ANY of the selected has_{provider} columns is true:
  ```go
  if filters.Kind != "" {
      query = query.Where("kind = ?", filters.Kind)
  }
  if len(filters.Providers) > 0 {
      // Whitelist the 4 supported provider keys to the matching column
      // name. Unknown values are dropped silently — frontend whitelist
      // mirrors this so unknown values never reach the SQL.
      colsByKey := map[string]string{
          "kodik":    "has_kodik",
          "animelib": "has_animelib",
          "hianime":  "has_hianime",
          "consumet": "has_consumet",
      }
      var orParts []string
      for _, p := range filters.Providers {
          if col, ok := colsByKey[p]; ok {
              orParts = append(orParts, col+" = true")
          }
      }
      if len(orParts) > 0 {
          query = query.Where("(" + strings.Join(orParts, " OR ") + ")")
      }
  }
  ```
  `strings` is already imported by this file.
- [ ] Add four `SetHas*` helpers immediately after `SetHasDub` (line 153),
  one per provider. Pattern (kodik shown — repeat verbatim for animelib /
  hianime / consumet with the matching column name + method name):
  ```go
  // SetHasKodik flips the animes.has_kodik column for one anime. Called
  // lazily by GetKodikTranslations whenever the catalog touches Kodik for
  // the anime — best-effort. Phase 15 (UX-31).
  func (r *AnimeRepository) SetHasKodik(ctx context.Context, animeID string, has bool) error {
      return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
          Update("has_kodik", has).Error
  }
  ```

#### W1.2 Handler: parse `kind` and `providers` query params (`services/catalog/internal/handler/catalog.go`)

- [ ] In `parseFilters()` at line 577, after the existing `filters.Year`
  parsing block (line 595) but before the `genres` block, add:
  ```go
  if kind := query.Get("kind"); kind != "" {
      // Whitelist matches the Shikimori-source enum and the frontend
      // radio set. Unknown values silently drop to no-filter.
      switch kind {
      case "tv", "movie", "ova", "ona", "special":
          filters.Kind = kind
      }
  }

  if providers := query.Get("providers"); providers != "" {
      // Comma-separated list, e.g. "kodik,hianime".
      raw := strings.Split(providers, ",")
      seen := map[string]bool{}
      for _, p := range raw {
          p = strings.TrimSpace(strings.ToLower(p))
          switch p {
          case "kodik", "animelib", "hianime", "consumet":
              if !seen[p] {
                  filters.Providers = append(filters.Providers, p)
                  seen[p] = true
              }
          }
      }
  }
  ```
  `strings` is already imported in `catalog.go`.

#### W1.3 Service: 4 parser-driven backfill writes (`services/catalog/internal/service/catalog.go`)

- [ ] **Kodik.** In `GetKodikTranslations` immediately after the existing
  `SetHasDub` block (around line 1474), add a parallel write for `has_kodik`.
  Reaching this point means Kodik returned ≥1 translation, so the anime is
  available on Kodik. Skip when already true to avoid noisy UPDATEs:
  ```go
  if !anime.HasKodik {
      if updateErr := s.animeRepo.SetHasKodik(ctx, anime.ID, true); updateErr != nil {
          s.log.Warnw("failed to persist anime.has_kodik",
              "anime_id", anime.ID, "error", updateErr)
      }
  }
  ```
- [ ] **AnimeLib.** In `GetAnimeLibTranslations` (line 2426), after the
  successful `s.animelibClient.GetEpisodeStreams(...)` call returns without
  error (around line 2438), fetch the anime row (via `s.animeRepo.GetByID`
  if not already in scope) and persist `has_animelib=true` when false:
  ```go
  if anime, err := s.animeRepo.GetByID(ctx, animeID); err == nil && anime != nil && !anime.HasAnimeLib {
      if updateErr := s.animeRepo.SetHasAnimeLib(ctx, animeID, true); updateErr != nil {
          s.log.Warnw("failed to persist anime.has_animelib",
              "anime_id", animeID, "error", updateErr)
      }
  }
  ```
  Implementer SHOULD pick the highest-confidence Animelib touch point in
  the call graph (the search/match path inside the same method) — the
  intent is "anime is reachable through the Animelib parser path". The
  Kodik-iframe-fallback path inside Animelib does NOT count (per
  `feedback_animelib_no_kodik_fallback.md` — AnimeLib path treats
  Kodik-only translations as empty).
- [ ] **HiAnime.** In `doHiAnimeSearch` (line 1888) when a successful match
  is found and `id != ""` is returned, persist `has_hianime=true` for that
  anime. Use the same idempotency guard. The match closure is the
  rightmost spot in the call graph where we know the anime resolved to a
  real HiAnime ID — earlier branches may fail per name-variant.
- [ ] **Consumet.** In `GetConsumetEpisodes` (line 2015), after the
  `bestConsumetMatch` resolution succeeds and at least one episode is
  returned, persist `has_consumet=true` for the anime. Same idempotency
  guard.
- [ ] All four writes are best-effort: errors are logged via `s.log.Warnw`,
  never propagated. The provider booleans are advisory hints for the
  filter; their absence does not affect playback or any other surface.

#### W1.4 Backend smoke + redeploy

- [ ] `cd services/catalog && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-catalog`.
- [ ] GORM AutoMigrate runs on startup; confirm the new columns landed:
  ```bash
  docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
    -c "\d animes" | grep -E "has_kodik|has_animelib|has_hianime|has_consumet"
  ```
  Expected: four boolean columns each with a `default false` and the
  per-column index from the GORM tag.
- [ ] Smoke each new query param:
  ```bash
  # Kind filter — should narrow significantly vs. no filter
  curl -s "http://localhost:8000/api/anime?kind=movie&page_size=3" | jq '.data | length, .[0].kind'

  # Provider filter — depends on lazy backfill; may return 0 rows until parsers touch the anime.
  # Touch one anime end-to-end first (e.g. Kodik translations call), then re-query.
  curl -s "http://localhost:8000/api/anime?providers=kodik&page_size=3" | jq '.data | length'

  # Combined: kind + providers + existing year_from
  curl -s "http://localhost:8000/api/anime?kind=tv&providers=kodik,hianime&year_from=2020&page_size=3" \
    | jq '.data | map({id, kind, has_kodik, has_hianime, year}) | .[]'
  ```
  Expected: kind=movie returns rows whose `kind` field is `"movie"`;
  combined query returns rows that match every clause.
- [ ] No new unit tests in this wave — the filter additions are
  exercised end-to-end via the frontend in Wave 3. Existing tests must
  still pass.

### Wave 2 — Frontend: composable + sidebar components

#### W2.1 Composable: `useBrowseFilters.ts` (`frontend/web/src/composables/useBrowseFilters.ts`)

- [ ] Create the file. Owns the reactive filter object, the URL ↔ state
  sync, the computed `apiParams` (the shape `loadAnime()` sends to
  `animeApi.getAnimeList`), and the active-filter count badge. Pattern:
  ```typescript
  import { computed, onMounted, ref, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'

  // Phase 15 (UX-31) — multi-axis browse filter state. The composable owns
  // both directions of the URL ↔ state sync: ?route.query is the source of
  // truth on mount + on browser back/forward; in-place changes (sidebar
  // click) call writeUrl() to mutate ?route.query and the watcher re-reads.

  export type Kind = '' | 'tv' | 'movie' | 'ova' | 'ona' | 'special'
  export type Provider = 'kodik' | 'animelib' | 'hianime' | 'consumet'
  export type Sort = 'popularity' | 'rating' | 'year' | 'updated' | 'title'

  const KIND_VALUES: Kind[] = ['', 'tv', 'movie', 'ova', 'ona', 'special']
  const PROVIDER_VALUES: Provider[] = ['kodik', 'animelib', 'hianime', 'consumet']
  const SORT_VALUES: Sort[] = ['popularity', 'rating', 'year', 'updated', 'title']

  export function useBrowseFilters() {
    const route = useRoute()
    const router = useRouter()

    const q          = ref('')
    const genres     = ref<string[]>([])
    const kind       = ref<Kind>('')
    const status     = ref<string>('') // 'ongoing' | 'released' | 'announced' | ''
    const yearFrom   = ref<number | null>(null)
    const yearTo     = ref<number | null>(null)
    const providers  = ref<Provider[]>([])
    const sort       = ref<Sort>('popularity')

    function readUrl() {
      const qry = route.query
      q.value         = (qry.q as string) || ''
      genres.value    = (qry.genre as string || '').split(',').filter(Boolean)
      kind.value      = KIND_VALUES.includes(qry.kind as Kind) ? (qry.kind as Kind) : ''
      status.value    = (qry.status as string) || ''
      yearFrom.value  = qry.year_from ? parseInt(qry.year_from as string, 10) || null : null
      yearTo.value    = qry.year_to ? parseInt(qry.year_to as string, 10) || null : null
      providers.value = ((qry.providers as string) || '')
        .split(',').map(s => s.trim().toLowerCase())
        .filter((p): p is Provider => PROVIDER_VALUES.includes(p as Provider))
      sort.value      = SORT_VALUES.includes(qry.sort as Sort) ? (qry.sort as Sort) : 'popularity'
    }

    function writeUrl() {
      const next: Record<string, string | undefined> = { ...route.query, page: undefined }
      next.q          = q.value || undefined
      next.genre      = genres.value.length ? genres.value.join(',') : undefined
      next.kind       = kind.value || undefined
      next.status     = status.value || undefined
      next.year_from  = yearFrom.value ? String(yearFrom.value) : undefined
      next.year_to    = yearTo.value ? String(yearTo.value) : undefined
      next.providers  = providers.value.length ? providers.value.join(',') : undefined
      next.sort       = sort.value !== 'popularity' ? sort.value : undefined
      router.replace({ query: next })
    }

    // Computed API params — feeds animeApi.getAnimeList. Mirrors the
    // backend whitelist exactly; sidebar values that fall outside the
    // whitelist are dropped at readUrl(), so we don't re-filter here.
    const apiParams = computed(() => {
      const p: Record<string, string | number> = { sort: sort.value }
      if (q.value)              p.q          = q.value
      if (genres.value.length)  p.genre      = genres.value.join(',')
      if (kind.value)           p.kind       = kind.value
      if (status.value)         p.status     = status.value
      if (yearFrom.value)       p.year_from  = yearFrom.value
      if (yearTo.value)         p.year_to    = yearTo.value
      if (providers.value.length) p.providers = providers.value.join(',')
      return p
    })

    // Active filter count for the mobile toggle badge. The search query
    // and sort axis are intentionally EXCLUDED from the count — they are
    // not "narrowing filters" in the UX sense (sort never narrows; the
    // search input has its own input affordance).
    const activeCount = computed(() => {
      let n = 0
      if (genres.value.length)   n++
      if (kind.value)            n++
      if (status.value)          n++
      if (yearFrom.value || yearTo.value) n++
      if (providers.value.length) n++
      return n
    })

    function reset() {
      q.value = ''
      genres.value = []
      kind.value = ''
      status.value = ''
      yearFrom.value = null
      yearTo.value = null
      providers.value = []
      sort.value = 'popularity'
      writeUrl()
    }

    onMounted(readUrl)

    // Browser back/forward — re-read URL when ?route.query changes outside our writeUrl.
    watch(() => route.query, readUrl, { deep: true })

    return {
      q, genres, kind, status, yearFrom, yearTo, providers, sort,
      apiParams, activeCount,
      writeUrl, reset,
    }
  }
  ```
- [ ] The composable does NOT call the network. It owns state + URL; the
  consumer (`Browse.vue`) decides when to `loadAnime()` (typically after a
  `writeUrl` triggered by a sidebar change). This keeps the composable
  test-free of axios mocks and keeps the existing `useAnime` composable
  unchanged.

#### W2.2 Component: `FilterSection.vue` (`frontend/web/src/components/browse/FilterSection.vue`)

- [ ] Create the file. Thin wrapper around native `<details>` so each
  section is keyboard-collapsible without ARIA work (browser handles it).
  Pattern:
  ```vue
  <template>
    <details
      :open="open"
      class="border-b border-white/10 py-3 group"
      @toggle="onToggle"
    >
      <summary
        class="flex items-center justify-between text-sm font-medium text-white/80 cursor-pointer select-none list-none px-1"
      >
        <span class="flex items-center gap-2">
          <slot name="label">{{ label }}</slot>
          <span
            v-if="count"
            class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-cyan-500/20 text-cyan-300 text-[10px] font-semibold"
          >{{ count }}</span>
        </span>
        <svg
          class="w-4 h-4 text-white/40 transition-transform duration-150 group-open:rotate-180"
          fill="none" stroke="currentColor" viewBox="0 0 24 24"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </summary>
      <div class="pt-3 px-1 space-y-2">
        <slot />
      </div>
    </details>
  </template>

  <script setup lang="ts">
  interface Props {
    label?: string
    open?: boolean
    count?: number
  }
  withDefaults(defineProps<Props>(), { open: true, count: 0 })
  const emit = defineEmits<{ (e: 'toggle', open: boolean): void }>()
  function onToggle(ev: Event) {
    emit('toggle', (ev.target as HTMLDetailsElement).open)
  }
  </script>
  ```
- [ ] The `count` prop renders a small cyan pill next to the section
  label when any value is selected in that axis. The parent
  (`BrowseSidebar.vue`) computes the count per section.

#### W2.3 Component: `BrowseSidebar.vue` (`frontend/web/src/components/browse/BrowseSidebar.vue`)

- [ ] Create the file. Renders the 7 sections in order: Search → Genres
  → Format → Status → Year → Provider → Sort → Reset. Uses
  `useBrowseFilters` directly (no props for filter state — single source
  of truth via the composable, same instance is read by `Browse.vue` for
  `loadAnime()`). One prop: the genre list (fetched at the Browse view
  level so it isn't duplicated). Pattern (skeleton — the implementer
  fleshes out each section with sensible Tailwind):
  ```vue
  <template>
    <aside class="bg-slate-900/40 border border-white/10 rounded-xl p-4 space-y-1">
      <header class="flex items-center justify-between pb-2">
        <h2 class="text-lg font-semibold text-white">{{ $t('browse.filters.title') }}</h2>
      </header>

      <!-- Genres — uses the same browseGenres prop passed from Browse.vue -->
      <FilterSection :label="$t('browse.filters.section.genres')" :count="filters.genres.value.length">
        <div class="max-h-48 overflow-y-auto pr-1 space-y-1">
          <label
            v-for="g in genres"
            :key="g.id"
            class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
          >
            <input
              type="checkbox"
              :value="g.id"
              :checked="filters.genres.value.includes(g.id)"
              class="rounded border-white/20 bg-transparent text-cyan-500 focus:ring-cyan-500"
              @change="onGenreToggle(g.id, ($event.target as HTMLInputElement).checked)"
            />
            <span>{{ localizedGenre(g) }}</span>
          </label>
        </div>
      </FilterSection>

      <!-- Format (kind) — radio set -->
      <FilterSection :label="$t('browse.filters.section.format')" :count="filters.kind.value ? 1 : 0">
        <label
          v-for="opt in kindOptions"
          :key="opt.value"
          class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
        >
          <input
            type="radio"
            name="kind-filter"
            :value="opt.value"
            :checked="filters.kind.value === opt.value"
            class="border-white/20 bg-transparent text-cyan-500 focus:ring-cyan-500"
            @change="filters.kind.value = opt.value; filters.writeUrl()"
          />
          <span>{{ opt.label }}</span>
        </label>
      </FilterSection>

      <!-- Status — radio set -->
      <FilterSection :label="$t('browse.filters.section.status')" :count="filters.status.value ? 1 : 0">
        <label
          v-for="opt in statusOptions"
          :key="opt.value"
          class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
        >
          <input
            type="radio"
            name="status-filter"
            :value="opt.value"
            :checked="filters.status.value === opt.value"
            class="border-white/20 bg-transparent text-cyan-500 focus:ring-cyan-500"
            @change="filters.status.value = opt.value; filters.writeUrl()"
          />
          <span>{{ opt.label }}</span>
        </label>
      </FilterSection>

      <!-- Year range -->
      <FilterSection :label="$t('browse.filters.section.year')" :count="(filters.yearFrom.value || filters.yearTo.value) ? 1 : 0">
        <div class="flex items-center gap-2">
          <input
            type="number"
            :min="MIN_YEAR" :max="MAX_YEAR"
            :value="filters.yearFrom.value ?? ''"
            :placeholder="$t('browse.filters.year.from')"
            class="w-1/2 px-2 py-1 text-sm bg-white/5 border border-white/10 rounded-md text-white"
            @change="onYearChange('from', ($event.target as HTMLInputElement).valueAsNumber)"
          />
          <span class="text-white/40">—</span>
          <input
            type="number"
            :min="MIN_YEAR" :max="MAX_YEAR"
            :value="filters.yearTo.value ?? ''"
            :placeholder="$t('browse.filters.year.to')"
            class="w-1/2 px-2 py-1 text-sm bg-white/5 border border-white/10 rounded-md text-white"
            @change="onYearChange('to', ($event.target as HTMLInputElement).valueAsNumber)"
          />
        </div>
      </FilterSection>

      <!-- Provider — checkbox list with per-provider accent colors -->
      <FilterSection :label="$t('browse.filters.section.provider')" :count="filters.providers.value.length">
        <label
          v-for="opt in providerOptions"
          :key="opt.value"
          class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
        >
          <input
            type="checkbox"
            :value="opt.value"
            :checked="filters.providers.value.includes(opt.value)"
            :class="['rounded border-white/20 bg-transparent focus:ring-2', opt.accent]"
            @change="onProviderToggle(opt.value, ($event.target as HTMLInputElement).checked)"
          />
          <span>{{ opt.label }}</span>
        </label>
      </FilterSection>

      <!-- Sort — Phase 11 5-axis dropdown reused as a radio set for sidebar density -->
      <FilterSection :label="$t('browse.filters.section.sort')" :count="filters.sort.value !== 'popularity' ? 1 : 0">
        <label
          v-for="opt in sortOptions"
          :key="opt.value"
          class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
        >
          <input
            type="radio"
            name="sort-filter"
            :value="opt.value"
            :checked="filters.sort.value === opt.value"
            class="border-white/20 bg-transparent text-cyan-500 focus:ring-cyan-500"
            @change="filters.sort.value = opt.value; filters.writeUrl()"
          />
          <span>{{ opt.label }}</span>
        </label>
      </FilterSection>

      <!-- Reset -->
      <div class="pt-3">
        <button
          type="button"
          class="w-full px-3 py-2 rounded-md bg-pink-500/10 border border-pink-400/20 text-pink-300 hover:text-pink-200 hover:bg-pink-500/20 text-sm font-medium transition-colors"
          @click="filters.reset()"
        >
          {{ $t('browse.filters.reset') }}
        </button>
      </div>
    </aside>
  </template>

  <script setup lang="ts">
  import { computed } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useBrowseFilters, type Provider } from '@/composables/useBrowseFilters'
  import FilterSection from './FilterSection.vue'
  import { getLocalizedGenre } from '@/utils/title'

  interface Genre { id: string; name: string; name_ru?: string }
  defineProps<{ genres: Genre[]; filters: ReturnType<typeof useBrowseFilters> }>()
  const { t, locale } = useI18n()

  const MIN_YEAR = 1960
  const MAX_YEAR = new Date().getFullYear() + 1

  const kindOptions = computed(() => [
    { value: '',        label: t('browse.filters.format.any') },
    { value: 'tv',      label: t('browse.filters.format.tv') },
    { value: 'movie',   label: t('browse.filters.format.movie') },
    { value: 'ova',     label: t('browse.filters.format.ova') },
    { value: 'ona',     label: t('browse.filters.format.ona') },
    { value: 'special', label: t('browse.filters.format.special') },
  ])

  const statusOptions = computed(() => [
    { value: '',          label: t('browse.filters.status.any') },
    { value: 'released',  label: t('browse.filters.status.released') },
    { value: 'ongoing',   label: t('browse.filters.status.ongoing') },
    { value: 'announced', label: t('browse.filters.status.anons') },
  ])

  // Per-provider Tailwind accent classes (locked in CONTEXT.md "specifics").
  const providerOptions = computed<{ value: Provider; label: string; accent: string }[]>(() => [
    { value: 'kodik',    label: t('browse.filters.provider.kodik'),    accent: 'text-cyan-500 focus:ring-cyan-500' },
    { value: 'animelib', label: t('browse.filters.provider.animelib'), accent: 'text-orange-500 focus:ring-orange-500' },
    { value: 'hianime',  label: t('browse.filters.provider.hianime'),  accent: 'text-purple-500 focus:ring-purple-500' },
    { value: 'consumet', label: t('browse.filters.provider.consumet'), accent: 'text-emerald-500 focus:ring-emerald-500' },
  ])

  const sortOptions = computed(() => [
    { value: 'popularity', label: t('browse.sort.popularity') },
    { value: 'rating',     label: t('browse.sort.rating') },
    { value: 'year',       label: t('browse.sort.year') },
    { value: 'updated',    label: t('browse.sort.updated') },
    { value: 'title',      label: t('browse.sort.title') },
  ])

  // Sidebar handlers — each mutates the composable + writes URL state.
  function localizedGenre(g: Genre) { return getLocalizedGenre(g.name, g.name_ru, locale.value) }

  // Re-typed for the implementer; props.filters carries the reactive refs.
  const props = defineProps<{ genres: Genre[]; filters: ReturnType<typeof useBrowseFilters> }>()

  function onGenreToggle(id: string, checked: boolean) {
    const set = new Set(props.filters.genres.value)
    if (checked) set.add(id); else set.delete(id)
    props.filters.genres.value = [...set]
    props.filters.writeUrl()
  }
  function onProviderToggle(p: Provider, checked: boolean) {
    const set = new Set(props.filters.providers.value)
    if (checked) set.add(p); else set.delete(p)
    props.filters.providers.value = [...set]
    props.filters.writeUrl()
  }
  function onYearChange(which: 'from' | 'to', n: number) {
    const v = Number.isFinite(n) && n >= MIN_YEAR && n <= MAX_YEAR ? n : null
    if (which === 'from') {
      props.filters.yearFrom.value = v
      // Client-side validation: from <= to (locked in CONTEXT.md specifics).
      if (v && props.filters.yearTo.value && v > props.filters.yearTo.value) {
        props.filters.yearTo.value = v
      }
    } else {
      props.filters.yearTo.value = v
      if (v && props.filters.yearFrom.value && v < props.filters.yearFrom.value) {
        props.filters.yearFrom.value = v
      }
    }
    props.filters.writeUrl()
  }
  </script>
  ```
  Note: the duplicate `defineProps` shown above is a documentation
  artifact — Vue's `<script setup>` allows only ONE `defineProps`.
  Implementer should consolidate the two reads into one `const props =
  defineProps<…>()` call at the top.
- [ ] All accent classes (`text-cyan-500 / text-orange-500 /
  text-purple-500 / text-emerald-500`) are locked in CONTEXT.md
  "specifics".

### Wave 3 — Browse.vue rebuild (desktop grid + mobile drawer)

#### W3.1 Replace filter UI with sidebar + mobile drawer (`frontend/web/src/views/Browse.vue`)

- [ ] Replace the entire `<div class="flex flex-wrap gap-3">` filter block
  (lines 20-73) with a two-axis layout:
  - Desktop (md+): `grid grid-cols-[280px_1fr] gap-6` — sidebar on the left,
    `AnimeCard` grid on the right.
  - Mobile (<md): single column; a toggle button at the top of the page
    opens a slide-in drawer containing the sidebar.
- [ ] New template skeleton replacing the old filter block:
  ```vue
  <!-- Mobile filter toggle (visible below md) -->
  <div class="md:hidden mb-4">
    <button
      ref="toggleButtonRef"
      type="button"
      class="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-white/5 border border-white/10 text-white/80 hover:text-white"
      :aria-expanded="drawerOpen"
      aria-controls="browse-filter-drawer"
      @click="drawerOpen = true"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2a1 1 0 01-.293.707L15 12.414V19a1 1 0 01-1.447.894l-4-2A1 1 0 019 17v-4.586L3.293 6.707A1 1 0 013 6V4z" />
      </svg>
      {{ $t('browse.filters.mobileToggle') }}
      <span
        v-if="filters.activeCount.value"
        class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-cyan-500/20 text-cyan-300 text-[10px] font-semibold"
        :aria-label="$t('browse.filters.activeCount', { count: filters.activeCount.value })"
      >{{ filters.activeCount.value }}</span>
    </button>
  </div>

  <div class="grid grid-cols-1 md:grid-cols-[280px_1fr] gap-6">
    <!-- Desktop sidebar -->
    <div class="hidden md:block">
      <BrowseSidebar :genres="browseGenres" :filters="filters" />
    </div>

    <!-- Results column (existing template — search input + results grid + pagination) -->
    <div>
      <!-- existing SearchAutocomplete moves here -->
      <!-- existing loading / error / empty / results / pagination blocks unchanged -->
    </div>
  </div>

  <!-- Mobile drawer (hidden on md+) -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="-translate-x-full opacity-0"
      enter-to-class="translate-x-0 opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="translate-x-0 opacity-100"
      leave-to-class="-translate-x-full opacity-0"
    >
      <div
        v-if="drawerOpen"
        id="browse-filter-drawer"
        ref="drawerRef"
        role="dialog"
        aria-modal="true"
        :aria-label="$t('browse.filters.title')"
        class="fixed inset-0 z-50 md:hidden flex"
      >
        <!-- Backdrop -->
        <div class="absolute inset-0 bg-black/60" @click="drawerOpen = false" />
        <!-- Panel -->
        <div class="relative w-[85%] max-w-[320px] h-full bg-slate-950 border-r border-white/10 overflow-y-auto p-4">
          <div class="flex items-center justify-between mb-3">
            <h2 class="text-lg font-semibold text-white">{{ $t('browse.filters.title') }}</h2>
            <button
              type="button"
              class="p-1 rounded text-white/60 hover:text-white"
              :aria-label="$t('common.close')"
              @click="drawerOpen = false"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <BrowseSidebar :genres="browseGenres" :filters="filters" />
        </div>
      </div>
    </Transition>
  </Teleport>
  ```
- [ ] Move the existing `<SearchAutocomplete>` block (lines 11-17) into
  the results column at the top so it stays accessible above the grid on
  both layouts.
- [ ] In `<script setup>`:
  - Import: `import { useBrowseFilters } from '@/composables/useBrowseFilters'`
  - Import: `import BrowseSidebar from '@/components/browse/BrowseSidebar.vue'`
  - Import: `import { useFocusTrap } from '@/composables/useFocusTrap'`
  - Replace the existing `searchQuery / selectedGenre / selectedYear /
    selectedStatus / sortBy / hasActiveFilters / sortOptions /
    statusOptions / yearOptions / clearFilters / handleFilter`
    declarations (lines 241-292 + 327-338) with:
    ```ts
    const filters = useBrowseFilters()
    const drawerOpen = ref(false)
    const drawerRef = ref<HTMLElement | null>(null)
    const toggleButtonRef = ref<HTMLButtonElement | null>(null)

    useFocusTrap({
      active: drawerOpen,
      container: drawerRef,
      returnFocusTo: toggleButtonRef,
    })

    // ESC closes the drawer (focus trap handles Tab cycling; ESC is
    // separate per Phase 6 navbar drawer pattern).
    function onDrawerKey(e: KeyboardEvent) {
      if (e.key === 'Escape' && drawerOpen.value) {
        drawerOpen.value = false
      }
    }
    onMounted(() => document.addEventListener('keydown', onDrawerKey))
    onBeforeUnmount(() => document.removeEventListener('keydown', onDrawerKey))

    // Re-fetch results whenever any active filter axis changes. Mirrors the
    // previous handleFilter() behaviour, but driven by the composable.
    watch(
      () => filters.apiParams.value,
      async () => {
        currentPage.value = 1
        await loadAnime()
      },
      { deep: true },
    )
    ```
  - Update `loadAnime()`:
    ```ts
    const loadAnime = async () => {
      const params: Record<string, string | number | boolean> = {
        page: currentPage.value,
        ...filters.apiParams.value,
      }
      await fetchAnimeList(params)
    }
    ```
  - Remove the old per-axis route-query watchers (lines 419-451) — the
    composable's `watch(() => route.query, readUrl)` replaces them.
  - Keep the existing `currentPage` watcher (line 453) as is.
  - Update `clearRecentSearches`, `handleSearch`, `goToPage` to read /
    write through `filters` instead of the removed local refs:
    - `handleSearch` updates `filters.q.value` then calls
      `filters.writeUrl()`.
- [ ] `searchQuery` ref stays (used by `SearchAutocomplete v-model`). Keep
  it in sync with `filters.q`:
  ```ts
  watch(() => filters.q.value, (v) => { if (searchQuery.value !== v) searchQuery.value = v }, { immediate: true })
  watch(searchQuery, (v) => { if (filters.q.value !== v) filters.q.value = v })
  ```
- [ ] `onBeforeUnmount` import: add to the existing `vue` import line
  alongside `computed, onMounted, ref, watch`.

### Wave 4 — i18n (en / ru / ja)

#### W4.1 Locale entries (`frontend/web/src/locales/{en,ru,ja}.json`)

- [ ] Add a new `browse.filters` nested namespace to each locale (alongside
  the existing `browse.sort.*` block from Phase 11). 26 keys × 3 locales =
  78 entries.
- [ ] **EN copy** (`frontend/web/src/locales/en.json`, inside the `browse`
  object):
  ```json
  "filters": {
    "title": "Filters",
    "reset": "Reset all filters",
    "mobileToggle": "Filters",
    "activeCount": "{count} active",
    "section": {
      "genres":   "Genres",
      "format":   "Format",
      "status":   "Status",
      "year":     "Year",
      "provider": "Available on",
      "sort":     "Sort"
    },
    "format": {
      "any":     "Any format",
      "tv":      "TV series",
      "movie":   "Movie",
      "ova":     "OVA",
      "ona":     "ONA",
      "special": "Special"
    },
    "status": {
      "any":      "Any status",
      "released": "Released",
      "ongoing":  "Ongoing",
      "anons":    "Announced"
    },
    "year": {
      "from": "From",
      "to":   "To"
    },
    "provider": {
      "kodik":    "Kodik (RU)",
      "animelib": "AnimeLib (RU)",
      "hianime":  "HiAnime (EN)",
      "consumet": "Consumet (EN)"
    }
  }
  ```
- [ ] **RU copy** (`frontend/web/src/locales/ru.json`):
  ```json
  "filters": {
    "title": "Фильтры",
    "reset": "Сбросить все фильтры",
    "mobileToggle": "Фильтры",
    "activeCount": "{count} активных",
    "section": {
      "genres":   "Жанры",
      "format":   "Формат",
      "status":   "Статус",
      "year":     "Год",
      "provider": "Доступно на",
      "sort":     "Сортировка"
    },
    "format": {
      "any":     "Любой формат",
      "tv":      "ТВ-сериал",
      "movie":   "Фильм",
      "ova":     "OVA",
      "ona":     "ONA",
      "special": "Спешл"
    },
    "status": {
      "any":      "Любой статус",
      "released": "Вышло",
      "ongoing":  "Онгоинг",
      "anons":    "Анонс"
    },
    "year": {
      "from": "С",
      "to":   "По"
    },
    "provider": {
      "kodik":    "Kodik (RU)",
      "animelib": "AnimeLib (RU)",
      "hianime":  "HiAnime (EN)",
      "consumet": "Consumet (EN)"
    }
  }
  ```
- [ ] **JA copy** (`frontend/web/src/locales/ja.json`):
  ```json
  "filters": {
    "title": "フィルター",
    "reset": "すべてのフィルターをリセット",
    "mobileToggle": "フィルター",
    "activeCount": "{count} 件",
    "section": {
      "genres":   "ジャンル",
      "format":   "種別",
      "status":   "状態",
      "year":     "年",
      "provider": "配信元",
      "sort":     "並び替え"
    },
    "format": {
      "any":     "すべての種別",
      "tv":      "TVシリーズ",
      "movie":   "映画",
      "ova":     "OVA",
      "ona":     "ONA",
      "special": "スペシャル"
    },
    "status": {
      "any":      "すべての状態",
      "released": "完結",
      "ongoing":  "放送中",
      "anons":    "予定"
    },
    "year": {
      "from": "から",
      "to":   "まで"
    },
    "provider": {
      "kodik":    "Kodik (RU)",
      "animelib": "AnimeLib (RU)",
      "hianime":  "HiAnime (EN)",
      "consumet": "Consumet (EN)"
    }
  }
  ```
- [ ] Adjust trailing commas in each locale file so the JSON stays valid.

### Wave 5 — Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — clean.
- [ ] `cd frontend/web && bunx eslint src/composables/useBrowseFilters.ts src/components/browse/FilterSection.vue src/components/browse/BrowseSidebar.vue src/views/Browse.vue` — zero errors / zero warnings.
- [ ] JSON validity for all three locale files:
  ```bash
  cd frontend/web && bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); console.log('ok')"
  ```
- [ ] `cd services/catalog && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-catalog && make redeploy-web` — both succeed;
  `make health` reports both services healthy.
- [ ] Confirm the 4 new columns landed on the `animes` table (see W1.4 psql
  command). All four should have a `default: false` and an index.
- [ ] **Manual smoke — format filter.** Visit `/browse`. Open the Format
  section in the sidebar. Pick "Movie" — `?kind=movie` appears in URL;
  cards refresh to movies only. Pick "Any format" — `?kind=` drops, all
  rows return. Reload mid-filter — selection persists.
- [ ] **Manual smoke — provider filter.** Pick "Kodik (RU)" alone — only
  rows with `has_kodik=true` show (initially few until the lazy backfill
  populates more rows). Tick "HiAnime (EN)" — OR-set, more rows appear.
  Untick "Kodik" — only HiAnime rows.
- [ ] **Manual smoke — year range.** Type 2020 in From, 2024 in To — URL
  shows `?year_from=2020&year_to=2024`; cards filter accordingly. Try
  inverted range (from > to) — UI clamps so from <= to (client-side
  validation per W2.3).
- [ ] **Manual smoke — combined.** Genre = "Romance" + Status =
  "Released" + Provider = "Kodik" + Sort = "rating" — URL has all four
  params; cards reflect all four constraints; reset button clears all
  and URL becomes clean `/browse`.
- [ ] **Manual smoke — mobile drawer.** Switch viewport to 375 × 812
  (DevTools mobile). The toggle button shows at the top with no badge.
  Apply a filter (genre = romance) — drawer closes via outside click —
  toggle button shows badge "1". Re-open drawer. Press Tab repeatedly —
  focus cycles within the drawer (useFocusTrap). Press ESC — drawer
  closes; focus returns to the toggle button.
- [ ] **Manual smoke — back/forward.** Apply a kind=tv + provider=kodik
  filter. Click an anime card → land on `/anime/:id`. Browser back → URL
  restores with both params; sidebar reflects them; cards refresh
  identically.
- [ ] **axe-core re-run** on /browse desktop + mobile via Chrome MCP:
  zero new violations. Particular checks:
  - `<details>` / `<summary>` accessible name from the section label text.
  - Mobile drawer has `role="dialog"`, `aria-modal="true"`,
    `aria-label` bound to the localised "Filters" string.
  - Toggle button has `aria-expanded` and `aria-controls` tying it to
    the drawer's id.
  - Provider checkboxes pass contrast even with the per-provider accent
    classes (cyan / orange / purple / emerald) against the dark sidebar.

## Files touched

```
services/catalog/internal/domain/anime.go                            (+ 4 HasX bools, + SearchFilters.Kind + SearchFilters.Providers)
services/catalog/internal/repo/anime.go                              (+ Search() kind + providers WHERE, + 4 SetHasX helpers)
services/catalog/internal/handler/catalog.go                         (+ parseFilters reads kind + providers query params)
services/catalog/internal/service/catalog.go                         (+ 4 lazy-backfill SetHasX writes wired into each parser's catalog entry point)
services/catalog/internal/parser/kodik/client.go                     (touch only if a helper is added — usually not needed; service derives has_kodik from "successful kodik translations call")
services/catalog/internal/parser/animelib/client.go                  (touch only if a helper is added — usually not needed)
services/catalog/internal/parser/hianime/client.go                   (touch only if a helper is added — usually not needed)
services/catalog/internal/parser/consumet/client.go                  (touch only if a helper is added — usually not needed)
frontend/web/src/composables/useBrowseFilters.ts                     (NEW — reactive filter state + URL sync + apiParams + activeCount + reset)
frontend/web/src/components/browse/FilterSection.vue                 (NEW — collapsible <details> section wrapper)
frontend/web/src/components/browse/BrowseSidebar.vue                 (NEW — 7-section sidebar + reset button, consumes useBrowseFilters)
frontend/web/src/views/Browse.vue                                    (+ desktop grid 280/1fr, + mobile drawer + focus trap + ESC, + composable wiring, − old filter UI)
frontend/web/src/locales/en.json                                     (+ browse.filters.* namespace — 26 keys EN)
frontend/web/src/locales/ru.json                                     (+ browse.filters.* namespace — 26 keys RU)
frontend/web/src/locales/ja.json                                     (+ browse.filters.* namespace — 26 keys JA)
.planning/workstreams/ui-ux-audit/phases/15-multi-axis-filter-sidebar/
  15-CONTEXT.md                                                      (already exists)
  15-PLAN.md                                                         (this file)
  15-SUMMARY.md                                                      (written at execute-phase end)
  15-VERIFICATION.md                                                 (written at execute-phase end)
```

No new database tables — 4 new columns added via GORM AutoMigrate on
catalog startup. No new external libraries. No gateway changes — the
gateway already proxies `/api/anime/*` to catalog and the new query
params pass through unchanged.

## Closes

| Requirement | Surface | Mechanism |
|---|---|---|
| UX-31 | /browse | Backend: 4 `has_{provider}` boolean columns on `animes` + `kind` + `providers` query params on `/api/anime`; each of the 4 parsers (kodik / animelib / hianime / consumet) lazily backfills its column. Frontend: new `useBrowseFilters` composable owns URL state, `BrowseSidebar.vue` renders 7 collapsible sections (search / genres / format / status / year / provider / sort) + reset button, desktop `grid-cols-[280px_1fr]` layout, mobile drawer with `role=dialog` / `aria-modal` / focus trap (Phase 6 `useFocusTrap`) / ESC close / outside-click close. Active filter count badge on the mobile toggle. URL contract: `?genre=A,B&kind=tv&status=released&year_from=2020&year_to=2024&providers=kodik,hianime&sort=rating`. |

## Wave outline

| Wave | Tasks | Rationale |
|---|---|---|
| 1 (Backend) | W1.1 domain + repo (4 columns + SetHasX + Search() filter) → W1.2 handler parseFilters (kind + providers) → W1.3 service lazy-backfill in 4 parser entry points → W1.4 catalog go test + redeploy + curl smoke + psql column check | One coherent backend slice. AutoMigrate adds the 4 columns on first startup; the SetHasX writes only fire on lazy backfill so existing-row impact is zero. |
| 2 (Composable + components) | W2.1 useBrowseFilters → W2.2 FilterSection → W2.3 BrowseSidebar | Pure frontend, no Browse.vue churn yet — keeps the components testable in isolation and avoids merge churn when Wave 3 rewires Browse.vue. |
| 3 (Browse.vue rebuild) | W3.1 replace filter UI with grid + drawer + composable wiring + focus trap + ESC + watch(apiParams) | Single-file rewrite consuming Wave 1's API and Wave 2's components. Heavier touch but isolated to Browse.vue. |
| 4 (i18n) | W4.1 26 keys × 3 locales | Single sweep across the three locale files. Ships after W3 so the implementer can visually verify each label in dev before committing. |
| 5 (Verification) | vue-tsc + eslint + JSON validity + go test + redeploys + make health + manual smoke per filter axis + mobile drawer a11y walkthrough + axe-core re-run | Single gate covering all five filter axes + the mobile drawer + i18n + a11y before commit. |
