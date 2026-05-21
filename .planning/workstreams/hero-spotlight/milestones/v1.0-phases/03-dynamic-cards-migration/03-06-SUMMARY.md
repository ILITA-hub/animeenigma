---
phase: 03-dynamic-cards-migration
plan: 06
subsystem: hero-spotlight
workstream: hero-spotlight
tags: [hero-spotlight, migration, docs, home, phase-3]
requirements_completed:
  - HSB-MIG-01
  - HSB-NF-05
dependency_graph:
  requires:
    - 03-05  # 5-new-spotlight-cards (Aggregator + 9 resolvers must exist before legacy row is removed)
  provides:
    - "Home.vue without the legacy trendingRecs row"
    - "CLAUDE.md contributor-facing spec for adding a 10th spotlight card"
  affects:
    - frontend/web/src/views/Home.vue
    - CLAUDE.md
tech_stack:
  added: []  # Pure removal + docs — no new tech
  patterns:
    - "Pre-edit grep audit → atomic Edit → post-edit grep gate (Phase A/E pattern)"
key_files:
  created:
    - .planning/workstreams/hero-spotlight/phases/03-dynamic-cards-migration/03-06-SUMMARY.md
  modified:
    - frontend/web/src/views/Home.vue
    - CLAUDE.md
decisions:
  - "Feature flags SPOTLIGHT_ENABLED + VITE_HERO_SPOTLIGHT_ENABLED stay (HSB-MIG-02 — one-release kill-switch retention)."
  - "Backend /api/users/recs endpoint stays — repurposed as the data source for the personal_pick resolver inside the spotlight aggregator."
  - "CLAUDE.md section inserted between ### Database migrations and ## Local Development Commands."
metrics:
  duration: "8 minutes"
  completed: "2026-05-21"
  tasks: 2
  files_modified: 2
  ux_delta: "+1 (Better)"
  cdi: "0.03 × 5"
  mvq: "Basilisk 90%/92%"
---

# Phase 03 Plan 06: Migration cut-over + CLAUDE.md docs Summary

Removed the legacy `trendingRecs` row from `Home.vue` (HSB-MIG-01) so the rotating 9-card spotlight is now the sole top-of-home discovery surface, and added the `Adding a Spotlight Card Type` reference section to `CLAUDE.md` under Common Tasks (HSB-NF-05).

## What Changed

### Task 1 — Remove trendingRecs row from Home.vue (commit `29e1c00`)

**Pre-edit grep audit.** Captured 31 hits across `<template>` and `<script setup>`; every hit was scoped to the trending row (no second consumers elsewhere in the file).

**Template removal (~95 lines, lines 40-138 in pre-edit file):**
- The `<!-- Trending Now Row (Phase 10 ... -->` comment block
- The `<div v-if="trendingRecs.length > 0" ...>` row container with its pin-reason header, dominant-signal chip, and 20 `<router-link>` poster cards
- The `<div v-else-if="trendingLoading" ...>` skeleton fallback

**Script removal:**

| Identifier | Type | Removed |
|---|---|---|
| `onRecClick(item: RecItem): void` | function | ✅ |
| `useRecs()` destructuring → `rawRecs`, `trendingLoading`, `rowLabelKey` | composable call | ✅ |
| `trendingRecs` | computed | ✅ |
| `trendingIds` | computed | ✅ |
| `useAnimeProgress(trendingIds)` → `trendingProgress` | composable call | ✅ |
| `dominantSignalKey` | computed | ✅ |
| `reasonI18nKey` | computed | ✅ |

**Import cleanup (post-removal unused-import sweep):**
- `useRecs`, `RecItem` (from `@/composables/useRecs`)
- `useAnimeProgress` (from `@/composables/useAnimeProgress`)
- `emitRecClick`, `PinSource` (from `@/utils/recsAnalytics`)
- `useRoute` (from `vue-router`) — also unused after `onRecClick` removal
- `computed` (from `vue`) — no remaining `computed(...)` calls in Home.vue
- `const route = useRoute()` local binding

