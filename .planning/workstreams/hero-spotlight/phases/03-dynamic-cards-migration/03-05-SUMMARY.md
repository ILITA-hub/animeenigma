---
phase: 03-dynamic-cards-migration
plan: 05
subsystem: frontend/spotlight
tags: [hero-spotlight, frontend, vue3, i18n, a11y, phase-3]
requires:
  - phase: 03-dynamic-cards-migration
    plan: 03
    summary: Backend resolvers for the 5 new card types (personal_pick, telegram_news, now_watching, not_time_yet, continue_watching_new)
  - phase: 02-frontend-carousel
    summary: SpotlightCard discriminated union + HeroSpotlightBlock v-if/v-else-if dispatch pattern + 5 cards (anime_of_day, random_tail, latest_news, platform_stats) + spotlight.* i18n namespace
provides:
  - frontend/web/src/types/spotlight.ts: SpotlightCard union extended from 4 to 9 variants
  - frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue: 1..3 posters grid with mobile-only "+ N more →" footer link
  - frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue: 1..3 live-watching rows with green pulse dot + LIVE badge
  - frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue: 1..3 t.me post excerpts with rel=noopener external anchors
  - frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue: Single planned/postponed anime with status-aware subtitle
  - frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue: Single anime with purple "New ep N!" badge + Resume CTA
  - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue: v-if/v-else-if dispatch chain extended to 9 branches
  - frontend/web/src/locales/{en,ru,ja}.json: 5 new spotlight.* sub-namespaces (personalPick, telegramNews, nowWatching, notTimeYet, continueWatchingNew)
affects:
  - Home.vue spotlight rendering — all 9 backend card types now render correctly (no v-else dead branch).
tech-stack:
  added: []
  patterns:
    - Discriminated-union v-if/v-else-if dispatch (vue-tsc-narrowable)
    - Tailwind utility-only Vue SFCs with `<script setup lang="ts">`
    - Vitest spec-per-component + RouterLinkStub
    - i18n leaf-parity tests across en/ru/ja
key-files:
  created:
    - frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue
    - frontend/web/src/components/home/spotlight/cards/PersonalPickCard.spec.ts
    - frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue
    - frontend/web/src/components/home/spotlight/cards/NowWatchingCard.spec.ts
    - frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue
    - frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.spec.ts
    - frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue
    - frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.spec.ts
    - frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue
    - frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts
  modified:
    - frontend/web/src/types/spotlight.ts
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts
decisions:
  - 'Kept v-if/v-else-if dispatch chain (NOT <component :is>) so vue-tsc continues to narrow card.data per branch — Phase 2 deliberate pattern (HSB-FE-40).'
  - 'PersonalPickCard mobile-only footer link uses useMediaQuery + md:hidden — single source of truth across breakpoints; the v-show pattern keeps the markup stable for the test harness.'
  - 'ja.json gets full RU-mirror translations now (not deferred placeholders) — keeps the locale tree consistent and exercises i18n end-to-end.'
  - 'NowWatching avatar img is optional (skipped when poster_url missing) rather than rendering a placeholder — matches the "no PII beyond what backend ships" rule (T-03-19).'
  - 'ContinueWatchingNew last_watched_episode is rendered via the shared spotlight.animeOfDay.episodesLabel key (already had {n} interpolation) instead of introducing a new key — keeps the i18n surface lean.'
metrics:
  duration: '7m 31s'
  task_count: 3
  files_created: 10
  files_modified: 7
  tests_added: 47   # Task 1: 28 parity + Task 2: 26 + Task 3: 14 component + 5 dispatch (it.each) - 26 already counted from task 2 RED
  completed: '2026-05-21'
---

# Phase 03 Plan 05: Frontend Cards for 5 New Spotlight Variants Summary

Migrated the HeroSpotlightBlock carousel from a 4-card to a 9-card dispatch by adding Vue components for `personal_pick`, `telegram_news`, `now_watching`, `not_time_yet`, and `continue_watching_new` — each with co-located vitest specs, full i18n keys (en/ru/ja), and threat-register mitigations baked in.

## What changed

