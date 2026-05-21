# Phase 2: Frontend HeroSpotlightBlock + Carousel - Context

**Gathered:** 2026-05-21
**Status:** Ready for planning
**Mode:** Auto-generated from approved design doc + REQUIREMENTS.md (autonomous mode)

<domain>
## Phase Boundary

Build the user-visible side of the spotlight block: a Vue 3
`HeroSpotlightBlock.vue` mounted in `Home.vue` immediately after
`<SystemStatusBanner />` and ABOVE the existing 3-column Ongoing/Top/Announced
grid (and above the still-present legacy `trendingRecs` row, which this phase
leaves in place — removal is Phase 3's job per `HSB-MIG-01`).

The block:
- Fetches `GET /api/home/spotlight` once on mount via the existing API client.
- Renders the 4 Phase-1 static card types (`anime_of_day`, `random_tail`,
  `latest_news`, `platform_stats`) inside a 7s auto-cycling carousel.
- Picks a random eligible card as the initial slide on every page load.
- Pauses auto-cycle on `mouseenter` / `focusin`; resumes on
  `mouseleave` / `focusout`.
- Manual nav via chevron buttons + dot indicators; keyboard
  ArrowLeft/ArrowRight when focused.
- Respects `prefers-reduced-motion: reduce` (no auto-cycle, no slide-fade).
- A11y: `role="region"`, `aria-roledescription="carousel"`, slide-level
  `aria-roledescription="slide"`, `aria-label`, `aria-live="polite"`.
- Skeleton placeholder while fetching (no layout shift).
- Hides itself silently on fetch failure or empty cards array (`v-if`).
- Gated by env `VITE_HERO_SPOTLIGHT_ENABLED` (default `true`).

In scope: the block + carousel state machine + 4 card components +
composable + i18n + e2e + env flag.

Out of scope for this phase:
- The 5 dynamic card components (`PersonalPick`, `NowWatching`,
  `Telegram`, `NotTimeYet`, `ContinueWatchingNew`) → Phase 3.
- Removal of the legacy `trendingRecs` block from `Home.vue` (HSB-MIG-01) → Phase 3.
- Authenticated request flows for login-only cards → Phase 3.

</domain>

<decisions>
## Implementation Decisions

### Component layout (one folder)
- New folder: `frontend/web/src/components/home/spotlight/`.
- `HeroSpotlightBlock.vue` — outer wrapper, holds carousel state machine.
- `CarouselControls.vue` — prev/next chevrons + dot indicators (extracted
  for testability and so Phase 3 can reuse).
- `cards/AnimeOfDayCard.vue`
- `cards/LatestNewsCard.vue`
- `cards/PlatformStatsCard.vue`
- `cards/RandomTailCard.vue`

### State + data fetching (composable)
- `frontend/web/src/composables/useSpotlight.ts` exports `useSpotlight()`
  returning `{ cards: Ref<SpotlightCard[]>, loading: Ref<boolean>, error:
  Ref<Error | null>, refresh: () => Promise<void> }`.
- Uses the existing axios/fetch wrapper used by other home composables
  (see `useAnime.ts` for the project's standard).
- One fetch on mount; no auto-refresh in Phase 2 (Phase 3's live cards
  will re-fetch every 30s — out of scope here).

### Carousel state machine (inside HeroSpotlightBlock.vue)
- `currentIndex: Ref<number>` — initialized to `randomInt(0, cards.length - 1)`
  inside `onMounted` after data loads.
- `auto: Ref<boolean>` — true unless `prefers-reduced-motion: reduce`.
- `intervalId: ReturnType<typeof setInterval> | null` — managed by
  `startCycle()` / `stopCycle()` / `goTo(index)` / `next()` / `prev()`.
- 7000ms interval. Wraps around: `(currentIndex + 1) % cards.length`.
- `mouseenter`/`focusin` on the wrapper → `stopCycle()`; `mouseleave`/
  `focusout` → `startCycle()` (only when `auto` is true).
- Keyboard handler on wrapper: `ArrowRight` → `next()`,
  `ArrowLeft` → `prev()`. Dots are buttons → focusable and clickable
  individually.
- Unmount → clear interval to avoid leaks.

### Skeleton state
- While `loading` true: render a single skeleton block matching final
  block height (clamp ~280-360px desktop, ~360-480px mobile). Use existing
  Tailwind `animate-pulse` utility for visual.
- After `loading` is false and `cards.length === 0`: block does NOT render
  (no error banner, no toast — design §6.4 spec).
- After load + ≥1 card: render carousel.

### A11y
- Wrapper `<section>` with `role="region"`, `aria-roledescription="carousel"`,
  `aria-label="Подборка дня"` (RU) / `"Today's spotlight"` (EN) via i18n.
- Active slide container: `role="group"`, `aria-roledescription="slide"`,
  `aria-label="<N> из <M>: <card title>"` (i18n-templated).
- Slide container has `aria-live="polite"` so screen readers announce changes.
- Dot buttons: `aria-label="Слайд <N>"`, `aria-current="true"` on active.
- Chevron buttons: `aria-label="Предыдущий слайд" / "Следующий слайд"`.

### Reduced motion
- `useReducedMotion()` composable (new — minimal, ~10 lines) returning a
  reactive boolean based on `window.matchMedia('(prefers-reduced-motion:
  reduce)').matches`. Watches for changes.
- Auto-cycle does not start when reduced motion is on. Slide cross-fade
  CSS class is conditionally applied.

### Card components (4 in this phase)
Each card receives a single `data` prop typed to its specific shape from
the discriminated union (see design doc §4.1):

- **AnimeOfDayCard.vue** — Poster left + meta right (title, score, episodes,
  genres) + CTA buttons "Смотреть" / "В список". Mobile: poster top, meta
  below.
- **LatestNewsCard.vue** — 3 changelog entries in row (desktop) / vertical
  stack (mobile). Each links to `/changelog` (full LastUpdates view).
- **PlatformStatsCard.vue** — Up to 3 metric chips (only non-null metrics
  render — `anime_added_7d` is the only non-null one in Phase 1; backend
  ensures eligibility). Delta indicators if present. Layout: 3-in-row
  desktop / stack mobile.
- **RandomTailCard.vue** — Single poster + meta with header "Random
  pick — discover something new" (i18n key).

### i18n
- Namespace: `spotlight.*` in both `en.json` and `ru.json`.
- Top-level keys: `spotlight.regionLabel`, `spotlight.slideLabel`,
  `spotlight.prevSlide`, `spotlight.nextSlide`.
- Per-card sub-namespaces: `spotlight.animeOfDay.{title,watchCta,addCta}`,
  `spotlight.latestNews.{title,readMore}`,
  `spotlight.platformStats.{title,animeAdded7d}`,
  `spotlight.randomTail.{title,discover}`.

### Feature flag
- `VITE_HERO_SPOTLIGHT_ENABLED` — defaults to `true`.
- Set in `frontend/web/.env.development` AND
  `frontend/web/.env.production.example`.
- `HeroSpotlightBlock.vue` short-circuits with `v-if="enabled"` where
  `enabled = import.meta.env.VITE_HERO_SPOTLIGHT_ENABLED !== 'false'`.
- When disabled, the block does not mount; the legacy `trendingRecs` row
  stays as-is (intentional during transition).

### Home.vue integration (Phase 2 — additive only)
- Add `<HeroSpotlightBlock />` import + mount immediately after
  `<SystemStatusBanner />`, BEFORE the existing `trendingRecs` `<div>` at
  line ~39.
- Do NOT remove or alter the `trendingRecs` markup, state, or imports —
  Phase 3 owns that removal.

### Testing strategy
- **Component-level (unit/integration) tests:** Vitest + @vue/test-utils
  for each card component (render with mock data, assert i18n keys present,
  CTA buttons render) and for `HeroSpotlightBlock.vue` (mount lifecycle,
  random-index init, interval start/stop, keyboard nav, reduced-motion
  branch).
- **E2E tests:** `frontend/web/tests/e2e/spotlight.spec.ts` (Playwright).
  Verify: block renders with N cards, cycles every 7s, hover pauses
  auto-cycle, arrow keys seek, reduced-motion emulation disables auto,
  mobile viewport stacks poster cards.
- **A11y check:** axe-core via Playwright on the rendered block. Zero
  violations gate.

### Claude's Discretion
- Exact Tailwind classes for the carousel (transition timing, dot indicator
  styling, chevron button placement) — follow existing project patterns
  (CollectionsRow.vue is the closest analog for "horizontal scrolling
  content block").
- Whether to extract a tiny `randomInt(min, max)` util (probably inline as
  one-liner).
- Whether `useReducedMotion` is shared or local — start local, promote to
  `composables/` if Phase 3's card components also need it.
- Whether the dot indicator container is a `<nav>` or a flat `<ul>` — both
  satisfy a11y; pick whichever reads cleaner.
- Cross-fade vs slide-in animation choice — design doc says "cross-fade",
  so use opacity transition; no Y-translation.
- Whether to lazy-import card components via `defineAsyncComponent` — for
  4 cards probably overkill; import statically.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`frontend/web/src/views/Home.vue`** — top of file already imports
  `SystemStatusBanner`. The new block mounts immediately after.
- **`frontend/web/src/components/home/CollectionsRow.vue`** — closest
  analog for "horizontal content block on the home page". Provides Tailwind
  class patterns for spacing (`max-w-7xl mx-auto px-4 lg:px-8`).
- **`frontend/web/src/components/home/ContinueWatchingRow.vue`** — another
  analog; check for skeleton-pattern usage.
- **`frontend/web/src/composables/useAnime.ts`** — existing pattern for
  fetch composables. Use the same axios wrapper.
- **`frontend/web/src/locales/{en,ru,ja}.json`** — existing i18n setup;
  add `spotlight.*` namespace to en + ru. (`ja.json` exists but Japanese is
  out of scope for v1 hero block — leave to a later phase if/when added.)

### Established Patterns
- **Composition API + `<script setup lang="ts">`** is the project's standard
  (see CollectionsRow.vue, ContinueWatchingRow.vue).
- **Tailwind utility classes** (no SCSS modules).
- **`vue-i18n`** for i18n with `t(...)` helper.
- **Tests:** Vitest under `frontend/web/tests/unit/` (if exists) + Playwright
  under `frontend/web/tests/e2e/`. Use `bunx` (not npx) per CLAUDE.md.
- **Feature flags** via `import.meta.env.VITE_*` (precedent: existing flags
  like `VITE_OURENGLISH_ENABLED` per CLAUDE.md memory).
- **Aspect ratio constraints** — many cards in the codebase use
  `aspect-square` / `aspect-video`-style utilities; the spotlight block
  uses a fixed min-height to avoid layout shift.

### Integration Points
- `Home.vue` mount point — line ~39 (immediately before the existing
  `trendingRecs` div). The new block goes ABOVE that line.
- API endpoint `GET /api/home/spotlight` — Phase 1 delivered this; Phase 2
  consumes it.
- Env files — `.env.development` and `.env.production.example` (commit
  these; never the actual `.env.production` with secrets).
- Tailwind config — no changes expected; the new block uses existing
  utility classes only.

</code_context>

<specifics>
## Specific Ideas

- **7-second auto-cycle interval** — non-negotiable per HSB-FE-03.
- **Random initial slide on every reload** — `Math.random()` + `Math.floor`
  inside `onMounted` after `cards` populates.
- **`prefers-reduced-motion` is honored** — both auto-cycle and the slide
  transition CSS class.
- **The block hides itself** when fetch returns 5xx, when the response is
  empty, or when the feature flag is off.
- **TypeScript types** — declare `SpotlightCard` discriminated union in
  `frontend/web/src/types/spotlight.ts` (new file) matching design doc §4.1.
  Card components are typed accordingly.
- **A11y is acceptance-criteria-tier** — axe-core reports zero violations
  on the block.

</specifics>

<deferred>
## Deferred Ideas

- All 5 dynamic card components and the live-data refresh — Phase 3.
- Removing the legacy `trendingRecs` block — Phase 3 (HSB-MIG-01).
- Authenticated request path (Authorization header) — Phase 3.
- Slide-order personalization, A/B testing, "do not show this card type
  again" preference — v1.1+.

</deferred>
</content>
</invoke>