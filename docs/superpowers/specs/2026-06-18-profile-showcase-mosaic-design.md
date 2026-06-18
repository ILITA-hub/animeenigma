# Profile Showcase — Mosaic Layout + Resizable Editor

**Date:** 2026-06-18
**Status:** Design locked (brainstorm complete) — ready for implementation plan
**Owner:** 0neymik0
**Interactive mock (reference):** `.brainstorm/content/profile-prod-v1.html` (served via `python3 -m http.server 8899` from `.brainstorm/content/`). Also: `profile-showcase-redesign-v1.html` (layout concepts), `profile-list-placement-v1.html` (tab placement), `profile-mosaic-editor-v1.html` (editor interactions).

**Scores:** UXΔ = +3 (Better) · CDI = 0.05 * 21 · MVQ = Griffin 85%/80%

---

## 1. Problem

The profile showcase ("витрина") блоки are already built (9 block types, 27 layout variants). But the **composition layer** is a flat vertical stack of identical bordered cards (`ProfileShowcase.vue`: `<section class="space-y-4">` → each block its own `rounded-xl border bg-card`). Every block looks like its neighbour → monotonous, reads like a settings page, not a personal "showcase wall". Additionally the large stack pushes the actual watchlist far down the page.

## 2. Goals

1. Replace the flat stack with a **user-arrangeable bento mosaic** — each block has a user-chosen width and height; blocks pack densely.
2. Let the owner **resize** blocks (corner drag-handle, snap to grid cells) and **rearrange** them (drag one block onto another → they swap position AND size).
3. Per-**variant** size constraints so a block can't be sized into a broken layout (a poster-row can't become 1×1).
4. Move the showcase into the profile **tab bar as the first tab** so it no longer competes with the watchlist for vertical space.
5. Reuse the existing 9 block components and their 27 variants unchanged — only the container, editor, and data model change.

## 3. Non-goals (YAGNI)

- Free x/y coordinate placement / intentional gaps (the dense auto-pack grid always compacts).
- A new DnD dependency (use the existing drag + a small custom resize handler).
- Re-implementing block visuals — variants are existing components.
- Per-device size editing on touch (sizes are edited on desktop; see §6).

## 4. Locked decisions

| # | Decision |
|---|----------|
| D1 | **Span grid with auto-pack** — `grid-auto-flow: dense`, 4 columns desktop, 2 columns mobile. Not free coordinates. |
| D2 | **Resize = corner drag-handle**, snaps to whole grid cells. Hidden on touch and on fixed-size blocks (min == max). |
| D3 | **Mobile = 2 columns**: width maps `1→1`, `≥2→full`; height preserved. Sizes are configured from desktop. |
| D4 | **Showcase becomes the first profile tab** ("Витрина"), default-active. Watchlist ("Список") is a sibling tab. |
| D5 | **Drag block onto block = swap** position + size (each adopting the other's size, clamped to its own variant constraints). |
| D6 | **Per-variant** min/max/default size constraints (not per-type). |
| D7 | Additive data model — no DB migration; blocks without `w/h` are backfilled to the variant default on read/render. |

## 5. Data model

### 5.1 Backend — `services/player/internal/domain/showcase.go`

Extend `Block` with two integer fields (additive; `Blocks` is a jsonb array, so **no migration**):

```go
type Block struct {
    Type    string          `json:"type"`
    Variant string          `json:"variant,omitempty"`
    Order   int             `json:"order"`
    Width   int             `json:"w,omitempty"`   // grid columns 1..4
    Height  int             `json:"h,omitempty"`   // grid rows 1..3
    Config  json.RawMessage `json:"config"`
}
```

Add a per-(type, variant) size-constraint table mirroring the frontend, and extend `ValidateBlocks` to clamp/validate `Width`/`Height` against the **current variant's** constraints:

```go
type SizeBound struct{ MinW, MaxW, MinH, MaxH, DefW, DefH int }

// VariantSizeAllowlist[type][variant] → bounds. First variant = default.
// Keep in sync with frontend src/types/showcase.ts (VC).
var VariantSizeAllowlist = map[string]map[string]SizeBound{ ... } // see §7 matrix
```

Validation rules added to `ValidateBlocks`:
- `w`/`h` outside the variant's `[Min,Max]` → clamp to nearest bound (be lenient — clamp, don't reject; a stale client must never 400 the save).
- `w == 0 || h == 0` (absent) → set to the variant default.
- Unknown type/variant already rejected by existing code.

`MaxBlocks = 12`, `MaxBlockItems = 12` unchanged.

### 5.2 Frontend — `frontend/web/src/types/showcase.ts`

