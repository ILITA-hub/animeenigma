---
phase: 03-primitive-set-swap
plan: 02
subsystem: frontend/design-system
tags: [reka-ui, primitives, select, dialog, portal, scroll-lock, ds-nf-04]
requires: ["03-01"]
provides:
  - "Select.vue on Reka SelectRoot behind the legacy options-array API"
  - "Modal.vue on Reka Dialog (in-place, alias-kept) with useBodyScrollLock retained"
  - "ui barrel exports both Modal and Dialog (alias) from Modal.vue"
affects:
  - frontend/web/src/views/Game.vue (Select + Modal consumer, zero edits)
  - frontend/web/src/views/Profile.vue (Select ×3 + Modal consumer, zero edits)
  - frontend/web/src/components/layout/FeedbackButton.vue (Modal consumer, zero edits)
  - frontend/web/src/components/player/OtherSubsPanel.vue (Modal consumer, zero edits)
tech-stack:
  added: []   # no new deps — reka-ui 2.9.8 already installed in wave 1
  patterns:
    - "Reka primitive behind a legacy options-array/v-model facade via a bridge handler + defineExpose for jsdom-driveable specs"
    - "data-[state=open|closed] / data-[highlighted] / data-[state=checked] Tailwind utilities replace manual reactive state classes"
    - "useBodyScrollLock kept on top of Reka Dialog (Reka does NOT auto scroll-lock)"
key-files:
  created:
    - frontend/web/src/components/ui/Select.spec.ts
    - frontend/web/src/components/ui/Modal.spec.ts
  modified:
    - frontend/web/src/components/ui/Select.vue
    - frontend/web/src/components/ui/Modal.vue
    - frontend/web/src/components/ui/index.ts
decisions:
  - "Kept filename Modal.vue + Modal barrel export; added Dialog alias to the SAME file (RESEARCH Decision #1) — no codemod, no consumer edits."
  - "Centered DialogContent via a fixed pointer-events-none flex wrapper (content re-enables pointer-events) so Reka pointer-down-outside still fires on the overlay while preserving the legacy centered layout."
  - "Bridged Reka's string-only Select model back to the original number|string values by identity-matching against options, so numeric option values survive the round-trip."
metrics:
  duration: ~4.5min wall (no day/hour units per project convention)
  completed: 2026-06-02
  tasks: 3
  files: 5
effort_metrics:
  UXdelta: "0 (Ambiguous — zero intended rendered change; swaps not redesigns)"
  CDI: "0.05 * 13"
  MVQ: "Griffin 80%/75%"
---

# Phase 3 Plan 02: Select + Modal→Dialog Reka Swap Summary

Swapped the two MEDIUM/HIGH-risk existing primitives — Select and Modal — onto Reka UI behind their exact existing prop/event/v-model/slot surfaces, with the Modal file re-skinned in place and a new `Dialog` barrel alias added so both names resolve to the same component. Zero consumer edits; `vue-tsc --noEmit` clean.

## What shipped

- **Select.vue** now renders on Reka `SelectRoot`/`SelectTrigger`/`SelectValue`/`SelectIcon`/`SelectPortal`/`SelectContent`/`SelectViewport`/`SelectItem`/`SelectItemText`/`SelectItemIndicator`. The legacy `options`-array API, `SelectOption` export, `xs|sm|md|lg` sizes, `modelValue`/`placeholder`/`label`/`disabled` props, and BOTH `update:modelValue` + `change` emits are preserved. Content uses `position="popper"` + `width: var(--reka-select-trigger-width)` for trigger-matched width. State styling moved to `data-[state=open]` (trigger ring/border + chevron rotation), `data-[highlighted]` (keyboard/hover focus), and `data-[state=checked]` (selected cyan) utilities.
- **Modal.vue** re-skinned in place on Reka `DialogRoot`/`DialogPortal`/`DialogOverlay`/`DialogContent`/`DialogTitle`. Exact prop surface (`modelValue`/`title`/`size`/`closable`/`closeOnBackdrop`/`closeOnEsc`), `update:modelValue`+`close` emits, and `#header`/`#footer`/default slots preserved. `useBodyScrollLock(toRef(props,'modelValue'))` kept EXACTLY (Reka does not auto scroll-lock). `closeOnEsc=false`/`closeOnBackdrop=false` map to `preventDefault()` on `@escape-key-down`/`@pointer-down-outside`/`@interact-outside`. `.modal-*` scale+fade transition replicated with `data-[state=open|closed]` opacity+scale utilities (visually-equivalent — pixel parity goes to the browser gate).
- **index.ts** keeps `export { default as Modal } from './Modal.vue'` and adds `export { default as Dialog } from './Modal.vue'` directly after; `SelectOption` re-export and all other lines untouched.

