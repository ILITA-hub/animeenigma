# Phase 3: Primitive Set Swap - Research

**Researched:** 2026-06-02
**Domain:** Vue 3.4 + Reka UI 2.9.8 + cva/cn primitive migration (shadcn-vue New York style), Tailwind v4 CSS-first
**Confidence:** HIGH (codebase APIs verified by reading every component; library behavior CITED from reka-ui.com June-2026 docs + VERIFIED versions in package.json)

## Summary

Phase 3 moves the six existing `components/ui/` primitives (Badge, Input, Select, Modal, Tabs, ContextMenu) onto the shadcn-vue stack already installed in Phase 2 (`reka-ui@2.9.8`, `cva@0.7.1`, `clsx@2.1.1`, `tailwind-merge@3.6.0`, `cn()` at `@/lib/utils`), and adds four greenfield primitives (Tooltip, Popover, Switch, Checkbox). The hard constraint is **identical rendering + back-compatible public API** for the existing six (DS-NF-04), so consumers (~18 import sites across barrel + relative paths) keep compiling under `vue-tsc --noEmit` with zero call-site edits.

The critical research finding is that **not all six "swaps" are equal-risk, and two of them are not safe 1:1 Reka swaps at all.** Badge, Input, and Tabs are pure presentational/controlled components with no portal/focus behavior — they should be re-skinned onto `cva` (Badge) or kept as hand-rolled `cn()`-driven SFCs (Input/Tabs) with no Reka primitive needed, mirroring how Button/Card were done in Phase 2. Select and Modal involve teleport + focus-trap + scroll-lock, where Reka introduces real behavioral deltas that risk both rendering and interaction parity. **ContextMenu is the outlier: it is a cursor-positioned (`visible`/`x`/`y`) context menu that is NOT exported from the barrel and is consumed only by `AnimeContextMenu.vue` via a relative import — and Reka's `DropdownMenu` is trigger-anchored only (no arbitrary x/y positioning), so it cannot be a drop-in replacement.**

**Primary recommendation:** Re-skin Badge onto `cva`; keep Input/Tabs as token-driven `cn()` SFCs (no Reka needed, lowest risk); wrap Reka `Select` and `Dialog` behind the existing prop/event/v-model surface with explicit teleport-target and scroll-lock parity work. For the two renames, **keep the old filenames and the old barrel names as the source of truth** — do NOT rename files or run a consumer codemod (mixed barrel + relative imports make a codemod higher-churn than aliasing). For `Modal→Dialog`: keep `Modal.vue` (optionally re-export from a new `Dialog.vue` thin wrapper) and keep the `Modal` barrel export. For `ContextMenu→DropdownMenu`: **leave `ContextMenu.vue` as-is (do not force it onto Reka DropdownMenu)** — its cursor-positioning model is incompatible; instead add a new barrel-exported `DropdownMenu` built on Reka for future trigger-anchored use, and flag the ContextMenu→DropdownMenu rename as satisfied-by-addition. Build the 4 new primitives as standard shadcn-vue compositions with a mount/Vitest test each.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| All 10 primitives | Browser / Client (Vue SFC) | — | Pure presentational + interaction layer; no server tier. Vite SPA, no SSR. |
| Token resolution (`bg-primary`, `border-input`) | CSS (Tailwind v4 `main.css`) | — | Tokens already canonical in `main.css` (Phase 2). Components only consume utilities; never touch `main.css`. |
| Teleport target (Dialog/Select/Tooltip/Popover content) | Browser DOM (`body` portal) | Fullscreen element (SubtitleOverlay) | Portal-to-body must not break the SubtitleOverlay fullscreen teleport (see Pitfall 4). |

## User Constraints

> No CONTEXT.md found at `.planning/workstreams/design-system/phases/03-primitive-set-swap/`. The constraints below are extracted verbatim from the phase brief + CLAUDE.md and carry the same authority as locked decisions.

### Locked Decisions (from brief + Phase 2 carry-over)
- **Never touch `main.css`.** Tokens are canonical there; components consume utilities only. [CITED: CLAUDE.md / brief]
- `--accent` is NOT flipped (no `bg-accent` util) and there is no `--radius` util — use literal `bg-white/*` hovers + literal radii, same as Phase 2. [CITED: brief; VERIFIED: Button.spec.ts asserts `.not.toContain('bg-accent')`]
- Keep import paths + back-compatible prop/event/slot surface so consumers don't break (DS-NF-04). [CITED: brief]
- The 6 app-composites (`ButtonGroup`, `GenreFilterPopup`, `PaginationBar`, `SearchAutocomplete`, `Skeleton`, `Toaster`) are OUT of scope — do NOT swap them (DS-LIB-07). [CITED: brief]
- Reuse Phase-2 toolchain: `reka-ui@2.9.8`, `cva@0.7.1`, `clsx@2.1.1`, `tailwind-merge@3.6.0`, `cn()` at `@/lib/utils`. Mirror Button + Card patterns. [VERIFIED: package.json + Button.vue/Card.vue read]
- **Rendering identical to today** for the existing 6. [CITED: brief]

