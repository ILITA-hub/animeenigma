---
phase: 02-frontend-carousel
plan: 04
workstream: hero-spotlight
subsystem: frontend
tags:
  - frontend
  - vue3
  - state-machine
  - a11y
  - hero-spotlight
dependency_graph:
  requires:
    - 02-01 (frontend/web/src/composables/useSpotlight.ts — fetch composable)
    - 02-01 (frontend/web/src/types/spotlight.ts — SpotlightCard discriminated union)
    - 02-02 (frontend/web/src/components/home/spotlight/CarouselControls.vue — chevrons + dots)
    - 02-03 (frontend/web/src/components/home/spotlight/cards/*.vue — 4 card components)
  provides:
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue
    - frontend/web/src/styles/main.css (.spotlight-fade-* cross-fade keyframes appended)
  affects:
    - 02-06 (Home.vue integration — will import + mount <HeroSpotlightBlock />)
tech_stack:
  added: []
  patterns:
    - "@vueuse/core useIntervalFn (auto-cleanup on unmount + HMR)"
    - "@vueuse/core useMediaQuery('(prefers-reduced-motion: reduce)') — established repo idiom"
    - "Vue 3 <transition name=...> with conditional 'none' for reduced-motion"
    - "watch(() => cards.value.length) seeds random initial index AFTER fetch resolves (Pitfall 4 fix)"
    - "requestAnimationFrame-deferred focusout handler (Pitfall 7 fix)"
    - "Discriminated-union dispatch via CARD_MAP[card.type] → :is binding"
    - "Vitest fake timers (vi.useFakeTimers + vi.advanceTimersByTime) for 7000ms cycle assertion"
key_files:
  created:
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue (243 lines)
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts (354 lines)
  modified:
    - frontend/web/src/styles/main.css (+11 lines — .spotlight-fade-* keyframes block appended)
decisions:
  - "Used inline useMediaQuery (RESEARCH.md Pattern 2) — did NOT create a useReducedMotion.ts wrapper; matches Hero.vue:130 and Carousel.vue:124 precedent"
  - "Used useIntervalFn over raw setInterval (RESEARCH.md Pattern 3) — auto-cleanup on unmount eliminates leak class; immediate:false defers first tick until after random-init watch fires"
  - "watch(() => cards.value.length) with {immediate:false} + initialized boolean — random seed runs exactly once per mount, even if Phase 3 introduces refresh() calls"
  - "Vue key on slide <component> is `${active.type}:${currentIndex}` — backend Phase 1 does NOT emit per-card envelope id (RESEARCH.md Pitfall 10 + types/spotlight.ts comment)"
  - "Slide aria-label uses spotlight.slideLabelWithTitle (n, total, title) — matches the en.json key created by Plan 02-05; cardTitle() resolves anime via getLocalizedTitle, multi-item cards via card-level title key"
  - "Spec reads aria-label via attributes() (unescaped) not via html() (which encodes \" as &quot;) — captured as a helper inside the spec"
metrics:
  duration: "~35 minutes"
  ux_delta: "+4 (Better) — interactive carousel block now ships; auto-cycle/pause/keyboard/reduced-motion behavior present"
  cdi: "0.05 * 13"
  mvq: "Phoenix 90%/87%"
  completed: "2026-05-21"
---

# Phase 02 Plan 04: HeroSpotlightBlock state machine + a11y wrapper Summary

Keystone plan delivering `HeroSpotlightBlock.vue` — the outer wrapper that owns the carousel state machine, consumes Plan 02-01's `useSpotlight()` composable, mounts Plan 02-02's `CarouselControls`, dispatches Plan 02-03's 4 cards via `<component :is>`, drives a 7-second auto-cycle with hover/focus pause and reduced-motion gating, randomizes the initial slide after fetch resolves, and exposes APG-compliant carousel a11y semantics. 12 Vitest state-machine assertions cover skeleton/loaded/hidden branches, random init, fake-timer-driven auto-advance, wraparound, reduced-motion, single-card no-cycle, and aria-live placement.

## What Was Built

**`HeroSpotlightBlock.vue`** — 243-line single-file component with three top-level template branches:

1. **Skeleton** (`enabled && loading`) — `aria-hidden="true"` wrapper, `.skeleton-shimmer` body, same `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]` heights as the loaded state to prevent CLS.
2. **Loaded** (`enabled && cards.length > 0 && active`) — `<section role="region" aria-roledescription="carousel" tabindex="0">` wrapping the `.glass-card` slide container. Slide container carries `role="group"`, `aria-roledescription="slide"`, `aria-live="polite"`, `aria-atomic="true"`. Active card mounted via `<component :is="cardFor(active.type)" :key="...">` inside `<transition :name="reducedMotion ? 'none' : 'spotlight-fade'" mode="out-in">`. `<CarouselControls>` mounted absolutely-positioned inside the glass-card.
3. **Hidden** (any other state) — block renders nothing (silent self-hide on empty/error/flag-off, per UI-SPEC §State Contract).

**State machine** (`<script setup>`):
- `currentIndex: Ref<number>` seeded by `Math.floor(Math.random() * n)` inside `watch(() => cards.value.length, ...)` so the random seed runs AFTER the async fetch resolves (Pitfall 4 fix). An `initialized` boolean ensures it runs exactly once per mount, robust to any future refresh() calls.
- `useIntervalFn(advance, 7000, { immediate: false })` from `@vueuse/core` — auto-cleans on unmount/HMR; controlled via `pause()` / `resume()`.
- `startCycle()` is a no-op when `reducedMotion.value` OR `cards.value.length <= 1`.
- `next()` / `prev()` / `goTo(i)` wrap-around via `(currentIndex + n ± 1) % n` then `restart()` (pause + startCycle) so manual seek resets the 7s clock.
- `mouseenter` / `focusin` → `stopCycle()`; `mouseleave` → `startCycle()`; `focusout` → `onFocusOut()` which defers via `requestAnimationFrame` then checks `document.activeElement` containment before resuming (Pitfall 7 fix prevents flicker when Tab moves between dot buttons).
- `@keydown.left` → `prev()`, `@keydown.right` → `next()` bound on the wrapper (which has `tabindex="0"`).
- `watch(reducedMotion, ...)` honors runtime OS-level toggles — if the user enables reduced-motion mid-session the cycle pauses immediately.

**`main.css` (+11 lines)** — appended cross-fade keyframes block:
```css
/* Spotlight (Phase 02) — cross-fade between carousel slides.
   The "none" transition name short-circuits Vue's transition entirely
   when prefers-reduced-motion is on. */
.spotlight-fade-enter-active,
.spotlight-fade-leave-active {
  transition: opacity 0.4s ease;
}
.spotlight-fade-enter-from,
.spotlight-fade-leave-to {
  opacity: 0;
}
```

**`HeroSpotlightBlock.spec.ts`** — 354 lines, 12 `it()` blocks, all passing. Covers:

| # | Test | Mechanism |
|---|------|-----------|
| 1 | renders skeleton when loading=true | DOM query for `.skeleton-shimmer` |
| 2 | renders section with role=region when cards populate | mounts then sets `cards.value = mockCards(4)` after mount so the SFC's watch fires |
| 3 | does NOT render section when cards.length===0 after load | DOM negation — no `<div>` or `<section>` rendered |
| 4 | does NOT render section when feature flag is "false" | `vi.stubEnv('VITE_HERO_SPOTLIGHT_ENABLED', 'false')` + `vi.resetModules()` + dynamic re-import |
| 5 | randomizes currentIndex after cards populate (statistical) | 30 trials × 4 cards; asserts ≥2 distinct indexes observed |
| 6 | advances currentIndex by 1 every 7000ms | `vi.useFakeTimers()` + `vi.advanceTimersByTime(7000)`; verifies wraparound `(initial+1)%4` |
| 7 | wraps around from last → first via next chevron | clicks `[aria-label="spotlight.nextSlide"]` until `prev===2 && current===0` |
| 8 | does NOT advance when reducedMotion=true | sets `mockReducedMotion.value = true` before mount; 8000ms tick leaves index unchanged |
| 9 | does NOT advance when cards.length===1 | single dot button rendered; 8000ms tick leaves index unchanged |
| 10 | aria-live=polite only on slide container, NOT on section | asserts `section.attributes('aria-live')` is undefined |
| 11 | slide aria-label uses spotlight.slideLabelWithTitle with n/total | parses attribute (unescaped) for `"total":3` + `"n":1..3` |
| 12 | renders without console.error for unknown card type | mounts with `{type: 'unknown'}`; asserts `console.error` spy not called |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Test Bug] `readActiveIndex` helper signature changed**
- **Found during:** Task 2 — first vitest run
- **Issue:** Original implementation read the slide's aria-label via `wrapper.html()` which HTML-encodes `"` as `&quot;` (the rendered attribute value is `"{&quot;n&quot;:1,..."`), so the regex `/"n":(\d+)/` never matched. Tests reported `initial=-1`, breaking the auto-advance and random-init assertions.
- **Fix:** Helper now takes the wrapper directly and reads `slide.attributes('aria-label')` which returns the **unescaped** string. Regex unchanged but now operates on `{"n":1,...}`-form input.
- **Files modified:** `HeroSpotlightBlock.spec.ts`
- **Commit:** 4738538

**2. [Rule 1 — Test Bug] Random-init / advance tests had cards pre-populated before mount**
- **Found during:** Task 2 — second vitest run after fix #1
- **Issue:** The SFC's `watch(() => cards.value.length, ..., { immediate: false })` only fires when length transitions. Tests that set `mockState.cards.value = mockCards(4)` BEFORE mount and re-used the shared mockState across mounts within a single test (the statistical random-init test mounts 30 times) never saw the 0→N transition on mounts 2..30, so `initialized` stayed false, `startCycle()` was never called, and `currentIndex` stayed 0 forever. Random-init test saw only 1 distinct value; advance test saw `next=0` instead of `(initial+1)%4`.
- **Fix:** Each affected test now sets `mockState.cards.value = []` BEFORE mount, then `mockState.cards.value = mockCards(N)` AFTER mount + `flushPromises()` so the watch fires on every trial. The behavior under real conditions is identical (cards are always empty until the async fetch resolves, which happens after mount).
- **Files modified:** `HeroSpotlightBlock.spec.ts` (3 it() blocks)
- **Commit:** 4738538

**3. [Plan adaptation — not a deviation per se] Vue key on slide `<component>`**
- **Plan said:** `:key="active.id"`.
- **What was shipped:** `:key="${active.type}:${currentIndex}"`.
- **Why:** `types/spotlight.ts` from Plan 02-01 explicitly notes the backend Phase 1 envelope has NO per-card `id` field (verified via `curl` of the live endpoint). RESEARCH.md Pitfall 10 specifies the `${type}:${index}` fallback exactly. Using `active.id` would have produced `:key="undefined"` for every slide, defeating Vue's keyed-transition optimization. This is consistent with the locked Plan 02-01 contract — no plan-level deviation, just honoring the actual runtime shape.

## Auth Gates

None — Phase 2 is anonymous-only frontend work; no auth surface touched.

## Verification

```
cd frontend/web
bunx tsc --noEmit                                       # exit 0
bunx eslint src/components/home/spotlight/              # exit 0
bunx vitest run src/components/home/spotlight/ src/composables/useSpotlight.spec.ts
                                                        # 7 test files, 57 tests, all pass
```

All Plan 02-04 acceptance grep gates pass:
- `useIntervalFn` present; no raw `setInterval(`
- `useMediaQuery.*prefers-reduced-motion` present
- `VITE_HERO_SPOTLIGHT_ENABLED` + `!== 'false'` present
- `role="region"` + `aria-roledescription="carousel"` + `aria-roledescription="slide"` + `aria-live="polite"` present; **aria-live count = 1** (slide container only, NOT on section)
- `min-h-[400px] md:min-h-[340px] lg:min-h-[320px]` present (exact)
- `spotlight.regionLabel` + `spotlight.slideLabelWithTitle` keys referenced
- `Math.floor(Math.random()` + `requestAnimationFrame` + `watch(.*cards.value.length` + `cards.value.length <= 1` all present
- No `font-bold` / `font-normal` (typography gate)
- `@keydown.left` + `@keydown.right` present
- `.spotlight-fade-enter-active` appended to `main.css`

Spec acceptance:
- 12 `it()` blocks (≥9 required)
- `vi.useFakeTimers` + `vi.advanceTimersByTime(7000)` + `useMediaQuery` mock all present

## Known Stubs

None. All data flows are wired:
- `useSpotlight()` from Plan 02-01 (fetches `GET /api/home/spotlight`).
- Cards from Plan 02-03 render real `data` from the backend.
- i18n keys from Plan 02-05 cover all aria-labels and titles.

## Self-Check: PASSED

Created files:
- `/data/animeenigma/frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` — FOUND
- `/data/animeenigma/frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts` — FOUND

Modified files:
- `/data/animeenigma/frontend/web/src/styles/main.css` — `.spotlight-fade-enter-active` rule FOUND

Commits:
- `f4a2cb2` — FOUND (`feat(02-04): HeroSpotlightBlock state machine + a11y wrapper`)
- `4738538` — FOUND (`test(02-04): HeroSpotlightBlock state machine spec — 12 it() blocks`)
