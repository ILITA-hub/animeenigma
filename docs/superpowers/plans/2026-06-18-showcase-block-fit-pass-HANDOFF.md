# Showcase Mosaic — Block-Fit Pass (HANDOFF for a fresh session)

**Status:** open follow-up. The showcase mosaic feature is SHIPPED+DEPLOYED (origin/main, dark-shipped behind `VITE_PROFILE_WALL_ADMIN_ONLY`). This doc is the entry point for the remaining **content-fit / "проверь все блоки"** pass.

**Current deployed state:** origin/main `7f3f09d5`. Editor chrome is CORRECT (⚙ + ✕ top-right in line with the block title, resize handle ◢ bottom-right, no shifted bar). Variant chip labels translated (`showcase.variant.*` en/ru/ja). Web live (:3003, 200).

## The remaining problem (owner-reported)

Real block components overflow the **fixed bento row height**. The grid uses `grid-auto-rows: 190px` (desktop) / `165px` (mobile) with cells `overflow-hidden` (editor) and blocks rendering at `h-full`. Several block components have **natural content height > the row unit**, so content clips or spills:

- **StatsBlock `rings` variant** — rings are `h-[104px] w-[104px]`; with the block title + `p-4 md:p-6` padding + ring labels, total > 190px → rings render larger than the cell and **spill out top/bottom**. (Owner screenshot confirmed.) Note: `stats` is **locked to 2×1 for all variants** (owner decision) — so rings MUST be made to fit 190px, not given more height.
- **FavoriteAnimeBlock `row`** — posters `w-[120px] sm:w-[132px]` at aspect 2/3 ≈ 180–198px tall + title + padding ≈ 256px > 190px → overflow (default size is 4×1 = 190px).
- Likely also: `favorite_character` portraits/hero, `card_collection` row, etc. — **audit all 9 blocks × their variants**.

The HTML mock (`.brainstorm/content/profile-prod-v1.html`) looked right because its mock elements were sized smaller to fit the cell — the real v1/v2 components were not.

### Why the easy fix doesn't work
`grid-auto-rows: minmax(190px, auto)` alone does NOT fix it: the cell's only in-flow child is the block wrapped at `h-full` (no intrinsic height) → the cell collapses to the 190px floor and still clips. Removing `h-full` lets the cell grow to content (no clip) BUT then small blocks no longer fill larger user-chosen cells (h=2/3) — they top-align with empty space. Need a solution that gives BOTH fill-when-bigger AND no-overflow-when-taller.

## Goal
Every block × variant either fits cleanly within its allocated cell (h×rowUnit) or the cell grows to fit — matching the HTML-mock quality. No clipping, no spill. Owner's bar: "в вёрстке html было лучше".

## Approach options (decide with owner first)
- **A — Per-component responsive internal sizing (recommended start):** make rings/posters/portraits size relative to available cell height (e.g. cap ring size, use `aspect-ratio` + `max-h`/container units, shrink at the constrained default size). Keep `grid-auto-rows` fixed + `h-full`. Most faithful to "fixed uniform mosaic"; touches each block's internals.
- **B — Grow rows + min-height blocks:** `grid-auto-rows: minmax(190px, auto)`, REMOVE `h-full` from block roots, give blocks internal `min-h-full` + flex-centering so they fill when small and grow when tall. Fewer per-block edits but changes the uniform-row look and needs careful row-span behavior check.
- **C — Hybrid:** fixed rows + block roots `overflow-hidden` + internal content scaled/scrolled. Avoids spill but may clip.

## Per-block audit checklist (all 9)
`frontend/web/src/components/profile/showcase/blocks/`
- [ ] `AboutBlock` — quote / bio / terminal / minimal / vn
- [ ] `FavoriteAnimeBlock` — row / podium / grid / list / banner  ← row overflow confirmed
- [ ] `StatsBlock` — tiles / rings / bars / strip  ← **rings overflow confirmed; locked 2×1**
- [ ] `FavoriteCharacterBlock` — circles / portraits / hero / hex
- [ ] `CardCollectionBlock` — row / fan / grid / hero / tilt3d
- [ ] `ContinueWatchingBlock` — cards
- [ ] `OpEdBlock` — grid
- [ ] `AnimeDnaBlock` — bars
- [ ] `CompatibilityBlock` — ring
Plus the grid containers: `ProfileShowcase.vue` (view) + `ShowcaseEditor.vue` (editor) both use `[grid-auto-rows:…]`.

## Hard constraints
- **Can't self-verify:** showcase is admin-gated (`VITE_PROFILE_WALL_ADMIN_ONLY`) and the automation browser is NOT logged into the SPA — **verify each iteration via owner screenshots**. Plan the work to minimize round-trips (batch a coherent set of block fixes per screenshot cycle).
- **Stats is locked 2×1** for every variant (owner). Rings must fit 190px, not grow.
- **Gates:** `make redeploy-web` runs `i18n-lint` + `lint-design` (DS-lint) + `vue-tsc` — all must pass. **vue-tsc catches .vue/.spec type errors that vitest/scoped-tsc miss** — expect it.
- **Deploy from a CLEAN `origin/main` worktree** (copy `docker/.env`, `bun install` first; compose project stays `docker`), NOT the shared dirty tree.
- **Commit path-scoped** (`git commit <pathspec>`); pushing hot/locale files by pathspec sweeps other agents' uncommitted edits — rebuild commits in a clean worktree if needed. Push, cherry-pick onto `main` on race.
- DS-lint: semantic tokens only; no inline color; only `font-medium`/`font-semibold`.

## Reference
- Target look: `.brainstorm/content/profile-prod-v1.html` (served via `python3 -m http.server 8899` from `.brainstorm/content/`).
- Design spec: `docs/superpowers/specs/2026-06-18-profile-showcase-mosaic-design.md` (§7 size matrix).
- Memory: `project_profile_showcase_wall.md` (v3 section).
