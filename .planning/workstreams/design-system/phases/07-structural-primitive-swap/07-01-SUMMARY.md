---
phase: 07-structural-primitive-swap
plan: 01
subsystem: frontend/design-system
tags: [vue, button-primitive, structural-swap, adjudication, ui-migration]
requires:
  - "@/components/ui Button + button-variants.ts (cva variant/size API)"
  - "design-system-lint.sh (off-palette/hex/alias gate)"
provides:
  - "Themes.vue / Browse.vue / AnimeCard.vue raw <button> sites adjudicated vs <Button>"
  - "Per-button keep-or-swap decision record (this SUMMARY's adjudication table)"
affects:
  - frontend/web/src/views/Themes.vue
  - frontend/web/src/views/Browse.vue
  - frontend/web/src/components/anime/AnimeCard.vue
tech-stack:
  added: []
  patterns:
    - "Strict 'no visible diff' adjudication: a documented KEEP is a valid outcome"
    - "Inline one-line keep-reason comment co-located with each kept <button>"
key-files:
  created: []
  modified:
    - frontend/web/src/views/Themes.vue
    - frontend/web/src/views/Browse.vue
    - frontend/web/src/components/anime/AnimeCard.vue
decisions:
  - "All 8 adjudicated raw buttons KEPT bespoke — each documented inline + in the table below; no swap reproduced its look AND behavior with zero visual diff under the 07-CONTEXT rule"
  - "Browse mobile-toggle KEPT specifically because it is the useFocusTrap returnFocusTo target — a component ref would not resolve to a focusable DOM element"
metrics:
  duration: "~10 min"
  completed: 2026-06-03
requirements: [DS-MIGRATE-06, DS-MIGRATE-01]
---

# Phase 07 Plan 01: Structural Primitive Swap (views + anime-card) Summary

Adjudicated every remaining raw `<button>` in `Themes.vue`, `Browse.vue`, and `AnimeCard.vue` against the `@/components/ui` `<Button>` primitive under the 07-CONTEXT strict "no visible diff" rule; under that rule all 8 sites are **documented KEEPs** (each with an inline one-line reason), behavior/a11y unchanged, tsc + lint-design clean. The two pre-existing `<Button variant="outline">` usages in Browse.vue (L80/L89) were left untouched.

## Per-Button Adjudication Table

| File | Site | Decision | Variant / Reason |
|------|------|----------|------------------|
| `components/anime/AnimeCard.vue` | `.play-btn` overlay | **KEEP** | 60px circular decorative play affordance; look is 100% scoped CSS (`border-radius:50%`, 60×60, `var(--destructive)`). No variant models a round 60px control (`icon` size is 40×40 square). No `@click` (card is the router-link). |
| `views/Themes.vue` | admin sync button | **KEEP** | Borderless two-state soft-bg pill (idle cyan-soft / syncing warning-soft). Closest variant `ghost` adds `border border-white/10` + a `bg-white/5` base that tailwind-merge can't strip cleanly → visible border diff. `@click`/`:disabled`/inline svg-spinner/label preserved. |
| `views/Themes.vue` | type-filter (`v-for` in `<ButtonGroup>`) | **KEEP** | Segmented-control item inside an `overflow-hidden border` container; items must be square-cornered/seamless. Every Button variant forces `rounded-lg`/`rounded-xl`, breaking the seam. `:aria-pressed` + two-state bg preserved. |
| `views/Themes.vue` | Retry | **KEEP** | Bare text-link (cyan text, no bg/border). Button has no text-only variant; `ghost`/`outline` add a filled/bordered box → visible diff. |
| `views/Browse.vue` | mobile filter toggle (L13) | **KEEP** | This button is the `useFocusTrap` `returnFocusTo` target (`toggleButtonRef` → `.focus()`). A template `ref` on `<Button>` resolves to the component instance, not the DOM element, which would break the drawer focus-return path. (bg/border match `ghost`, but the ref dependency overrides.) |
| `views/Browse.vue` | clear-recent (L56) | **KEEP** | Bare text-link (pink text, no bg/border). No text-only variant. |
| `views/Browse.vue` | recent-search pills (`v-for`, L61) | **KEEP** | `rounded-full` chip with bespoke `text-white/70` and a bg-only hover (no border-color shift). `ghost`'s `rounded-lg` + `border-white/20` hover differ; reproducing the chip needs contorting ghost's hover-border away. |
| `views/Browse.vue` | refresh-Shikimori (L107) | **KEEP** | Bare text-link (cyan) with inline spinner/icon swap. No text-only variant. |
| `views/Browse.vue` | mobile drawer close (L178) | **KEEP** | Bare `p-1` icon-only close (~28px hit area, no bg/border). `size="icon"` is 40×40 and `ghost` adds bg+border → a visible enlargement/box diff for an inline header close affordance. |

**Untouched (already done, not re-adjudicated):** `Browse.vue` L80 + L89 `<Button variant="outline">` — left exactly as-is.

## Why all-KEEP is the correct outcome here

The 07-CONTEXT rule is explicit: swap ONLY when a `Button variant size` reproduces the look AND preserves ALL behavior with **no visible diff** — otherwise keep bespoke and record the reason; "a documented keep is a valid outcome." Every site in these three files is either a bare text-link (no text-only Button variant exists), a specialized shape (60px circle, `rounded-full` chip, seamless segmented item), a borderless soft-bg pill (every variant carries a bg/border), or carries a `ref` focus dependency that a component wrapper would break. The likely-swap candidates the plan flagged each failed the strict test on a concrete, named visual or behavioral diff. The structural debt is now adjudicated and documented rather than force-swapped into a pixel-shifting regression.

## Deviations from Plan

None — plan executed as written. The adjudication landed on all-KEEP, which the plan and 07-CONTEXT explicitly sanction as a valid per-button outcome (the requirement is "where they fit", not "swap unconditionally").

## Verification

- `cd frontend/web && bunx vue-tsc --noEmit` → **exit 0**, zero errors (whole project); none in the 3 touched files.
- `cd frontend/web && bash scripts/design-system-lint.sh` → **exit 0** (RULE 1/2/3 all 0 — no off-palette/hex/alias reintroduced).
- These three files have NO co-located spec → no spec realignment needed (full vitest/vite-build deferred to 07-02 phase-close).
- Staging discipline honored: each task staged ONLY its files by explicit path; pre-existing uncommitted changes (ActivityFeed.vue analytics, scraper, changelog, root STATE.md) untouched. No `git add -A`/`.`.

## Commits

- `28e27090` — refactor(07-01): adjudicate AnimeCard + Themes.vue raw buttons vs <Button>
- `dcfaf350` — refactor(07-01): adjudicate Browse.vue raw buttons vs <Button>

## Self-Check: PASSED
