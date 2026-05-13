# Phase 15: Multi-axis catalog filter sidebar (Dragon) - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, Dragon-level feature — competitive moat)

<domain>
## Phase Boundary

Rebuild `/browse` filter UI as a persistent sidebar exposing 6 filter axes + sort:
- **Genre** (existing) — multi-select.
- **Format** (`kind` column) — TV / Movie / OVA / ONA / Special. Single-select.
- **Status** — Released / Ongoing / Anons. Single-select. (Existing field.)
- **Year** — single year OR range. Use `year_from` / `year_to` (existing backend support).
- **Provider/audio-source (UNIQUELY ANIMEENIGMA)** — Kodik / AnimeLib / HiAnime / Consumet. Multi-select. Closes Tier E #3.
- **Sort** — popularity / rating / year / recently-updated / A-Z (from Phase 11).

URL-state-persisted (`?genre=a,b&kind=tv&status=released&year_from=2020&year_to=2024&provider=kodik,hianime&sort=rating`).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**Backend:**
- Extend `SearchFilters` struct: add `Kind string` (single value), `Providers []string` (multi-value).
- Extend `Anime` model: add `HasKodik`, `HasAnimeLib`, `HasHiAnime`, `HasConsumet` boolean columns (default false, indexed). Mirrors Phase 9 `HasDub` pattern.
- Each parser sets its corresponding boolean when ingesting an anime:
  - Kodik parser → `HasKodik = true`
  - AnimeLib parser → `HasAnimeLib = true`
  - HiAnime parser → `HasHiAnime = true`
  - Consumet parser → `HasConsumet = true`
- Lazy backfill: existing rows start with all false; over time as search touches each anime, parsers populate the columns. No eager backfill in this phase.
- Search query filter: when `Providers=[kodik, hianime]`, add `WHERE (has_kodik=true OR has_hianime=true)` (OR semantics — title is available via ANY of the selected providers).
- Kind filter: `WHERE kind = ?`.

**Frontend:**
- Replace existing `/browse` filter UI (currently a horizontal bar of dropdowns + GenreFilterPopup) with a vertical sidebar.
- Desktop layout: `grid grid-cols-[280px_1fr]` with sidebar on the left + card grid on the right. `max-w-7xl` wrapper unchanged.
- Mobile layout: collapsible drawer. Toggle button at top of Browse view: "Filters" with count badge of active filters. Drawer slides from left.
- Sidebar sections (collapsible via native `<details>`):
  1. Search query (existing input)
  2. Genres (multi-select; reuses GenreFilterPopup logic but flattened into the sidebar)
  3. Format (radio buttons: TV / Movie / OVA / ONA / Special / Any)
  4. Status (radio buttons: Released / Ongoing / Anons / Any)
  5. Year (two inputs: From / To, with year-picker constraints 1960..currentYear+1)
  6. Provider (checkbox list: Kodik / AnimeLib / HiAnime / Consumet)
  7. Sort (existing dropdown from Phase 11)
  8. Reset button at bottom
- URL state syncs both ways: changing a filter calls `router.replace({ query: ... })`; reading a filter from `route.query` initializes state on mount.

**Component structure:**
- Extract: `frontend/web/src/components/browse/BrowseSidebar.vue` (the sidebar itself).
- Extract: `frontend/web/src/components/browse/FilterSection.vue` (collapsible section wrapper for each filter group).
- Extract: `frontend/web/src/composables/useBrowseFilters.ts` (URL-state + reactive filter object that emits API params).

**i18n keys:**
- `browse.filters.title` (sidebar header), `browse.filters.reset`
- `browse.filters.section.genres` / `format` / `status` / `year` / `provider` / `sort`
- `browse.filters.format.tv` / `.movie` / `.ova` / `.ona` / `.special` / `.any`
- `browse.filters.status.released` / `.ongoing` / `.anons` / `.any`
- `browse.filters.provider.kodik` / `.animelib` / `.hianime` / `.consumet`
- `browse.filters.year.from` / `.to`
- `browse.filters.activeCount` (with {count} placeholder)
- `browse.filters.mobileToggle`
- Total: ~26 keys × 3 locales = ~78 entries.

### Locked from ROADMAP

- Phase 15 depends on Phase 11 (sort dropdown + URL-state pattern). Both complete.
- Phase 16 (schedule view) and Phase 17 (collections) don't depend on this — independent.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/catalog/internal/domain/anime.go` — `Anime` struct + `SearchFilters`. Phase 9 added `HasDub bool` pattern.
- `services/catalog/internal/repo/anime.go` — `Search()` with Where chain. Add `Kind` and `Providers` branches.
- `services/catalog/internal/handler/anime.go` — handler parses query params; add `kind`, `providers` parsing.
- `services/catalog/internal/parser/kodik/client.go` — sets `HasDub`. Add `HasKodik = true` on ingest.
- `services/catalog/internal/parser/animelib/client.go` — set `HasAnimeLib = true`.
- `services/catalog/internal/parser/hianime/client.go` — set `HasHiAnime = true`.
- `services/catalog/internal/parser/consumet/client.go` — set `HasConsumet = true`.
- `frontend/web/src/views/Browse.vue` — current filter UI; replace with `<BrowseSidebar />`.
- `frontend/web/src/components/anime/GenreFilterPopup.vue` (or similar) — genre selection logic to lift into the sidebar.

### Established Patterns

- Phase 9's `HasDub` boolean column + parser update is the exact mirror pattern for the 4 new provider booleans.
- Phase 11's `useBrowseFilters` (or equivalent) URL state pattern: read `route.query` on mount; emit via `router.replace`.

### Integration Points

- No new tables. 4 new columns on `animes`.
- GORM auto-migrates the new columns on catalog service startup.
- Gateway routing unchanged (`/api/anime` already proxies to catalog).
- All 4 parsers update on next anime ingest (lazy backfill).

</code_context>

<specifics>
## Specific Ideas

- The provider filter is the headline feature of this phase. Display it prominently in the sidebar (third section, above year). Use the existing brand colors per provider (Kodik cyan / AnimeLib orange / HiAnime purple / Consumet green) for the checkbox accents.
- Mobile drawer follows the Phase 6 navbar drawer a11y pattern: `role="dialog"`, `aria-modal`, focus trap, ESC to close.
- Year range input: two number inputs side-by-side. Default range `1960` to `currentYear + 1`. Validate `from <= to` client-side.
- Active filter count badge: count any non-empty filter (genres > 0, kind != null, providers > 0, year_from || year_to, status != null).

</specifics>

<deferred>
## Deferred Ideas

- Eager backfill of provider booleans on existing anime rows — Phase 20 polish or future scheduler job.
- Save filter sets as named presets — Phase 17 territory (collections feature builds the same primitive).
- Filter analytics (which filter combinations are popular) — future ops work.
- Reverse filtering (exclude genres) — defer; current pattern only includes.

</deferred>
