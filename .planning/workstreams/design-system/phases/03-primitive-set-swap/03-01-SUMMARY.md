---
phase: 03-primitive-set-swap
plan: 01
subsystem: frontend/design-system
tags: [ui-primitives, cva, cn, badge, input, tabs, re-skin, back-compat]
requires:
  - Phase-2 toolchain (reka-ui/cva/clsx/tailwind-merge already installed)
  - frontend/web/src/lib/utils.ts (cn helper)
  - frontend/web/src/components/ui/button-variants.ts (cva pattern reference)
provides:
  - frontend/web/src/components/ui/badge-variants.ts (badgeVariants + BadgeVariants)
  - Badge/Input/Tabs re-skinned on cva/cn() behind identical import paths
affects:
  - ~7 Badge consumers, ~5 Input consumers, ~3 Tabs consumers (zero edits required)
tech-stack:
  added: []
  patterns:
    - "cva variant map mirrors button-variants.ts shape"
    - "cn() replaces [...].join(' ') for tailwind-merge dedup"
    - "additive class?: HTMLAttributes['class'] prop (back-compatible)"
key-files:
  created:
    - frontend/web/src/components/ui/badge-variants.ts
    - frontend/web/src/components/ui/Badge.spec.ts
    - frontend/web/src/components/ui/Input.spec.ts
    - frontend/web/src/components/ui/Tabs.spec.ts
  modified:
    - frontend/web/src/components/ui/Badge.vue
    - frontend/web/src/components/ui/Input.vue
    - frontend/web/src/components/ui/Tabs.vue
    - frontend/web/src/components/ui/index.ts
decisions:
  - "Barrel index.ts edit grouped into the Badge commit (plan's atomic grouping), so Task 3 became a verification-only step rather than a separate commit."
  - "Literal Tailwind colors preserved verbatim (no bg-primary/bg-accent tokenization) — same rule Phase 2 followed."
  - "Specs authored to assert TODAY's rendered output FIRST (green against the old .join() impl), then the cn() swap kept them green — proving byte-identical rendering."
metrics:
  tasks: 3
  files: 8
  commits: 3
  completed: 2026-06-02
---

# Phase 3 Plan 01: Primitive Set Swap (Badge/Input/Tabs) Summary

Re-skinned the three lowest-risk UI primitives — Badge (onto `cva`), Input and Tabs (onto `cn()`) — behind their exact existing import paths with byte-for-byte-identical rendered output and zero consumer edits, proven by a clean `vue-tsc --noEmit` and 37 green co-located spec assertions.

## What Shipped

- **Badge** — new `badge-variants.ts` (`cva` map: 8 literal-color variants + 3 sizes + `defaultVariants`), `Badge.vue` now renders `cn(badgeVariants({ variant, size }), props.class)` with an additive `class?` prop. Identical literal classes (`bg-cyan-500/20`, `bg-purple-500/20`, etc.) — no tokenization.
- **Input** — `inputClasses` and the inline `:class` array converted from `[...].join(' ')` to `cn(...)`. `inheritAttrs:false`, `v-bind="$attrs"`, the scoped webkit-search-clear `<style>`, all props/emits/v-model, and every literal class string preserved unchanged. No Reka primitive.
- **Tabs** — `tabListClasses` and `getTabClasses` converted to `cn(...)`. All `role`/`aria-*` a11y wiring (UA-069), dynamic `#[value]` panel slots, count pill, and disabled handling preserved. No Reka primitive.
- **Barrel** — additive `export { badgeVariants, type BadgeVariants } from './badge-variants'` directly after the `Badge` export. No existing export removed or moved.

## TDD RED -> GREEN Evidence

| Task | RED | GREEN |
|------|-----|-------|
| 1 Badge | `Badge.spec.ts` failed to import (`./badge-variants` not found) | After creating `badge-variants.ts` + re-skinning `Badge.vue`: 17/17 pass |
| 2 Input+Tabs | Specs authored to lock CURRENT `.join()` output (20/20 pass = contract captured) | After `cn()` swap: 20/20 still pass — proves identical rendering |
| 3 Barrel/verify | n/a (verification step) | full wave-1: 37/37 specs pass, vue-tsc exit 0 |

## Verification Results

- `bunx vitest run src/components/ui/{Badge,Input,Tabs}.spec.ts` -> **37 passed (37)**
- `bunx vue-tsc --noEmit` -> **exit 0** (all ~15 Badge/Input/Tabs consumers still compile — DS-NF-04 proven)
- `grep -L "reka-ui" Input.vue Tabs.vue` -> both listed (neither imports Reka)
- `git diff --stat fb9cd7bd^..HEAD` -> only Badge/Input/Tabs + their specs + `badge-variants.ts` + `index.ts` (1 line); **main.css NOT touched**

## Commits (all unpushed)

| SHA | Message |
|-----|---------|
| `fb9cd7bd` | feat(design-system/03): Badge on cva + badgeVariants barrel export (DS-LIB-05) |
| `0db9f032` | feat(design-system/03): Input on cn() SFC, API preserved (DS-LIB-05) |
| `aee08405` | feat(design-system/03): Tabs on cn() SFC, a11y + slots preserved (DS-LIB-05) |

Each commit carries the 3 required co-authors. Nothing pushed.

## Effort Metrics (no days/hours)

- **UXΔ = 0 (Ambiguous)** — zero intended rendered change; pure mechanics swap.
- **CDI = 0.03 * 5** — tiny spread (4 primitives + barrel), low shift, Effort_Fib 5.
- **MVQ = Sprite 90%/85%** — small, surgical, high slop-resistance via spec-locked rendering.

## Deviations from Plan

**1. [Plan grouping] Barrel `index.ts` edit landed in the Task 1 (Badge) commit**
- **Found during:** Task 1
- **Reason:** The plan's own suggested atomic grouping (line 189) bundles `index.ts` with the Badge commit. Followed that grouping; Task 3 therefore reduced to a verification-only step (its `<files>` was just `index.ts`, already committed).
- **Impact:** None — still 3 atomic commits, explicit-path staging, all gates green. No separate barrel-only commit.

Otherwise plan executed exactly as written.

## Self-Check: PASSED

- FOUND: frontend/web/src/components/ui/badge-variants.ts
- FOUND: frontend/web/src/components/ui/Badge.spec.ts / Input.spec.ts / Tabs.spec.ts
- FOUND commits: fb9cd7bd, 0db9f032, aee08405
- main.css absent from diff; scope limited to Badge/Input/Tabs + specs + barrel
