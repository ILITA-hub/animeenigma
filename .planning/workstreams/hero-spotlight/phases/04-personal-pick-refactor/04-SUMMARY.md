---
phase: 04-personal-pick-refactor
plan: 04
workstream: hero-spotlight
milestone: v1.1-polish
subsystem: ui
tags: [vue3, pinia, tailwind, i18n, hero-spotlight, personalization]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: SpotlightBackdrop, SpotlightIcon, cta-card class, transition lock
provides:
  - Two-zone featured + secondary PersonalPickCard layout
  - Username-personalized title (titleWithName) across en/ru/ja
  - Per-item reason chip (sparkles icon + bg-cyan-500/20)
  - Mobile full-width cta-card "+ N more →" footer button
  - Single-root-element discipline for spotlight cards (transition compatibility)
affects:
  - 05-now-watching-refactor (template root-element pattern)
  - 06-telegram-news-refactor (template root-element pattern)
  - 07-latest-news-refactor (template root-element pattern)
  - 08-platform-stats-refactor (template root-element pattern)
  - 09-not-time-yet-refactor (template root-element pattern)
  - 10-continue-watching-new-refactor (template root-element pattern)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Single root <article> with internal v-if (never top-level v-if on root)"
    - "useAuthStore for username-personalized titles in spotlight cards"
    - "grid-cols-[3fr_2fr] for featured + secondary card layouts"
    - "Pinia setup in vitest specs that mount cards using stores (createPinia + setActivePinia)"
    - "createI18n stubbing in vue-i18n mocks for tests that load @/i18n indirectly"

key-files:
  created:
    - .planning/workstreams/hero-spotlight/phases/04-personal-pick-refactor/04-SUMMARY.md
    - .planning/workstreams/hero-spotlight/phases/04-personal-pick-refactor/deferred-items.md
  modified:
    - frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue
    - frontend/web/src/components/home/spotlight/cards/PersonalPickCard.spec.ts
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts

key-decisions:
  - "Title precedence: anon → titleAnon; personal + username → titleWithName; personal w/o username → title (graceful fallback for SSR / unhydrated store)"
  - "Single-root <article> with internal <template v-if='featured'> — never put v-if on the root, never put DOM comments at template root level (Vue 3 + Transition mode='out-in' wedges otherwise)"
  - "Featured = items[0], secondary = items.slice(1, 3) (max 2 stacked rows on desktop)"
  - "Mobile footer router-link uses absolute bottom-3 + cta-card classes (full-width glass-pill) instead of a small corner link"
  - "Reason chip uses bg-cyan-500/20 + text-cyan-200 + sparkles icon, self-start (left-aligned chip not full width)"
  - "Move SFC documentation from template-level <!-- comment --> to a JSDoc block inside <script setup> to avoid multi-root templates"

patterns-established:
  - "Spotlight card root must be a SINGLE element node — never start with a doc comment, never wrap in v-if"
  - "When a card SFC imports a Pinia store, vitest specs for the carousel (HeroSpotlightBlock.spec.ts) must install Pinia in their mount helper"
  - "When a card SFC indirectly imports @/i18n (via @/stores/auth), vue-i18n mocks must also stub createI18n"

requirements-completed: [HSB-V11-PP-01, HSB-V11-PP-02, HSB-V11-PP-03, HSB-V11-PP-04]

# Metrics
metric_string: "UXΔ = +4 (Better) · CDI = 0.05 * 13 · MVQ = Kraken 88%/85%"
duration: 45min
completed: 2026-05-24
---

# Phase 04 Plan 04: PersonalPickCard refactor — featured-plus-secondary layout Summary

**Two-zone PersonalPickCard with backdrop-blurred featured pick (60% / `3fr`), stacked secondary picks (40% / `2fr`), username-personalized title across en/ru/ja, per-item reason chips, and a full-width mobile footer CTA — replacing the 3-equal-poster grid that was truncating titles.**

## Performance

- **Duration:** ~45 min
- **Started:** 2026-05-24T10:09:41Z
- **Completed:** 2026-05-24T~10:54Z
- **Tasks:** 5 plan tasks + 1 auto-fix (Rule 1 — transition-wedge bug)
- **Files modified:** 7

## Accomplishments