**No unexpected consumers.** Every removed identifier had zero hits outside the deleted region. The `t` (from `useI18n()`) and `getLocalizedTitle` imports stay — both are still used by the 3-column grid below.

**Net delta:** -154 / +2 lines.

### Task 2 — Add CLAUDE.md spotlight-card docs (commit `0907b14`)

Inserted a new `### Adding a Spotlight Card Type` subsection in `CLAUDE.md`, anchored:

- **Above:** `### Database migrations` (which ends at the `**Note**: GORM only creates ...` line)
- **Below:** `## Local Development Commands`

The 19-line block enumerates the 5 anchors a contributor must touch to add a 10th card variant. Every cited file path was cross-verified to exist on disk post-Phase-3:

| Anchor | Cited path | Exists? |
|---|---|---|
| 1 — Backend resolver template | `services/catalog/internal/service/spotlight/cards/anime_of_day.go` | ✅ |
| 2 — Backend types union | `services/catalog/internal/service/spotlight/types.go` | ✅ |
| 3 — Backend DI | `services/catalog/cmd/catalog-api/main.go` | ✅ |
| 4 — Frontend SFC template | `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue` | ✅ |
| 5 — Dispatch + types + i18n | `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`, `frontend/web/src/types/spotlight.ts`, `frontend/web/src/locales/{en,ru}.json`, `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` | ✅ all five |
| Regression e2e | `frontend/web/e2e/spotlight.spec.ts` (path corrected from the plan's mention of `spotlight-full.spec.ts`) | ✅ |

## Verification

| Gate | Result |
|---|---|
| `grep -c "trendingRecs\|trendingProgress\|trendingLoading\|rowLabelKey\|reasonI18nKey\|onRecClick\|dominantSignalKey\|trendingIds" frontend/web/src/views/Home.vue` | `0` ✅ |
| `grep -c "<HeroSpotlightBlock" frontend/web/src/views/Home.vue` | `1` ✅ |
| `cd frontend/web && bunx tsc --noEmit` | clean ✅ |
| `cd frontend/web && bunx eslint src/views/Home.vue` | clean ✅ |
| `grep -q "SPOTLIGHT_ENABLED" docker/.env.example` | present ✅ (HSB-MIG-02 deferral honored) |
| `grep -q "VITE_HERO_SPOTLIGHT_ENABLED" frontend/web/.env.example` | present ✅ (HSB-MIG-02 deferral honored) |
| `grep -q "### Adding a Spotlight Card Type" CLAUDE.md` | present ✅ |
| `grep -q "services/catalog/internal/service/spotlight/cards" CLAUDE.md` | present ✅ |
| `grep -q "frontend/web/src/components/home/spotlight" CLAUDE.md` | present ✅ |
| `grep -q "spotlight-keys.spec.ts" CLAUDE.md` | present ✅ |
| `cd frontend/web && bun run build` | exit 0 ✅ (only standard chunk-size warning) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Doc accuracy] Plan referenced `e2e/spotlight-full.spec.ts`; actual file is `e2e/spotlight.spec.ts`**

- **Found during:** Task 2 pre-write file-existence cross-check.
- **Issue:** The verbatim CLAUDE.md block in the plan cited `frontend/web/e2e/spotlight-full.spec.ts` as the canonical e2e regression spec. That file does not exist; the actual spec under `frontend/web/e2e/` is `spotlight.spec.ts`.
- **Fix:** Corrected the cited path to `frontend/web/e2e/spotlight.spec.ts` before inserting the section. Documentation accuracy is a Rule 1 correctness requirement (a contributor following the doc with a wrong path would hit `ENOENT` and waste cycles).
- **Files modified:** `CLAUDE.md`
- **Commit:** `0907b14`

**2. [Rule 1 — Bonus dead-code removal] Removed `useRoute` import + `const route = useRoute()` + `computed` import**

