---
phase: 03-primitive-set-swap
plan: 03
subsystem: frontend/design-system
tags: [reka-ui, primitives, dropdown-menu, tooltip, popover, switch, checkbox, portal, ds-lib-06, ds-lib-08]
requires: ["03-02"]
provides:
  - "Trigger-anchored Reka DropdownMenu (with reference-prop anchored mode) exported from @/components/ui — the foundation Plan 04 rebuilds AnimeContextMenu/kebab on"
  - "Tooltip primitive on Reka Tooltip + a single app-root TooltipProvider in App.vue"
  - "Popover primitive on Reka Popover with v-model:open"
  - "Switch primitive (plain boolean v-model)"
  - "Checkbox primitive (boolean | 'indeterminate' v-model)"
  - "ui barrel re-exports DropdownMenuItem/Separator/Label sub-parts for Plan 04 menu composition"
affects: []   # greenfield additions — zero existing consumers, zero call-site risk
tech-stack:
  added: []   # no new deps — reka-ui 2.9.8 already installed; lucide intentionally NOT added (inline SVG used instead)
  patterns:
    - "Reka portal primitive behind a slotted facade (#trigger + default content slot) with a bridge handler + defineExpose for jsdom-driveable specs"
    - "Anchored-mode dropdown via a `reference` (ReferenceElement) prop forwarded to DropdownMenuContent :reference — opens without a literal trigger child"
    - "Plain boolean v-model on SwitchRoot/CheckboxRoot (Reka 2.x), bridged with a get/set computed (NOT :checked)"
    - "Local no-op ResizeObserver polyfill in popper-based specs (jsdom lacks RO) — scoped to the spec, not shared test infra"
    - "Inline-SVG indicator icons (no lucide dependency) matching the Select/Modal inline-SVG approach"
    - "Single app-root TooltipProvider as a transparent Slot wrap (no extra DOM node, no layout shift)"
key-files:
  created:
    - frontend/web/src/components/ui/DropdownMenu.vue
    - frontend/web/src/components/ui/DropdownMenu.spec.ts
    - frontend/web/src/components/ui/Tooltip.vue
    - frontend/web/src/components/ui/Tooltip.spec.ts
    - frontend/web/src/components/ui/Popover.vue
    - frontend/web/src/components/ui/Popover.spec.ts
    - frontend/web/src/components/ui/Switch.vue
    - frontend/web/src/components/ui/Switch.spec.ts
    - frontend/web/src/components/ui/Checkbox.vue
    - frontend/web/src/components/ui/Checkbox.spec.ts
  modified:
    - frontend/web/src/components/ui/index.ts
    - frontend/web/src/App.vue
decisions:
  - "DropdownMenu exposes a `reference: ReferenceElement` prop forwarded to DropdownMenuContent :reference for anchored mode; the #trigger slot is optional when reference is set — this is exactly the API Plan 04's anime-card kebab consumes."
  - "Checkbox indicator uses an inline-SVG checkmark (+ a Minus path for 'indeterminate') instead of lucide-vue-next, because lucide is NOT a project dependency and guardrail 3 forbids adding deps; matches the established Select/Modal inline-SVG pattern."
  - "DropdownMenu fade+scale transition uses real data-[state=open|closed] opacity/scale utilities (same as Modal.vue) rather than tailwindcss-animate's animate-in/fade-in-0 classes, which the project does not ship."
  - "TooltipProvider wraps the #app inner shell (inside the existing #app div) so #app stays the single root and the provider renders as a transparent Slot — no extra wrapper element, no layout shift."
  - "Barrel re-exports Reka DropdownMenuItem/Separator/Label so Plan 04 composes the menu body without importing reka-ui directly."
metrics:
  duration: ~8min wall (no day/hour units per project convention)
  completed: 2026-06-02
  tasks: 4
  files: 12
effort_metrics:
  UXdelta: "+1 (Better — five new affordances available for Plan 04 + future consumers)"
  CDI: "0.04 * 8"
  MVQ: "Sprite 85%/85%"
---

# Phase 3 Plan 03: Five New Reka Primitives (DropdownMenu/Tooltip/Popover/Switch/Checkbox) Summary

Added the five NEW shadcn-vue-on-Reka primitives this phase introduces — all greenfield, token-driven, each with a Vitest mount test — plus a single app-root `TooltipProvider`. The headline is the trigger-anchored `DropdownMenu` with a `reference`-prop anchored mode, which is the foundation Plan 04 rebuilds the anime-card kebab action-menu on. Zero existing consumers touched; clean `vue-tsc` (full build, not just --noEmit) and a clean production build.