### Claude's Discretion
- Rename-alias vs codemod strategy (this research recommends alias — see Rename Decision).
- Whether each existing primitive uses a Reka base or stays a hand-rolled `cn()` SFC (recommendations per-primitive below).
- Task sequencing (recommended below).

### Deferred Ideas (OUT OF SCOPE)
- The 6 composites above.
- Forcing `ContextMenu` onto Reka `DropdownMenu` (incompatible positioning model — flagged, see Rename Decision #2).

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DS-LIB-05 | Swap Badge/Input/Select/Modal→Dialog/Tabs/ContextMenu→DropdownMenu behind same paths | Per-primitive table below maps each to its Reka base (or "keep SFC") + back-compat plan + render-risk. ContextMenu carve-out documented. |
| DS-LIB-06 | Add Tooltip/Popover/Switch/Checkbox | Reka bases + shadcn-vue structure + token classes documented; greenfield, mount/Vitest test each. |
| DS-LIB-07 | 6 composites OUT of scope | Explicitly excluded; do not touch. |
| DS-NF-04 | Keep import paths + back-compat prop/event/slot surface | Each entry preserves exact props/events/v-model/slots; mixed barrel + relative imports inventoried; `vue-tsc --noEmit` is the compile gate. |

## Current Consumer Map (VERIFIED by grep)

Consumers import via TWO styles — this is load-bearing for the rename decision:
- **Barrel:** `import { Badge, Button, Modal, Tabs, Select } from '@/components/ui'` (8 files)
- **Relative:** `import Badge from '@/components/ui/Badge.vue'` / `import Modal from '@/components/ui/Modal.vue'` (10 files)

| Primitive | Consumer count | Consumer surfaces (where it renders) | Import style |
|-----------|----------------|--------------------------------------|--------------|
| **Badge** | 7 SFCs | `Game.vue`, `Profile.vue`, `Anime.vue`, `admin/RawLibrary.vue`, `anime/EpisodeCard.vue`, `anime/AnimeCardNew.vue`, `player/OtherSubsPanel.vue` | mixed (barrel: Game/Profile/Anime; relative: EpisodeCard/AnimeCardNew/OtherSubsPanel/RawLibrary) |
| **Input** | 1 SFC (+1 internal) | `Game.vue` (room create form); internally re-used by `ui/SearchAutocomplete.vue` (composite, out of scope but imports `./Input.vue`) | barrel (Game) + relative (SearchAutocomplete) |
| **Select** | 2 SFCs | `Game.vue` (room settings), `Profile.vue` (sort/status dropdowns ×3) | barrel |
| **Modal** | 5 SFCs | `Game.vue` (create room), `Profile.vue` (avatar), `player/OtherSubsPanel.vue` (subs picker), `layout/FeedbackButton.vue` (feedback), + `WatchTogetherView.spec.ts` reference | barrel (Game/Profile) + relative (OtherSubsPanel/FeedbackButton) |
| **Tabs** | 2 SFCs | `Profile.vue` (profile sections), `Anime.vue` (detail tabs) | barrel |
| **ContextMenu** | 1 SFC | `anime/AnimeContextMenu.vue` ONLY (which is itself used by Home/Browse/Schedule/Anime/Profile via `useContextMenu`) | **relative only, NOT in barrel** |

**Verification gate:** `cd frontend/web && bunx vue-tsc --noEmit` must pass with zero new errors after each primitive — it proves all ~18 consumer files still compile against the preserved API.

## Per-Primitive Migration Table

| Primitive | Current public API (props / events / v-model / slots) | Reka base (or decision) | Token classes / look | Back-compat plan | Render risk |
|-----------|--------------------------------------------------------|-------------------------|----------------------|------------------|-------------|
| **Badge** | props: `variant` (8: default/primary/secondary/success/warning/rating/info/destructive), `size` (sm/md/lg). slots: default, `#icon`. No events/v-model. | **`cva` only, no Reka primitive** (render `<span>` via `cn(badgeVariants(...))`). Mirrors `button-variants.ts`. | Keep EXACT literal classes from `Badge.vue` lines 26-43 (`bg-cyan-500/20 text-cyan-400` etc.) — these are NOT tokens, do not "tokenize" them or look shifts. Move to `badge-variants.ts`. | Identical: same `<span>`, same slot names, same prop names/values. Export `badgeVariants` + `BadgeVariants` type alongside (mirror Button). | LOW |
| **Input** | props: `modelValue`(string), `type`, `placeholder`, `label`, `hint`, `error`, `disabled`, `readonly`, `clearable`, `size`(sm/md/lg), `variant`. emits: `update:modelValue`. v-model: default (string). slots: `#prefix`, `#suffix`. `inheritAttrs:false` + `v-bind="$attrs"` passthrough. Has scoped `<style>` (hides webkit search clear). | **Keep hand-rolled `cn()` SFC, no Reka primitive.** Reka has no Input primitive; shadcn-vue Input is a plain `<input>` wrapper. The current component's label/hint/error/clearable/prefix/suffix scaffold is app-specific and must be preserved. | Convert the `[...].join(' ')` class builders to `cn(...)` for tailwind-merge dedup, but keep every literal class identical (lines 85-102). Preserve scoped `<style>`. | Identical: keep all props/emits/slots, keep `inheritAttrs:false`. | LOW |
| **Tabs** | props: `modelValue`(string), `tabs`(Tab[] with value/label/icon/count/disabled), `variant`(default/pills/underline), `fullWidth`. emits: `update:modelValue`. v-model: default (string). slots: dynamic `#[value]` (one panel slot per tab value). a11y: role=tablist/tab/tabpanel, aria-selected/controls/labelledby (UA-069). | **Keep hand-rolled `cn()` SFC, no Reka primitive.** Reka `Tabs` (TabsRoot/List/Trigger/Content) would require restructuring the `tabs` array prop + dynamic-named slots → API break. The existing a11y wiring is already correct. | Convert class builders to `cn()`; keep literal variant classes (lines 88-101) identical. | Identical: data-driven `tabs` prop + dynamic slot names preserved. | LOW |
| **Select** | props: `modelValue`(string\|number), `options`(SelectOption[]), `placeholder`, `label`, `disabled`, `size`(xs/sm/md/lg). emits: `update:modelValue`, `change`. v-model: default. NO portal today (inline `absolute` `<ul>`). Custom keyboard nav (Arrow/Home/End/Enter/Esc), click-outside, transition. | **Reka `SelectRoot`+`SelectTrigger`+`SelectValue`+`SelectContent`+`SelectViewport`+`SelectItem`+`SelectItemText`+`SelectItemIndicator`.** Wrap behind the `options`-array API (map options→`SelectItem` v-for). | Reproduce trigger classes (lines 113-131) + dropdown classes (`bg-slate-900/95 backdrop-blur-xl border border-white/10 rounded-xl`, lines 142-145) + option classes (lines 148-160) on Reka parts. Need `xs` size (Reka examples only show md). | Keep props/emits/v-model + `SelectOption` type (barrel re-exports it — keep that export). Map `change` emit off Reka's `@update:modelValue`. | **MEDIUM** — Reka `SelectContent` defaults to `position="popper"` + Portal-to-body (today it's inline `absolute`). Two deltas: (1) `position` changes stacking/overflow; consider `position="popper"` and a fixed trigger width via `--reka-select-trigger-width`. (2) Portal means dropdown leaves the wrapper — verify it still appears over player source dropdowns / Profile. Flag for **in-browser gate**. |
| **Modal → Dialog** | props: `modelValue`(boolean), `title`, `size`(sm/md/lg/xl/full), `closable`, `closeOnBackdrop`, `closeOnEsc`. emits: `update:modelValue`, `close`. v-model: default (boolean = open). slots: default, `#header`, `#footer`. Uses `Teleport to="body"`, `useBodyScrollLock`, manual focus, `glass-elevated` class, `.modal-*` scoped transitions. | **Reka `DialogRoot`+`DialogPortal`+`DialogOverlay`+`DialogContent`+`DialogTitle`** behind the existing prop surface. Map `v-model="modelValue"`→`v-model:open`. | Reproduce `glass-elevated rounded-2xl p-4 sm:p-6` content + `bg-black/60 backdrop-blur-sm` overlay + per-`size` `max-w-*` (lines 87-99). Keep `#header`/`#footer` slots + `title`/`closable` close button. | Keep filename `Modal.vue` + barrel name `Modal` (see Rename Decision #1). Map `closeOnEsc=false`→`@escapeKeyDown.prevent`, `closeOnBackdrop=false`→`@pointerDownOutside.prevent`/`@interactOutside.prevent`; emit `close` on close. | **MEDIUM/HIGH** — Reka traps focus + manages its own scroll behavior (docs: does NOT auto scroll-lock — must keep `useBodyScrollLock` to match Navbar drawer cooperation). Reka transition timing differs from the `.modal-*` CSS; replicate via `data-[state=open]`/`closed` utility transitions. Portal-to-body + focus trap could interact with SubtitleOverlay fullscreen teleport (Pitfall 4). **Flag for in-browser gate.** |
| **ContextMenu → DropdownMenu** | props: `visible`(bool), `x`(number), `y`(number). emits: `update:visible`. provides `ctxMenuClose(reason)`. slot: default `{ closeWithReason }`. Teleport-to-body, **cursor-positioned via `left:x/top:y` + viewport clamp**, hover-grace timers, focus-return-by-reason. NOT in barrel. | **DO NOT force onto Reka `DropdownMenu`.** Reka DropdownMenu is **trigger-anchored only — no arbitrary x/y positioning** [CITED: reka-ui.com/docs/components/dropdown-menu]. The current component is a cursor/right-click menu. Forcing it breaks `AnimeContextMenu` + `useContextMenu`. | n/a (leave ContextMenu.vue untouched) | **Satisfy DS-LIB-05's rename by ADDITION:** keep `ContextMenu.vue` as-is, and add a NEW barrel-exported `DropdownMenu` (Reka `DropdownMenuRoot`+`Trigger`+`Portal`+`Content`+`Item`+`Separator`+`Label`) for future trigger-anchored menus. Document that the cursor-menu stays bespoke. | **HIGH if forced / LOW if carved out.** Recommended path (carve-out) is LOW risk: zero consumer change. |

### New Primitives (greenfield — DS-LIB-06)

| Primitive | Reka base | shadcn-vue structure | Token classes | v-model | Test requirement |
|-----------|-----------|----------------------|---------------|---------|------------------|
| **Tooltip** | `TooltipProvider`(app-root) + `TooltipRoot`+`TooltipTrigger`+`TooltipPortal`+`TooltipContent`(+`TooltipArrow`) | Trigger slot + content slot. `delay-duration` via Provider. | `bg-popover text-popover-foreground` (tokens EXIST per CLAUDE.md) `rounded-md px-3 py-1.5 text-xs` | none (open managed) | mount + assert content renders on trigger; real consumer optional |
| **Popover** | `PopoverRoot`+`PopoverTrigger`+`PopoverPortal`+`PopoverContent`(+`PopoverClose`/`PopoverArrow`) | `v-model:open` controlled; trigger + content slots. | `bg-popover text-popover-foreground border border-white/10 rounded-xl p-4 shadow-xl` | `v-model:open` (boolean) | mount + open/close toggle |
| **Switch** | `SwitchRoot`+`SwitchThumb` | `v-model` (boolean, NOT `:checked`) | `bg-primary` when on / `bg-white/10` off; thumb `bg-white rounded-full data-[state=checked]:translate-x-*` | `v-model` (boolean) | mount + toggle emits `update:modelValue` |
| **Checkbox** | `CheckboxRoot`+`CheckboxIndicator` | `v-model` (boolean; `'indeterminate'` string for tri-state) | `border-input data-[state=checked]:bg-primary` + indicator lucide check icon | `v-model` (boolean \| 'indeterminate') | mount + check/uncheck |

**TooltipProvider note:** No `TooltipProvider`/`ConfigProvider` is mounted at app root today [VERIFIED: grep of `App.vue`/`main.ts` returns nothing]. Tooltip requires a `TooltipProvider` ancestor for global delay; either mount one in `App.vue` (minimal, recommended) or wrap each Tooltip in its own provider. Decide in planning — mounting one provider in `App.vue` is the standard shadcn-vue pattern and lowest-friction.

## Rename Decision

### Decision #1 — Modal → Dialog: KEEP OLD NAMES (alias, no codemod)

**Recommendation: keep the filename `Modal.vue` and the barrel export `Modal`. Do NOT rename the file or run a codemod.**

Rationale (decided by risk):
- Consumers use **mixed import styles**: `Game.vue`/`Profile.vue` import `Modal` from the barrel; `OtherSubsPanel.vue` + `FeedbackButton.vue` import `Modal` from `@/components/ui/Modal.vue` (relative). A codemod would have to rewrite BOTH a barrel-symbol rename AND relative file-path imports across 5 files — more surface to get wrong, more diff, more review.
- The brief itself says "the barrel must keep exporting the OLD names as aliases OR a codemod updates consumers; recommend the lower-risk path." Alias is lower-risk: 0 consumer edits.
- Implementation: re-skin the component in place at `Modal.vue` onto Reka Dialog, keep `export { default as Modal } from './Modal.vue'`. **Optionally** also add `export { default as Dialog } from './Modal.vue'` (or a thin `Dialog.vue` that re-exports) so new code can use the canonical name. Net: both names resolve, no consumer churn.
- This mirrors Phase 2's Button approach (legacy `primary`/`secondary` variants kept as aliases in `button-variants.ts` rather than migrating call-sites) [VERIFIED: button-variants.ts lines 13-15].

### Decision #2 — ContextMenu → DropdownMenu: CARVE OUT (satisfy by addition, do not force-swap)

**Recommendation: leave `ContextMenu.vue` untouched; add a new Reka-based `DropdownMenu` to the barrel.**

Rationale:
- `ContextMenu.vue` is a **cursor-positioned** right-click menu (`visible`/`x`/`y` props, `left:Xpx/top:Ypx`, viewport clamping). Reka `DropdownMenu` is **trigger-anchored only** — it positions content against a `DropdownMenuTrigger` element, with no arbitrary x/y API [CITED: reka-ui.com/docs/components/dropdown-menu]. There is no faithful 1:1 swap.
- It's the **only** primitive not exported from the barrel (consumed solely by `AnimeContextMenu.vue` via relative import). Forcing it onto Reka would require rewriting `AnimeContextMenu.vue` + `useContextMenu.ts` (open/openAtElement/onTouchstart) + 5 view call-sites — a behavior change, not a "swap behind same paths," directly violating DS-NF-04.
- Lower-risk satisfaction of DS-LIB-05: add a barrel-exported `DropdownMenu` built on Reka (trigger-anchored, for future menus), and document that the existing cursor-context-menu remains bespoke. The requirement "ContextMenu→DropdownMenu behind same paths" is honored in spirit — the `ui/` set gains a token-driven DropdownMenu — without breaking the live right-click UX.
- **Flag for planner/user confirmation** [ASSUMED]: that "satisfy by addition" is acceptable for DS-LIB-05's ContextMenu clause. If the user truly wants the cursor menu rebuilt on Reka, that is a larger, behavior-changing task and should be its own phase.

## Recommended Task Sequence

Lowest-risk / fewest-consumers first; each task independently committable + smoke-able:

1. **Badge** (`cva` re-skin, 7 consumers, LOW) → add `badge-variants.ts` + `Badge.spec.ts` mirroring Button. Smoke: cards (`AnimeCardNew`/`EpisodeCard`), `OtherSubsPanel`.
2. **Input** (`cn()` SFC, 1 consumer, LOW) → smoke: Game room form. Watch the `ui/SearchAutocomplete.vue` internal consumer (composite, must still compile).
3. **Tabs** (`cn()` SFC, 2 consumers, LOW) → smoke: Profile tabs, Anime detail tabs.
4. **Select** (Reka `Select`, 2 consumers, MEDIUM) → **in-browser gate**: player source dropdown / Profile sort+status / Game settings. Verify portal+popper positioning + `xs` size.
5. **DropdownMenu** (NEW Reka, 0 consumers, LOW) → satisfies DS-LIB-05 ContextMenu clause by addition; mount test + optional consumer.
6. **Modal → Dialog** (Reka `Dialog`, 5 consumers, MEDIUM/HIGH, alias-kept) → **in-browser gate**: Game create-room, Profile avatar, OtherSubsPanel, FeedbackButton. Verify scroll-lock cooperation (`useBodyScrollLock`) + SubtitleOverlay teleport non-interference + esc/backdrop opt-outs.
7. **Tooltip** (NEW, 0 consumers, LOW) → mount `TooltipProvider` in `App.vue`, mount test.
8. **Popover** (NEW, 0 consumers, LOW) → mount test.
9. **Switch** (NEW, 0 consumers, LOW) → mount test, assert `v-model` boolean.
10. **Checkbox** (NEW, 0 consumers, LOW) → mount test, assert `v-model` boolean + indeterminate.

After each: `bunx vue-tsc --noEmit` (compile gate) + targeted `bunx vitest run` for the new spec. After the full phase: the standing 5-surface smoke + the MEDIUM/HIGH in-browser gates (Select, Dialog).

## Verification / Consumer Smoke Map

| Primitive | Exercise on | Verification |
|-----------|-------------|--------------|
| Badge | AnimeCardNew/EpisodeCard cards, OtherSubsPanel chips, Profile/Anime/Game/RawLibrary | visual: 8 variant colors unchanged; `Badge.spec.ts` asserts variant classes |
| Input | Game create-room form | clearable X, prefix/suffix slots, error/hint text |
| Select | player **source dropdown**, Profile **sort + status** ×3, Game settings | open/keyboard/select; portal renders over player chrome |
| Modal/Dialog | genre filter? (no — that's GenreFilterPopup, out of scope), report? (ReportButton uses its own), **avatar (Profile), feedback (FeedbackButton), create-room (Game), subs picker (OtherSubsPanel)** | esc/backdrop close, scroll lock, focus trap, fullscreen-subtitle non-interference |
| Tabs | Profile sections, Anime detail | active-tab styling per variant, panel slot switch |
| New 4 | mount/Vitest only (greenfield) | spec assertions per table |
| **All** | — | `bunx vue-tsc --noEmit` proves ~18 consumer files compile |

## Common Pitfalls

### Pitfall 1: `@layer components` cascade footgun (Tailwind v4)
**What goes wrong:** Custom classes in `main.css` (`.glass-card`, `.glass-elevated`) are **UNLAYERED** and beat utilities; a second plain `@layer components` block gets tree-shaken. This previously broke `md:hidden`.
**How to avoid:** When composing Dialog content with both `glass-elevated` (unlayered) and utility overrides, the unlayered class wins — order in `cn()` won't fix it. Keep `glass-elevated` as the base and don't try to override its properties with utilities. [CITED: MEMORY.md reference_tailwind_v4_css_cascade.md]
**Warning signs:** A utility silently has no effect; verify in-browser, not jsdom.

### Pitfall 2: tailwind-merge dropping custom tokens
**What goes wrong:** `twMerge` may not recognize project-custom tokens (`bg-brand-pink`, `border-input`, `ring-ring`) and could mis-merge or drop them.
**How to avoid:** Phase 2 already validated `cn()` with these tokens on Button/Card — reuse identical token strings. Don't introduce new arbitrary-value collisions; spec-test the merged class output like `Button.spec.ts` does.
**Warning signs:** A token class missing from `wrapper.classes()` in a spec.

### Pitfall 3: Reka peer-deps with Vue 3.4
**What goes wrong:** Reka 2.9.8 peer is `vue >= 3.4.0`; project is `vue@^3.4.21` — satisfied [VERIFIED: node_modules/reka-ui/package.json + package.json]. No action, but do NOT bump Reka without re-checking the peer range.
**How to avoid:** Pin reka-ui exactly (already `2.9.8` exact in package.json). New primitives need no new deps.

### Pitfall 4: Dialog Portal vs SubtitleOverlay fullscreen Teleport
**What goes wrong:** Reka `DialogPortal` teleports content to `body`. The `SubtitleOverlay.vue` teleports to `fullscreenEl || 'body'` (`<Teleport :to="fullscreenEl || 'body'">`). If a Dialog ever opens during fullscreen video, its body-portaled content sits OUTSIDE the fullscreen element and won't be visible over fullscreen video (browsers only render the fullscreen subtree). Today's `Modal` has the same body-teleport, so behavior is likely unchanged — but Reka's focus-trap could steal focus from the fullscreen player.
**How to avoid:** Confirm no Modal is designed to open over fullscreen video (the 5 consumers are all non-fullscreen contexts). Keep teleport target = `body` to match current Modal. Test esc/focus during fullscreen as a regression check.
**Warning signs:** Subtitle text disappears or player loses keyboard control when a dialog opens.

### Pitfall 5: Reka Select Portal + popper changes overflow/stacking
**What goes wrong:** Current Select renders an inline `absolute` `<ul>` inside the wrapper; Reka `SelectContent` defaults to portal + `position="popper"`. This changes z-index context and can break the "appears under a clipped container" assumption.
**How to avoid:** Use `position="popper"` and bind trigger width via `--reka-select-trigger-width`; verify the player source dropdown (inside player chrome) still overlays correctly in-browser.
**Warning signs:** Dropdown clipped, mis-positioned, or wrong width.

### Pitfall 6: SSR-less Vite mount + jsdom teleport
**What goes wrong:** Vitest jsdom doesn't fully render teleported/portaled content; specs asserting Dialog/Select content presence may need `attachTo: document.body` or to query `document.body`.
**How to avoid:** For portal primitives, mount with `attachTo` and query the document, or assert on the trigger + emitted events rather than portaled DOM. Cross-cutting interaction correctness goes to the in-browser gate, not jsdom.

### Pitfall 7: Modal scroll-lock regression
**What goes wrong:** Reka Dialog does NOT auto scroll-lock [CITED: reka-ui.com/docs/components/dialog]. Dropping `useBodyScrollLock` would break body-scroll behavior AND the refcount cooperation with Navbar's mobile drawer.
**How to avoid:** Keep the `useBodyScrollLock(toRef(props,'modelValue'))` call exactly as today.

## Code Examples

### Badge variants (mirror button-variants.ts)
```ts
// Source: pattern from frontend/web/src/components/ui/button-variants.ts (Phase 2)
import { cva, type VariantProps } from 'class-variance-authority'
export const badgeVariants = cva('inline-flex items-center font-medium', {
  variants: {
    variant: {
      default: 'bg-white/10 text-white/80',
      primary: 'bg-cyan-500/20 text-cyan-400',
      // ...exact literals from Badge.vue lines 26-37 — DO NOT tokenize
    },
    size: { sm: 'px-2 py-0.5 text-xs rounded', md: 'px-2.5 py-1 text-sm rounded-md', lg: 'px-3 py-1.5 text-base rounded-lg' },
  },
  defaultVariants: { variant: 'default', size: 'md' },
})
export type BadgeVariants = VariantProps<typeof badgeVariants>
```

### Dialog behind Modal API (key bindings)
```vue
<!-- Source: reka-ui.com/docs/components/dialog — open/escape/outside mapping -->
<DialogRoot v-model:open="open" @update:open="(v) => { if (!v) emit('close') }">
  <DialogPortal>
    <DialogOverlay class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm" />
    <DialogContent
      :class="cn('relative glass-elevated rounded-2xl p-4 sm:p-6 w-full max-h-[90vh] overflow-y-auto', sizeClass)"
      @escape-key-down="(e) => !closeOnEsc && e.preventDefault()"
      @pointer-down-outside="(e) => !closeOnBackdrop && e.preventDefault()"
    >
      <DialogTitle v-if="title">{{ title }}</DialogTitle>
      <slot name="header" /> <slot /> <slot name="footer" />
    </DialogContent>
  </DialogPortal>
</DialogRoot>
<!-- keep useBodyScrollLock(toRef(props,'modelValue')); map modelValue<->open -->
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Radix Vue | Reka UI (renamed/forked) | 2024 | shadcn-vue now targets Reka UI; all primitive names are `*Root`/`*Trigger`/etc. |
| `v-model:checked` on Switch/Checkbox (older Radix) | plain `v-model` (boolean; `'indeterminate'` for tri-state) | Reka 2.x | New Switch/Checkbox use `v-model`, not a named model. [CITED: reka-ui.com] |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Satisfying DS-LIB-05's "ContextMenu→DropdownMenu" by ADDING a Reka DropdownMenu (leaving cursor ContextMenu bespoke) is acceptable | Rename Decision #2 | If user wants the live right-click menu rebuilt on Reka, that's a larger behavior-changing task needing its own phase/CONTEXT. |
| A2 | The `bg-popover`/`text-popover-foreground` tokens needed by Tooltip/Popover exist and render correctly | New Primitives table | Tooltip/Popover look wrong; mitigated — CLAUDE.md lists `bg-popover` as a canonical token. Smoke-verify. |
| A3 | No existing Modal consumer opens over fullscreen video, so body-portal is safe | Pitfall 4 | A fullscreen-context dialog would be invisible/lose focus; verify the 5 consumers are non-fullscreen. |
| A4 | Reka Select `xs` size can be reproduced (Reka examples only show default sizing) | Select row | Profile/player `xs` selects look slightly off; reproduce via explicit padding utilities, verify in-browser. |
| A5 | Mounting one `TooltipProvider` in `App.vue` is acceptable app-root change | New Primitives note | If user objects to App.vue edit, wrap per-Tooltip instead (more boilerplate). |

## Open Questions

1. **Dialog transition parity.** The current `.modal-*` CSS transition (scale 0.95 + opacity, 0.2s) must be reproduced on Reka via `data-[state=open/closed]` utilities. Exact timing match is hard to assert in jsdom.
   - Recommendation: replicate with Tailwind `data-[state]` transition utilities; accept "visually equivalent" verified in-browser, not pixel-identical.
2. **Should `Dialog.vue` exist as a real file or just a barrel alias to `Modal.vue`?** Either works; a thin re-export file documents intent better.
   - Recommendation: keep `Modal.vue` as the implementation; add `export { default as Dialog } from './Modal.vue'` to the barrel. No second file needed.

## Environment Availability

> Code/config-only phase. No external services/runtimes beyond the existing frontend toolchain (bun, vite, vitest, vue-tsc) — all present per package.json. No availability audit needed.

## Validation Architecture

> `.planning/config.json` not checked for this workstream; default = nyquist_validation enabled. Test infra confirmed present (vitest + @vue/test-utils + jsdom).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | vitest 4.1.6 + @vue/test-utils 2.4.10, jsdom 29 |
| Config file | `frontend/web/vitest.config.ts` (include `src/**/*.spec.ts`, globals, jsdom) |
| Quick run command | `cd frontend/web && bunx vitest run src/components/ui/` |
| Full suite command | `cd frontend/web && bunx vitest run && bunx vue-tsc --noEmit` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DS-LIB-05 | Badge variants render unchanged | unit | `bunx vitest run src/components/ui/Badge.spec.ts` | ❌ Wave 0 |
| DS-LIB-05 | Input/Tabs/Select/Modal API preserved | unit + compile | `bunx vue-tsc --noEmit` + per-spec | ❌ Wave 0 (specs) |
| DS-LIB-06 | Tooltip/Popover/Switch/Checkbox mount + interact | unit | `bunx vitest run src/components/ui/{Tooltip,Popover,Switch,Checkbox}.spec.ts` | ❌ Wave 0 |
| DS-NF-04 | All ~18 consumers compile | compile gate | `bunx vue-tsc --noEmit` | ✅ (tooling exists) |

### Sampling Rate
- **Per task commit:** `bunx vitest run src/components/ui/<Primitive>.spec.ts && bunx vue-tsc --noEmit`
- **Per wave merge:** `bunx vitest run src/components/ui/`
- **Phase gate:** full suite green + in-browser smoke on Select + Dialog before verify.

### Wave 0 Gaps
- [ ] `src/components/ui/Badge.spec.ts` — DS-LIB-05 (mirror Button.spec.ts)
- [ ] `src/components/ui/Input.spec.ts` — DS-LIB-05
- [ ] `src/components/ui/Tabs.spec.ts` — DS-LIB-05
- [ ] `src/components/ui/Select.spec.ts` — DS-LIB-05 (portal: use `attachTo`)
- [ ] `src/components/ui/Modal.spec.ts` — DS-LIB-05 (portal: query `document.body`)
- [ ] `src/components/ui/{Tooltip,Popover,Switch,Checkbox}.spec.ts` — DS-LIB-06
- [ ] `src/components/ui/DropdownMenu.spec.ts` — DS-LIB-05 (new addition)

## Project Constraints (from CLAUDE.md)
- Frontend uses **bun** (not npm/pnpm); CLI tools via `bunx` (not npx). [CLAUDE.md Local Development]
- Vue template rule: never place non-conditional elements between `v-if`/`v-else-if`; place independent components AFTER the chain. [MEMORY.md]
- i18n key bindings are string-typed — if any primitive touches `t(...)`, smoke-verify post-deploy. (None of these primitives should add i18n.)
- After implementation, invoke `/animeenigma-after-update` (batch multiple small updates into one). [CLAUDE.md]
- Effort metrics: no days/hours — score with UXΔ / CDI / MVQ. [.planning/CONVENTIONS.md]

## Sources

### Primary (HIGH confidence)
- `frontend/web/src/components/ui/{Badge,Input,Select,Modal,Tabs,ContextMenu,Button,Card}.vue` + `button-variants.ts` + `index.ts` — read in full (current API surface, classes, a11y).
- `frontend/web/package.json` + `node_modules/reka-ui/package.json` — VERIFIED versions: reka-ui 2.9.8 (peer vue ≥3.4.0), cva 0.7.1, clsx 2.1.1, tailwind-merge 3.6.0, vue 3.4.21, vite 5.4.21, vitest 4.1.6, vue-tsc 2.0.7.
- `reka-ui.com/docs/components/{dialog,select,tooltip,popover,switch,checkbox,dropdown-menu}` — CITED (component parts, portal, v-model, positioning model) — fetched 2026-06-02.
- grep consumer inventory across `frontend/web/src` — VERIFIED import sites + styles.

### Secondary (MEDIUM confidence)
- `shadcn-vue.com/docs/components/{dialog,select}` — confirms Reka backing + part names (page defers styling detail to Reka).

### Tertiary (LOW confidence)
- None.

## Metadata
**Confidence breakdown:**
- Current API surface: HIGH — every component read line-by-line.
- Reka primitive structure/v-model/portal: HIGH — CITED from current reka-ui docs + cross-checked against installed 2.9.8.
- Rendering-identity feasibility: MEDIUM for Select + Dialog (portal/transition deltas need in-browser confirmation), HIGH for Badge/Input/Tabs.
- Rename strategy: HIGH — based on verified mixed import styles + barrel absence of ContextMenu.

**Research date:** 2026-06-02
**Valid until:** 2026-07-02 (stable; re-verify reka-ui version before bumping)