- **Found during:** Task 1, after the documented Phase D import sweep.
- **Issue:** The plan called out removing `useRecs`/`useAnimeProgress`/`emitRecClick`/`RecItem`/`PinSource`. After deleting `onRecClick` (which used `route.path`) and the four `computed(...)` blocks, both `useRoute` and `computed` became unused.
- **Fix:** Removed `useRoute` from the `vue-router` import (kept `useRouter` since `goToSearch` still uses it), removed `computed` from the `vue` import (kept `ref`, `onMounted`), and removed `const route = useRoute()`. Without this cleanup tsc would have flagged unused-imports if the project enabled `noUnusedLocals`/`noUnusedParameters`, and the standard ESLint config flagged the unused declaration.
- **Files modified:** `frontend/web/src/views/Home.vue`
- **Commit:** `29e1c00`

No other deviations. No architectural changes needed; no checkpoints reached; no authentication gates encountered.

## Identifiers — Pre-Edit vs Post-Edit Audit

The plan required surfacing any identifier that had a second consumer outside the trending block. **None had a second consumer.** Confirmation table:

| Identifier | Pre-edit hits in Home.vue | Hits outside the trending region | Result |
|---|---|---|---|
| `trendingRecs` | 10 | 0 | Cleanly removed |
| `trendingProgress` | 5 | 0 | Cleanly removed |
| `trendingLoading` | 2 | 0 | Cleanly removed |
| `rowLabelKey` | 2 | 0 | Cleanly removed |
| `reasonI18nKey` | 3 | 0 | Cleanly removed |
| `onRecClick` | 2 | 0 | Cleanly removed |
| `dominantSignalKey` | 2 | 0 | Cleanly removed |
| `trendingIds` | 2 | 0 | Cleanly removed |
| `useRecs` (import) | 1 | 0 | Cleanly removed |
| `useAnimeProgress` (import) | 1 | 0 | Cleanly removed |
| `emitRecClick` (import) | 1 | 0 | Cleanly removed |
| `RecItem` (type import) | 1 | 0 | Cleanly removed |
| `PinSource` (type import) | 1 | 0 | Cleanly removed |

Composable file `frontend/web/src/composables/useRecs.ts` still defines and exports `rowLabelKey` — that's the composable's own public API, untouched by this plan and still consumable by any other view that needs the recs row label.

## Feature Flags — HSB-MIG-02 Compliance

Per HSB-MIG-02 explicit deferral, the kill-switches stay for one release:

- `SPOTLIGHT_ENABLED` — present in `docker/.env.example` ✅
- `VITE_HERO_SPOTLIGHT_ENABLED` — present in `frontend/web/.env.example` ✅

Removal will land in a future plan (TBD by the workstream's post-Plan-07 retrospective).

## Known Stubs

None. This plan was pure removal + documentation; it did not introduce any new component/data wiring that could harbor stub data.

## Threat Flags

None. The pre-removal threat register (T-03-21, T-03-22) anticipated this exact shape — frontend-only removal of a row whose backend remains intact, mitigated by the pre/post grep audit and the tsc/eslint gates. No new surface area introduced.

## Follow-ups for Plan 07 (verification)

Plan 07 owns the end-to-end Playwright regression for the full migrated home page. It should validate:

1. The `<HeroSpotlightBlock />` mount is the only top-of-home discovery surface (no legacy trending row visible).
2. The 9-card rotation produces a `personal_pick` card for logged-in users with watch history — confirming the backend `/api/users/recs` data path that previously fed the row now feeds the spotlight aggregator.
3. The skeleton/loading state is owned by `HeroSpotlightBlock` (the legacy `trendingLoading` skeleton no longer fires).
4. `bun run build` continues to pass (already gated here as the final pre-commit check).

## Self-Check: PASSED

- File `frontend/web/src/views/Home.vue` — modified, present
- File `CLAUDE.md` — modified, present
- File `.planning/workstreams/hero-spotlight/phases/03-dynamic-cards-migration/03-06-SUMMARY.md` — created (this file)
- Commit `29e1c00` (feat 03-06: remove legacy trendingRecs row) — present in git log
- Commit `0907b14` (docs 03-06: add Adding a Spotlight Card Type) — present in git log