## What shipped

- **DropdownMenu.vue** — Reka `DropdownMenuRoot`/`Trigger`/`Portal`/`Content`. Controlled `open` + `update:open` (v-model:open); optional `#trigger` slot; default slot = menu body. Token content surface `bg-popover text-popover-foreground border border-white/10 rounded-xl ...` with a real-utility fade+scale transition. **Anchored mode:** a `reference: ReferenceElement` prop is forwarded to `DropdownMenuContent :reference`, so Plan 04 can anchor the menu to the kebab button WITHOUT a literal trigger child. `align`/`side`/`sideOffset` forwarded (defaults `start`/`bottom`/`4`).
- **Tooltip.vue** — Reka `TooltipRoot`/`Trigger`/`Portal`/`Content`/`Arrow`. `delayDuration`/`side`/`sideOffset` props; `#trigger` + content slots; token content `bg-popover text-popover-foreground rounded-md px-3 py-1.5 text-xs ...`. Relies on the app-root provider below.
- **App.vue** — imports `TooltipProvider` from `reka-ui` and wraps the inner `#app` shell (`:delay-duration="300"`). It renders as a transparent Slot (no extra DOM node), so `#app` remains the single root and there is no layout shift. No existing App logic, watchers, or error handling touched.
- **Popover.vue** — Reka `PopoverRoot`(v-model:open)/`Trigger`/`Portal`/`Content`/`Arrow`. `open` + `update:open`; `side`/`align`/`sideOffset`; token content `bg-popover ... border border-white/10 rounded-xl p-4 shadow-xl`.
- **Switch.vue** — Reka `SwitchRoot`/`Thumb`, **plain boolean v-model** (get/set computed, NOT `:checked`). `data-[state=checked]:bg-primary` / `data-[state=unchecked]:bg-white/10`; thumb `bg-white rounded-full ... data-[state=checked]:translate-x-5`; `disabled` support.
- **Checkbox.vue** — Reka `CheckboxRoot`/`Indicator`, **boolean | 'indeterminate' v-model**. `border-input data-[state=checked]:bg-primary data-[state=indeterminate]:bg-primary`; inline-SVG check (and a Minus for indeterminate); `disabled` support.
- **index.ts** — adds 5 default exports (`Checkbox`, `DropdownMenu`, `Popover`, `Switch`, `Tooltip`) and re-exports `DropdownMenuItem`/`DropdownMenuSeparator`/`DropdownMenuLabel` from reka-ui for Plan 04 menu composition. All existing lines untouched.

## DropdownMenu barrel API (Plan 04 contract)

```ts
import { DropdownMenu, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuLabel } from '@/components/ui'
```

```vue
<!-- Anchored mode (anime-card kebab) — no literal trigger child needed -->
<DropdownMenu :open="open" :reference="kebabVirtualEl" align="end" side="bottom" @update:open="open = $event">
  <DropdownMenuItem @select="...">Add to list</DropdownMenuItem>
  <DropdownMenuSeparator />
  <DropdownMenuLabel>...</DropdownMenuLabel>
</DropdownMenu>
```

- Props: `open?: boolean`, `reference?: ReferenceElement`, `align?: 'start'|'center'|'end'` (def `start`), `side?: 'top'|'right'|'bottom'|'left'` (def `bottom`), `sideOffset?: number` (def `4`), `class?`.
- Emits: `update:open`. Slots: `#trigger` (optional when `reference` set) + default (menu body).
- `reference` is forwarded verbatim to `DropdownMenuContent :reference` (verified real in reka-ui@2.9.8).

## TDD evidence

- **Task 1 (DropdownMenu):** RED — spec fails to resolve `./DropdownMenu.vue` (no tests run). GREEN — 6/6 after the SFC + `onOpenUpdate`/`contentClasses` expose.
- **Task 2 (Tooltip + Popover):** RED — both specs fail to resolve their SFCs (no tests run). GREEN — Tooltip 3/3, Popover 4/4 (13/13 across all three new portal specs run together after adding a local ResizeObserver polyfill — see deviations).
- **Task 3 (Switch + Checkbox):** RED — both specs fail to resolve their SFCs (no tests run). GREEN — Switch 5/5, Checkbox 6/6 (11/11).
- **Task 4 (barrel + verify):** no separate RED/GREEN (config task); full ui/ suite 98/98 after barrel edit.

## Verification

