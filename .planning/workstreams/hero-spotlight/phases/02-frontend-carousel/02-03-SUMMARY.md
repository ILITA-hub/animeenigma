---
phase: 02-frontend-carousel
plan: 03
workstream: hero-spotlight
subsystem: frontend
tags:
  - frontend
  - vue3
  - components
  - hero-spotlight
dependency_graph:
  requires:
    - 02-01 (frontend/web/src/types/spotlight.ts — type contracts)
  provides:
    - frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue
    - frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue
    - frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue
    - frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue
  affects:
    - 02-04 (HeroSpotlightBlock.vue — will mount these cards via `<component :is>`)
    - 02-05 (locale JSON — must add spotlight.* keys these components reference)
tech_stack:
  added: []
  patterns:
    - Vue 3 `<script setup lang="ts">` Composition API
    - vue-i18n `useI18n()` + t() for all user-facing strings
    - @vue/test-utils + RouterLinkStub for unit tests
    - Vitest co-located `.spec.ts` files
    - Tailwind v4 utility classes (no SCSS, no <style> blocks)
key_files:
  created:
    - frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue (129 lines)
    - frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts (134 lines)
    - frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue (119 lines)
    - frontend/web/src/components/home/spotlight/cards/RandomTailCard.spec.ts (113 lines)
    - frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue (78 lines)
    - frontend/web/src/components/home/spotlight/cards/LatestNewsCard.spec.ts (125 lines)
    - frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue (71 lines)
    - frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts (150 lines)
  modified: []
decisions:
  - >-
    PlatformStatsCard uses camelCase i18n keys (animeAdded7d) to match
    UI-SPEC Copywriting Contract — a camelize() helper bridges the
    backend's snake_case `m.key` (anime_added_7d) so Plan 02-05 can
    ship camelCase-only locale JSON.
  - >-
    LatestNewsCard renders `entry.message` (the actual ChangelogEntry
    shape from Plan 02-01) — not `entry.title` + `entry.summary` as
    the plan template suggested. The card splits `message` at the
    first sentence boundary so the line-clamp-2 title + line-clamp-3
    body hierarchy still has two distinct strings. (Documented Rule 1
    deviation below.)
  - >-
    LatestNewsCard readMore link points to "/" (Home) — no /changelog
    route exists in the router. LastUpdates.vue is a Home-page tab.
    Plan 02-05 / future enhancement may add a dedicated /changelog
    route; for Phase 2, "/" lands the user where the changelog lives.
metrics:
  ux_delta: "+3 (Better) — the visual content; this is what users actually see"
  cdi: "0.04 * 8"
  mvq: "Griffin 92%/88%"
  duration_seconds: 433
  completed: 2026-05-21
---

# Phase 02 Plan 03: Frontend spotlight cards (Anime-of-day, Random-tail, Latest-news, Platform-stats)

> 4 presentational Vue 3 SFCs that render the variant-specific layouts for the v1.0 HeroSpotlightBlock carousel — typed against Plan 02-01's discriminated union, fully i18n-driven, drift-gates enforced via co-located Vitest specs.

## One-liner

Built 4 stateless variant cards (AnimeOfDayCard, RandomTailCard, LatestNewsCard, PlatformStatsCard) that receive typed `data` props from the carousel parent and render the UI-SPEC visual contracts — typography limited to font-medium/font-semibold, tablet padding p-4, all text via `t('spotlight.*')`, 33 unit tests gate the design contract.

## Files Created

| File | Lines | Provides |
|------|-------|----------|
| `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue` | 129 | Anime-of-day card: poster + meta + Watch/Add CTAs |
| `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts` | 134 | 8 unit tests covering layout, score chip, genres, CTAs, typography, padding, i18n |
| `frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue` | 119 | Random-tail card: poster + cyan-300/80 eyebrow + single Open CTA |
| `frontend/web/src/components/home/spotlight/cards/RandomTailCard.spec.ts` | 113 | 6 unit tests covering visual deltas vs AnimeOfDayCard |
| `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue` | 78 | Changelog excerpts card: 3-col grid desktop, stacked mobile |
| `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.spec.ts` | 125 | 9 unit tests covering route target, cap-at-3, message rendering, line-clamp |
| `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue` | 71 | Platform metrics card: adaptive 1/2/3-col grid |
| `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts` | 150 | 10 unit tests covering adaptive grid, delta colors, camelCase keys |