## TDD evidence

- **Task 1 (Select):** RED — `w.vm.onSelect is not a function` (1 failed / 6 passed). GREEN — 7/7 pass after the Reka rewrite exposed the `onSelect` bridge.
- **Task 2 (Modal):** RED — 5 failed / 3 passed (`onEscapeKeyDown`/`onPointerDownOutside`/`onOpenUpdate` undefined; no `role="dialog"`; no scroll-lock). GREEN — 8/8 pass after the Reka Dialog rewrite + spec-level wrapper-unmount discipline (so the module-level `useBodyScrollLock` refcount resets between tests).

## Verification

- `bunx vitest run src/components/ui/` → 7 files / **74 tests pass** (Badge/Input/Tabs/Select/Modal/Button/Card).
- `bunx vue-tsc --noEmit` → **EXIT 0** (proves Game.vue + Profile.vue Select consumers and Game/Profile/FeedbackButton/OtherSubsPanel Modal consumers compile against the preserved API + the Dialog alias resolves).
- `bun run build` → **clean** production build.
- `grep "useBodyScrollLock" Modal.vue` → present (2 hits). `grep "as Dialog" index.ts` → present. `grep SelectRoot Select.vue` / `DialogRoot Modal.vue` → present.
- `git diff HEAD~2 --name-only | grep -c main.css` → **0** (main.css NOT in either of my commits; styles/main.css untouched).

## Deviations from Plan

None — plan executed as written. Task 3 created no separate commit by design: the plan's own commit-grouping spec places `index.ts` in the Task-2 commit, so the barrel edit shipped atomically with Modal.vue. Both commits were staged by explicit path (never `git add -A`).

One in-spec robustness addition (not a plan deviation): the Modal spec tracks mounted wrappers and unmounts them in `afterEach` so the module-level `useBodyScrollLock` refcount resets between tests — required for a reliable scroll-lock assertion.

## Commits (UNPUSHED — main repo, in-place per user instruction)

- `617a76d1` feat(design-system/03): Select on Reka Select, options-array API + both emits preserved (DS-LIB-05)
- `335c81f8` feat(design-system/03): Modal on Reka Dialog + Dialog barrel alias, scroll-lock kept (DS-LIB-05, DS-NF-04)

Both carry the 3 required co-authors. Nothing pushed.

## REQUIRED orchestrator in-browser gate (jsdom cannot verify — DS-NF-06)

1. **Select — player Source dropdown** (busiest `xs`/`sm` consumer, OurEnglish/Raw): dropdown opens, is correctly POSITIONED + WIDTH-MATCHED to the trigger over player chrome (RESEARCH Pitfall 5), keyboard nav (Arrow/Home/End/Enter/Esc) works, selection updates. Spot-check Profile sort + status (×3) + Game settings.
2. **Dialog — the 4 Modal consumers** (Profile avatar modal, FeedbackButton modal, Game create-room modal, OtherSubsPanel subtitle picker): opens CENTERED with the glass-elevated look, Esc + backdrop close, the opt-out variants (`closeOnEsc=false`/`closeOnBackdrop=false`) do NOT close, body scroll is LOCKED while open, focus is TRAPPED, and the transition reads as the same scale+fade.
3. **SubtitleOverlay non-interference** (RESEARCH Pitfall 4 / assumption A3): confirm none of the 4 Modals open over fullscreen video and that opening any Dialog does not steal keyboard control from a fullscreen player.
4. **Standing 5-surface smoke**: Home / Browse / Anime detail / a player / 404 — no incidental regression.

## Known Stubs

None.

## Threat Flags

None — no new network/auth/file/schema surface introduced (typed props only; portal targets unchanged at body).

## Self-Check: PASSED

All 5 scoped source files + the SUMMARY exist; both commits (`617a76d1`, `335c81f8`) present in git log.
