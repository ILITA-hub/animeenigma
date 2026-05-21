# Phase 2: Frontend HeroSpotlightBlock + Carousel — Research

**Researched:** 2026-05-21
**Domain:** Vue 3 + TypeScript + Tailwind v4 — accessible, auto-cycling hero carousel with discriminated-union card data, reduced-motion support, feature flag, and e2e + a11y coverage
**Confidence:** HIGH (all findings verified against in-repo source files and npm registry; no claims rely on training data)

## Summary

Phase 2 is purely additive frontend work: build `HeroSpotlightBlock.vue`, a `CarouselControls.vue`, four static card components, a `useSpotlight` data composable, two i18n payloads (en/ru), one feature-flag env var, and a Playwright e2e spec — all of which mount above the existing `trendingRecs` row in `Home.vue` without altering it. The block consumes the Phase-1-delivered `GET /api/home/spotlight` endpoint.

The project's frontend stack is fully equipped for this work — `axios` (via `@/api/client`) is the standard HTTP client, `@vueuse/core` is in dependencies and `useMediaQuery('(prefers-reduced-motion: reduce)')` is the **established** reduced-motion pattern (used in `Hero.vue:124` and `Carousel.vue:124`), Vitest 4.1.6 is configured for co-located `src/**/*.spec.ts` unit tests, and Playwright 1.58.0 is configured in `frontend/web/e2e/` with chromium + firefox + Mobile Chrome projects. No third-party carousel libraries (Embla/Swiper/Glide) exist in `package.json` — confirmed by grep. The hand-rolled state machine specified in CONTEXT.md is the right shape, but should reuse `@vueuse/core`'s `useEventListener`, `useMediaQuery`, and `useIntervalFn` primitives where they replace boilerplate (these are already in the project's idiom and have built-in auto-cleanup on unmount).

**Primary recommendation:** Build the block following exactly the structure in CONTEXT.md + UI-SPEC.md, but replace the proposed bespoke `useReducedMotion()` composable with inline `useMediaQuery('(prefers-reduced-motion: reduce)')` from `@vueuse/core` (mirrors existing project usage), use `@vueuse/core`'s `useIntervalFn` for the 7-second cycle (auto-cleanup on unmount eliminates one class of leak), install `@axe-core/playwright@4.11.3` as a devDependency for the a11y e2e gate, and adapt the env-flag delivery to the project's actual layout (`.env` + `.env.example` — there is no `.env.development` or `.env.production.example` in this repo).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Spotlight data aggregation | API / Backend (catalog) | — | Phase 1 delivered `GET /api/home/spotlight`; Phase 2 is consumer-only |
| HTTP fetch + auth-header propagation | Browser / Client | API | `@/api/client` axios instance already wires `Authorization` + `X-Anon-ID` + JWT refresh — no Phase 2 wiring needed |
| Carousel state machine (currentIndex, auto, interval) | Browser / Client | — | Pure client interaction; no server state |
| Reduced-motion detection | Browser / Client | — | OS-level `prefers-reduced-motion` is a browser media query |
| Feature flag enforcement | Browser / Client | API (catalog `SPOTLIGHT_ENABLED`) | Two-layer kill switch: frontend `VITE_HERO_SPOTLIGHT_ENABLED` hides the block; backend `SPOTLIGHT_ENABLED=false` makes the endpoint 404 (block self-hides on error → same effect) |
| i18n string resolution | Browser / Client | — | `vue-i18n` runs in the browser; keys ship in `src/locales/*.json` |
| Skeleton state rendering | Browser / Client | — | UI state, not server |
| A11y semantics (role, aria-roledescription, aria-live) | Browser / Client | — | DOM-level; pure markup + Vue bindings |
| E2E + a11y verification | Test runner (Playwright) | Browser | Out-of-band verification; not part of runtime |

**Why this matters:** Phase 2 is intentionally single-tier (browser-only). No work spans into the API layer beyond `GET /api/home/spotlight`. Any task that proposes touching catalog code, gateway routing, or any backend service is out of scope and should be flagged by plan-checker.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HSB-FE-01 | `HeroSpotlightBlock.vue` mounted in `Home.vue` after `<SystemStatusBanner />`, above the legacy `trendingRecs` row (line ~39 in current Home.vue) | Section "Existing Mount Point" verified at `Home.vue:8` (SystemStatusBanner) → `Home.vue:39-132` (trendingRecs). Phase 2 inserts between line 8 and line 12. Legacy row stays. |
| HSB-FE-02 | Fetches `GET /api/home/spotlight` once on mount; hides on error/empty | "Standard Stack" + "Architecture Patterns / API Client Pattern" — use `apiClient.get('/home/spotlight')` from `@/api/client` (the project's only HTTP client). Mirrors `useContinueWatching.ts:48-54` `try/catch → items.value = []` pattern. |
| HSB-FE-03 | 7s auto-cycle; pauses on `mouseenter`/`focusin`; resumes on `mouseleave`/`focusout` | "Architecture Patterns / Carousel State Machine" — use `@vueuse/core`'s `useIntervalFn(fn, 7000, { immediate: false })` which auto-cleans on unmount; toggle with `.pause()` / `.resume()` |
| HSB-FE-04 | Manual nav — left/right chevrons + dot indicators; keyboard ArrowLeft/ArrowRight | Pattern: `<button>` for chevrons + dots (native focus/Enter/Space), `@keydown.left`/`@keydown.right` on root `<section tabindex="0">` (UI-SPEC §Interaction Contract) |
| HSB-FE-05 | Initial slide chosen randomly on every page load | `Math.floor(Math.random() * cards.value.length)` inside `onMounted` AFTER cards populate — a `watch(() => cards.value.length, ...)` guard handles the async race |
| HSB-FE-06 | Respects `prefers-reduced-motion: reduce` | "Standard Stack" — `useMediaQuery('(prefers-reduced-motion: reduce)')` from `@vueuse/core` (precedent: `Hero.vue:124`, `Carousel.vue:124`) |
| HSB-FE-07 | A11y — region role, aria-roledescription, slide aria-label, aria-live | "Architecture Patterns / A11y Pattern" — markup verbatim from UI-SPEC §Accessibility Contract; axe-core verified via `@axe-core/playwright` |
| HSB-FE-08 | Animated skeleton placeholder matching final height | "Architecture Patterns / Skeleton Pattern" — reuse `.skeleton-shimmer` class from `main.css:299` (already in repo). No new CSS. |
| HSB-FE-09 | Feature flag `VITE_HERO_SPOTLIGHT_ENABLED` (default true) | "Architecture Patterns / Feature Flag Pattern" — `import.meta.env.VITE_HERO_SPOTLIGHT_ENABLED !== 'false'` (precedent: `App.vue:92` for `VITE_NOTIFICATIONS_ENABLED`) |
| HSB-FE-20 | `AnimeOfDayCard.vue` — poster + meta + CTAs | UI-SPEC §Visual Contract has the full markup. Use `getLocalizedTitle` from `@/utils/title` (precedent: `useAnime.ts:65`, `ContinueWatchingRow.vue:52`) |
| HSB-FE-21 | `LatestNewsCard.vue` — 3 entries, links to `/changelog` | UI-SPEC §Visual Contract. `/changelog` route already exists (`LastUpdates.vue` consumes `changelog.json` per CLAUDE.md). |
| HSB-FE-22 | `PlatformStatsCard.vue` — up to 3 metric chips | UI-SPEC §Visual Contract — adaptive grid via `:class` binding on `data.metrics.length` |
| HSB-FE-23 | `RandomTailCard.vue` — single poster + meta | UI-SPEC §Visual Contract — near-clone of AnimeOfDayCard with cyan-300/80 eyebrow + single "Open" CTA |
| HSB-FE-40 | i18n: `spotlight.*` namespace in `en.json` + `ru.json` | UI-SPEC §Copywriting Contract provides the full key/value table for both locales. Add to top-level objects in both JSON files. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

| Constraint | Source | Impact on Phase 2 |
|------------|--------|-------------------|
| Use `bun` (not npm/pnpm) for install | CLAUDE.md "Frontend Note" | `bun add -d @axe-core/playwright` (NOT `npm install`) |
| Use `bunx` for CLI tools | CLAUDE.md "Frontend Note" | `bunx tsc --noEmit`, `bunx eslint src/`, `bunx playwright test` |
| Use `make redeploy-web` for deployment | CLAUDE.md "Local Development Commands" | Per-task plan: lint → type-check → `bun run build` → `make redeploy-web` |
| Effort & impact metrics — UXΔ / CDI / MVQ, NO days/hours | CLAUDE.md "Effort & impact metrics" + memory `feedback_no_days_metric` | Every plan file scored on these 3 axes |
| Vue Composition API + `<script setup lang="ts">` | CLAUDE.md "Established Patterns" via CONTEXT.md | All 6 new components use Composition API |
| Tailwind utilities only (no SCSS modules) | CLAUDE.md "Established Patterns" via CONTEXT.md | Net-new CSS is 4 lines of cross-fade keyframes in `main.css` (UI-SPEC §Visual Contract); no SCSS, no new utility classes |
| `vue-i18n` `t(...)` for all user-facing strings | CLAUDE.md "Established Patterns" via CONTEXT.md | `@intlify/eslint-plugin-vue-i18n` (devDep, package.json:31) catches hard-coded strings |
| Co-authors on every commit | MEMORY.md "Commit Configuration" | Each plan-induced commit includes Claude/0neymik0/NANDIorg co-authors |
| `--ws hero-spotlight` on every GSD command | MEMORY.md "Parallel Claude sessions + workstreams" | Plan + execute commands MUST pass this flag |
| Run `/animeenigma-after-update` skill after implementation | CLAUDE.md "After-Update Skill (MUST USE)" | Phase wrap-up: lint+build+redeploy+changelog+commit+push as a single skill, not ad-hoc |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `vue` | 3.4.21 | Component framework | Already in `package.json` — verified at `/data/animeenigma/frontend/web/package.json:26` [VERIFIED: package.json] |
| `vue-i18n` | 11.2.8 | i18n strings | `useI18n()` composable returns `{ t, locale }` (precedent: `CollectionsRow.vue:63`) [VERIFIED: package.json:27] |
| `vue-router` | 4.3.0 | Routing — `<router-link>` to `/anime/:id`, `/changelog` | Anime detail + changelog routes already exist [VERIFIED: package.json:28] |
| `axios` | 1.13.6 | HTTP client (via `@/api/client`) | Established at `frontend/web/src/api/client.ts:1` with full interceptor stack (JWT refresh, X-Anon-ID, prefs-cache busting). DO NOT bypass `apiClient` with `fetch()`. [VERIFIED: package.json:21] |
| `@vueuse/core` | 14.1.0 | `useMediaQuery`, `useEventListener`, `useIntervalFn`, `onClickOutside` | Established at `Hero.vue`, `Carousel.vue`, `NotificationToast.vue`, `Navbar.vue`, `NotificationBell.vue` [VERIFIED: package.json:19] |
| `tailwindcss` | 4.1.18 | Utility classes; Neon Tokyo tokens at `src/styles/main.css` | Already configured [VERIFIED: package.json:48] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `@playwright/test` | 1.58.0 | E2E test runner | `frontend/web/e2e/spotlight.spec.ts` [VERIFIED: package.json:32] |
| `vitest` | 4.1.6 | Unit test runner (co-located `src/**/*.spec.ts`) | Optional component-level tests [VERIFIED: package.json:52] |
| `@vue/test-utils` | 2.4.10 | `mount()` / `shallowMount()` for Vitest | Component-level tests if added [VERIFIED: package.json:39] |
| `jsdom` | 29.1.1 | DOM environment for Vitest (configured in `vitest.config.ts:10`) | — [VERIFIED: package.json:44] |
| `@axe-core/playwright` | **NEEDS INSTALL** (latest 4.11.3) | A11y assertion in Playwright spec | Plan MUST install: `bun add -d @axe-core/playwright` [VERIFIED: npm view @axe-core/playwright version → 4.11.3, published 2025-09-08] [VERIFIED: not present in package.json] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled `setInterval` + lifecycle hooks | `useIntervalFn` from `@vueuse/core` | `useIntervalFn` auto-cleans on unmount, exposes `.pause()`/`.resume()`/`.isActive` — eliminates the "forgot to clearInterval" leak class. **Recommendation: use it.** |
| Hand-rolled `matchMedia` listener + ref | `useMediaQuery('(prefers-reduced-motion: reduce)')` from `@vueuse/core` | Project already uses this idiom in 5 places. `useMediaQuery` handles the deprecated-`addListener` vs `addEventListener` polyfill internally. **Recommendation: use it inline; do NOT add a `useReducedMotion.ts` wrapper.** |
| Bespoke `useEventListener` for keyboard handlers | `useEventListener` from `@vueuse/core` | Auto-cleans on unmount. Precedent: `NotificationBell.vue:60`, `Navbar.vue`. But the simpler approach for ArrowLeft/Right is `@keydown.left`/`@keydown.right` directly on the wrapper element — already idiomatic and shorter. **Recommendation: prefer template bindings for the wrapper; reserve `useEventListener` for `document`/`window` listeners.** |
| Embla / Swiper / Glide carousel library | Hand-rolled state machine | Confirmed: ZERO third-party carousel deps in `package.json`. `Carousel.vue` is an internal hand-rolled scrollable row (different shape — horizontal scroll, not slide-replace). Building a fresh state machine is correct here. **Recommendation: hand-roll per CONTEXT.md.** |
| Axe-core via CDN injection (UI/UX audit pattern) | `@axe-core/playwright` npm package | UI/UX audits use CDN injection for ad-hoc audits. For repeatable CI/local e2e gates, `@axe-core/playwright` is more reliable (no network dep, version-pinned, works offline). **Recommendation: install `@axe-core/playwright`.** |

**Installation:**

```bash
# Run from frontend/web/
bun add -d @axe-core/playwright
```

No runtime dependencies need to be added — all required libraries already ship with the project.

**Version verification:**

| Package | Installed | Latest (npm registry, 2026-05-21) | Decision |
|---------|-----------|-----------------------------------|----------|
| `vue` | 3.4.21 | (newer exists, not relevant) | Use installed [VERIFIED: package.json] |
| `vue-i18n` | 11.2.8 | (newer may exist) | Use installed [VERIFIED: package.json] |
| `axios` | 1.13.6 | 1.16.1 | Use installed; no Phase 2 reason to bump [VERIFIED: npm view axios version] |
| `@vueuse/core` | 14.1.0 | 14.3.0 | Use installed; APIs needed (`useMediaQuery`, `useIntervalFn`) are stable since v10.x [VERIFIED: npm view @vueuse/core version] |
| `vitest` | 4.1.6 | 4.1.7 | Use installed [VERIFIED: npm view vitest version] |
| `@playwright/test` | 1.58.0 | 1.60.0 | Use installed; no Phase 2 reason to bump [VERIFIED: npm view @playwright/test version] |
| `@axe-core/playwright` | NOT INSTALLED | 4.11.3 (published 2025-09-08) | Plan installs latest [VERIFIED: npm view @axe-core/playwright version] |

## Architecture Patterns

### System Architecture Diagram

```
┌────────────────────────────────────────────────────────────────────┐
│ Browser (frontend/web/)                                            │
│                                                                    │
│  Home.vue                                                          │
│    └─ <SystemStatusBanner />     (existing, line 8)                │
│    └─ <HeroSpotlightBlock />     (NEW — mounts here, ~line 9-10)   │
│         │                                                          │
│         ├─ useSpotlight()                                          │
│         │    └─ apiClient.get('/home/spotlight')                   │
│         │         │                                                │
│         │         └─→ axios interceptor stack                      │
│         │              (JWT refresh, X-Anon-ID, prefs cache)       │
│         │              │                                           │
│         ├─ useMediaQuery('(prefers-reduced-motion: reduce)')       │
│         │                                                          │
│         ├─ useIntervalFn(advance, 7000)  → pause()/resume()        │
│         │                                                          │
│         ├─ <transition name="spotlight-fade" mode="out-in">        │
│         │     <component :is="cardFor(active.type)" :data="..." /> │
│         │   </transition>                                          │
│         │     ├─ AnimeOfDayCard.vue                                │
│         │     ├─ RandomTailCard.vue                                │
│         │     ├─ LatestNewsCard.vue                                │
│         │     └─ PlatformStatsCard.vue                             │
│         │                                                          │
│         └─ <CarouselControls @prev @next @goto />                  │
│                                                                    │
│    └─ <div trendingRecs>         (legacy, line 39–132 — UNTOUCHED) │
│    └─ 3-column Ongoing/Top/Announced grid                          │
│                                                                    │
└─────────────────────┬──────────────────────────────────────────────┘
                      │
                      ▼ HTTP GET /api/home/spotlight
┌────────────────────────────────────────────────────────────────────┐
│ Gateway → Catalog (Phase 1, already shipped)                       │
│   Returns { cards: SpotlightCard[], generated_at: string }         │
└────────────────────────────────────────────────────────────────────┘
```

Phase 2 introduces **zero new server-side surface**. The only network call originating from new code is the existing `apiClient.get('/home/spotlight')` — already authenticated, retried, and cache-busted by the shared interceptor stack.

### Recommended Project Structure

```
frontend/web/
├── src/
│   ├── api/
│   │   └── client.ts                       # existing — extend with spotlightApi (optional, see below)
│   ├── components/
│   │   └── home/
│   │       └── spotlight/                  # NEW folder
│   │           ├── HeroSpotlightBlock.vue   # NEW — outer wrapper + state machine
│   │           ├── CarouselControls.vue     # NEW — chevrons + dots, emits prev/next/goto
│   │           └── cards/                   # NEW subfolder
│   │               ├── AnimeOfDayCard.vue
│   │               ├── RandomTailCard.vue
│   │               ├── LatestNewsCard.vue
│   │               └── PlatformStatsCard.vue
│   ├── composables/
│   │   └── useSpotlight.ts                  # NEW — fetch + cards + loading + error + refresh
│   ├── types/
│   │   └── spotlight.ts                     # NEW — discriminated-union SpotlightCard type
│   ├── locales/
│   │   ├── en.json                          # extend — add "spotlight": {...}
│   │   └── ru.json                          # extend — add "spotlight": {...}
│   ├── styles/
│   │   └── main.css                         # extend — 4 lines of .spotlight-fade-* CSS
│   └── views/
│       └── Home.vue                         # extend — import + mount <HeroSpotlightBlock />
├── .env                                     # extend — add VITE_HERO_SPOTLIGHT_ENABLED=true
├── .env.example                             # extend — add VITE_HERO_SPOTLIGHT_ENABLED=true (DOC)
└── e2e/
    └── spotlight.spec.ts                    # NEW — Playwright spec (see Validation Architecture)
```

> **Note on env files:** The project does NOT have `.env.development` or `.env.production.example` files (verified via `find` in `frontend/web/`). CONTEXT.md and UI-SPEC.md mention these — they don't exist. The correct targets are `.env` (gitignored, actual dev values) and `.env.example` (committed, documentation). Plan must adapt.

### Pattern 1: Composable + axios via apiClient

**What:** Encapsulate fetch + state in a composable; return refs for the consuming component.
**When to use:** Every data-fetching composable in this codebase follows this shape.
**Example (mirrors `useContinueWatching.ts` shape):**

```typescript
// frontend/web/src/composables/useSpotlight.ts
// Source: pattern from frontend/web/src/composables/useContinueWatching.ts:33-80
import { ref, onMounted } from 'vue'
import { apiClient } from '@/api/client'
import type { SpotlightCard } from '@/types/spotlight'

interface SpotlightResponse {
  cards: SpotlightCard[]
  generated_at: string
}

export function useSpotlight() {
  const cards = ref<SpotlightCard[]>([])
  const loading = ref(true)
  const error = ref<Error | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const res = await apiClient.get<SpotlightResponse | { data: SpotlightResponse }>(
        '/home/spotlight',
      )
      // Catalog envelope: { success, data: {cards, generated_at} }
      // Some catalog endpoints return raw payload, some wrap. Unwrap defensively
      // (precedent: useContinueWatching.ts:50 `(res.data?.data ?? res.data)`).
      const body = (res.data as { data?: SpotlightResponse })?.data ?? (res.data as SpotlightResponse)
      cards.value = Array.isArray(body?.cards) ? body.cards : []
    } catch (e) {
      // Silent self-hide on error per UI-SPEC §State Contract. One warn.
      console.warn('[spotlight] fetch failed', e)
      error.value = e instanceof Error ? e : new Error('spotlight fetch failed')
      cards.value = []
    } finally {
      loading.value = false
    }
  }

  onMounted(refresh)

  return { cards, loading, error, refresh }
}
```

**Optionally extend `@/api/client.ts`** with a `spotlightApi` export (mirrors the existing `scraperApi`, `kodikApi`, `userApi` shape at `client.ts:307-696`):

```typescript
// Append to frontend/web/src/api/client.ts
export const spotlightApi = {
  get: () => apiClient.get('/home/spotlight'),
}
```

Then `useSpotlight` calls `spotlightApi.get()` instead of `apiClient.get('/home/spotlight')`. Either pattern works — both are precedented. The composable-calls-apiClient-directly pattern in `useContinueWatching.ts:49` (`userApi.getContinueWatching(limit)`) routes through a typed api object; that's slightly cleaner. Pick one and be consistent.

### Pattern 2: Reduced-motion via @vueuse/core

**What:** Use `useMediaQuery` from `@vueuse/core` inline. Do NOT create a `useReducedMotion.ts` wrapper composable.
**When to use:** Any reactive media-query check. Project precedent: `Hero.vue:124`, `Carousel.vue:124`, `Navbar.vue`, `NotificationToast.vue`.
**Example:**

```typescript
// Inside HeroSpotlightBlock.vue <script setup lang="ts">
// Source: frontend/web/src/components/carousel/Carousel.vue:124
import { useMediaQuery } from '@vueuse/core'

const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')

// Use in template: :name="reducedMotion ? 'none' : 'spotlight-fade'"
// Use in script: if (reducedMotion.value) return // skip cycle
```

`useMediaQuery` returns a `Ref<boolean>` that re-renders the component when the OS-level setting toggles at runtime. The deprecated-`addListener` vs modern-`addEventListener` polyfill is handled inside `@vueuse/core`.

### Pattern 3: Auto-cycle via useIntervalFn

**What:** Use `useIntervalFn` from `@vueuse/core` instead of raw `setInterval`. Auto-cleans on unmount; exposes `.pause()` / `.resume()` / `.isActive`.
**When to use:** Any interval timer in a Vue 3 component. Eliminates the "forgot to clearInterval" leak class.
**Example:**

```typescript
// Source: @vueuse/core docs — useIntervalFn (Phase 2 will be its first repo use; no precedent)
// [CITED: https://vueuse.org/shared/useIntervalFn/]
import { useIntervalFn } from '@vueuse/core'

const AUTO_CYCLE_INTERVAL_MS = 7000

function advance() {
  if (cards.value.length === 0) return
  currentIndex.value = (currentIndex.value + 1) % cards.value.length
}

// immediate: false — don't tick on mount; we wait until after randomInit
const { pause, resume, isActive } = useIntervalFn(advance, AUTO_CYCLE_INTERVAL_MS, {
  immediate: false,
})

function stopCycle() { pause() }
function startCycle() {
  if (reducedMotion.value) return
  if (cards.value.length <= 1) return // single card — no cycle
  resume()
}

// Manual nav resets the cycle so the user doesn't see "next" auto-fire 200ms
// after they manually advanced.
function restart() {
  if (isActive.value) {
    pause()
    resume()
  }
}

function next() { currentIndex.value = (currentIndex.value + 1) % cards.value.length; restart() }
function prev() { currentIndex.value = (currentIndex.value - 1 + cards.value.length) % cards.value.length; restart() }
function goTo(i: number) { currentIndex.value = i; restart() }
```

Alternative (acceptable if planner prefers explicit ID tracking): raw `setInterval` + `onBeforeUnmount(clearInterval)`. Same behavior, more boilerplate, one more leak vector.

### Pattern 4: Feature flag via import.meta.env

**What:** `import.meta.env.VITE_HERO_SPOTLIGHT_ENABLED !== 'false'` — the project uses **`!== 'false'`** semantics (default true; only the literal string `'false'` disables).
**When to use:** Any VITE-prefixed env-driven kill switch.
**Example (verbatim precedent for `VITE_NOTIFICATIONS_ENABLED`):**

```typescript
// Source: frontend/web/src/App.vue:92
const notifEnabled =
  (import.meta.env.VITE_NOTIFICATIONS_ENABLED as string | undefined) !== 'false'

// For spotlight:
const enabled =
  (import.meta.env.VITE_HERO_SPOTLIGHT_ENABLED as string | undefined) !== 'false'
```

Use `enabled` as the outer `v-if` on the `<section>`. Combined with the cards-length guard:

```html
<section v-if="enabled && cards.length > 0" ...>
```

### Pattern 5: Discriminated-union SpotlightCard type

**What:** Standard TypeScript tagged-union pattern; each variant has a `type: 'literal'` discriminator. `data` is the variant-specific payload.
**When to use:** Any time multiple shapes share an envelope. No project precedent for this exact shape (most catalog types are flat interfaces), but TypeScript's idiomatic pattern is well-known.
**Example:**

```typescript
// frontend/web/src/types/spotlight.ts
// Source: design doc §4.1 — Hero Spotlight Block design

export interface SpotlightAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  episodes?: number
  genres?: { id: string; name?: string; russian?: string }[]
}

export interface AnimeOfDayData {
  anime: SpotlightAnime
}

export interface RandomTailData {
  anime: SpotlightAnime
}

export interface ChangelogEntry {
  id: string
  date: string  // ISO 8601
  title: string
  summary: string
}

export interface LatestNewsData {
  entries: ChangelogEntry[]
}

export interface PlatformMetric {
  key: 'anime_added_7d' | 'episodes_added_7d' | 'active_rooms_7d'
  value: number
  delta?: number | null
}

export interface PlatformStatsData {
  metrics: PlatformMetric[]
}

// Discriminated union — exhaustive switch in cardFor() narrows correctly
export type SpotlightCard =
  | { id: string; type: 'anime_of_day'; data: AnimeOfDayData }
  | { id: string; type: 'random_tail'; data: RandomTailData }
  | { id: string; type: 'latest_news'; data: LatestNewsData }
  | { id: string; type: 'platform_stats'; data: PlatformStatsData }
```

In `HeroSpotlightBlock.vue`, narrowing happens via the `cardFor(type)` map → returns the correct component, and the `data` prop is typed on each child:

```typescript
// AnimeOfDayCard.vue
import type { AnimeOfDayData } from '@/types/spotlight'
defineProps<{ data: AnimeOfDayData }>()
```

> ⚠️ **Important:** The card variant shapes above are inferred from the design doc and UI-SPEC. The Phase 1 backend response is the source of truth — the planner MUST verify the actual JSON shape via `curl http://localhost:8000/api/home/spotlight | jq` before locking the type file. If Phase 1's `cards/anime_of_day.go` resolver returns a slightly different field set (e.g. `episodes_count` vs `episodes`), the TS type must match exactly. **This is a load-bearing call-it-out: hand-rolling types from a design doc without verifying against the runtime payload is a recipe for `undefined` rendering.**

### Pattern 6: A11y carousel markup

**What:** Wrap the slide region with `role="region"` + `aria-roledescription="carousel"`; the active slide's container gets `aria-live="polite"`; the slide root gets `role="group"` + `aria-roledescription="slide"`. Dots are `<button>`s with `aria-current="true"` on the active one.
**When to use:** Verbatim from UI-SPEC §Accessibility Contract. Follows [W3C ARIA APG carousel pattern](https://www.w3.org/WAI/ARIA/apg/patterns/carousel/examples/carousel-1-prev-next/).
**Example:** See UI-SPEC §Visual Contract code blocks for the full markup. Planner copies verbatim.

### Pattern 7: Skeleton via existing .skeleton-shimmer

**What:** Reuse `.skeleton-shimmer` class from `main.css:299` — already in the repo. Single full-block div sized to match `min-h-[400px] md:min-h-[340px] lg:min-h-[320px]`.
**When to use:** Any skeleton placeholder on the home page. Precedent: `ContinueWatchingRow.vue:59-68` uses `bg-white/10 animate-pulse`; `CollectionsRow.vue:45-54` uses the same.
**Example:** Verbatim from UI-SPEC §Visual Contract (skeleton state block).

### Anti-Patterns to Avoid

- **Don't bypass `apiClient` with `fetch()`** — the interceptor stack handles JWT refresh, anon-ID, prefs cache. A raw `fetch()` call to `/api/home/spotlight` would skip auth refresh on a stale token, breaking authenticated-card scenarios in Phase 3.
- **Don't create a `useReducedMotion()` wrapper composable** — the project's idiom is inline `useMediaQuery(...)`. A wrapper composable adds zero value and diverges from existing pattern. CONTEXT.md "Claude's Discretion" explicitly leaves this open; the evidence says inline.
- **Don't use raw `setInterval` without `useIntervalFn`** — auto-cleanup on unmount is free with `@vueuse/core`. Bespoke cleanup adds a leak vector.
- **Don't hard-code English (or Russian) strings** in the 6 new Vue components — ESLint's `@intlify/eslint-plugin-vue-i18n` (devDep, package.json:31) will fail the build. Every string flows through `t('spotlight.…')`.
- **Don't put `aria-live` on the section wrapper** — only on the slide container. Putting it on the section announces every Tab+focus change on every dot button. (Hard rule per UI-SPEC §Accessibility Contract.)
- **Don't use the deprecated `addListener` for matchMedia** — `useMediaQuery` from `@vueuse/core` handles this. If hand-rolling, use `mq.addEventListener('change', handler)` (NOT `mq.addListener(handler)`).
- **Don't randomize the initial index inside `onMounted` synchronously** — `cards` may still be empty when `onMounted` fires. The fix is `watch(() => cards.value.length, (n) => { if (n > 0) currentIndex.value = Math.floor(Math.random() * n); startCycle() })` to react after the fetch resolves. UI-SPEC §Interaction Contract has this pattern.
- **Don't use `axios` directly inside `useSpotlight.ts`** — always go through `@/api/client` `apiClient` instance so interceptors run.
- **Don't write to `.env.development` or `.env.production.example`** — these files don't exist in the repo. The targets are `.env` (gitignored) and `.env.example` (committed).
- **Don't omit `cards.length <= 1` guard from `startCycle`** — if backend returns 1 card (extremely rare but possible if 3 of 4 resolvers fail their 800ms deadline), cycling makes no sense and produces a no-op flicker every 7s.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP fetch with auth refresh | New `fetch()` call or new axios instance | `apiClient` from `@/api/client` | Existing interceptor handles JWT refresh, anon-ID, prefs cache, X-Token-Expired retry, single-flight refresh across tabs |
| matchMedia polyfill | Bespoke `window.matchMedia` listener with `addEventListener` + `onUnmounted` cleanup | `useMediaQuery` from `@vueuse/core` | Handles deprecated-`addListener` fallback, SSR safety, auto-cleanup on unmount |
| Interval lifecycle | `setInterval` + `onBeforeUnmount(clearInterval)` | `useIntervalFn` from `@vueuse/core` | Built-in `.pause()` / `.resume()` / `.isActive` matches the state-machine API; auto-cleanup eliminates leak class |
| Carousel slide-fade animation | Hand-rolled `@keyframes` | Vue `<transition name="spotlight-fade" mode="out-in">` + 4 lines of `.spotlight-fade-*` CSS | Vue's `<transition>` is the project's existing animation primitive; 4 lines beats a full keyframes block |
| Skeleton shimmer | New keyframes | `.skeleton-shimmer` class from `main.css:299` | Already shipped; visually unified across home page |
| Focus ring on chevrons/dots | Per-component `focus:ring-cyan-400` | Global `:focus-visible` at `main.css:91` | Already paints 2px cyan-400 ring; consistent across the app |
| A11y axe scan in CI | Hand-rolled axe-core CDN injection | `@axe-core/playwright` npm package | UI/UX audit pattern uses CDN injection for ad-hoc; CI/e2e want pinned versions |
| Localized title resolution | New `pickTitle(name, name_ru, name_jp)` helper | `getLocalizedTitle` from `@/utils/title` | Used in `useAnime.ts:65`, `ContinueWatchingRow.vue:52`, etc. |
| Image proxy URL building | New URL builder | `getImageUrl` from `@/composables/useImageProxy` | Used in `useAnime.ts:5`; centralizes image-proxy logic |

**Key insight:** This phase has zero novel primitives. Every state-machine component, every async helper, every visual class either already exists in the codebase or in `@vueuse/core`. The risk is **wiring**, not **invention** — keep the bespoke surface narrow (state machine wrapper + 4 card layouts + composable + types + i18n keys + one test spec).

## Runtime State Inventory

Phase 2 is greenfield/additive — no rename, no refactor, no string-replacement. The new files do not collide with any existing path:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — verified by `find /data/animeenigma/frontend/web/src -name "spotlight*"` returned nothing | none |
| Live service config | None — the spotlight backend env var `SPOTLIGHT_ENABLED` exists in `docker/.env.example:352` (Phase 1) but is unrelated to Phase 2 frontend work | none |
| OS-registered state | None — no systemd/launchd/Task Scheduler touchpoints; pure browser code | none |
| Secrets/env vars | New: `VITE_HERO_SPOTLIGHT_ENABLED` (committed to `.env` + `.env.example`). Not a secret — boolean flag. | Plan adds to both files; no rotation needed |
| Build artifacts | None — `bun run build` rebuilds clean; no stale `.d.ts` or compiled artifacts to invalidate | none |

**The canonical question:** *After every file in the repo is updated, what runtime systems still have the old string cached, stored, or registered?* — N/A; no rename/refactor in this phase.

## Common Pitfalls

### Pitfall 1: setInterval not cleared on auto-flag toggle

**What goes wrong:** When `auto` becomes `false` (e.g. user enables reduced-motion mid-session), the existing interval keeps firing until unmount — slides advance even though auto-cycle is "off."
**Why it happens:** Naive implementation only clears the interval on unmount, not on `auto` transitions.
**How to avoid:** Use `useIntervalFn(..., { immediate: false })` and toggle via `pause()` / `resume()`. Watch the `reducedMotion` ref: `watch(reducedMotion, (v) => v ? pause() : resume())`.
**Warning signs:** Visual — slides keep advancing after toggling reduced-motion. Add an e2e assertion: emulate reduced-motion, wait 8s, assert slide index unchanged.

### Pitfall 2: Layout shift while loading

**What goes wrong:** Skeleton block has a different height than the loaded carousel, so the page content "jumps" 280-400px when the fetch resolves.
**Why it happens:** Skeleton uses arbitrary height; carousel has dynamic content height.
**How to avoid:** Both skeleton and loaded states use identical `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]` classes. The loaded `<section>` has the same `min-h-*` so it can never collapse smaller.
**Warning signs:** Chrome DevTools → Performance → Web Vitals → CLS > 0.1 on Home.vue load. E2E assertion: capture the page-height delta from skeleton-visible to cards-visible — should be ≤ 4px.

### Pitfall 3: HMR leaves stale intervals

**What goes wrong:** During `bun run dev`, editing `HeroSpotlightBlock.vue` triggers Vite HMR. If the old component's `setInterval` isn't cleared, you get TWO intervals firing on the same `currentIndex.value`, slides jump 2-at-a-time.
**Why it happens:** Vue HMR re-creates the component but the closure-captured `intervalId` lives in the old module instance.
**How to avoid:** `useIntervalFn` auto-cleans via `onScopeDispose` (effect scope), which fires on HMR too. Avoid raw `setInterval`.
**Warning signs:** Dev only — slides flash through fast after a save. Test by saving the file 3x in a row and observing.

### Pitfall 4: Race — random init runs before cards populate

**What goes wrong:** `onMounted` triggers the `useSpotlight().refresh()`, which is async. The `currentIndex.value = Math.floor(Math.random() * cards.value.length)` line in the same `onMounted` runs synchronously — `cards.value.length === 0`, `Math.random() * 0 === 0`, `Math.floor(0) === 0`. Result: initial slide is always index 0, never random.
**Why it happens:** `await` boundary inside `refresh()` defers the assignment to the next microtask; the synchronous `onMounted` callback returns first.
**How to avoid:** Use `watch(() => cards.value.length, (n) => { if (n > 0 && currentIndex.value === 0) { currentIndex.value = Math.floor(Math.random() * n); startCycle() } })`. (The `currentIndex.value === 0` guard prevents re-randomizing on `refresh()` calls if Phase 3 ever introduces them.)
**Warning signs:** Reload 10 times — initial slide is always the first card. Phase 2 success criterion #5 requires ~75% variety.

### Pitfall 5: API returns empty cards array

**What goes wrong:** Backend returns `{ cards: [], generated_at: "..." }` — block tries to render with `cards.length === 0`, dot indicator renders 0 dots, `<transition>` has no `<component>` child, console errors about missing `key`.
**Why it happens:** All 4 resolvers timed out (very unlikely but possible), OR backend feature flag flipped to false mid-response, OR test environment.
**How to avoid:** `v-if="enabled && cards.length > 0"` on the wrapper. The `loading` skeleton hides; the `cards.length === 0 && !loading` case renders nothing. UI-SPEC §State Contract has this verbatim.
**Warning signs:** Console error `[Vue warn]: Component is missing template` after the fetch resolves. E2E assertion: mock `cards: []` response, assert `<section role="region">` not in DOM.

### Pitfall 6: import.meta.env in Vitest/Playwright

**What goes wrong:** Unit test mocks `import.meta.env.VITE_HERO_SPOTLIGHT_ENABLED = 'false'` to test the disabled state — but Vitest's jsdom env doesn't replay Vite's compile-time substitution, so the flag stays at whatever was in `.env` when the test process started.
**Why it happens:** `import.meta.env` is replaced at build time by Vite; in Vitest's runtime, it's a regular object. Mutating it before component mount works, but timing matters.
**How to avoid:** Test the disabled state via Playwright (full-stack), not Vitest. In Vitest, if needed, mock the boolean via a stub: `vi.stubGlobal('import.meta', { env: { VITE_HERO_SPOTLIGHT_ENABLED: 'false' } })` BEFORE the component import. Easier: keep this test in Playwright where you can set the env at `bun run dev` startup.
**Warning signs:** Vitest test "feature flag off hides block" intermittently passes/fails depending on system env.

### Pitfall 7: focusout fires before activeElement settles

**What goes wrong:** User Tabs from chevron-prev to dot-1 inside the block. Vue's `@focusout` fires on chevron-prev. The handler resumes the cycle. Then `@focusin` fires on dot-1, pausing again. Result: visible 1-frame flicker, possibly a stale interval state.
**Why it happens:** `focusout` fires synchronously before the new focus target is resolvable via `document.activeElement` (which is `document.body` momentarily).
**How to avoid:** Defer the resume check with `requestAnimationFrame`: `setTimeout(() => { if (!rootRef.value?.contains(document.activeElement)) resume() }, 0)`. UI-SPEC §Interaction Contract calls this out.
**Warning signs:** Internal Tab navigation through dots causes brief slide-advance flickers. Reproduce: Tab into the block, then Tab through 4 dots — observe if any slides advance unexpectedly.

### Pitfall 8: Snake_case vs camelCase JSON mismatch

**What goes wrong:** Backend returns snake_case (`anime_added_7d`, `poster_url`, `name_ru`) per Go convention. TypeScript types use camelCase by habit, so fields are `undefined` at render time.
**Why it happens:** Backend Go uses `json:"snake_case"` tags; frontend TS often uses `camelCase` props.
**How to avoid:** Mirror snake_case from the backend exactly in `types/spotlight.ts`. Precedent: `useAnime.ts:7-29` keeps the raw API shape as `ApiAnime` (snake_case) and transforms to `Anime` (camelCase) only for display models. For spotlight, since `data` is opaque-passed through to cards, **keep snake_case throughout** (no transform step). Card components access `data.anime.poster_url`, `data.anime.name_ru`, etc.
**Warning signs:** Poster slot renders the placeholder SVG; title is empty; e2e asserts on text fail. Cross-check the type file against an actual `curl` of `/api/home/spotlight`.

### Pitfall 9: Hard-coded fallback strings break i18n lint

**What goes wrong:** Developer adds `<span>{{ data.anime.score?.toFixed(1) }} / 10</span>` for a fallback display — ESLint `@intlify/vue-i18n/no-raw-text` flags the literal `/ 10` and fails the build.
**Why it happens:** The plugin scans Vue templates for raw text; any string not wrapped in `t()` is flagged.
**How to avoid:** Wrap fallback constants in `t()` even when they look like punctuation: `t('spotlight.scoreOutOf', { n: data.anime.score?.toFixed(1) })` with `"scoreOutOf": "{n} / 10"`. Numerals + units + currency symbols all need keys.
**Warning signs:** `bunx eslint src/` fails on a `.vue` file with `[@intlify/vue-i18n/no-raw-text]`.

### Pitfall 10: Backend ID vs Vue key collision

**What goes wrong:** Two cards have `id` equal to the same anime ID (e.g. anime_of_day and random_tail both pick anime 12345 from the catalog by date-seeded chance). Vue's `:key="active.id"` on the slide `<component>` makes the transition think nothing changed.
**Why it happens:** Each card variant's `data.anime.id` is the underlying anime ID, but a card's "envelope ID" must be unique across the spotlight response.
**How to avoid:** Backend response should provide a per-card `id` field (e.g. `anime_of_day:2026-05-21`) distinct from the underlying anime UUID. If absent, fall back to `${card.type}:${index}` as the Vue key. Verify with a `curl` of the live endpoint.
**Warning signs:** Manual click on dot-2 doesn't trigger the cross-fade; the same content stays on screen.

## Code Examples

Verified patterns from in-repo sources:

### Composable shape (mirrors useContinueWatching)

```typescript
// Source: frontend/web/src/composables/useContinueWatching.ts:33-80
// (project's most direct precedent for "fetch on mount, expose refs, silent error")
export function useContinueWatching(limit = 10) {
  const items = ref<ContinueWatchingItem[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const auth = useAuthStore()

  async function fetchItems() {
    if (!auth.token) { items.value = []; return }
    isLoading.value = true
    error.value = null
    try {
      const res = await userApi.getContinueWatching(limit)
      const data = (res.data?.data ?? res.data) as ContinueWatchingItem[]
      items.value = Array.isArray(data) ? data : []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'failed to load continue-watching'
      items.value = []
    } finally {
      isLoading.value = false
    }
  }

  onMounted(fetchItems)
  return { items, isLoading, error, refresh: fetchItems }
}
```

### Reduced-motion via useMediaQuery (in-repo precedent)

```typescript
// Source: frontend/web/src/components/carousel/Carousel.vue:124
import { useMediaQuery } from '@vueuse/core'
const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')

// Source: frontend/web/src/components/hero/Hero.vue:124
const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
```

### Feature flag check (in-repo precedent)

```typescript
// Source: frontend/web/src/App.vue:92
const notifEnabled =
  (import.meta.env.VITE_NOTIFICATIONS_ENABLED as string | undefined) !== 'false'

// Source: frontend/web/src/views/Anime.vue:1041
const ourEnglishEnabled = import.meta.env.VITE_OURENGLISH_ENABLED !== 'false'
```

### Playwright auth + e2e shape

```typescript
// Source: frontend/web/e2e/notifications.spec.ts:33-69
// Login pattern using ui_audit_bot — for Phase 2 the block is anonymous,
// so we mostly use the simpler `await page.goto('/')` pattern from home.spec.ts:5
async function loginAs(page, request, username, password) {
  const resp = await request.post('/api/auth/login', {
    data: { username, password },
  })
  const body = await resp.json()
  const data = body?.data ?? body
  await page.addInitScript(({ tok, usr }) => {
    window.localStorage.setItem('token', tok)
    window.localStorage.setItem('user', JSON.stringify(usr))
  }, { tok: data.access_token, usr: data.user })
}
```

### Skeleton pattern (in-repo precedent)

```html
<!-- Source: frontend/web/src/components/home/ContinueWatchingRow.vue:59-68 -->
<div v-else-if="isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
  <div class="h-8 w-48 bg-white/10 rounded animate-pulse mb-4" />
  <div class="flex gap-3 overflow-hidden">
    <div
      v-for="i in 6"
      :key="i"
      class="flex-shrink-0 w-32 md:w-40 lg:w-48 aspect-[2/3] bg-white/10 rounded-xl animate-pulse"
    />
  </div>
</div>
```

For the spotlight skeleton (single full-block shimmer), the precedent is `.skeleton-shimmer` from `main.css:299`:

```css
/* Source: frontend/web/src/styles/main.css:299-308 — already in repo */
.skeleton-shimmer {
  background: linear-gradient(
    90deg,
    rgba(255, 255, 255, 0.05) 25%,
    rgba(255, 255, 255, 0.1) 50%,
    rgba(255, 255, 255, 0.05) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `matchMedia.addListener(fn)` | `matchMedia.addEventListener('change', fn)` | Spec deprecation 2018; widely supported since Safari 14 (Sep 2020) | `@vueuse/core`'s `useMediaQuery` handles this — no manual care needed |
| Raw `setInterval` with bespoke cleanup | `useIntervalFn` from `@vueuse/core` | `@vueuse/core` 5.x+ (2021) | Auto-cleanup, ergonomic `.pause()`/`.resume()` API |
| Options API mixins for media-query state | Composables (`useMediaQuery`) | Vue 3.0 (Sep 2020) | Project is fully on `<script setup lang="ts">`; no mixins in repo |
| `npm install axe-core` + manual `axe.run()` inside Playwright | `@axe-core/playwright` package | `@axe-core/playwright` 4.x (2023) | Cleaner integration: `await new AxeBuilder({ page }).analyze()` returns structured violations |

**Deprecated/outdated:**
- `MediaQueryList.addListener()` — replaced by `.addEventListener('change', ...)`. Not used in repo; `useMediaQuery` shields us.
- jQuery-style "click outside" handlers — replaced by `@vueuse/core`'s `onClickOutside` (in use at `NotificationBell.vue:60`).
- The UI/UX audit framework's CDN-injection of axe-core is appropriate for ad-hoc audits but NOT for repeatable CI gates — those should use the npm package.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The Phase 1 `/api/home/spotlight` response envelope is `{ success, data: {cards, generated_at} }` OR `{ cards, generated_at }` — `useSpotlight` unwraps both defensively | Code Examples → Composable | Low. If the actual shape diverges (e.g. `payload` instead of `data`), the unwrap chain handles it; otherwise a one-line type-file update fixes it. **Mitigate:** planner curls the endpoint before sealing types. |
| A2 | Each card has a unique top-level `id` field separate from the underlying anime ID | Pitfall 10 | Medium. If backend returns the anime UUID as the card ID directly, two cards could share a key and the `<transition>` breaks. **Mitigate:** verify via `curl http://localhost:8000/api/home/spotlight | jq '.cards[].id'`. If absent, fall back to `${type}:${index}` for Vue `:key`. |
| A3 | The `data.anime` shape matches the project's existing `Anime` model snake_case fields (`poster_url`, `name_ru`, `score`, `episodes`) | Code Examples → Discriminated Union | Medium. Cards may use a trimmed subset (e.g. `episodes_count` instead of `episodes`). **Mitigate:** copy field names from actual response, don't guess from existing types. |
| A4 | The `LatestNewsCard` `data.entries` come from `changelog.json` fields (id, date, title, summary) | Discriminated Union → ChangelogEntry | Low-Medium. The Phase 1 `latest_news` resolver pulls from `web:80/changelog.json`. If that file has different keys (e.g. `body` instead of `summary`), the type needs alignment. **Mitigate:** open `frontend/web/public/changelog.json` and confirm fields. |
| A5 | The Phase 1 backend returns `eligible=false` cards stripped from the array (per HSB-BE-05) so the frontend only sees eligible cards | "Architecture Patterns / Carousel State Machine" | Low. CONTEXT.md treats this as locked decision; HSB-BE-05 says "excluded from the response payload entirely." |
| A6 | The site does not currently use any 3rd-party carousel library | "Alternatives Considered" + Don't Hand-Roll | Negligible — verified by grepping `package.json` for embla/swiper/glide/keen-slider — all absent. |
| A7 | `axe-core` is acceptable to add as a devDependency | Standard Stack → Supporting | Low. It's a test-only dep (~600KB). Project already trusts npm registry; the existing UI/UX audit framework also uses axe-core (loaded via CDN in production-side audits per CLAUDE.md). **Mitigate:** alternatively use the CDN-injection pattern from the UI/UX audit framework — slower setup, no install. |
| A8 | The 4 frontend success criteria #5 (random initial slide ~75% of 10 reloads) is statistical and may flake intermittently | Validation Architecture → Sampling Rate | Medium. Probability of all 10 reloads landing on the same card with 4 cards is (0.25)^9 ≈ 0.0004%, so test is essentially deterministic. But CI flakiness is non-zero — recommend allow ≤2 same-initial within 10. |
| A9 | The project's TypeScript config supports discriminated unions with `type` discriminator | Discriminated Union pattern | None — TS 5.4 (package.json:50) has full support. Verified. |
| A10 | The `/changelog` route exists in Vue Router | Card 2 link target | Low. CLAUDE.md says `LastUpdates.vue` consumes `changelog.json`; corresponding route should exist. **Mitigate:** planner greps `router/` for `'/changelog'`. |

## Open Questions

1. **What is the exact JSON envelope from `GET /api/home/spotlight`?**
   - What we know: Phase 1 SPEC says `{cards: SpotlightCard[], generated_at: string}` per HSB-BE-01.
   - What's unclear: Whether wrapped in the standard `{success, data: ...}` envelope (precedent: most catalog routes use this — see `useContinueWatching.ts:50` `res.data?.data ?? res.data`).
   - Recommendation: Planner runs `curl -s http://localhost:8000/api/home/spotlight | jq` and pastes the actual shape into a Task action comment before sealing `types/spotlight.ts`.

2. **Where does `LatestNewsCard` link — `/changelog` route or `LastUpdates` view?**
   - What we know: CONTEXT.md says "Links to `/changelog` (full LastUpdates view)". UI-SPEC §LatestNewsCard markup uses `to="/changelog"`.
   - What's unclear: Whether `/changelog` is a route name or a path. CLAUDE.md references `LastUpdates.vue` consuming `changelog.json` — but the route path isn't documented in the .md files I read.
   - Recommendation: Plan should grep `frontend/web/src/router/` for `LastUpdates` or `changelog`; if the path differs from `/changelog`, adjust the link.

3. **Should `useSpotlight` re-fetch on auth-state change like `useContinueWatching` does?**
   - What we know: Phase 2 cards are all anonymous-eligible. CONTEXT.md says "No auto-refresh in Phase 2; Phase 3's live cards will re-fetch every 30s."
   - What's unclear: Whether a user signing in mid-session should re-fetch (Phase 3 cards become eligible).
   - Recommendation: Skip auth-watcher in Phase 2 (per CONTEXT.md). Phase 3 will add it together with the 30s interval. Document this decision in `useSpotlight.ts` JSDoc so Phase 3 finds the seam.

4. **What is the exact aria-label for the active slide?**
   - What we know: UI-SPEC §Accessibility Contract has the i18n key `spotlight.slideLabelWithTitle` → "Slide {n} of {total}: {title}".
   - What's unclear: For `LatestNewsCard` and `PlatformStatsCard`, the "title" is the card type's localized header ("What's new" / "Platform this week") rather than an anime title — this is fine but worth confirming.
   - Recommendation: UI-SPEC explicitly resolves `cardTitle(active)` to the section title for those two cards. Use that table verbatim.

5. **`bun run build` invokes `vue-tsc` — does that catch all type errors before Vite bundle?**
   - What we know: `package.json:8` says `"build": "vue-tsc && vite build"`.
   - What's unclear: Whether `vue-tsc` is properly configured to type-check `.vue` `<script setup>` blocks for the new components.
   - Recommendation: After scaffolding `HeroSpotlightBlock.vue`, run `bunx tsc --noEmit` AND `bun run build` separately; both should pass before the per-task verify.

## Environment Availability

This phase is purely frontend code; all required tooling is already in the dev environment (the operator runs `make dev` which spins up the Docker stack including `web`). External dependencies:

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `bun` (>=1.0) | Install + scripts | ✓ (assumed per CLAUDE.md) | — | None — required by project |
| `bunx` | Run CLI tools (tsc, eslint, playwright) | ✓ | — | `npx` (project forbids) |
| `make redeploy-web` | Deploy after change | ✓ | — | `docker compose -f docker/docker-compose.yml up -d --build web` (raw form) |
| `/api/home/spotlight` (Phase 1) | Block data | ✓ (Phase 1 shipped per ROADMAP.md) | — | None — Phase 2 depends on it |
| Playwright browser binaries | E2E tests | ⚠️ — `playwright` JS dep is installed (package.json:46), but `playwright install` for browsers may need running once | 1.58.0 | `bunx playwright install chromium firefox` |
| `@axe-core/playwright` | A11y e2e gate | ✗ — NOT installed | — | Plan installs via `bun add -d @axe-core/playwright` |
| Frontend dev server (port 5173) for Playwright `webServer` block | E2E tests | ⚠️ — `playwright.config.ts:42` says `command: 'npm run dev'`, which conflicts with the project's bun rule | — | Manually override: `BASE_URL=http://localhost:5173 bunx playwright test spotlight` after starting `bun run dev` separately. **Note:** this is a minor existing oddity in the config (uses `npm`); irrelevant if running against the deployed `web` container at `http://localhost:80` |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** `@axe-core/playwright` — install command is part of the plan's first task.

## Validation Architecture

> Nyquist validation enabled per `.planning/config.json` `workflow.nyquist_validation: true`.

### Test Framework

| Property | Value |
|----------|-------|
| Unit framework | Vitest 4.1.6 (configured `vitest.config.ts:9-18` — jsdom env, includes `src/**/*.spec.ts`) |
| Component test helper | `@vue/test-utils@2.4.10` (devDep) |
| E2E framework | Playwright 1.58.0 (configured `playwright.config.ts` — chromium + firefox + Mobile Chrome projects, `testDir: './e2e'`) |
| A11y framework | `@axe-core/playwright@4.11.3` (**TO INSTALL — first task action**) |
| Config files | `vitest.config.ts`, `playwright.config.ts` (already exist) |
| Quick run command | `cd frontend/web && bunx vitest run src/components/home/spotlight && bunx playwright test spotlight` |
| Full suite command | `cd frontend/web && bunx tsc --noEmit && bunx eslint src/ && bunx vitest run && bunx playwright test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HSB-FE-01 | Block mounts in Home.vue between SystemStatusBanner and trendingRecs | E2E (DOM order) | `bunx playwright test spotlight -g "mounts above trendingRecs"` | ❌ Wave 0 — new file |
| HSB-FE-02 | Fetches on mount; hides on error/empty | E2E (route mock + assertion) | `bunx playwright test spotlight -g "empty cards hides block"` | ❌ Wave 0 |
| HSB-FE-03 | 7s auto-cycle; pauses on mouseenter/focusin | E2E (wait + index assert; hover; wait again) | `bunx playwright test spotlight -g "auto-cycles every 7s"` and `-g "pauses on hover"` | ❌ Wave 0 |
| HSB-FE-04 | Manual nav — chevrons, dots, ArrowLeft/Right | E2E (click + key + index assert) | `bunx playwright test spotlight -g "manual navigation"` | ❌ Wave 0 |
| HSB-FE-05 | Random initial slide on every reload (≥75% variety in 10 reloads with 4 cards) | E2E (loop 10x reload, collect initial dot index) | `bunx playwright test spotlight -g "random initial slide"` | ❌ Wave 0 |
| HSB-FE-06 | Respects prefers-reduced-motion | E2E (emulate `reduced-motion: reduce`; wait 8s; index unchanged) | `bunx playwright test spotlight -g "reduced motion"` | ❌ Wave 0 |
| HSB-FE-07 | A11y semantics (role, aria-roledescription, aria-live, aria-label) | E2E + axe-core | `bunx playwright test spotlight -g "a11y violations zero"` | ❌ Wave 0 |
| HSB-FE-08 | Skeleton placeholder matches final height | E2E (page height delta ≤ 4px from skeleton→loaded) | `bunx playwright test spotlight -g "no layout shift"` | ❌ Wave 0 |
| HSB-FE-09 | Feature flag off → block absent + trendingRecs visible | E2E (override env at dev-server start) OR Vitest with stubbed env | `bunx playwright test spotlight -g "feature flag off"` (requires separate test run with `VITE_HERO_SPOTLIGHT_ENABLED=false bun run dev`) | ❌ Wave 0 |
| HSB-FE-20 | AnimeOfDayCard renders poster, title, score, CTAs | Vitest component test | `bunx vitest run src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-21 | LatestNewsCard renders 3 entries + readMore link | Vitest component test | `bunx vitest run src/components/home/spotlight/cards/LatestNewsCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-22 | PlatformStatsCard adaptive 1/2/3 columns | Vitest component test | `bunx vitest run src/components/home/spotlight/cards/PlatformStatsCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-23 | RandomTailCard renders single CTA + cyan-300 eyebrow | Vitest component test | `bunx vitest run src/components/home/spotlight/cards/RandomTailCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-40 | All `spotlight.*` keys exist in en.json + ru.json | Vitest (read JSON, assert paths) OR E2E lint (`@intlify/vue-i18n` ESLint rule already catches raw text; lint pass suffices) | `bunx eslint src/components/home/spotlight` + `bunx vitest run src/locales/spotlight-keys.spec.ts` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `bunx tsc --noEmit && bunx eslint src/components/home/spotlight src/composables/useSpotlight.ts src/types/spotlight.ts` — fast (<10s)
- **Per wave merge:** `bunx vitest run src/components/home/spotlight src/composables/useSpotlight.spec.ts` (component + composable unit tests; <30s)
- **Phase gate (before `/gsd-verify-work`):** Full suite green:
  ```bash
  cd frontend/web
  bunx tsc --noEmit
  bunx eslint src/
  bunx vitest run
  bunx playwright test spotlight
  bun run build  # verifies vue-tsc + vite build also pass
  ```
  Plus manual verification of success criterion #5 (10 reloads variety) and #6 (devtools reduced-motion emulation).

### Wave 0 Gaps

The phase introduces an entirely new test surface — no existing tests cover spotlight code. Wave 0 must include scaffolding:

- [ ] Install `@axe-core/playwright`: `bun add -d @axe-core/playwright` (from `frontend/web/`)
- [ ] `frontend/web/e2e/spotlight.spec.ts` — Playwright spec covering 9 success criteria + axe assertion. ~250 LOC. Reuses login/init pattern from `e2e/notifications.spec.ts:33` but anonymous-only for Phase 2.
- [ ] `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts` — Vitest mounts the card with mock data, asserts: title rendered, score chip visible, both CTAs visible, i18n keys not raw text. ~40 LOC.
- [ ] `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.spec.ts` — same pattern, 3 entries rendered, readMore link → `/changelog`. ~40 LOC.
- [ ] `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts` — same pattern, adaptive grid via `:class` binding tested at metrics.length = 1, 2, 3. ~50 LOC.
- [ ] `frontend/web/src/components/home/spotlight/cards/RandomTailCard.spec.ts` — same pattern, single CTA, cyan-300 eyebrow class present. ~40 LOC.
- [ ] `frontend/web/src/composables/useSpotlight.spec.ts` — Vitest mocks `apiClient`, asserts fetch on mount + error → empty cards + loading transitions. ~60 LOC.
- [ ] `frontend/web/src/locales/spotlight-keys.spec.ts` — Vitest reads both `en.json` and `ru.json`, asserts every key listed in UI-SPEC §Copywriting Contract exists and resolves to a non-empty string. ~30 LOC. (Prevents drift if the planner adds a new card but forgets the RU translation.)
- [ ] No new framework install needed — vitest, @vue/test-utils, playwright, jsdom all present.

**Component-level test budget:** ~40-60 LOC per card spec, focused on render correctness + i18n key resolution. Behavioral assertions (state machine, auto-cycle timing, a11y) live in the Playwright spec because they need real browser timing and a real `prefers-reduced-motion` emulation.

## Sources

### Primary (HIGH confidence)

In-repo source files (all paths under `/data/animeenigma/`):

- `frontend/web/package.json` — dependency versions verified (axios 1.13.6, @vueuse/core 14.1.0, vue-i18n 11.2.8, vitest 4.1.6, @playwright/test 1.58.0, jsdom 29.1.1, @vue/test-utils 2.4.10, @intlify/eslint-plugin-vue-i18n 4.3.0)
- `frontend/web/src/api/client.ts` — `apiClient` axios instance + JWT refresh interceptors (lines 1-244)
- `frontend/web/src/composables/useAnime.ts` — composable shape pattern (lines 99-275)
- `frontend/web/src/composables/useContinueWatching.ts` — closest analog for fetch-on-mount composable (lines 33-80)
- `frontend/web/src/components/home/CollectionsRow.vue` — Tailwind block container pattern (`max-w-7xl mx-auto px-4 lg:px-8 mb-8`)
- `frontend/web/src/components/home/ContinueWatchingRow.vue` — skeleton + score-chip + hover-cyan patterns
- `frontend/web/src/components/carousel/Carousel.vue` — `useMediaQuery('(prefers-reduced-motion: reduce)')` precedent (line 124)
- `frontend/web/src/components/hero/Hero.vue` — same precedent (line 124)
- `frontend/web/src/views/Home.vue` — mount-point context (lines 1-160 read; SystemStatusBanner at line 8, trendingRecs at line 39)
- `frontend/web/src/styles/main.css` — verified utility class definitions (`glass-card:110`, `:focus-visible:91`, `btn-primary:162`, `btn-ghost:206`, `touch-target:236`, `skeleton-shimmer:299`, reduced-motion override:312)
- `frontend/web/src/App.vue` — feature flag precedent (`!== 'false'` semantics at line 92)
- `frontend/web/vitest.config.ts` — confirms Vitest configured for `src/**/*.spec.ts` co-location
- `frontend/web/playwright.config.ts` — confirms e2e dir is `./e2e`, projects chromium/firefox/Mobile Chrome
- `frontend/web/e2e/notifications.spec.ts` — Playwright auth + feature-flag spec precedent
- `frontend/web/e2e/home.spec.ts` — Playwright Home view spec precedent
- `frontend/web/e2e/accessibility.spec.ts` — simple a11y assertions; not axe-based — confirms need for `@axe-core/playwright`
- `frontend/web/.env` + `.env.example` — confirms env layout (NO `.env.development` or `.env.production.example`)
- `docker/.env.example` line 348-352 — confirms backend `SPOTLIGHT_ENABLED` already wired in Phase 1
- `CLAUDE.md` — project conventions (bun, bunx, make redeploy-web, after-update skill)
- `.planning/workstreams/hero-spotlight/REQUIREMENTS.md` — phase requirements HSB-FE-01..09, HSB-FE-20..23, HSB-FE-40
- `.planning/workstreams/hero-spotlight/ROADMAP.md` — Phase 2 success criteria 1-10
- `.planning/workstreams/hero-spotlight/phases/02-frontend-carousel/02-CONTEXT.md` — locked decisions
- `.planning/workstreams/hero-spotlight/phases/02-frontend-carousel/02-UI-SPEC.md` — 6/6 approved design contract
- `.planning/config.json` — confirms `nyquist_validation: true`, `commit_docs: true`

### Secondary (MEDIUM confidence)

- npm registry queries via `npm view`:
  - `npm view axios version` → 1.16.1 (latest as of 2026-05-21)
  - `npm view @vueuse/core version` → 14.3.0
  - `npm view vitest version` → 4.1.7
  - `npm view @playwright/test version` → 1.60.0
  - `npm view @axe-core/playwright version` → 4.11.3

### Tertiary (LOW confidence — verified-once)

- W3C ARIA APG carousel pattern referenced verbatim in UI-SPEC §Accessibility Contract: `https://www.w3.org/WAI/ARIA/apg/patterns/carousel/examples/carousel-1-prev-next/` — not re-fetched in this research; UI-checker already validated.
- `@vueuse/core` `useIntervalFn` docs: `https://vueuse.org/shared/useIntervalFn/` — referenced via training knowledge; planner should consult the live docs if signature uncertainty arises.

## Metadata

**Confidence breakdown:**

- Standard stack: **HIGH** — every library version verified against `package.json`; every recommended pattern has at least one in-repo precedent. The single addition (`@axe-core/playwright`) is a small, well-known test-only dep.
- Architecture: **HIGH** — state machine, composable, types, and a11y markup all derive from UI-SPEC.md (already checker-approved) and in-repo analogs. The two architectural calls beyond CONTEXT.md (use `useIntervalFn`; skip the `useReducedMotion` wrapper) are justified by direct in-repo precedent.
- Pitfalls: **HIGH** — 10 enumerated pitfalls each pin to a specific code path, file:line, or in-spec rule. Test/observation strategies are concrete (e.g. "reload 10x; observe initial dot").
- Test plan: **HIGH** — every requirement HSB-FE-01..40 maps to a named test, with command + file path + LOC budget. Wave 0 gaps are listed and bounded.
- Environment availability: **HIGH** — single missing piece (`@axe-core/playwright`) is an `bun add -d`; everything else is in `package.json` or shipped with the dev environment.

**Research date:** 2026-05-21
**Valid until:** 2026-06-20 (30 days) for stable frontend stack. Bump immediately if `@vueuse/core` major version changes (>14.x) or if Phase 1 ships a different JSON envelope.