**Total:** 919 lines across 8 files.

## Test Counts

| Card | Tests | Notes |
|------|-------|-------|
| AnimeOfDayCard | 8 | Plan asked for ≥6 |
| RandomTailCard | 6 | Plan asked for ≥5 |
| LatestNewsCard | 9 | Plan asked for ≥5 |
| PlatformStatsCard | 10 | Plan asked for ≥6 |
| **Total** | **33** | All green via `bunx vitest run src/components/home/spotlight/cards/` |

## Per-task commits

| Task | Component | Commit |
|------|-----------|--------|
| 1 | AnimeOfDayCard | `2339553` |
| 2 | RandomTailCard | `54dcd56` |
| 3 | LatestNewsCard | `ffddb00` |
| 4 | PlatformStatsCard | `35f3256` |

## Verification

| Gate | Command | Result |
|------|---------|--------|
| Vitest (4 specs, 33 tests) | `bunx vitest run src/components/home/spotlight/cards/` | PASS |
| TypeScript | `bunx tsc --noEmit` | PASS (exit 0) |
| ESLint (4 SFCs + 4 specs) | `bunx eslint src/components/home/spotlight/cards/` | PASS (no output) |
| Typography drift gate | `grep -rE "font-bold\|font-normal" cards/*.vue` | PASS (no match) |
| Tablet padding gate | `grep -E " p-5( \|\"\|$)" cards/*.vue` | PASS (no match) |
| XSS / v-html gate | `grep -r "v-html" cards/` | PASS (no match) |
| Stateless-card gate | `grep -rE "setInterval\|setTimeout" cards/` | PASS (no match) |

## Decisions Made

### 1. PlatformStatsCard i18n key naming — **camelCase**

UI-SPEC §Copywriting Contract declares i18n keys in camelCase
(`spotlight.platformStats.animeAdded7d`), but the backend's `PlatformMetric.key`
is snake_case (`anime_added_7d`). Plan 02-03 explicitly required the executor to
pick ONE convention and document it for Plan 02-05.

**Decision:** keep camelCase in the locale JSON. The card has a small `camelize()`
helper that converts `m.key` to camelCase before the `t()` lookup. This mirrors
the rest of the project's locale keys (all camelCase) and avoids forcing
non-conforming snake_case keys into the i18n JSON.

**Implications for Plan 02-05:** ship these exact keys in en.json + ru.json:

```
spotlight.platformStats.title
spotlight.platformStats.animeAdded7d
spotlight.platformStats.episodesAdded7d
spotlight.platformStats.activeRooms7d
spotlight.platformStats.deltaPositive
spotlight.platformStats.noChange
```

### 2. LatestNewsCard readMore link path — **"/"**

Plan 02-03 directed the executor to grep the router for the actual changelog
route. I ran `grep -rn "LastUpdates\|changelog\|last-updates"
frontend/web/src/router/` and confirmed:

- No `/changelog` route exists.
- No `/last-updates` route exists.
- `LastUpdates.vue` is mounted at `frontend/web/src/views/Home.vue:407` as a
  tab/section on the home page (alongside `ActivityFeed`).

**Decision:** the readMore link points to `"/"`. Navigating home lands the user
on the page where the changelog content lives.

**Forward-compatible:** if a dedicated `/changelog` route is added later, change
the one `to="/"` to `to="/changelog"` — no other refactor needed.

### 3. LatestNewsCard ChangelogEntry shape — `message` (not `title/summary`)