```ts
export interface ShowcaseBlock {
  type: ShowcaseBlockType
  variant?: string
  order: number
  w?: number   // 1..4 columns
  h?: number   // 1..3 rows
  config: /* unchanged union */
}

// Per-variant bounds — MUST mirror Go VariantSizeAllowlist.
export interface SizeBound { minW: number; maxW: number; minH: number; maxH: number; defW: number; defH: number }
export const VARIANT_SIZE: Record<ShowcaseBlockType, Record<string, SizeBound>> = { /* §7 */ }
export const sizeFor = (t: ShowcaseBlockType, variant?: string): SizeBound => /* lookup w/ default-variant fallback */
```

**Parity rule (governance):** `SHOWCASE_VARIANTS` ↔ Go `VariantAllowlist` and `VARIANT_SIZE` ↔ Go `VariantSizeAllowlist` must stay in exact parity (same as the existing variant-allowlist parity contract). A divergence test is added (see §10).

## 6. View rendering — `ProfileShowcase.vue`

Replace the `space-y-4` stack with a CSS-grid mosaic:

```
grid grid-cols-2 md:grid-cols-4 gap-3 [grid-auto-flow:dense]
+ grid-auto-rows: <fixed row unit, ~190px desktop / ~165px mobile>
```

Each block is wrapped in a cell that carries **static** span classes (Tailwind-safe — no dynamic class interpolation) mapped from `w`/`h`:

| w | class | h | class |
|---|-------|---|-------|
| 1 | `col-span-1 md:col-span-1` | 1 | `row-span-1` |
| 2 | `col-span-2 md:col-span-2` | 2 | `row-span-2` |
| 3 | `col-span-2 md:col-span-3` | 3 | `row-span-3` |
| 4 | `col-span-2 md:col-span-4` | | |

(Mobile: w=1→1 col, w≥2→full width = 2 cols; height preserved.) The cell stretches the block (`h-full`); existing block components render inside unchanged. Blocks missing `w/h` use `sizeFor(type, variant)` defaults.

Variant layouts that overflow a chosen size **wrap or scroll within the cell** rather than clip badly — confirmed in mock for: `favorite_anime: grid` (auto-fill wrap), `favorite_character: circles` (auto-fill wrap), `hex` (flex wrap), `*: row` (horizontal scroll). This wrapping behaviour is part of the component contract.

## 7. Per-variant size matrix (the contract)

Columns 1..4, rows 1..3. Format `min → max (default)`. Fixed = min == max (resize handle hidden).

| Block · variant | W min→max | H min→max | default |
|---|---|---|---|
| about · quote | 2→4 | 1→2 | 2×1 |
| about · bio / terminal | 2→4 | 1→2 | 2×2 |
| about · minimal | 2→4 | 1→2 | 2×1 |
| about · vn | **2 (fixed)** | **1 (fixed)** | 2×1 |
| favorite_anime · row | 2→4 | 1 (fixed) | 4×1 |
| favorite_anime · grid | 2→4 | 1→3 | 4×2 |
| favorite_anime · list | 2 (fixed) | 1→3 | 2×2 |
| favorite_anime · banner | 2→4 | 1→3 | 2×2 |
| favorite_anime · podium | **2×2 (fixed)** | | 2×2 |
| favorite_character · circles | 1→4 | 1→3 | 2×1 |
| favorite_character · portraits | 2→4 | 1→3 | 2×2 |
| favorite_character · hero | 2→4 | 1→3 | 2×2 |
| favorite_character · hex | 1→4 | 1→3 | 2×2 |
| card_collection · row | 2→4 | 1 (fixed) | 2×1 |
| card_collection · fan | 2→4 | 2→3 | 2×2 |
| card_collection · grid | 2→4 | 1→3 | 2×2 |
| card_collection · hero | 2→4 | 1→2 | 2×2 |
| card_collection · tilt3d | 2→4 | 2→3 | 3×2 |
| stats · tiles/rings/bars/strip | **2×1 (fixed, all)** | | 2×1 |
| op_ed · grid | 2→4 | 1→3 | 2×2 |
| anime_dna · bars | 1→2 | 1→3 | 1×2 |
| compatibility · ring | 1→2 | 1 (fixed) | 2×1 |
| continue_watching · cards | 2→4 | 1→3 | 2×2 |

Notes: `stats` is fixed 2×1 for every variant (4 stat tiles render 4-across at h=1; 2×2 at h≥2). On variant switch, the block's size is re-clamped into the new variant's range.

## 8. Tab integration — `Profile.vue` (decision D4)