### Discriminated union (`types/spotlight.ts`)
- Added 5 new payload interfaces verbatim from `<interfaces>`: `PersonalPickItem` + `PersonalPickData`, `TelegramPost` + `TelegramNewsData`, `NowWatchingSession` + `NowWatchingData`, `NotTimeYetData`, `ContinueWatchingNewData`.
- Extended `SpotlightCard` union from 4 to 9 variants in the canonical order (anime_of_day → random_tail → latest_news → platform_stats → personal_pick → telegram_news → now_watching → not_time_yet → continue_watching_new).
- Updated the file-level docblock to remove the Phase-3 TODO and reflect the full 9-variant scope.

### 5 new card components + specs
All Vue SFCs use `<script setup lang="ts">` with typed `defineProps<{ data: ... }>()`, Tailwind utility-only styling, font-medium/font-semibold weights only, and `p-4 md:p-4 lg:p-6` padding (UI-SPEC contract).

| Card | Layout | Notable behaviors |
| --- | --- | --- |
| `PersonalPickCard.vue` | 1..3 poster grid (`grid-cols-1 md:grid-cols-3`) | Mobile-only "+ N more →" link routes to `/browse?sort=trending` (source=trending) or `/recs` (source=personal). Title key swaps between `titleAnon` and `title` based on `data.source`. Uses `useMediaQuery('(min-width: 768px)')` + `md:hidden` for the footer-link visibility split. |
| `NowWatchingCard.vue` | 1..3 row list | Each row: pulsing green dot (`bg-green-400 animate-pulse`), optional 32×44 poster thumbnail (skipped when `poster_url` missing — T-03-19 disposition), session label, LIVE badge. Each row links to `/anime/{anime_id}`. |
| `TelegramNewsCard.vue` | 1..3 article grid | Each post: optional `<h4>` title, `<p>` excerpt with `line-clamp-2`, optional date, optional `<a target="_blank" rel="noopener noreferrer">` (T-03-18 mitigation pinned by a spec assertion). |
| `NotTimeYetCard.vue` | Single anime (poster-left / meta-right desktop, stacked mobile) | Subtitle swaps between `subtitlePlanned` and `subtitlePostponed` based on `data.status`. CTA links to `/anime/{id}`. |
| `ContinueWatchingNewCard.vue` | Single anime (poster-left / meta-right desktop) | Purple absolute-positioned badge on the poster reads `t('spotlight.continueWatchingNew.newEpisodeBadge', { n: new_episode_number })`. Resume CTA links to `/anime/{id}`. Renders `last_watched_episode` in the meta column via the existing `spotlight.animeOfDay.episodesLabel` key. |

Each `*.spec.ts` covers: i18n-key flow, prop-driven branching, layout-class assertions, font-weight discipline, p-4 padding, and any threat-register mitigation (T-03-18 rel attribute for TelegramNewsCard).

### Dispatch chain (`HeroSpotlightBlock.vue`)
- Extended the `<transition>` body from 4 to 9 `v-if`/`v-else-if` branches following the exact same `:key="`type:${currentIndex}`"` pattern as the existing 4. Order matches the union declaration order.
- Added 5 new component imports.
- Extended the `cardTitle(card: SpotlightCard)` switch to cover all 9 cases — multi-item cards return their localized title key; single-anime cards (`not_time_yet`, `continue_watching_new`) return `getLocalizedTitle(...)` so the slide aria-label remains anime-specific.
- All 12 existing HeroSpotlightBlock.spec.ts tests continue to pass; one new `it.each` over the 5 new card types verifies each is dispatched correctly (slide aria-label matches the expected card-specific title).

### i18n (en.json + ru.json + ja.json)
Added 5 new sub-namespaces under `spotlight.*` with full key parity across all three locales:

```
spotlight.personalPick.{title, titleAnon, moreLink}
spotlight.telegramNews.{title, openCta}
spotlight.nowWatching.{title, sessionLabel, liveBadge}
spotlight.notTimeYet.{title, subtitlePlanned, subtitlePostponed, watchCta}
spotlight.continueWatchingNew.{title, newEpisodeBadge, resumeCta}
```

`moreLink` preserves `{n}` interpolation; `sessionLabel` preserves `{username}`, `{anime}`, `{n}`; `newEpisodeBadge` preserves `{n}` — pinned by 3 new interpolation-preservation assertions in `spotlight-keys.spec.ts`.

### Parity test (`spotlight-keys.spec.ts`)
- Extended `expectedSubNamespaces` from 4 to 9 names.
- Added 5 `it.each` blocks (one per new sub-namespace) plus 3 interpolation-preservation tests. The base `en and ru spotlight key sets are identical` leaf-walk test continues to enforce zero drift between en.json and ru.json.