Plan 02-03's template markup referenced `entry.title` + `entry.summary`, but
those fields do not exist in the type system. Plan 02-01's `ChangelogEntry` is
`{ date: string, type?: string, message: string }` — matching what the backend
actually ships (see types/spotlight.ts header comment, item #2).

**Decision (Rule 1 - Bug fix):** render `entry.message`. To preserve the
UI-SPEC visual hierarchy (line-clamp-2 title + line-clamp-3 body), the card
splits `message` at the first sentence boundary. If no sentence break exists,
the whole message becomes the title and the body paragraph is hidden via
`v-if="entryBody(entry.message)"`.

This is the approach Plan 02-01's header comment explicitly recommended:
"Card components must consume `message` for the body and may use the leading
sentence of `message` as a title fallback."

## Deviations from Plan

### Rule 1 — Bug fixes

**1. LatestNewsCard ChangelogEntry shape mismatch**

- **Found during:** Task 3
- **Issue:** Plan template referenced `entry.title` + `entry.summary` but
  those fields don't exist in `ChangelogEntry` (Plan 02-01 types).
- **Fix:** Render `entry.message` with a sentence-boundary split for the
  title/body visual hierarchy.
- **Files modified:** `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue`
- **Commit:** `ffddb00`

### Rule 3 — Blocking-issue fixes

**2. No /changelog route exists**

- **Found during:** Task 3
- **Issue:** Plan acceptance criterion required the readMore link to point at
  a real router path. The plan defaulted to `/changelog`. No such route exists.
- **Fix:** Point the link at `"/"`. The changelog content (LastUpdates.vue) is
  rendered on the Home view.
- **Files modified:** `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue`
- **Commit:** `ffddb00`

### Rule 1 — Bug fix (test isolation)

**3. AnimeOfDayCard HTML comments leaked into wrapper.html() output**

- **Found during:** Task 1 (RED→GREEN transition)
- **Issue:** The SFC's initial template had explanatory HTML comments
  containing the substrings `text-yellow-400` and `p-5` (in "p-4 NOT p-5"
  prose). `wrapper.html()` includes HTML comments — so the drift-gate
  assertions (`expect(html).not.toContain('text-yellow-400')` when score
  undefined; `not.toMatch(/\bp-5\b/)`) failed against the comment text,
  not the rendered classes.
- **Fix:** Removed the inline template comments. Documentation moved
  out of the template. The class-string assertions now test only the
  rendered DOM as intended.
- **Files modified:** `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue`
- **Commit:** `2339553` (included in initial Task 1 commit after fix)

### Schema mismatch from Plan template (pre-existing; not auto-fixed)

The plan template referenced `data.anime.episodes` for the episode count,
but the actual `SpotlightAnime` type uses `episodes_count` (with
`episodes_aired` as a sibling). All 4 cards consume `episodes_count` to
match the real type. This is a doc/code-drift in the plan template, not
something to fix in the type system — types/spotlight.ts is the source
of truth per Plan 02-01.

## i18n keys established (Plan 02-05 must ship values for these)

### `spotlight.animeOfDay.*`

- `spotlight.animeOfDay.title` — eyebrow label
- `spotlight.animeOfDay.watchCta` — primary CTA
- `spotlight.animeOfDay.addCta` — secondary CTA
- `spotlight.animeOfDay.episodesLabel` — episode count line (param: `n`)

### `spotlight.randomTail.*`

- `spotlight.randomTail.title` — eyebrow label
- `spotlight.randomTail.subtitle` — desktop-only one-liner below eyebrow
- `spotlight.randomTail.discoverCta` — single Open CTA
- Also reuses `spotlight.animeOfDay.episodesLabel` for the episode line

### `spotlight.latestNews.*`

- `spotlight.latestNews.title` — card header
- `spotlight.latestNews.readMore` — right-aligned router-link to "/"

### `spotlight.platformStats.*` (camelCase)

- `spotlight.platformStats.title` — card header
- `spotlight.platformStats.animeAdded7d` — metric label
- `spotlight.platformStats.episodesAdded7d` — metric label
- `spotlight.platformStats.activeRooms7d` — metric label
- `spotlight.platformStats.deltaPositive` — positive delta indicator (param: `n`)
- `spotlight.platformStats.noChange` — zero/null delta indicator (renders "—")

## Self-Check: PASSED

### Files exist

- AnimeOfDayCard.vue — FOUND
- AnimeOfDayCard.spec.ts — FOUND
- RandomTailCard.vue — FOUND
- RandomTailCard.spec.ts — FOUND
- LatestNewsCard.vue — FOUND
- LatestNewsCard.spec.ts — FOUND
- PlatformStatsCard.vue — FOUND
- PlatformStatsCard.spec.ts — FOUND

### Commits exist

- `2339553` — Task 1 AnimeOfDayCard — FOUND
- `54dcd56` — Task 2 RandomTailCard — FOUND
- `ffddb00` — Task 3 LatestNewsCard — FOUND
- `35f3256` — Task 4 PlatformStatsCard — FOUND
