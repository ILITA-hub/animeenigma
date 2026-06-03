---
phase: 07-structural-primitive-swap
plan: 02
subsystem: frontend/design-system
tags: [vue, button-primitive, structural-swap, adjudication, ui-migration, phase-close]
requires:
  - "@/components/ui Button + button-variants.ts (cva variant/size API)"
  - "design-system-lint.sh (off-palette/hex/alias gate)"
  - "07-01-SUMMARY adjudications (Themes/Browse/AnimeCard)"
provides:
  - "SubtitleSettingsMenu.vue + CarouselDots.vue raw <button> sites adjudicated vs <Button>"
  - "Phase-close verification result (full vitest + vue-tsc + vite build + lint-design)"
  - "DS-MIGRATE-06 + DS-MIGRATE-01 closure verdict (consolidated 07-01 + 07-02)"
affects:
  - frontend/web/src/components/player/SubtitleSettingsMenu.vue
  - frontend/web/src/components/home/spotlight/CarouselDots.vue
tech-stack:
  added: []
  patterns:
    - "Strict 'no visible diff' adjudication: a swap requires the variant to reproduce look AND behavior; otherwise documented KEEP"
    - "First in-phase real swap: <Button variant=ghost size=sm> + cn()/tailwind-merge class overrides reproducing bespoke chrome byte-for-byte"
    - "Inline one-line keep-reason comment co-located with each kept <button>"
key-files:
  created:
    - .planning/workstreams/design-system/phases/07-structural-primitive-swap/07-02-SUMMARY.md
  modified:
    - frontend/web/src/components/player/SubtitleSettingsMenu.vue
    - frontend/web/src/components/home/spotlight/CarouselDots.vue
decisions:
  - "SubtitleSettingsMenu gear toggle SWAPPED to <Button variant=ghost size=sm> — genuine labeled action button; class overrides (px-4 py-2 rounded-md bg-white/10 hover:bg-white/15 hover:border-white/10) reproduce the look with zero visible diff; Button forwards data-test/:disabled/:title/:aria-*/aria-haspopup/@click"
  - "4 nudge steppers + reset link KEPT bespoke — below Button size scale / no text-only variant; each documented inline"
  - "CarouselDots dot pills KEPT bespoke — 4px scoped-CSS pill geometry + spec class-contract do not fit the Button icon-size API"
  - "DS-MIGRATE-06 + DS-MIGRATE-01: CLOSED — all 10 audit-enumerated raw <button> sites adjudicated (1 swapped, 9 documented governance-only keeps)"
metrics:
  duration: "~6 min"
  completed: 2026-06-03
requirements: [DS-MIGRATE-06, DS-MIGRATE-01]
---

# Phase 07 Plan 02: Structural Primitive Swap (player + spotlight) Summary

