---
phase: 02-frontend-carousel
plan: 05
workstream: hero-spotlight
subsystem: frontend
tags:
  - frontend
  - i18n
  - hero-spotlight
dependency_graph:
  requires:
    - 02-01 (types/spotlight.ts)
    - 02-02 (CarouselControls.vue — consumes spotlight.{prev,next,goTo}Slide)
    - 02-03 (card components — consume spotlight.{animeOfDay,randomTail,latestNews,platformStats}.*)
  provides:
    - frontend/web/src/locales/en.json (spotlight.* namespace)
    - frontend/web/src/locales/ru.json (spotlight.* namespace)
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts (en/ru parity gate)
  affects:
    - 02-04 (HeroSpotlightBlock — will pull spotlight.regionLabel, slideLabel, slideLabelWithTitle, pauseAutoplay once mounted)
tech_stack:
  added: []
  patterns:
    - vue-i18n nested JSON locale files (existing precedent)
    - Vitest co-located parity spec under src/locales/__tests__/
    - Deep leafPaths() walk for set-equality comparison across locales
key_files:
  created:
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts (124 lines, 13 it()/it.each() groups, 44 generated test cases)
  modified:
    - frontend/web/src/locales/en.json (+34 lines — new `spotlight` top-level object)
    - frontend/web/src/locales/ru.json (+34 lines — new `spotlight` top-level object, Russian translations)
decisions:
  - >-
    platformStats sub-keys ship as camelCase (animeAdded7d,
    episodesAdded7d, activeRooms7d, deltaPositive, noChange).
    Confirms and ratifies Plan 02-03 SUMMARY decision #1 — the
    PlatformStatsCard already calls t(`spotlight.platformStats.${camelize(m.key)}`)
    so the JSON must use camelCase for the lookup to resolve.
  - >-
    Shipped the FULL UI-SPEC §Copywriting Contract key inventory
    (24 leaf strings) — not just the subset currently referenced
    by Plans 02-02 + 02-03. The unused-for-now keys
    (regionLabel, slideLabel, slideLabelWithTitle, pauseAutoplay,
    animeOfDay.scoreLabel, latestNews.entryDate) are consumed by
    Plan 02-04 (HeroSpotlightBlock) — pre-shipping them keeps that
    plan's surface area unchanged.
metrics:
  ux_delta: "+1 (Better) — invisible localization; runtime users see correct language strings; lint catches raw text leaks"
  cdi: "0.01 * 3"
  mvq: "Sprite 88%/85%"
  duration_seconds: 121
  completed: 2026-05-21
---

# Phase 02 Plan 05: spotlight.* i18n payload + en/ru parity gate

> Centralises every user-facing string referenced by the v1.0 HeroSpotlightBlock carousel and its variant cards into the project's existing `vue-i18n` JSON locale files, and ships a Vitest parity gate that prevents future en/ru key drift.

## One-liner

Added a `spotlight` top-level namespace (24 leaf strings across 5 sub-namespaces) to both `en.json` and `ru.json` covering carousel chrome (prev/next/goTo/pauseAutoplay) + 4 card variants (animeOfDay, randomTail, latestNews, platformStats); the new Vitest spec asserts identical key sets and non-empty values across locales — 44 test cases, all green.

## Files Created

| File | Lines | Provides |
|------|-------|----------|
| `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` | 124 | en/ru parity gate (44 generated test cases via 5 narrative `it()` blocks + 8 `it.each()` blocks) |

## Files Modified

| File | Change |
|------|--------|
| `frontend/web/src/locales/en.json` | +34 lines — appended `spotlight` top-level object (11 keys, 24 leaf strings) |
| `frontend/web/src/locales/ru.json` | +34 lines — same `spotlight` structure with Russian translations |

## Spotlight leaf-key inventory shipped (24 total)

### Carousel chrome (7 leaves — top-level spotlight.*)
- `regionLabel` → "Today's spotlight" / "Подборка дня"
- `slideLabel` → "Slide {n} of {total}" / "Слайд {n} из {total}"
- `slideLabelWithTitle` → "Slide {n} of {total}: {title}" / "Слайд {n} из {total}: {title}"
- `prevSlide` → "Previous slide" / "Предыдущий слайд"
- `nextSlide` → "Next slide" / "Следующий слайд"
- `goToSlide` → "Go to slide {n}" / "Перейти к слайду {n}"
- `pauseAutoplay` → "Autoplay paused" / "Автопрокрутка приостановлена"