- Add `{ value: 'showcase', label: t('profile.tabs.showcase') }` as the **first** entry of the `tabs` computed array (gated by `profileWallVisible`).
- Move `<ProfileShowcase>` out of its standalone slot (lines 72–78) into a `<template #showcase>` panel inside `<Tabs>`.
- Default `activeTab`: `'showcase'` when the showcase is visible; if the viewed user's showcase is **empty AND the viewer is not the owner**, fall back to `'watchlist'` (avoid landing a visitor on an empty wall). `ProfileShowcase` emits `@loaded(count)`; `Profile.vue` adjusts the default once.
- When `profileWallVisible` is false (dark-ship `VITE_PROFILE_WALL_ADMIN_ONLY`), no showcase tab; default stays `'watchlist'`.

## 9. Editor — `ShowcaseEditor.vue`

Edit mode lives inside the showcase tab (existing "Редактировать" / "Готово" toggle).

1. **Reorder + swap (D5):** dragging a block onto another swaps their DOM order *and* their `w/h` (each clamped to its own variant bounds via `sizeFor`). Drag target highlights with a "⇄ поменять местами" affordance. (SortableJS, which vuedraggable wraps, has a `swap` plugin; size-exchange is layered on the swap callback.)
2. **Resize (D2):** a corner handle (bottom-right). Pointer drag → pixel delta converted to cell delta using measured column width + row height → live `w/h` update, committed (clamped) on `pointerup`, then the block re-renders so variant layout adapts to the new size. Handle hidden when the variant is fixed (min == max) and on touch.
3. **Add block:** "＋ Добавить блок" opens a **picker** of all 9 types; already-present types are shown disabled ("· добавлен" — one block per type). Selecting a type inserts it at the default variant's default size.
4. **Per-block config (⚙):** opens a modal with:
   - **Variant picker** — chips of the block's real variants (`SHOWCASE_VARIANTS[type]`). On save, size is re-clamped to the chosen variant's bounds.
   - **Content fields by type:** about → title + text; favorite_anime / favorite_character / op_ed → searchable add/remove list (max 12) seeded from current selection; card_collection → owned-card toggle grid; stats / continue_watching / anime_dna / compatibility → auto (layout only). The config seeds from the block's *current* saved config (not defaults) so edits persist.
5. **Delete** (✕) per block.
6. **Save** persists `{type, variant, order, w, h, config}[]` via `showcaseApi.saveShowcase`.

Touch: resize handle hidden; reorder/swap still available; sizes configured on desktop.

## 10. Validation, parity, testing

- **Backend:** `ValidateBlocks` clamps `w/h` to variant bounds + backfills defaults. New unit tests in `showcase_test.go`: clamp out-of-range, backfill absent, fixed-variant enforcement, swap-produced sizes valid.
- **Parity test (frontend):** a Vitest test asserts `VARIANT_SIZE` keys === `SHOWCASE_VARIANTS` keys, and (hardcoded mirror) matches the Go matrix — fails CI on divergence. Mirrors the existing allowlist-parity expectation.
- **Frontend unit:** `ProfileShowcase` renders grid spans from `w/h`; `sizeFor` fallback; editor resize clamps; swap exchanges sizes with clamping; picker disables present types; config seeds from saved selection.
- **DS-lint:** grid spans are **static class strings** (Rule 1 safe) and inline `style` only carries layout (`grid-column`/`grid-row`/`grid-auto-rows`), never color (Rule 8 safe). Verify `make lint-frontend` stays green.
- **e2e:** extend `frontend/web/e2e/` showcase coverage — enter edit, resize a block, swap two blocks, switch a variant, add/remove a favorite, save, reload, assert persistence.

## 11. Files touched (estimate)

- `services/player/internal/domain/showcase.go` (+ `_test.go`) — Block `w/h`, `VariantSizeAllowlist`, validation.
- `frontend/web/src/types/showcase.ts` — `w/h`, `VARIANT_SIZE`, `sizeFor` (+ parity spec).
- `frontend/web/src/components/profile/showcase/ProfileShowcase.vue` — bento grid + span mapping + `@loaded`.
- `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue` — resize handle, swap+size-exchange, add-picker, per-block config modal, per-variant clamping.
- `frontend/web/src/views/Profile.vue` — showcase-as-first-tab, default-tab logic.
- `frontend/web/src/locales/{en,ru,ja}.json` — `profile.tabs.showcase` + any new editor strings (all 3 locales — i18n-lint gate).

## 12. Migration / rollout

Additive: existing saved showcases (no `w/h`) render at variant defaults; first re-save writes `w/h`. Stays behind the existing `VITE_PROFILE_WALL_ADMIN_ONLY` dark-ship gate. Standard after-update: redeploy `player` + `web`, changelog (Trump-mode), commit+push.