- **Clean typecheck (guardrail 7):** deleted every `*.tsbuildinfo` (non-node_modules), then `bunx vue-tsc` (NOT --noEmit — what the Docker build runs) → **EXIT 0**.
- `bunx vitest run src/components/ui/` → 12 files / **98 tests pass**.
- `bun run build` → **clean** production build (the real gate that type-checks .spec.ts too) — EXIT 0.
- `git diff HEAD~4 --name-only | grep -c main.css` → **0** (main.css NOT in any of the 4 commits; styles/main.css untouched — `git status` shows it unmodified).
- `grep -c TooltipProvider src/App.vue` → 5 (import + open/close tags + comment). `#app` remains the single root.
- Barrel new exports present: `Checkbox`, `DropdownMenu`, `Popover`, `Switch`, `Tooltip` + the three DropdownMenu sub-parts.

## Deviations from Plan

**[Rule 3 — blocking issue] lucide-vue-next is not installed; used inline SVG instead.** The plan's Checkbox action said to `import { Check } from 'lucide-vue-next'`. lucide-vue-next is NOT a project dependency (verified: absent from package.json + node_modules; the only "lucide" hit in src is a comment in SpotlightIcon.vue). Guardrail 3 forbids adding deps. Resolution: render the checkmark (and a Minus for indeterminate) as inline `<svg>`, matching the established Select.vue/Modal.vue inline-SVG icon pattern. Behavior contract ("CheckboxIndicator renders a check icon when checked") is satisfied. Files: `Checkbox.vue`. Commit: `bf7088f6`.

**[Rule 3 — blocking issue] tailwindcss-animate not installed; used real data-[state] utilities for the DropdownMenu transition.** The plan suggested `data-[state=open]:animate-in ... fade-in-0 ... zoom-in-95` for transition parity. Those utilities require the tailwindcss-animate plugin, which the project does not ship (only `bg-popover` et al. are real tokens). Resolution: use real `data-[state=open|closed]` opacity/scale utilities (the exact approach Modal.vue already uses) so the classes actually resolve. Files: `DropdownMenu.vue`. Commit: `83e13c06`.

**In-spec robustness (not a plan deviation):** jsdom lacks `ResizeObserver`, which Reka's popper touches on the open-portal mounts. Added a scoped no-op `ResizeObserver` polyfill in `beforeAll` of the DropdownMenu/Tooltip/Popover specs — local to the spec files, no change to the shared `vitest.config.ts` or any shared setup.

**Barrel addition (in-scope, beyond the literal task text):** also re-exported `DropdownMenuItem`/`DropdownMenuSeparator`/`DropdownMenuLabel` from reka-ui, per guardrail 4 ("Export DropdownMenu + DropdownMenuItem and any sub-parts the plan specifies") so Plan 04 can compose the menu body from the barrel.

## Commits (UNPUSHED — main repo, in-place per user instruction)

- `83e13c06` feat(design-system/03): add Reka DropdownMenu primitive (DS-LIB-08)
- `be89c360` feat(design-system/03): add Tooltip + mount TooltipProvider in App.vue (DS-LIB-06)
- `f70c973a` feat(design-system/03): add Popover primitive (DS-LIB-06)
- `bf7088f6` feat(design-system/03): add Switch + Checkbox primitives + barrel exports (DS-LIB-06)

All four carry the 3 required co-authors. Nothing pushed.

## REQUIRED orchestrator in-browser gate (LIGHT — greenfield, no current consumers)

1. **App.vue TooltipProvider wrap did NOT shift layout** — during the standing 5-surface smoke (Home / Browse / Anime detail / a player / 404), confirm the app shell renders exactly as before (TooltipProvider should be a transparent Slot/Fragment). Any new stray DOM node / layout shift → file back to planner.

DropdownMenu's live behavior (anchored kebab) is gated in Plan 04 where it gets its first real consumer.

## Known Stubs

None. These are greenfield primitives with no data wiring; they ship with their full prop/slot/emit contract.

## Threat Flags

None. Per the plan threat model: portaled content is app-authored (no untrusted data); Switch/Checkbox v-model is pure UI boolean state (no auth/permission semantics); the App.vue TooltipProvider wrap is additive (Fragment/Slot — orchestrator smoke confirms no layout shift).

## Self-Check: PASSED

All 10 created source files + the 2 modified files exist; the SUMMARY exists; all 4 commits (`83e13c06`, `be89c360`, `f70c973a`, `bf7088f6`) present in git log.