- **HSB-V11-PP-01 — Featured + secondary layout:** `grid-cols-[3fr_2fr]` on desktop; featured pick uses a large poster + title + reason chip + sparkles kicker; secondary picks stack to the right as compact rows. Mobile keeps featured-only.
- **HSB-V11-PP-02 — Username personalization:** Title computed from `useAuthStore()` — anon = `titleAnon` ("Trending now"), personal + username = `titleWithName` ("For you, ui_audit_bot"), personal w/o username = `title` ("Picked for you"). New i18n key added to en/ru/ja with `{name}` interpolation.
- **HSB-V11-PP-03 — Per-item reason chip:** Renders `t(item.reason_i18n_key)` in a `bg-cyan-500/20` rounded pill with a small sparkles icon, on both featured and secondary items.
- **HSB-V11-PP-04 — Mobile footer "+ N more →" button:** Full-width `cta-card` glass-pill positioned `absolute bottom-3 inset-x-3` (was a small corner text-link).
- **Bonus (Rule 1 auto-fix):** Diagnosed + fixed a Vue 3 `<Transition mode="out-in">` wedge: with `v-if` on the `<article>` root + a sibling doc-comment, the next card never mounted after navigating away from PersonalPickCard. Refactored to single-element root; documented the constraint inline.

## Task Commits

Each logical unit was committed atomically:

1. **Tasks 1–4: Layout + username + reason chip + i18n keys** — `665d671` (feat)
2. **Task 5: Spec updates + i18n parity + Pinia/createI18n test fixes** — `13ab2a7` (test)
3. **Rule 1 auto-fix: Restore single-root template to unwedge transition** — `ff31ca5` (fix)

## Files Created/Modified