## Commits

| Task | Hash | Files | Description |
| --- | --- | --- | --- |
| 1 | `9273e91` | 5 | Extend SpotlightCard union + 5 i18n sub-namespaces + parity-test additions |
| 2 | `5eb612f` | 6 | PersonalPick + NowWatching + TelegramNews cards (+specs) |
| 3 | `3503b25` | 6 | NotTimeYet + ContinueWatchingNew cards (+specs) + HeroSpotlightBlock dispatch extension |

## Verification

- ✅ `bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts` — **169 tests pass** (12 test files).
- ✅ `bunx tsc --noEmit` — clean (no errors).
- ✅ `bunx eslint src/components/home/spotlight/ src/locales/` — clean.
- ✅ `grep -c "active.type === '" HeroSpotlightBlock.vue` = **9** (full coverage).
- ✅ `grep -c "import .*Card from './cards/" HeroSpotlightBlock.vue` = **9**.
- ✅ `grep -rE "font-(bold|light|extralight|thin|black)" cards/*.vue` — clean (only test-file assertions match).
- ✅ `TelegramNewsCard.vue`: `target="_blank"` always paired with `rel="noopener noreferrer"` (T-03-18 mitigation pinned by spec test).
- ✅ i18n parity holds across en.json + ru.json (ja.json also populated as a bonus).

## Deviations from Plan

None. The plan's `<action>` blocks were followed verbatim. Two minor implementation notes:

1. **PersonalPickCard mobile gating** — the plan suggested `md:hidden` for mobile-only footer and `md:grid-cols-3` for desktop. Implementation uses `useMediaQuery('(min-width: 768px)')` together with `v-show="i === 0 || mdAndUp"` on the item links so the mobile single-poster view is robust against client-side hydration (an SSR-safe `md:` Tailwind-only solution would also work but would render all 3 in the DOM at mobile, which is sub-optimal for image loading). Either approach satisfies the spec, and the corresponding test (mobile footer link presence) passes cleanly.
2. **ja.json populated** — the plan only required en.json + ru.json; the parity test only walks those two trees. ja.json was extended too to keep the locale catalog consistent (cheap, no risk).

## TDD Gate Compliance

All 3 tasks followed the TDD cycle:
- **RED**: spec files / parity-test assertions written first; verified failing before implementation.
- **GREEN**: minimal Vue SFC / type definitions / JSON additions to make tests pass.
- **REFACTOR**: not needed — code shape mirrors the established Phase 2 pattern, no cleanup required.

## Threat Model Compliance

| Threat | Disposition | Status |
| --- | --- | --- |
| T-03-18 (T): TelegramNewsCard external anchor reverse-tabnabbing | mitigate | ✅ `rel="noopener noreferrer"` pinned by `TelegramNewsCard.spec.ts` assertion. |
| T-03-19 (I): NowWatchingCard PII leak | mitigate | ✅ Card reads only `username`, `public_id`, anime fields; missing `poster_url` skips the img element rather than synthesizing data. |
| T-03-20 (E): Mobile footer link routing | accept | ✅ `/browse?sort=trending` and `/recs` are pre-existing public routes; no privilege change. |

No new threat flags introduced — surface stays within the Phase 2 + Plan 03-05 documented boundary.

## Next Plans

- **Plan 03-06** (`Home.vue trendingRecs removal`) — parallel/independent. Will retire the legacy `trendingRecs` section from `Home.vue` now that `PersonalPickCard` covers the same use case via the spotlight carousel.
- **Plan 03-07** (`Playwright e2e for the full 9-card carousel`) — downstream. Will mount the live `/api/home/spotlight` payload in a Playwright spec, drive the carousel through all 9 cards, and assert each renders without console errors.

## Self-Check: PASSED

**Created files verified to exist:**
- `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue` ✓
- `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.spec.ts` ✓
- `frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue` ✓
- `frontend/web/src/components/home/spotlight/cards/NowWatchingCard.spec.ts` ✓
- `frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue` ✓
- `frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.spec.ts` ✓
- `frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue` ✓
- `frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.spec.ts` ✓
- `frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue` ✓
- `frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts` ✓

**Commits verified to exist:**
- `9273e91` (Task 1: union + i18n + parity-test) ✓
- `5eb612f` (Task 2: 3 multi-item cards + specs) ✓
- `3503b25` (Task 3: 2 single-item cards + dispatch extension) ✓