### `spotlight.animeOfDay.*` (5 leaves)
- `title`, `watchCta`, `addCta`, `scoreLabel`, `episodesLabel`

### `spotlight.randomTail.*` (3 leaves)
- `title`, `subtitle`, `discoverCta`

### `spotlight.latestNews.*` (3 leaves)
- `title`, `readMore`, `entryDate`

### `spotlight.platformStats.*` (6 leaves, camelCase)
- `title`, `animeAdded7d`, `episodesAdded7d`, `activeRooms7d`, `deltaPositive`, `noChange`

## Test counts

| Spec | it()/it.each() blocks | Generated test cases |
|------|-----------------------|----------------------|
| `spotlight-keys.spec.ts` | 13 | 44 |

All 44 cases green via `bunx vitest run src/locales/`.

## Per-task commits

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Add spotlight.* payload to en.json + ru.json | `e5debe0` |
| 2 | Vitest parity spec (en/ru key equality + non-empty leaves) | `929d191` |

## Verification

| Gate | Command | Result |
|------|---------|--------|
| JSON syntax — en | `node -e "JSON.parse(...en.json)"` | PASS |
| JSON syntax — ru | `node -e "JSON.parse(...ru.json)"` | PASS |
| TypeScript | `bunx tsc --noEmit` | PASS (exit 0) |
| Vitest — spotlight-keys spec | `bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts` | PASS (44/44) |
| Vitest — no card regressions | `bunx vitest run src/components/home/spotlight/cards/` | PASS (33/33) |
| Existing keys preserved | `grep '"nav"\|"home"' en.json` | PASS |
| spotlight namespace present | `grep '"regionLabel"\|"animeOfDay"\|"platformStats"' both locales` | PASS |
| Russian phrasing landed | `grep 'Подборка дня' ru.json` | PASS |

## Decisions Made

### 1. PlatformStatsCard key naming convention — **camelCase** (ratified from Plan 02-03)

Plan 02-03's executor chose camelCase i18n keys (`spotlight.platformStats.animeAdded7d`) and added a small `camelize()` helper in `PlatformStatsCard.vue` that converts the backend's snake_case `m.key` (`anime_added_7d`) before the `t()` lookup. Plan 02-05 confirms this decision by shipping camelCase-only JSON keys.

If the future ever wants to swap to snake_case in the JSON, the change is: (1) update PlatformStatsCard to drop the camelize() helper and call `t(\`spotlight.platformStats.${m.key}\`)` directly, (2) rename the JSON keys to snake_case in both en.json and ru.json, (3) update the `platformStatsKeys` array in the parity spec. Three coupled edits, all in three known files — no hidden coupling.

### 2. Ship the FULL UI-SPEC inventory now, not just what Plans 02-02 + 02-03 use

The current set of consumers (`CarouselControls.vue` + 4 card components) references 16 of the 24 keys. The remaining 8 (`regionLabel`, `slideLabel`, `slideLabelWithTitle`, `pauseAutoplay`, `animeOfDay.scoreLabel`, `latestNews.entryDate`) are reserved for Plan 02-04's `HeroSpotlightBlock` (carousel container that wraps cards with the region landmark + slide labels + autoplay logic).

**Reason:** locales drift the fastest of any project surface; the parity gate is most effective when it gates the full contract surface from day one rather than chasing keys as Plan 02-04 ships. Plan 02-04 can land without touching JSON files.

## Deviations from Plan

None — plan executed exactly as written. The plan supplied verbatim key/value pairs for both locales; Task 1 inserted them as a sibling of existing top-level keys (after the `notifications` block). The Vitest spec from Task 2 matched the plan's reference implementation, with minor TS-strictness tweaks (`Record<string, unknown>` instead of `any` casts) to keep `tsc --noEmit` clean under the project's strict settings.

The `__tests__` subfolder under `src/locales/` did not exist; Vitest config's `include: ['src/**/*.spec.ts']` picks it up via the recursive glob — no config change needed.

## Self-Check: PASSED

### Files exist

- `frontend/web/src/locales/en.json` — FOUND (modified, +34 lines)
- `frontend/web/src/locales/ru.json` — FOUND (modified, +34 lines)
- `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` — FOUND (new, 124 lines)

### Commits exist

- `e5debe0` — Task 1 (locale payload) — FOUND in `git log --oneline -5`
- `929d191` — Task 2 (parity spec) — FOUND in `git log --oneline -5`

### Spec passes

- `bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts` → **44 / 44 green** (verified at 2026-05-21 06:01:20Z)
