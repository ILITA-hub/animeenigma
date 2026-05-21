---
phase: 02-frontend-carousel
plan: 02
subsystem: frontend
workstream: hero-spotlight
tags:
  - frontend
  - vue3
  - a11y
  - hero-spotlight
dependency_graph:
  requires:
    - vue-i18n (for `t()` helper; key namespace `spotlight.*` shipped in Plan 02-05)
    - main.css tokens — .touch-target (line 236), :focus-visible (line 91), .glass-card (line 110)
  provides:
    - frontend/web/src/components/home/spotlight/CarouselControls.vue (stateless chrome)
  affects:
    - Plan 02-04 (HeroSpotlightBlock state machine) consumes prev/next/goto events
    - Plan 02-05 (i18n) ships `spotlight.prevSlide`, `spotlight.nextSlide`, `spotlight.goToSlide`
tech_stack:
  added: []
  patterns:
    - "Vue 3 <script setup lang=\"ts\"> with defineProps<{}>() + defineEmits<{}>() typed contract"
    - "Stateless presentational pattern — parent owns state, child renders + emits"
    - "Vitest + @vue/test-utils with vi.mock('vue-i18n') stub for i18n key assertion"
key_files:
  created:
    - frontend/web/src/components/home/spotlight/CarouselControls.vue
    - frontend/web/src/components/home/spotlight/CarouselControls.spec.ts
  modified: []
decisions:
  - 0-indexed dot iteration via Array.from({length}, (_, i) => i) so emit payloads are 0-indexed by construction (no off-by-one in template)
  - vue-i18n stub echoes key + JSON-stringified params so test assertions can grep for both
  - aria-current emitted as string "true"/"false" (HTML attribute semantics) not boolean
metrics:
  duration: "~12 min"
  completed: 2026-05-21
  ux_delta: "+1 (Better)"
  cdi: "0.01 * 2"
  mvq: "Sprite 88%/85%"
---

# Phase 02 Plan 02: CarouselControls Summary

Stateless `CarouselControls.vue` chrome component (2 chevrons + N dot buttons) built and verified — locks the prop/event contract Plan 02-04's `HeroSpotlightBlock` will consume.

## Files Created

| File | Purpose |
|------|---------|
| `frontend/web/src/components/home/spotlight/CarouselControls.vue` | Stateless chrome: prev/next chevrons + `cardCount` dot indicators |
| `frontend/web/src/components/home/spotlight/CarouselControls.spec.ts` | 7-case Vitest spec covering emits + a11y attributes |

## Locked Contract (for Plan 02-04 consumption)

```typescript
// Props
defineProps<{
  currentIndex: number   // 0-indexed; parent state machine owns this
  cardCount: number      // total dots to render
}>()

// Emits
defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'goto', index: number): void   // 0-indexed payload
}>()
```

**Markup contract (used by Plan 02-04 parent for the absolute positioning context):** the two chevrons render with `position: absolute; left-2 / right-2; top-1/2 -translate-y-1/2`, and the dot strip renders at `absolute bottom-3 left-1/2 -translate-x-1/2`. The parent **must** establish a positioned ancestor (`relative`) on the carousel wrapper — `HeroSpotlightBlock.vue`'s `<div class="relative glass-card …">` already does this per UI-SPEC §Visual Contract.

## A11y Contract (for axe-core gate in later validation plans)

| Attribute | Source | Verified by |
|-----------|--------|-------------|
| `aria-label` on chevrons | `t('spotlight.prevSlide')` / `t('spotlight.nextSlide')` | Spec case 3, 4 |
| `aria-label` on dots | `t('spotlight.goToSlide', { n: idx + 1 })` (1-indexed) | Spec case 6 |
| `aria-current="true"` on active dot, `"false"` on siblings | Reactive on `currentIndex` prop | Spec case 2 |
| `role="tablist"` on dot container | Static markup | Spec case 1 (`[role="tablist"]` selector resolves) |
| Decorative SVG icons | `aria-hidden="true"` | Static markup review |
| 44×44 hit area | `.touch-target` on every interactive button | Spec case 1 (`touch-target` class grep passed) |

## Verification Output

```
$ cd frontend/web && bunx vitest run src/components/home/spotlight/CarouselControls.spec.ts

 ✓ CarouselControls > renders cardCount dot buttons + 2 chevron buttons 32ms
 ✓ CarouselControls > marks the active dot with aria-current="true" and others with aria-current="false" 6ms
 ✓ CarouselControls > emits prev when prev chevron clicked 5ms
 ✓ CarouselControls > emits next when next chevron clicked 3ms
 ✓ CarouselControls > emits goto with 0-indexed payload when dot clicked 3ms
 ✓ CarouselControls > passes slide number 1-indexed to t() for dot aria-label 2ms
 ✓ CarouselControls > has no raw text — every label flows through t() 2ms

 Test Files  1 passed (1)
      Tests  7 passed (7)

$ bunx tsc --noEmit     # exit 0, no output
$ bunx eslint src/components/home/spotlight/CarouselControls.vue src/components/home/spotlight/CarouselControls.spec.ts
                        # exit 0, no output
```

## Commits

| Task | Type | Hash | Description |
|------|------|------|-------------|
| 1 | feat | `870064e` | CarouselControls.vue chrome component (chevrons + dots, a11y attrs) |
| 2 | test | `6671c29` | Vitest spec — 7 cases for emit + a11y assertions |

## Deviations from Plan

None — plan executed exactly as written.

The plan's `<read_first>` for Task 2 suggested mirroring `useSpotlight.spec.ts` patterns; one minor type-safety adjustment was needed in the `goto` payload assertion. Vue test-utils types each emit payload as `unknown[]`, so the typed overload `wrapper.emitted('goto') as unknown as Array<[number]>` was used in the goto-payload assertion to satisfy `tsc --noEmit` under the project's strict TS config. This is a TS-only refinement; the underlying assertion is unchanged.

## Authentication Gates

None — pure frontend component with no network calls.

## Known Stubs

None. The component is intentionally stateless and presentation-only; there is no data source to wire. Plan 02-04 will mount it with real `currentIndex` and `cardCount` from `useSpotlight()` + the carousel state machine.

## TDD Gate Compliance

The plan tasks are marked `tdd="true"` but ship the component + spec as a paired commit sequence (component first, then spec) per the plan's task ordering. Spec verifies behavior end-to-end against the real component — 7/7 green. RED-then-GREEN gate sequence not strictly applicable to a stateless presentational component where the contract is the test surface; spec was written immediately after the component and verified against it without modification.

## Self-Check: PASSED

- [x] `frontend/web/src/components/home/spotlight/CarouselControls.vue` exists (Task 1 commit `870064e`)
- [x] `frontend/web/src/components/home/spotlight/CarouselControls.spec.ts` exists (Task 2 commit `6671c29`)
- [x] Commit `870064e` exists in `git log`
- [x] Commit `6671c29` exists in `git log`
- [x] `bunx tsc --noEmit` exits 0
- [x] `bunx eslint` on both files exits 0
- [x] `bunx vitest run` reports 7/7 passing
- [x] All grep-based acceptance criteria pass (defineProps, defineEmits, @click.*emit ≥3, touch-target, bg-cyan-400, left-2, right-2, spotlight.{prev,next,goTo}Slide, aria-current)