Adjudicated the 5 raw `<button>` in `SubtitleSettingsMenu.vue` and the dot-pill `<button v-for>` in `CarouselDots.vue` against the `@/components/ui` `<Button>` primitive under the 07-CONTEXT strict "no visible diff" rule. The subtitle-timing **gear toggle SWAPPED** to `<Button variant="ghost" size="sm">` (the phase's first genuine swap) with `cn()`/tailwind-merge class overrides that reproduce the prior bespoke chrome byte-for-byte while preserving every `data-test`/`:disabled`/`:aria-*`/`aria-haspopup`/`@click`; the 4 compact nudge steppers, the reset text-link, and the CarouselDots dots are **documented KEEPs**. Phase-close gate is green (vue-tsc 0, vitest 831 passed / 1 known pre-existing failure, vite build clean, lint-design 0). HUMAN-UAT in-browser smoke auto-approved (auto mode).

## Per-Button Adjudication Table

| File | Site | Decision | Variant / Reason |
|------|------|----------|------------------|
| `components/player/SubtitleSettingsMenu.vue` | gear toggle (was L3) | **SWAP** | `<Button variant="ghost" size="sm">` + `class="px-4 py-2 rounded-md bg-white/10 hover:bg-white/15 hover:border-white/10 disabled:opacity-40"`. Genuine labeled action button. Ghost base (`bg-white/5 hover:bg-white/10 rounded-lg border border-white/10`) + class overrides via `cn()`/tailwind-merge → byte-identical render (bg-white/10, rounded-md, static border, gap-2, `[&_svg]:size-4` matching the inline `w-4 h-4`). Button forwards `data-test="sub-timing-gear"`, `:disabled` (`disabled\|\|loading`), `:title`, `:aria-label`, `:aria-expanded`, `aria-haspopup`, `@click`. Spec (`disabled` attr when `!hasActiveSub`) stays green. |
| `components/player/SubtitleSettingsMenu.vue` | nudge −1s | **KEEP** | Compact player-chrome stepper `px-2 py-1 rounded` (smaller than Button's smallest `sm` = `px-3 py-1.5`), no border (ghost forces `border border-white/10 hover:border-white/20`), bespoke `focus-visible:ring-cyan-400/60` (Button base uses `ring-ring`). A swap would have to neutralize bg/hover/rounded/border/focus-ring — the variant doesn't model it. Documented inline. |
| `components/player/SubtitleSettingsMenu.vue` | nudge −0.1s | **KEEP** | Same compact-stepper rationale as −1s. |
| `components/player/SubtitleSettingsMenu.vue` | nudge +0.1s | **KEEP** | Same compact-stepper rationale. |
| `components/player/SubtitleSettingsMenu.vue` | nudge +1s | **KEEP** | Same compact-stepper rationale. |
| `components/player/SubtitleSettingsMenu.vue` | reset (was L35) | **KEEP** | Bare underlined text link (`text-xs text-white/50 hover:text-white/80 underline`, no bg/border). Button has no text-only variant; `ghost`/`outline` add a filled/bordered box → visible diff. Documented inline. |
| `components/home/spotlight/CarouselDots.vue` | dot-pill `<button v-for>` (was L39) | **KEEP** | 4px pill geometry forced by scoped CSS (`.dot-pill` 26×4 / 36×4, `!important` glass bg, cyan glow) + a spec asserting raw `bg-white/10` / `bg-purple-*` / `scale-110` classes. Button min size `icon` = 40×40, `rounded-xl`, variant bg — cannot express a 4px pill and would change the rendered class set → break both visuals and spec. Canonical "specialized control the Button API doesn't model." Documented inline. |

**Outcome:** 1 SWAP, 6 documented KEEPs in this plan. The gear swap is the structural close-out proof that the swap path itself is exercised in the phase (07-01 landed all-KEEP).

## Phase-Close Verification (Task 2)

Run over the WHOLE phase's swaps (07-01 + 07-02 both landed):

| Command | Result |
|---------|--------|
| `bunx vue-tsc --noEmit` | **exit 0** — zero errors (whole project) |
| `bunx vitest run` | **831 passed, 1 failed** — the single failure is the known pre-existing `AnimeContextMenu.spec.ts:227` (documented in 07-CONTEXT). NO new failures. `Test Files 1 failed \| 62 passed (63)` / `Tests 1 failed \| 831 passed (832)` |
| `bunx vite build` | **exit 0** — bundle built clean (no type/import/build errors) |
| `bash scripts/design-system-lint.sh` | **exit 0** — RULE 1/2/3 all 0 (no off-palette/hex/alias reintroduced) |

Co-located specs `SubtitleSettingsMenu.spec.ts` (6 tests) + `CarouselDots.spec.ts` (9 tests) → **15 passed**: the gear swap kept all `data-test` + `:disabled` assertions green; the CarouselDots class-contract assertions (`bg-white/10`, `bg-purple-*`, `scale-110`, `aria-current`, `aria-label`, `title`) stay valid (kept bespoke). No spec realignment needed.

Terminal printed `PHASE-CLOSE-GATE-PASS`.

## HUMAN-UAT (DS-NF-06)

The `checkpoint:human-verify` (in-browser smoke of affected surfaces: Browse, Themes, Home spotlight dots, a JP-subtitle player at desktop + mobile widths) was **auto-approved** under active auto mode. jsdom-level verification (full vitest + vite build) is already green; the cascade-bug-catching in-browser pass is recorded as a HUMAN-UAT gate that the orchestrator may surface for a real-browser confirmation if desired.

## DS-MIGRATE-06 + DS-MIGRATE-01 Closure Verdict

✅ **CLOSED — residual bespoke controls accepted as governance-only with recorded reasons.**

Consolidating both plans, all 10 audit-enumerated raw `<button>` sites are adjudicated:
- **07-01** (Themes / Browse / AnimeCard): 8 sites, all documented KEEPs (bare text-links, a 60px circle play affordance, a `rounded-full` chip, a seamless segmented-control item, a borderless soft-bg pill, a `ref`-focus-dependency toggle, a `p-1` icon close).
- **07-02** (SubtitleSettingsMenu / CarouselDots): 7 sites — **1 SWAP** (gear toggle → ghost), 6 KEEPs (4 compact steppers, 1 reset link, 1 dot-pill).

Net: **1 swapped, 9 documented governance-only keeps.** The "where they exist/fit" requirement is satisfied — the one site whose appearance + behavior fit a Button variant with zero visible diff was swapped; the rest are specialized controls (text-links, sub-scale steppers, geometry-overridden pills, ref-dependent toggles) the Button variant/size API doesn't model without a pixel-shifting regression. The structural primitive-migration debt is now adjudicated and documented rather than force-swapped. Per Phase-6 decision, no lint rule was added to flag raw `<button>` — primitive reuse stays governance-only.

## Deviations from Plan

None — plan executed as written. Task 1 landed a mix (1 swap + 6 keeps), which the plan and 07-CONTEXT explicitly sanction as the expected honest outcome. No spec realignment was required (the gear swap preserved every `data-test`/`:disabled`; the kept buttons retained their raw markup).

## Staging Discipline

Each commit staged ONLY this plan's files by explicit path (`frontend/web/src/components/player/SubtitleSettingsMenu.vue`, `frontend/web/src/components/home/spotlight/CarouselDots.vue`). Pre-existing uncommitted changes (ActivityFeed.vue analytics, scraper Go files, changelog.json, root STATE.md, etc.) left untouched. No `git add -A`/`.`/`-am`.

## Commits

- `484de820` — refactor(07-02): adjudicate SubtitleSettingsMenu + CarouselDots raw buttons vs <Button>

## Self-Check: PASSED