### Modified
- `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue` — Full template + script refactor: two-zone grid, SpotlightBackdrop, username-personalized title, reason chip, mobile footer CTA, JSDoc with single-root warning.
- `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.spec.ts` — Rewritten: 16 assertions (was 8). Adds mocked `useAuthStore`, stubs SpotlightBackdrop + SpotlightIcon, tests featured aria-label, secondary count, username title precedence, reason chip, backdrop poster URL, typography contract.
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts` — Install Pinia + stub `createI18n` in mountBlock helper so PersonalPickCard's `useAuthStore` + `@/i18n` imports don't break the dispatch-chain tests for the 5 Phase-3 card variants.
- `frontend/web/src/locales/en.json` — Added `personalPick.titleWithName: "For you, {name}"`.
- `frontend/web/src/locales/ru.json` — Added `personalPick.titleWithName: "Для вас, {name}"`.
- `frontend/web/src/locales/ja.json` — Added `personalPick.titleWithName: "{name}さんへのおすすめ"`.
- `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` — Added `titleWithName` to the parity matrix + cross-locale `{name}` interpolation assertion across en/ru/ja.

### Created
- `.planning/workstreams/hero-spotlight/phases/04-personal-pick-refactor/04-SUMMARY.md` — this file
- `.planning/workstreams/hero-spotlight/phases/04-personal-pick-refactor/deferred-items.md` — pre-existing a11y/test failures out of scope.

## Verification

| Check | Result |
|------|--------|
| `bunx vitest run src/components/home/spotlight/cards/PersonalPickCard.spec.ts` | 16/16 ✅ |
| `bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts` | 27/27 ✅ |
| `bunx vitest run src/components/home/spotlight/ src/locales/__tests__/` | 223/223 ✅ |
| `bunx tsc --noEmit` | clean ✅ |
| `bunx eslint src/components/home/spotlight/ src/locales/` | clean ✅ |
| `bunx playwright test spotlight-full.spec.ts:168 (5 new cards render)` | passes ✅ |
| `bunx playwright test spotlight + spotlight-full + spotlight-transition-lock` | 13 passed, 2 flaky-then-pass-on-retry, 3 pre-existing failures (see Deferred) |

## Decisions Made

1. **Title precedence:** Anonymous (`source === 'trending'`) → `titleAnon`; personal + username → `titleWithName` interpolated; personal w/o username → `title`. The `auth.user?.username` short-circuit keeps the component safe in SSR / unhydrated-store contexts.
2. **Single-root template constraint:** The `<article>` is always rendered; the conditional `v-if="featured"` lives INSIDE the `<template>` block. Doc comments moved from template-level to a JSDoc inside `<script setup>` to avoid multi-root issues. **This is a hard constraint for all 9 spotlight cards** because `HeroSpotlightBlock` wraps each card in `<transition mode="out-in">` and any comment / falsy v-if at the root wedges the cross-fade.
3. **Featured + secondary slicing:** `featured = items[0]`, `secondary = items.slice(1, 3)` — max 3 items displayed, with the rest surfaced via the `+ N more →` footer.
4. **Mobile footer styling:** `cta-card` (full-width glass-pill from Phase 01) replaces the small corner text-link to give the affordance proper tap-target weight on mobile.
5. **Reason chip styling:** `bg-cyan-500/20` + `text-cyan-200` + `self-start` so the chip wraps its content (not full width) and only renders when the backend supplies `reason_i18n_key`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Transition wedge — next card never mounts after navigating away from PersonalPickCard**

- **Found during:** Verification of `spotlight-full.spec.ts:168` (each of the 5 new card types renders without crashing).
- **Issue:** The initial Phase 04 commit (`665d671`) used `<article v-if="featured">` at the top-level template root, with a leading `<!-- doc comment -->` above it. When the user navigated from PersonalPickCard (dot idx 4) to TelegramNewsCard (dot idx 5), the carousel went **blank**. The aria-label updated to the new slide, but the inner DOM rendered to `<!---->` and the next card never mounted. Auto-cycle continued mutating aria-label while the surface stayed empty.
- **Root cause:** Vue 3 warns `[Vue warn]: Component inside <Transition> renders non-element root node that cannot be animated` when the root is (a) a conditional comment from a top-level v-if, or (b) part of a multi-root template (here: comment + article). `HeroSpotlightBlock` wraps each card in `<transition mode="out-in">`, which silently wedges the cross-fade OUT phase — and `mode="out-in"` only mounts the next card AFTER OUT completes.
- **Fix:** Refactored template so the root `<article>` is always rendered; moved `v-if="featured"` to an internal `<template v-if>`. Moved SFC documentation from a template-level comment to a JSDoc block inside `<script setup>` so the SFC has exactly one root node. Added a CRITICAL comment in the JSDoc warning future refactors against reintroducing the bug.
- **Files modified:** `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue`
- **Verification:** Debug spec confirmed the slide DOM after clicking dot idx 5 now contains the full TelegramNewsCard markup (was `<!---->` before). `spotlight-full.spec.ts:168` now passes.
- **Committed in:** `ff31ca5`

**2. [Rule 1 - Bug] HeroSpotlightBlock dispatch-chain tests crash with "getActivePinia()" after the new card imports useAuthStore**

- **Found during:** Task 5 verification (`bunx vitest run src/components/home/spotlight/`).
- **Issue:** PersonalPickCard now imports `@/stores/auth` (a Pinia store). The HeroSpotlightBlock vitest helper `mountBlock()` did not install Pinia, so the existing `dispatches new card type X via v-if/v-else-if chain` tests for all 5 Phase-3 card variants started crashing with `[🍍]: "getActivePinia()" was called but there was no active Pinia.` Additionally, the `@/stores/auth` import chain loads `@/i18n` and calls `createI18n(...)` at module init, which threw `No "createI18n" export is defined on the "vue-i18n" mock`.
- **Fix:** Added `createPinia()` + `setActivePinia(pinia)` + `plugins: [pinia]` to `mountBlock()`. Stubbed `createI18n` on the existing `vi.mock('vue-i18n', ...)` block as a noop install.
- **Files modified:** `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts`
- **Verification:** `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts` — 17/17 ✅.
- **Committed in:** `13ab2a7`

---

**Total deviations:** 2 auto-fixed (both Rule 1 — bugs caused directly by this task's changes).
**Impact on plan:** Both fixes were essential for shipping. The transition-wedge bug would have been a user-visible regression (blank carousel after navigating away from personal_pick); the Pinia/createI18n stubs were needed to keep the existing test suite green. No scope creep — everything else stays exactly per plan.

## Issues Encountered

- **Pre-existing test failures (3) — out of scope.** Documented in `deferred-items.md`. Two are axe-core `heading-order` violations from the page-wide DOM (`h1 → spotlight h3 → Ongoing h2`), reproducing identically on the prior commit. One is a layout-coords assertion in `spotlight.spec.ts:38` that also fails on the prior commit. None caused by Phase 04.
- **Two transition-timing flakes** in arrow-key + dot-indicator tests — pass on retry, fail intermittently regardless of Phase 04. Logged in `deferred-items.md`.

## User Setup Required

None — purely a frontend SFC refactor + new i18n keys. No env vars, no schema migrations, no external service configuration.

## Next Phase Readiness

- **Phase 05 (NowWatchingCard refactor):** can proceed immediately. Follow the single-root-element + JSDoc-instead-of-template-comment pattern documented here to avoid the transition-wedge bug. Use the same Pinia + createI18n test setup if NowWatchingCard imports any Pinia store.
- **All 9 cards (Phases 02–10):** the Vue 3 single-root constraint applies universally. Phase 02 + 03 (AnimeOfDay + RandomTail, already on `main`) happen to have single-element roots so they're not affected, but any future card that adds `v-if` to the article or starts with a leading template comment will silently wedge the carousel.

## Self-Check: PASSED

- [x] `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue` exists with 198 lines, two-zone template, single root.
- [x] `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.spec.ts` exists, 16 assertions.
- [x] `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` all contain `personalPick.titleWithName` with `{name}` interpolation.
- [x] Commit `665d671` (feat) present.
- [x] Commit `13ab2a7` (test) present.
- [x] Commit `ff31ca5` (fix) present.
- [x] `.planning/workstreams/hero-spotlight/phases/04-personal-pick-refactor/deferred-items.md` exists.

---

*Phase: 04-personal-pick-refactor*
*Completed: 2026-05-24*
